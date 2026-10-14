// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func TestStackAndActivateAreRefusedTogetherBeforeAnything(t *testing.T) {
	if err := validateSessionStackOptions(true, "3", true); err == nil || !strings.Contains(err.Error(), "--activate") {
		t.Fatalf("want a --activate refusal, got %v", err)
	}
	if err := validateSessionStackOptions(true, "3", false); err != nil {
		t.Fatalf("a stack without activate is the normal case: %v", err)
	}
	for _, empty := range []string{"", "  "} {
		if err := validateSessionStackOptions(true, empty, false); err == nil || !strings.Contains(err.Error(), "--stack") {
			t.Fatalf("a --stack given with %q must be refused, not read as no --stack: %v", empty, err)
		}
	}
	if err := validateSessionStackOptions(false, "", true); err != nil {
		t.Fatalf("activate without a stack is the old way and stays allowed: %v", err)
	}
}

func TestStackWithActivateStopsBeforeAnyRpc(t *testing.T) {
	t.Setenv("WAVETERM_WORKSPACEID", "11111111-1111-1111-1111-111111111111")
	oldStack, oldActivate, oldName, oldFile, oldRestart, oldMeta := sessionStack, sessionActivate, sessionName, sessionArgsFile, sessionRestart, sessionMeta
	oldTab, oldCreate := sessionTabOfBlock, sessionCreateBlock
	flag := sessionStartCmd.Flags().Lookup("stack")
	defer func() {
		sessionStack, sessionActivate, sessionName, sessionArgsFile, sessionRestart, sessionMeta = oldStack, oldActivate, oldName, oldFile, oldRestart, oldMeta
		sessionTabOfBlock, sessionCreateBlock = oldTab, oldCreate
		flag.Changed = false
	}()
	called := false
	sessionTabOfBlock = func(string, string) (string, error) { called = true; return "", nil }
	sessionCreateBlock = func(wshrpc.CommandCreateBlockData) (string, error) { called = true; return "", nil }
	sessionActivate, sessionName, sessionArgsFile, sessionRestart, sessionMeta = true, "t", "", false, nil
	if err := sessionStartCmd.Flags().Set("stack", "3"); err != nil {
		t.Fatal(err)
	}
	err := sessionStartRun(sessionStartCmd, []string{"pwsh", "-NoProfile"})
	if err == nil || !strings.Contains(err.Error(), "--activate") {
		t.Fatalf("want the --activate refusal, got %v", err)
	}
	if called {
		t.Fatal("a refused request must not look anything up or create anything")
	}
}

func TestAnEmptyStackGivenOnTheCommandLineStopsBeforeAnyRpc(t *testing.T) {
	t.Setenv("WAVETERM_WORKSPACEID", "11111111-1111-1111-1111-111111111111")
	oldStack, oldActivate, oldName, oldFile, oldRestart, oldMeta, oldWs := sessionStack, sessionActivate, sessionName, sessionArgsFile, sessionRestart, sessionMeta, sessionWorkspace
	oldTab, oldCreate := sessionTabOfBlock, sessionCreateBlock
	flag := sessionStartCmd.Flags().Lookup("stack")
	defer func() {
		sessionStack, sessionActivate, sessionName, sessionArgsFile, sessionRestart, sessionMeta, sessionWorkspace = oldStack, oldActivate, oldName, oldFile, oldRestart, oldMeta, oldWs
		sessionTabOfBlock, sessionCreateBlock = oldTab, oldCreate
		flag.Changed = false
	}()
	called := false
	sessionTabOfBlock = func(string, string) (string, error) { called = true; return "", nil }
	sessionCreateBlock = func(wshrpc.CommandCreateBlockData) (string, error) { called = true; return "", nil }
	sessionActivate, sessionName, sessionArgsFile, sessionRestart, sessionMeta = false, "t", "", false, nil
	sessionWorkspace = "22222222-2222-2222-2222-222222222222"
	if err := sessionStartCmd.Flags().Set("stack", ""); err != nil {
		t.Fatal(err)
	}
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("went on to the tab path and panicked: %v", r)
			}
		}()
		err = sessionStartRun(sessionStartCmd, []string{"pwsh", "-NoProfile"})
	}()
	if err == nil || !strings.Contains(err.Error(), "--stack") || strings.Contains(err.Error(), "panicked") {
		t.Fatalf("want a --stack refusal, got %v", err)
	}
	if called {
		t.Fatal("a refused request must not look anything up or create anything")
	}
}

func TestMemberLookPutsNameColourAndSymbolOnTheBlock(t *testing.T) {
	look, err := buildSessionMemberLook("  build  ", " #2D7DD2 ", " star ")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		waveobj.MetaKey_FrameTitle:       "build",
		waveobj.MetaKey_IconColor:        "#2D7DD2",
		waveobj.MetaKey_FrameBorderColor: "#2D7DD2",
		waveobj.MetaKey_FrameIcon:        "star",
	}
	if !reflect.DeepEqual(look, want) {
		t.Fatalf("got %v want %v", look, want)
	}
	// none of the tab keys: a stacked session has no tab of its own to style
	for _, k := range []string{TabMetaFlagColor, TabMetaSymbol} {
		if _, ok := look[k]; ok {
			t.Fatalf("the tab key %s must not be written for a stacked session", k)
		}
	}
	empty, err := buildSessionMemberLook("", "", "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("nothing given must give nothing: %v %v", empty, err)
	}
}

func TestMemberLookRefusesWhatThePageWouldNotDraw(t *testing.T) {
	long := strings.Repeat("a", maxSessionMemberTitleLen+1)
	// a name of 31 characters outside the basic plane counts 62 UTF-16 units, as the page counts them
	astral := strings.Repeat("\U0001F680", 31)
	cases := []struct{ name, color, symbol string }{
		{"a\x1bb", "", ""},
		{"a\u202eb", "", ""},
		{"a\u200bb", "", ""},
		{long, "", ""},
		{astral, "", ""},
		{"", "not-a-colour", ""},
		{"", "#12", ""},
		{"", "", "a\u202eb"},
		{"", "", "\x00"},
	}
	for _, c := range cases {
		if _, err := buildSessionMemberLook(c.name, c.color, c.symbol); err == nil {
			t.Errorf("%q %q %q must be refused", c.name, c.color, c.symbol)
		}
	}
	if _, err := buildSessionMemberLook(strings.Repeat("a", maxSessionMemberTitleLen), "", ""); err != nil {
		t.Fatalf("the longest name that is drawn whole must be accepted: %v", err)
	}
}

func TestFindBlockTab(t *testing.T) {
	entries := []wshrpc.BlocksListEntry{{BlockId: "b1", TabId: "t1"}, {BlockId: "b2", TabId: "t2"}}
	if tab, err := findBlockTab(entries, "b2", "ws"); err != nil || tab != "t2" {
		t.Fatalf("got %q %v", tab, err)
	}
	if _, err := findBlockTab(entries, "nope", "ws"); err == nil || !strings.Contains(err.Error(), "--workspace") {
		t.Fatalf("an unknown block must be refused with the way out, got %v", err)
	}
	if _, err := findBlockTab(nil, "b1", "ws"); err == nil {
		t.Fatal("an empty list has no block")
	}
}

func TestStackCreateRequestTargetsTheGroupAndTakesNoFocus(t *testing.T) {
	meta := map[string]any{"a": 1}
	d := buildSessionStackCreate("tabX", "blockT", meta)
	if d.TabId != "tabX" || d.TargetBlockId != "blockT" || d.TargetAction != wshrpc.CreateBlockAction_Stack {
		t.Fatalf("bad request: %+v", d)
	}
	if d.Focused || d.Magnified {
		t.Fatalf("a stacked session never takes focus and is never magnified: %+v", d)
	}
	if d.BlockDef == nil || !reflect.DeepEqual(map[string]any(d.BlockDef.Meta), meta) {
		t.Fatalf("the block meta must be the one given: %+v", d.BlockDef)
	}
}

func TestStartSessionInStackUsesTheTabOfTheTargetAndKeepsTheProgramMeta(t *testing.T) {
	t.Setenv("WAVETERM_WORKSPACEID", "ws1")
	oldTab, oldCreate, oldResolve := sessionTabOfBlock, sessionCreateBlock, resolveBlockRef
	defer func() { sessionTabOfBlock, sessionCreateBlock, resolveBlockRef = oldTab, oldCreate, oldResolve }()
	resolveBlockRef = func(ref string) (string, error) {
		if ref != "3" {
			return "", fmt.Errorf("unknown ref %s", ref)
		}
		return "block-full-id", nil
	}
	var askedWs, askedBlock string
	sessionTabOfBlock = func(ws string, b string) (string, error) { askedWs, askedBlock = ws, b; return "tab-of-target", nil }
	var got wshrpc.CommandCreateBlockData
	sessionCreateBlock = func(d wshrpc.CommandCreateBlockData) (string, error) { got = d; return "new-block", nil }

	meta := buildSessionBlockMeta("pwsh", []string{"-File", "s.ps1"}, `C:\w`, nil, "")
	applySessionOptions(meta, true, map[string]any{"session:id": "abc"})
	look, _ := buildSessionMemberLook("build", "#2D7DD2", "star")
	out, err := startSessionInStack("ws1", "3", meta, look)
	if err != nil {
		t.Fatal(err)
	}
	if askedWs != "ws1" || askedBlock != "block-full-id" {
		t.Fatalf("the tab is looked up from the resolved block in the given workspace: %q %q", askedWs, askedBlock)
	}
	if got.TabId != "tab-of-target" || got.TargetBlockId != "block-full-id" || got.TargetAction != wshrpc.CreateBlockAction_Stack {
		t.Fatalf("bad request: %+v", got)
	}
	m := got.BlockDef.Meta
	if m[waveobj.MetaKey_Cmd] != "pwsh" || m[waveobj.MetaKey_CmdRunOnStart] != true || m["session:id"] != "abc" {
		t.Fatalf("the program, restart mode and session meta must survive: %v", m)
	}
	if m[waveobj.MetaKey_FrameTitle] != "build" || m[waveobj.MetaKey_FrameIcon] != "star" || m[waveobj.MetaKey_FrameBorderColor] != "#2D7DD2" {
		t.Fatalf("the look must be on the block: %v", m)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("one JSON object: %v (%q)", err, out)
	}
	want := map[string]string{"tabid": "tab-of-target", "blockid": "new-block", "stackedwith": "block-full-id"}
	if !reflect.DeepEqual(parsed, want) {
		t.Fatalf("got %v want %v", parsed, want)
	}
}

func TestStartSessionInStackStopsAtTheFirstFailureAndCreatesNothing(t *testing.T) {
	t.Setenv("WAVETERM_WORKSPACEID", "ws")
	oldTab, oldCreate, oldResolve := sessionTabOfBlock, sessionCreateBlock, resolveBlockRef
	defer func() { sessionTabOfBlock, sessionCreateBlock, resolveBlockRef = oldTab, oldCreate, oldResolve }()
	created := false
	sessionCreateBlock = func(wshrpc.CommandCreateBlockData) (string, error) { created = true; return "x", nil }
	resolveBlockRef = func(string) (string, error) { return "", fmt.Errorf("no such block") }
	if _, err := startSessionInStack("ws", "9", map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), "--stack") {
		t.Fatalf("an unresolvable target must be refused naming --stack, got %v", err)
	}
	resolveBlockRef = func(string) (string, error) { return "b", nil }
	sessionTabOfBlock = func(string, string) (string, error) { return "", fmt.Errorf("not in the workspace") }
	if _, err := startSessionInStack("ws", "9", map[string]any{}, nil); err == nil {
		t.Fatal("a target that is not found must be refused")
	}
	if created {
		t.Fatal("nothing may be created when the target cannot be found")
	}
	sessionTabOfBlock = func(string, string) (string, error) { return "t", nil }
	sessionCreateBlock = func(wshrpc.CommandCreateBlockData) (string, error) {
		return "", fmt.Errorf("target must be in the same tab")
	}
	if _, err := startSessionInStack("ws", "9", map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), "stack of b") {
		t.Fatalf("a refused create must name the target, got %v", err)
	}
}

func TestStackInAnotherWorkspaceTakesOnlyAFullBlockId(t *testing.T) {
	t.Setenv("WAVETERM_WORKSPACEID", "ws-here")
	oldTab, oldCreate, oldResolve := sessionTabOfBlock, sessionCreateBlock, resolveBlockRef
	defer func() { sessionTabOfBlock, sessionCreateBlock, resolveBlockRef = oldTab, oldCreate, oldResolve }()
	var resolved []string
	resolveBlockRef = func(ref string) (string, error) { resolved = append(resolved, ref); return "block-in-this-tab", nil }
	var askedWs string
	sessionTabOfBlock = func(ws string, b string) (string, error) { askedWs = ws; return "tab-there", nil }
	created := false
	sessionCreateBlock = func(wshrpc.CommandCreateBlockData) (string, error) { created = true; return "new", nil }

	for _, ref := range []string{"3", "this", "block:3",
		"urn:uuid:0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90", "0b6f3c1e4f1a4d7e9a432f1d5c7b8e90", "{0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90}",
		"0B6F3C1E-4F1A-4D7E-9A43-2F1D5C7B8E90", "block:urn:uuid:0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90", " 0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90"} {
		_, err := startSessionInStack("ws-other", ref, map[string]any{}, nil)
		if err == nil || !strings.Contains(err.Error(), "full block id") {
			t.Fatalf("%q in another workspace must be refused asking for a full block id, got %v", ref, err)
		}
	}
	if len(resolved) != 0 || askedWs != "" || created {
		t.Fatalf("a refused reference must reach no lookup and no create: resolved=%v askedWs=%q created=%v", resolved, askedWs, created)
	}

	full := "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90"
	for _, ref := range []string{full, "block:" + full} {
		resolved, askedWs, created = nil, "", false
		resolveBlockRef = func(r string) (string, error) { resolved = append(resolved, r); return full, nil }
		out, err := startSessionInStack("ws-other", ref, map[string]any{}, nil)
		if err != nil {
			t.Fatalf("%q is a full id and works from any workspace: %v", ref, err)
		}
		if askedWs != "ws-other" || !created || !strings.Contains(out, full) {
			t.Fatalf("%q: the tab must be looked up in the requested workspace: askedWs=%q created=%v out=%s", ref, askedWs, created, out)
		}
	}

	resolved = nil
	if _, err := startSessionInStack("ws-here", "3", map[string]any{}, nil); err != nil || len(resolved) != 1 {
		t.Fatalf("a number in the caller's own workspace stays allowed: err=%v resolved=%v", err, resolved)
	}
}

func TestSessionStartOutputWithoutStackIsTheOldTwoKeys(t *testing.T) {
	if got := sessionStartOutput("t", "b", ""); got != `{"blockid":"b","tabid":"t"}` {
		t.Fatalf("got %s", got)
	}
}
