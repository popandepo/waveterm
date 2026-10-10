// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func doCall(t *testing.T, h http.Handler, method string, path string, token string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newTestHandler() (http.Handler, *fakeBackend) {
	svc, fb := newSvc()
	return MakeHandler(testToken, svc), fb
}

// checks 1 and 2: no token and a token that differs in one character are refused, and nothing is called
func TestTokenRequired(t *testing.T) {
	h, fb := newTestHandler()
	wrong := testToken[:len(testToken)-1] + "0"
	if wrong == testToken {
		wrong = testToken[:len(testToken)-1] + "1"
	}
	for _, tok := range []string{"", wrong, testToken + "0", testToken[:10]} {
		for _, path := range []string{"/v1/list", "/v1/focus", "/v1/goto"} {
			rec := doCall(t, h, "POST", path, tok, `{"blockid":"`+blk1+`"}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("token %q on %s: want 401, got %d: %s", tok, path, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), blk1) || strings.Contains(rec.Body.String(), "workspaces") {
				t.Fatalf("a refused call returned data: %s", rec.Body.String())
			}
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made without a valid token: %v", fb.calls)
	}
}

func TestListThroughHandler(t *testing.T) {
	h, _ := newTestHandler()
	rec := doCall(t, h, "POST", "/v1/list", testToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resp.Workspaces, fixtureTree()) {
		t.Fatalf("tree: %+v", resp.Workspaces)
	}
}

func TestGotoThroughHandler(t *testing.T) {
	h, fb := newTestHandler()
	body := `{"workspaceid":"` + ws1 + `","tabid":"` + tabB + `","blockid":"` + blk2 + `"}`
	rec := doCall(t, h, "POST", "/v1/goto", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !reflect.DeepEqual(fb.calls, []string{"SetActiveTab:" + tabB, "SetBlockFocus:" + blk2}) {
		t.Fatalf("calls: %v", fb.calls)
	}
}

func TestErrorStatuses(t *testing.T) {
	h, _ := newTestHandler()
	cases := []struct {
		path string
		body string
		want int
	}{
		{"/v1/goto", `{"workspaceid":"x","tabid":"y"}`, http.StatusBadRequest},
		{"/v1/goto", `{"workspaceid":"` + ws1 + `","tabid":"` + tabC + `"}`, http.StatusConflict},
		{"/v1/goto", `{"workspaceid":"99000000-0000-0000-0000-000000000099","tabid":"` + tabA + `"}`, http.StatusNotFound},
		{"/v1/focus", `{"blockid":"` + blk2 + `"}`, http.StatusConflict},
		{"/v1/focus", `not json`, http.StatusBadRequest},
		{"/v1/nothing", `{}`, http.StatusNotFound},
	}
	for _, c := range cases {
		rec := doCall(t, h, "POST", c.path, testToken, c.body)
		if rec.Code != c.want {
			t.Fatalf("%s %s: want %d, got %d: %s", c.path, c.body, c.want, rec.Code, rec.Body.String())
		}
	}
}

func TestBackendFailureIsA500WithoutDetails(t *testing.T) {
	svc, fb := newSvc()
	fb.treeErr = context.DeadlineExceeded
	h := MakeHandler(testToken, svc)
	rec := doCall(t, h, "POST", "/v1/list", testToken, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
}

// check 14: a body over 1 MiB is refused
func TestBodyLimit(t *testing.T) {
	h, fb := newTestHandler()
	big := `{"blockid":"` + strings.Repeat("a", 2<<20) + `"}`
	rec := doCall(t, h, "POST", "/v1/focus", testToken, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", rec.Code)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
}

// check 3: tokens are 32 random bytes in hex and differ between runs
func TestTokenIsRandom(t *testing.T) {
	a, err := MakeToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := MakeToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || len(b) != 64 {
		t.Fatalf("length %d and %d", len(a), len(b))
	}
	if a == b {
		t.Fatalf("two tokens are equal")
	}
}

func TestWriteTokenFileLeavesOnlyTheToken(t *testing.T) {
	dir := t.TempDir()
	if err := WriteTokenFile(dir, testToken); err != nil {
		t.Fatal(err)
	}
	if err := WriteTokenFile(dir, testToken+"x"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTokenFile(dir)
	if err != nil || got != testToken+"x" {
		t.Fatalf("read back %q, %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != TokenFileName {
		t.Fatalf("leftover files: %v", entries)
	}
}

// ACL check (C3): after the folder and token file are locked down, only the current user may have an
// allow entry. Red if Everyone, Users or Authenticated Users appear.
func TestWindowsAclIsUserOnly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	dir := filepath.Join(t.TempDir(), "api")
	if err := PrepareApiDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteTokenFile(dir, testToken); err != nil {
		t.Fatal(err)
	}
	cur, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dir, filepath.Join(dir, TokenFileName)} {
		out, err := exec.Command("icacls", p).CombinedOutput()
		if err != nil {
			t.Fatalf("icacls %s: %v: %s", p, err, out)
		}
		text := string(out)
		for _, bad := range []string{"Everyone", "BUILTIN\\Users", "Authenticated Users"} {
			if strings.Contains(text, bad) {
				t.Fatalf("%s grants %s:\n%s", p, bad, text)
			}
		}
		if !strings.Contains(strings.ToLower(text), strings.ToLower(cur.Username)) {
			t.Fatalf("%s does not name the current user %s:\n%s", p, cur.Username, text)
		}
		if p == dir && strings.Contains(text, "(I)") {
			t.Fatalf("%s still inherits entries from its parent:\n%s", p, text)
		}
	}
}

// the real thing end to end: a listener on a unix socket in a locked-down folder, the client reading
// the token file, list and goto and a refused call
func TestSocketRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "api")
	svc, fb := newSvc()
	srv, err := StartServer(dir, svc)
	if err != nil {
		t.Skipf("cannot listen on a unix socket here: %v", err)
	}
	defer srv.Close()

	c, err := MakeClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ws, fixtureTree()) {
		t.Fatalf("tree over the socket: %+v", ws)
	}
	if err := c.Goto(context.Background(), Address{WorkspaceId: ws1, TabId: tabB, BlockId: blk2}, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fb.calls, []string{"SetActiveTab:" + tabB, "SetBlockFocus:" + blk2}) {
		t.Fatalf("calls: %v", fb.calls)
	}
	fb.calls = nil
	err = c.Goto(context.Background(), Address{WorkspaceId: ws1, TabId: tabC}, false)
	if err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("mismatch should come back as a 409 error, got %v", err)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}

	bad := &Client{Dir: dir, Token: strings.Repeat("0", 64)}
	if _, err := bad.List(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a wrong token must give 401, got %v", err)
	}
}

// a restart writes a new token; the old one stops working
func TestNewTokenAtEveryStart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "api")
	svc, _ := newSvc()
	srv1, err := StartServer(dir, svc)
	if err != nil {
		t.Skipf("cannot listen on a unix socket here: %v", err)
	}
	old, _ := ReadTokenFile(dir)
	srv1.Close()
	srv2, err := StartServer(dir, svc)
	if err != nil {
		t.Fatal(err)
	}
	defer srv2.Close()
	fresh, _ := ReadTokenFile(dir)
	if old == "" || fresh == "" || old == fresh {
		t.Fatalf("token did not change: %q %q", old, fresh)
	}
	c := &Client{Dir: dir, Token: old}
	if _, err := c.List(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("the old token must be refused, got %v", err)
	}
}

// Codex finding 1: a pre-existing folder with explicit broad access must end up user-only
func TestAclReplacesPreExistingBroadEntries(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	dir := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"Everyone", "*S-1-5-32-545", "*S-1-5-11"} {
		out, err := exec.Command("icacls", dir, "/grant", who+":(OI)(CI)R").CombinedOutput()
		if err != nil {
			t.Fatalf("setting up %s: %v: %s", who, err, out)
		}
	}
	if err := PrepareApiDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteTokenFile(dir, testToken); err != nil {
		t.Fatal(err)
	}
	cur, _ := user.Current()
	for _, p := range []string{dir, filepath.Join(dir, TokenFileName)} {
		out, _ := exec.Command("icacls", p).CombinedOutput()
		text := string(out)
		for _, bad := range []string{"Everyone", `BUILTIN\Users`, "Authenticated Users"} {
			if strings.Contains(text, bad) {
				t.Fatalf("%s still grants %s:\n%s", p, bad, text)
			}
		}
		if !strings.Contains(strings.ToLower(text), strings.ToLower(cur.Username)) {
			t.Fatalf("%s lost the current user:\n%s", p, text)
		}
	}
}

// Codex finding 2: the whole body must be one JSON value inside the limit, on every endpoint
func TestBodyMustEndAtTheFirstValue(t *testing.T) {
	h, fb := newTestHandler()
	good := `{"workspaceid":"` + ws1 + `","tabid":"` + tabB + `"}`
	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid then more than the limit", good + strings.Repeat(" ", 2<<20), http.StatusRequestEntityTooLarge},
		{"valid then a second value", good + `{}`, http.StatusBadRequest},
		{"valid then junk", good + ` x`, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := doCall(t, h, "POST", "/v1/goto", testToken, c.body)
		if rec.Code != c.want {
			t.Fatalf("%s: want %d, got %d", c.name, c.want, rec.Code)
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("a refused body still changed state: %v", fb.calls)
	}
	rec := doCall(t, h, "POST", "/v1/list", testToken, strings.Repeat("a", 2<<20))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("list must limit its body too, got %d", rec.Code)
	}
}

// Codex finding 3: POST only
func TestOnlyPostIsAccepted(t *testing.T) {
	h, fb := newTestHandler()
	for _, m := range []string{"GET", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"} {
		for _, path := range []string{"/v1/list", "/v1/focus", "/v1/goto"} {
			rec := doCall(t, h, m, path, testToken, "")
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s: want 405, got %d", m, path, rec.Code)
			}
			if rec.Header().Get("Allow") != "POST" {
				t.Fatalf("%s %s: missing Allow header", m, path)
			}
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
}
