// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const (
	ws1  = "11111111-1111-1111-1111-111111111111"
	ws2  = "22222222-2222-2222-2222-222222222222"
	win1 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	win2 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	tabA = "a0000000-0000-0000-0000-00000000000a"
	tabB = "b0000000-0000-0000-0000-00000000000b"
	tabC = "c0000000-0000-0000-0000-00000000000c"
	blk1 = "01000000-0000-0000-0000-000000000001"
	blk2 = "02000000-0000-0000-0000-000000000002"
	blk3 = "03000000-0000-0000-0000-000000000003"
)

// two workspaces: ws1 has tabs A (active) and B, ws2 has tab C
func fixtureTree() []Workspace {
	return []Workspace{
		{WorkspaceId: ws1, WindowId: win1, Name: "main", ActiveTabId: tabA, Tabs: []Tab{
			{TabId: tabA, Name: "A", Blocks: []Block{{BlockId: blk1, View: "term"}}},
			{TabId: tabB, Name: "B", Blocks: []Block{{BlockId: blk2, View: "web"}}},
		}},
		{WorkspaceId: ws2, WindowId: win2, Name: "other", ActiveTabId: tabC, Tabs: []Tab{
			{TabId: tabC, Name: "C", Blocks: []Block{{BlockId: blk3, View: "term"}}},
		}},
	}
}

type fakeBackend struct {
	tree    []Workspace
	treeErr error
	calls   []string

	// writes holds every byte string handed to Input, in order; screenFn builds the screen from them so a test can
	// change what is shown between two looks.
	writes    []string
	screenFn  func(writes []string) []string
	screenErr error
	inputErr  error
	looks     int
}

func (f *fakeBackend) Screen(ctx context.Context, blockId string) ([]string, error) {
	f.looks++
	if f.screenErr != nil {
		return nil, f.screenErr
	}
	if f.screenFn == nil {
		return nil, nil
	}
	return f.screenFn(f.writes), nil
}

func (f *fakeBackend) Input(ctx context.Context, blockId string, data []byte) error {
	if f.inputErr != nil {
		return f.inputErr
	}
	f.writes = append(f.writes, string(data))
	return nil
}

func (f *fakeBackend) Tree(ctx context.Context) ([]Workspace, error) { return f.tree, f.treeErr }
func (f *fakeBackend) FocusWindow(ctx context.Context, windowId string) error {
	f.calls = append(f.calls, "FocusWindow:"+windowId)
	return nil
}
func (f *fakeBackend) SetActiveTab(ctx context.Context, workspaceId string, tabId string) error {
	f.calls = append(f.calls, "SetActiveTab:"+tabId)
	return nil
}
func (f *fakeBackend) SetBlockFocus(ctx context.Context, tabId string, blockId string) error {
	f.calls = append(f.calls, "SetBlockFocus:"+blockId)
	return nil
}

func newSvc() (*Service, *fakeBackend) {
	fb := &fakeBackend{tree: fixtureTree()}
	return &Service{B: fb}, fb
}

// check 4: the tree keeps every block under its own tab and workspace, background tabs included
func TestListKeepsShape(t *testing.T) {
	svc, _ := newSvc()
	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fixtureTree()) {
		t.Fatalf("tree changed: %+v", got)
	}
	if got[0].Tabs[1].Blocks[0].BlockId != blk2 {
		t.Fatalf("background tab block lost")
	}
}

// check 5: only allow-listed meta keys leave the process
func TestBlockFromMetaAllowList(t *testing.T) {
	meta := map[string]any{
		"view":        "term",
		"slot":        "left",
		"controller":  "shell",
		"connection":  "livedb",
		"frame:title": "my title",
		"cmd:env":     map[string]any{"SECRET": "x"},
		"cmd":         "pwsh -secret",
		"url":         "https://example.com/reset/SECRETPATH?token=abc#frag",
		"cmd:cwd":     "C:/private",
		"session:id":  "0a1b2c3d-0000-4000-8000-0123456789ab",
	}
	b := BlockFromMeta(blk1, meta)
	if b.SessionId != "0a1b2c3d-0000-4000-8000-0123456789ab" {
		t.Fatalf("session id missing: %+v", b)
	}
	for _, bad := range []any{"not-a-guid", "0A1B2C3D-0000-4000-8000-0123456789AB", 7, nil} {
		if got := BlockFromMeta(blk1, map[string]any{"session:id": bad}).SessionId; got != "" {
			t.Fatalf("session id %v must not pass: %q", bad, got)
		}
	}
	if b.View != "term" || b.Slot != "left" || b.Controller != "shell" || b.Connection != "livedb" || b.Title != "my title" {
		t.Fatalf("allowed keys missing: %+v", b)
	}
	if b.Url != "https://example.com" {
		t.Fatalf("url must keep scheme and host only: %q", b.Url)
	}
	raw := strings.ToLower(strings.Join([]string{b.View, b.Slot, b.Controller, b.Connection, b.Title, b.Url}, "|"))
	for _, bad := range []string{"secret", "private", "token", "frag", "reset"} {
		if strings.Contains(raw, bad) {
			t.Fatalf("leaked %q in %+v", bad, b)
		}
	}
}

// check 6: focus of a block in the active tab is one SetBlockFocus and nothing else
func TestFocusBlockInActiveTab(t *testing.T) {
	svc, fb := newSvc()
	if err := svc.Focus(context.Background(), Address{BlockId: blk1}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fb.calls, []string{"SetBlockFocus:" + blk1}) {
		t.Fatalf("calls: %v", fb.calls)
	}
}

func TestFocusBlockInBackgroundTabIsRefused(t *testing.T) {
	svc, fb := newSvc()
	err := svc.Focus(context.Background(), Address{BlockId: blk2})
	if !errors.Is(err, ErrNotActive) {
		t.Fatalf("want ErrNotActive, got %v", err)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("no call may be made: %v", fb.calls)
	}
}

func TestFocusTab(t *testing.T) {
	svc, fb := newSvc()
	if err := svc.Focus(context.Background(), Address{TabId: tabB}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fb.calls, []string{"SetActiveTab:" + tabB}) {
		t.Fatalf("calls: %v", fb.calls)
	}
	fb.calls = nil
	if err := svc.Focus(context.Background(), Address{TabId: tabA}); err != nil {
		t.Fatal(err)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("focusing the already active tab must change nothing: %v", fb.calls)
	}
}

// check 7: unknown ids change nothing
func TestFocusUnknown(t *testing.T) {
	svc, fb := newSvc()
	for _, a := range []Address{
		{BlockId: "99000000-0000-0000-0000-000000000099"},
		{TabId: "99000000-0000-0000-0000-000000000099"},
		{},
	} {
		err := svc.Focus(context.Background(), a)
		if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrBadId) {
			t.Fatalf("%+v: want not found or bad id, got %v", a, err)
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("mutation called: %v", fb.calls)
	}
}

// check 8: window=false must never call FocusWindow; window=true calls it first
func TestGotoOrder(t *testing.T) {
	svc, fb := newSvc()
	if err := svc.Goto(context.Background(), Address{WorkspaceId: ws1, TabId: tabB, BlockId: blk2}, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fb.calls, []string{"SetActiveTab:" + tabB, "SetBlockFocus:" + blk2}) {
		t.Fatalf("window=false calls: %v", fb.calls)
	}
	fb.calls = nil
	if err := svc.Goto(context.Background(), Address{WorkspaceId: ws2, TabId: tabC, BlockId: blk3}, true); err != nil {
		t.Fatal(err)
	}
	// tab C is already active in ws2, so only the window and the block focus are needed
	if !reflect.DeepEqual(fb.calls, []string{"FocusWindow:" + win2, "SetBlockFocus:" + blk3}) {
		t.Fatalf("window=true calls: %v", fb.calls)
	}
}

func TestGotoTabOnly(t *testing.T) {
	svc, fb := newSvc()
	if err := svc.Goto(context.Background(), Address{WorkspaceId: ws1, TabId: tabB}, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fb.calls, []string{"SetActiveTab:" + tabB}) {
		t.Fatalf("calls: %v", fb.calls)
	}
}

// checks 9 and 10: a wrong combination is refused before any call
func TestGotoMismatchesMakeNoCall(t *testing.T) {
	svc, fb := newSvc()
	cases := []Address{
		{WorkspaceId: ws1, TabId: tabC},                // tab belongs to the other workspace
		{WorkspaceId: ws1, TabId: tabA, BlockId: blk2}, // block is in another tab
		{WorkspaceId: ws2, TabId: tabC, BlockId: blk1}, // block is in another workspace
	}
	for _, a := range cases {
		if err := svc.Goto(context.Background(), a, true); !errors.Is(err, ErrMismatch) {
			t.Fatalf("%+v: want ErrMismatch, got %v", a, err)
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
}

func TestGotoUnknownWorkspace(t *testing.T) {
	svc, fb := newSvc()
	err := svc.Goto(context.Background(), Address{WorkspaceId: "99000000-0000-0000-0000-000000000099", TabId: tabA}, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
}

// check 11: going to the active tab does not switch tabs again
func TestGotoActiveTabDoesNotSwitch(t *testing.T) {
	svc, fb := newSvc()
	if err := svc.Goto(context.Background(), Address{WorkspaceId: ws1, TabId: tabA, BlockId: blk1}, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fb.calls, []string{"SetBlockFocus:" + blk1}) {
		t.Fatalf("calls: %v", fb.calls)
	}
}

// check 13: ids must be uuids; nothing else (spaces, percent, newline, prefixes) reaches the backend
func TestBadIdsAreRefusedBeforeAnyCall(t *testing.T) {
	svc, fb := newSvc()
	bad := []string{"", "tab:1", "a b", "100%", "x\ny", tabA + "\n", tabA[:8], "../" + tabA, strings.ToUpper(tabA) + "z"}
	for _, id := range bad {
		if id == "" {
			continue
		}
		if err := svc.Goto(context.Background(), Address{WorkspaceId: ws1, TabId: id}, true); !errors.Is(err, ErrBadId) {
			t.Fatalf("tab id %q: want ErrBadId, got %v", id, err)
		}
		if err := svc.Goto(context.Background(), Address{WorkspaceId: id, TabId: tabA}, true); !errors.Is(err, ErrBadId) {
			t.Fatalf("workspace id %q: want ErrBadId, got %v", id, err)
		}
		if err := svc.Focus(context.Background(), Address{BlockId: id}); !errors.Is(err, ErrBadId) {
			t.Fatalf("block id %q: want ErrBadId, got %v", id, err)
		}
	}
	if len(fb.calls) != 0 {
		t.Fatalf("calls made: %v", fb.calls)
	}
}

func TestGotoNeedsWorkspaceAndTab(t *testing.T) {
	svc, _ := newSvc()
	if err := svc.Goto(context.Background(), Address{TabId: tabA}, false); !errors.Is(err, ErrBadId) {
		t.Fatalf("missing workspace: %v", err)
	}
	if err := svc.Goto(context.Background(), Address{WorkspaceId: ws1}, false); !errors.Is(err, ErrBadId) {
		t.Fatalf("missing tab: %v", err)
	}
}

// Codex finding 4: no path at all, and nothing for urls without a host
func TestBlockUrlKeepsOnlySchemeAndHost(t *testing.T) {
	cases := map[string]string{
		"https://user:pw@example.com:8443/reset/abc": "https://example.com:8443",
		"http://127.0.0.1:18766/":                    "http://127.0.0.1:18766",
		"about:blank":                                "",
		"file:///C:/private/secret.txt":              "",
		"not a url %%":                               "",
	}
	for in, want := range cases {
		got := BlockFromMeta(blk1, map[string]any{"url": in}).Url
		if got != want {
			t.Fatalf("%q: want %q, got %q", in, want, got)
		}
	}
}
