// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func swapHooks(t *testing.T) {
	t.Helper()
	oldPrepare, oldVerify, oldWrite := prepareDirFn, verifyPrivateFn, writeTokenFn
	t.Cleanup(func() { prepareDirFn, verifyPrivateFn, writeTokenFn = oldPrepare, oldVerify, oldWrite })
}

func requireNothingServed(t *testing.T, dir string) {
	t.Helper()
	if exists(filepath.Join(dir, TokenFileName)) {
		t.Fatalf("a token file exists after a refused start")
	}
	if exists(filepath.Join(dir, SocketFileName)) {
		t.Fatalf("a socket file exists after a refused start")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func statusWithAuthorization(h http.Handler, authorization string) int {
	req := httptest.NewRequest("POST", "/v1/list", nil)
	req.Header.Set("Authorization", authorization)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// Review finding F2: when the folder lock fails the server must not start, write a token or leave a socket.
// The folder exists beforehand so a start that ignored the error would get as far as writing the token.
func TestStartServerRefusesWhenTheLockFails(t *testing.T) {
	swapHooks(t)
	dir := shortApiDir(t)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	prepareDirFn = func(string) error { return errors.New("lock failed") }
	verifyPrivateFn = func(string, bool) error { return nil }
	svc, _ := newSvc()
	srv, err := StartServer(dir, svc)
	if err == nil {
		srv.Close()
		t.Fatalf("StartServer served although the lock failed")
	}
	requireNothingServed(t, dir)
}

func TestStartServerRefusesWhenTheSocketIsNotPrivate(t *testing.T) {
	swapHooks(t)
	dir := shortApiDir(t)
	realVerify := verifyPrivateFn
	verifyPrivateFn = func(path string, isDir bool) error {
		if filepath.Base(path) == SocketFileName {
			return errors.New("socket is open to others")
		}
		return realVerify(path, isDir)
	}
	svc, _ := newSvc()
	if srv, err := StartServer(dir, svc); err == nil {
		srv.Close()
		t.Fatalf("StartServer served with a socket that is not private")
	}
	requireNothingServed(t, dir)
}

func TestStartServerRefusesWhenTheTokenCannotBeWritten(t *testing.T) {
	swapHooks(t)
	dir := shortApiDir(t)
	writeTokenFn = func(string, string) error { return errors.New("disk full") }
	svc, _ := newSvc()
	if srv, err := StartServer(dir, svc); err == nil {
		srv.Close()
		t.Fatalf("StartServer served without a token file")
	}
	requireNothingServed(t, dir)
}

func TestTokenFileIsRemovedWhenItIsNotPrivate(t *testing.T) {
	swapHooks(t)
	dir := t.TempDir()
	verifyPrivateFn = func(string, bool) error { return errors.New("open to others") }
	if err := WriteTokenFile(dir, testToken); err == nil {
		t.Fatalf("WriteTokenFile accepted a file that is not private")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("files left behind: %v", entries)
	}
}

// the lock comes first, the socket listens before the token is written
func TestStartOrder(t *testing.T) {
	swapHooks(t)
	dir := shortApiDir(t)
	var events []string
	realPrepare, realWrite := prepareDirFn, writeTokenFn
	prepareDirFn = func(d string) error {
		events = append(events, "lock")
		return realPrepare(d)
	}
	writeTokenFn = func(d string, tok string) error {
		if exists(filepath.Join(d, SocketFileName)) {
			events = append(events, "token-after-listen")
		} else {
			events = append(events, "token-before-listen")
		}
		return realWrite(d, tok)
	}
	svc, _ := newSvc()
	srv, err := StartServer(dir, svc)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if !reflect.DeepEqual(events, []string{"lock", "token-after-listen"}) {
		t.Fatalf("order: %v", events)
	}
}

// F7: a second start on a live socket is refused and leaves the first server and its token alone
func TestSecondStartOnALiveSocketIsRefused(t *testing.T) {
	dir := shortApiDir(t)
	svc, _ := newSvc()
	first, err := StartServer(dir, svc)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	tokenBefore, _ := ReadTokenFile(dir)
	if second, err := StartServer(dir, svc); err == nil {
		second.Close()
		t.Fatalf("a second server took over a live socket")
	}
	tokenAfter, _ := ReadTokenFile(dir)
	if tokenBefore != tokenAfter {
		t.Fatalf("the refused start replaced the token")
	}
	c := &Client{Dir: dir, Token: tokenBefore}
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("the first server stopped working: %v", err)
	}
}

// F7: a stale file at the socket path (no server behind it) does not block a start
func TestStaleSocketFileIsReplaced(t *testing.T) {
	dir := shortApiDir(t)
	if err := PrepareApiDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SocketFileName), []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	svc, _ := newSvc()
	srv, err := StartServer(dir, svc)
	if err != nil {
		t.Fatalf("a stale socket file blocked the start: %v", err)
	}
	defer srv.Close()
	c, err := MakeClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.List(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestApiFolderThatIsAFileIsRefused(t *testing.T) {
	path := shortApiDir(t)
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareApiDir(path); err == nil {
		t.Fatalf("a regular file was accepted as the api folder")
	}
}

// F6: unknown keys, duplicate keys, keys in the wrong case and junk bodies are refused with no call
func TestStrictJson(t *testing.T) {
	h, fb := newTestHandler()
	ok := `"workspaceid":"` + ws1 + `","tabid":"` + tabB + `"`
	bad := map[string]string{
		"unknown key":   `{` + ok + `,"input":"rm"}`,
		"duplicate key": `{` + ok + `,"tabid":"` + tabA + `"}`,
		"wrong case":    `{"WORKSPACEID":"` + ws1 + `","tabid":"` + tabB + `"}`,
		"array":         `[]`,
		"null":          `null`,
		"window string": `{` + ok + `,"window":"yes"}`,
	}
	for name, body := range bad {
		rec := doCall(t, h, "POST", "/v1/goto", testToken, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
	rec := doCall(t, h, "POST", "/v1/goto", testToken, `{`+ok+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("a good body was refused: %d %s", rec.Code, rec.Body.String())
	}
}

func TestListTakesNoBodyOrAnEmptyObject(t *testing.T) {
	h, _ := newTestHandler()
	for _, body := range []string{"", "  ", "{}"} {
		if rec := doCall(t, h, "POST", "/v1/list", testToken, body); rec.Code != http.StatusOK {
			t.Fatalf("body %q: want 200, got %d", body, rec.Code)
		}
	}
	for _, body := range []string{"this is not json", `{"x":1}`, `[]`, `{}{}`} {
		if rec := doCall(t, h, "POST", "/v1/list", testToken, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: want 400, got %d", body, rec.Code)
		}
	}
}

func TestEmptyServerTokenRefusesEverything(t *testing.T) {
	svc, fb := newSvc()
	h := MakeHandler("", svc)
	if code := statusWithAuthorization(h, ""); code != http.StatusUnauthorized {
		t.Fatalf("no header: %d", code)
	}
	if code := statusWithAuthorization(h, "Bearer "); code != http.StatusUnauthorized {
		t.Fatalf("an empty bearer token was accepted: %d", code)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
}

func TestUnauthorizedComesBeforeMethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler()
	rec := doCall(t, h, "GET", "/v1/list", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 before 405, got %d", rec.Code)
	}
}

func TestServerErrorBodyHasNoDetails(t *testing.T) {
	svc, fb := newSvc()
	fb.treeErr = errors.New("secret-backend-detail C:/private/path")
	h := MakeHandler(testToken, svc)
	rec := doCall(t, h, "POST", "/v1/list", testToken, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret-backend-detail") || strings.Contains(body, "private") {
		t.Fatalf("the 500 body carries the error text: %s", body)
	}
}

func TestBadIdIsNotEchoedInFull(t *testing.T) {
	h, _ := newTestHandler()
	long := strings.Repeat("a", 900000)
	rec := doCall(t, h, "POST", "/v1/goto", testToken, `{"workspaceid":"`+ws1+`","tabid":"`+long+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Body.Len() > 512 {
		t.Fatalf("the answer is %d bytes: a bad id was echoed back", rec.Body.Len())
	}
}

// F9: both mismatch checks of focus, and a tab given with the wrong workspace
func TestFocusMismatches(t *testing.T) {
	svc, fb := newSvc()
	cases := []Address{
		{BlockId: blk1, TabId: tabB},
		{BlockId: blk1, WorkspaceId: ws2},
		{TabId: tabA, WorkspaceId: ws2},
	}
	for _, a := range cases {
		if err := svc.Focus(context.Background(), a); !errors.Is(err, ErrMismatch) {
			t.Fatalf("%+v: want ErrMismatch, got %v", a, err)
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
}

// F9: a field added to Block must be added here on purpose, and nothing from the meta that is not
// allow-listed may show up in the block, whatever a new field is called
func TestBlockHasOnlyTheAllowListedFields(t *testing.T) {
	typ := reflect.TypeOf(Block{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Name)
	}
	want := []string{"BlockId", "View", "Slot", "Controller", "Connection", "Title", "Url", "Status", "ExitCode", "SessionId"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Block fields changed: %v. A new field leaks meta unless this allow-list test is updated with it.", got)
	}
	meta := map[string]any{
		"cmd": "SECRET-CMD", "cmd:cwd": "SECRET-CWD", "cmd:env": map[string]any{"K": "SECRET-ENV"},
		"cmd:args": []any{"SECRET-ARG"}, "file": "SECRET-FILE", "term:theme": "SECRET-THEME",
	}
	b := BlockFromMeta(blk1, meta)
	raw := strings.ToUpper(strings.Join([]string{b.BlockId, b.View, b.Slot, b.Controller, b.Connection, b.Title, b.Url}, "|"))
	if strings.Contains(raw, "SECRET") {
		t.Fatalf("meta leaked: %+v", b)
	}
}

// Timing cannot be unit tested. These two choices are named here so that changing them breaks a test
// and has to be argued in review: crypto/rand for the token, a constant-time compare for the check.
func TestSecurityChoicesAreInTheSource(t *testing.T) {
	tokenSrc, err := os.ReadFile("token.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tokenSrc), `"crypto/rand"`) || strings.Contains(string(tokenSrc), `"math/rand`) {
		t.Fatalf("token.go must use crypto/rand")
	}
	handlerSrc, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerSrc), "subtle.ConstantTimeCompare") {
		t.Fatalf("handler.go must compare tokens with subtle.ConstantTimeCompare")
	}
}

// A unix socket path is limited to about 108 bytes and t.TempDir() puts the test name into the path, so
// tests that start a server use a short folder. Cleanup removes the known files one by one.
func shortApiDir(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp("", "wa")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "api")
	t.Cleanup(func() {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			os.Remove(filepath.Join(dir, e.Name()))
		}
		os.Remove(dir)
		os.Remove(base)
	})
	return dir
}

func TestLongSocketPathIsRefusedWithAClearError(t *testing.T) {
	long := filepath.Join(t.TempDir(), strings.Repeat("d", 120), "api")
	svc, _ := newSvc()
	srv, err := StartServer(long, svc)
	if err == nil {
		srv.Close()
		t.Fatalf("a socket path longer than the limit was accepted")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Fatalf("the error should say the path is too long: %v", err)
	}
}

// Each of the two checks on the token file is tested alone, since either one hides the other
func TestTokenFileCheckedWhileStillATempFile(t *testing.T) {
	swapHooks(t)
	dir := t.TempDir()
	verifyPrivateFn = func(path string, isDir bool) error {
		if strings.HasSuffix(path, ".tmp") {
			return errors.New("temp file open to others")
		}
		return nil
	}
	if err := WriteTokenFile(dir, testToken); err == nil {
		t.Fatalf("a temp token file that is not private was moved into place")
	}
	if exists(filepath.Join(dir, TokenFileName)) {
		t.Fatalf("a token file was left in place")
	}
}

func TestTokenFileCheckedAgainAfterTheRename(t *testing.T) {
	swapHooks(t)
	dir := t.TempDir()
	verifyPrivateFn = func(path string, isDir bool) error {
		if filepath.Base(path) == TokenFileName {
			return errors.New("final file open to others")
		}
		return nil
	}
	if err := WriteTokenFile(dir, testToken); err == nil {
		t.Fatalf("a token file that is not private after the rename was accepted")
	}
	if exists(filepath.Join(dir, TokenFileName)) {
		t.Fatalf("the token file was left in place")
	}
}

// Status and ExitCode come from the block controller, never from block meta: a block cannot claim a status by writing one.
func TestStatusIsNeverTakenFromMeta(t *testing.T) {
	b := BlockFromMeta(blk1, map[string]any{"status": "running", "exitcode": 0, "block:status": "exited"})
	if b.Status != "" || b.ExitCode != nil {
		t.Fatalf("status came from meta: %+v", b)
	}
}
