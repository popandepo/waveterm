// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package staterecord

import (
	"encoding/json"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

const layoutWithStack = `{"id":"root","children":[
 {"id":"stk1","kind":"stack","activeChildId":"n2","children":[
   {"id":"n1","data":{"blockId":"b1"}},{"id":"n2","data":{"blockId":"b2"}},{"id":"n3","data":{"blockId":"b3"}}]},
 {"id":"n4","data":{"blockId":"b4"}}]}`

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func fixture(t *testing.T) Snapshot {
	return Snapshot{
		Workspaces: []WorkspaceSource{{
			WindowId:  "win1",
			Workspace: &waveobj.Workspace{OID: "ws1", Name: "main", TabIds: []string{"tab1", "tab2"}, ActiveTabId: "tab1"},
		}},
		Tabs: map[string]*waveobj.Tab{
			"tab1": {OID: "tab1", Name: "T1", BlockIds: []string{"b1", "b2", "b3", "b4"}, LayoutState: "lay1"},
			"tab2": {OID: "tab2", Name: "T2", BlockIds: []string{"b5"}, LayoutState: "lay2"},
		},
		Blocks: map[string]*waveobj.Block{
			"b1": {OID: "b1", Meta: waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": `"C:\Program Files\PowerShell\7\pwsh.exe" -NoProfile`, "cmd:cwd": `C:\work\one`, "frame:title": "one",
				"session:name": "tether", "session:id": "11111111-2222-3333-4444-555555555555", "session:model": "claude-sonnet-5-5", "session:effort": "low"}},
			"b2": {OID: "b2", Meta: waveobj.MetaMapType{"view": "term", "controller": "shell", "cmd:cwd": `C:\work\two`}},
			"b3": {OID: "b3", Meta: waveobj.MetaMapType{"view": "web", "url": "http://127.0.0.1:18766/?token=SECRET", "frame:title": "status"}},
			"b4": {OID: "b4", Meta: waveobj.MetaMapType{"view": "term", "controller": "shell", "cmd:env": map[string]any{"K": "SECRET"}}},
			"b5": {OID: "b5", Meta: waveobj.MetaMapType{"view": "term", "session:id": "99999999-2222-3333-4444-555555555555", "session:name": "other"}},
		},
		Layouts: map[string]*waveobj.LayoutState{
			"lay1": {OID: "lay1", RootNode: decode(t, layoutWithStack)},
			"lay2": {OID: "lay2", RootNode: decode(t, `{"id":"r2","data":{"blockId":"b5"}}`)},
		},
	}
}

func TestRecordNamesWorkspaceTabTileStackAndGroup(t *testing.T) {
	rec := BuildRecord(fixture(t))
	if rec.Version != RecordVersion || len(rec.Workspaces) != 1 {
		t.Fatalf("record: %+v", rec)
	}
	ws := rec.Workspaces[0]
	if ws.WorkspaceId != "ws1" || ws.Name != "main" || ws.WindowId != "win1" || len(ws.Tabs) != 2 {
		t.Fatalf("workspace: %+v", ws)
	}
	t1 := ws.Tabs[0]
	if t1.TabId != "tab1" || t1.Name != "T1" || t1.Position != 0 || !t1.Active || ws.Tabs[1].Active {
		t.Fatalf("tab: %+v", t1)
	}
	if len(t1.Tiles) != 2 {
		t.Fatalf("tab 1 has a stack and a plain tile: %+v", t1.Tiles)
	}
	stack := t1.Tiles[0]
	if stack.Kind != KindStack || stack.StackId != "stk1" || stack.Position != 0 || stack.ActiveBlockId != "b2" || len(stack.Group) != 3 {
		t.Fatalf("stack tile: %+v", stack)
	}
	for i, want := range []string{"b1", "b2", "b3"} {
		if stack.Group[i].BlockId != want || stack.Group[i].Position != i {
			t.Fatalf("group order: %+v", stack.Group)
		}
	}
	if !stack.Group[1].Active || stack.Group[0].Active || stack.Group[2].Active {
		t.Fatalf("exactly the active tab of the group is active: %+v", stack.Group)
	}
	plain := t1.Tiles[1]
	if plain.Kind != KindTile || plain.StackId != "" || plain.Position != 1 || len(plain.Group) != 1 || plain.Group[0].BlockId != "b4" {
		t.Fatalf("plain tile: %+v", plain)
	}
}

func TestBlockFactsAndSessionMeta(t *testing.T) {
	rec := BuildRecord(fixture(t))
	b1 := rec.Workspaces[0].Tabs[0].Tiles[0].Group[0]
	if b1.View != "term" || b1.Title != "one" || b1.Cwd != `C:\work\one` || b1.Program != "pwsh.exe" || b1.Controller != "cmd" {
		t.Fatalf("block facts: %+v", b1)
	}
	if b1.Session["name"] != "tether" || b1.Session["id"] != "11111111-2222-3333-4444-555555555555" || b1.Session["model"] != "claude-sonnet-5-5" || b1.Session["effort"] != "low" || len(b1.Session) != 4 {
		t.Fatalf("session: %+v", b1.Session)
	}
	if rec.Workspaces[0].Tabs[0].Tiles[1].Group[0].Session != nil {
		t.Fatalf("a block without session meta has no session entry")
	}
}

func TestRecordNeverCarriesSecrets(t *testing.T) {
	data, err := json.Marshal(BuildRecord(fixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, bad := range []string{"SECRET", "token=", "cmd:env", `Program Files`} {
		if contains(s, bad) {
			t.Fatalf("the record contains %q: %s", bad, s)
		}
	}
}

func contains(s string, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSessionsIndexListsEveryBlockWithASessionId(t *testing.T) {
	rec := BuildRecord(fixture(t))
	if len(rec.Sessions) != 2 {
		t.Fatalf("sessions: %+v", rec.Sessions)
	}
	s := rec.Sessions[0]
	if s.BlockId != "b1" || s.WorkspaceId != "ws1" || s.WorkspaceName != "main" || s.TabId != "tab1" || s.TabName != "T1" ||
		s.TilePosition != 0 || s.Kind != KindStack || s.StackId != "stk1" || s.GroupPosition != 0 || s.Session["id"] != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("session index entry: %+v", s)
	}
	o := rec.Sessions[1]
	if o.BlockId != "b5" || o.Kind != KindTile || o.StackId != "" || o.TabName != "T2" {
		t.Fatalf("second entry: %+v", o)
	}
}

func TestBlocksMissingFromTheLayoutAreStillRecorded(t *testing.T) {
	snap := fixture(t)
	snap.Tabs["tab1"].BlockIds = append(snap.Tabs["tab1"].BlockIds, "b6")
	snap.Blocks["b6"] = &waveobj.Block{OID: "b6", Meta: waveobj.MetaMapType{"view": "term"}}
	rec := BuildRecord(snap)
	found := false
	for _, tile := range rec.Workspaces[0].Tabs[0].Tiles {
		for _, g := range tile.Group {
			found = found || g.BlockId == "b6"
		}
	}
	if !found {
		t.Fatalf("a block that the saved layout does not show yet (the page saves 100 ms later) must still be in the record")
	}
}

func TestMissingLayoutOrBlockDoesNotBreakTheRecord(t *testing.T) {
	snap := fixture(t)
	delete(snap.Layouts, "lay2")
	delete(snap.Blocks, "b3")
	rec := BuildRecord(snap)
	if len(rec.Workspaces[0].Tabs) != 2 {
		t.Fatalf("both tabs are still there")
	}
	for _, g := range rec.Workspaces[0].Tabs[0].Tiles[0].Group {
		if g.BlockId == "b3" {
			t.Fatalf("a block that cannot be read is left out, not invented")
		}
	}
}

func TestSameStateGivesTheSameContent(t *testing.T) {
	a, _ := json.Marshal(BuildRecord(fixture(t)).WithoutStamp())
	b, _ := json.Marshal(BuildRecord(fixture(t)).WithoutStamp())
	if string(a) != string(b) {
		t.Fatalf("the record must be deterministic so that an unchanged state is not rewritten")
	}
}

func TestProgramNameIsOnlyTheExecutable(t *testing.T) {
	cases := map[string]string{
		"pwsh -File x.ps1 --token abc":                                   "pwsh",
		`"C:\Program Files\PowerShell\7\pwsh.exe" -NoProfile -Token abc`: "pwsh.exe",
		`C:\Windows\System32\cmd.exe /c claude --resume SECRET`:          "cmd.exe",
		"/usr/bin/bash -c 'curl http://x/?token=SECRET'":                 "bash",
		"claude":                "claude",
		"":                      "",
		`'C:\a b\tool.exe' --x`: "tool.exe",
		"/usr/bin/python script.exe --token SECRET":                       "python",
		"python script.exe --token SECRET":                                "python",
		`"unmatched C:\x\tool.exe --token SECRET`:                         "",
		`'unmatched --token SECRET`:                                       "",
		`C:\Program Files\PowerShell\7\pwsh.exe -File x --api-key=SECRET`: "",
		`C:\tools\my app\run.cmd arg`:                                     "",
		`"safe"evil --arg`:                                                "",
		`"safe" --arg`:                                                    "safe",
		`"safe"`:                                                          "safe",
		"\"safe\"\n--arg":                                                 "safe",
		"\"safe\"\t--arg":                                                 "safe",
		"\"safe\"\r\n":                                                    "safe",
		"claude\n--arg":                                                   "claude",
		"\"safe\" x":                                                      "",
		`'safe'evil`:                                                      "",
		"cmd;rm -rf x":                                                    "",
		`"a;b" arg`:                                                       "",
	}
	for in, want := range cases {
		if got := programName(in); got != want {
			t.Errorf("%q: want %q, got %q", in, want, got)
		}
		if contains(programName(in), "SECRET") || contains(programName(in), " ") {
			t.Errorf("%q leaked arguments: %q", in, programName(in))
		}
	}
}

func TestOnlyTheFieldsThatPutASessionBackAreRecorded(t *testing.T) {
	snap := fixture(t)
	snap.Blocks["b1"].Meta["session:token"] = "TOP-SECRET"
	snap.Blocks["b1"].Meta["session:api-key"] = "KEY"
	snap.Blocks["b1"].Meta["session:permissionmode"] = "auto"
	snap.Blocks["b1"].Meta["session:status-session"] = "tether-1a2b"
	snap.Blocks["b1"].Meta["session:status-tab"] = "tether"
	snap.Blocks["b1"].Meta["session:name"] = string(make([]byte, 400))
	rec := BuildRecord(snap)
	s := rec.Workspaces[0].Tabs[0].Tiles[0].Group[0].Session
	for _, key := range []string{"id", "model", "effort", "permissionmode", "status-session", "status-tab"} {
		if s[key] == "" {
			t.Fatalf("%s must be recorded: %v", key, s)
		}
	}
	if _, bad := s["token"]; bad {
		t.Fatalf("a session:token must never reach the file: %v", s)
	}
	if _, bad := s["api-key"]; bad {
		t.Fatalf("an unknown session key must not reach the file: %v", s)
	}
	if _, long := s["name"]; long {
		t.Fatalf("an over-long value is dropped, not truncated into something misleading")
	}
	data, _ := json.Marshal(rec)
	if contains(string(data), "TOP-SECRET") || contains(string(data), "KEY\"") {
		t.Fatalf("secret in the file")
	}
}

func TestOnlyBlocksOfTheTabAreRecordedAndEachOnce(t *testing.T) {
	snap := fixture(t)
	snap.Blocks["ghost"] = &waveobj.Block{OID: "ghost", Meta: waveobj.MetaMapType{"view": "term"}}
	snap.Layouts["lay1"].RootNode = decode(t, `{"id":"root","children":[
 {"id":"stk1","kind":"stack","activeChildId":"n2","children":[
   {"id":"n1","data":{"blockId":"b1"}},{"id":"n2","data":{"blockId":"b2"}},{"id":"ng","data":{"blockId":"ghost"}},{"id":"n3","data":{"blockId":"b3"}}]},
 {"id":"n4","data":{"blockId":"b4"}},
 {"id":"n4b","data":{"blockId":"b4"}},
 {"id":"n5","data":{"blockId":"b5"}}]}`)
	rec := BuildRecord(snap)
	seen := map[string]int{}
	for _, tab := range rec.Workspaces[0].Tabs {
		for _, tile := range tab.Tiles {
			for _, g := range tile.Group {
				seen[g.BlockId]++
			}
		}
	}
	if seen["ghost"] != 0 {
		t.Fatalf("a block that the layout names but the tab does not hold is a ghost: %v", seen)
	}
	if seen["b5"] != 1 || seen["b5"] > 1 {
		t.Fatalf("b5 belongs to the other tab and is recorded there only: %v", seen)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("block %s is recorded %d times", id, n)
		}
	}
	for _, tile := range rec.Workspaces[0].Tabs[0].Tiles {
		for _, g := range tile.Group {
			if g.BlockId == "b5" {
				t.Fatalf("b5 is not a block of tab 1")
			}
		}
	}
}

func TestABlockListedTwiceAndMissingFromTheLayoutIsOneTile(t *testing.T) {
	snap := fixture(t)
	snap.Tabs["tab2"].BlockIds = []string{"b5", "b2", "b2"}
	snap.Layouts["lay2"] = &waveobj.LayoutState{OID: "lay2", RootNode: decode(t, `{"id":"r2","data":{"blockId":"b5"}}`)}
	rec := BuildRecord(snap)
	seen := map[string]int{}
	for _, tile := range rec.Workspaces[0].Tabs[1].Tiles {
		for _, g := range tile.Group {
			seen[g.BlockId]++
		}
	}
	if seen["b2"] != 1 || seen["b5"] != 1 {
		t.Fatalf("each block is shown once, got %v", seen)
	}
}

func TestAMissingWorkspaceObjectIsLeftOut(t *testing.T) {
	snap := fixture(t)
	snap.Workspaces = append([]WorkspaceSource{{WindowId: "gone", Workspace: nil}}, snap.Workspaces...)
	rec := BuildRecord(snap)
	if len(rec.Workspaces) != 1 || rec.Workspaces[0].WorkspaceId != "ws1" {
		t.Fatalf("a workspace that is gone is left out: %+v", rec.Workspaces)
	}
}

func TestTheStatusOfAProgramIsInTheRecordAndAbsentWhenUnknown(t *testing.T) {
	snap := fixture(t)
	code := 3
	snap.Statuses = map[string]ProcStatus{"b1": {Status: "running"}, "b2": {Status: "exited", ExitCode: &code}, "b3": {}}
	rec := BuildRecord(snap)
	group := rec.Workspaces[0].Tabs[0].Tiles[0].Group
	byId := map[string]BlockRec{}
	for _, g := range group {
		byId[g.BlockId] = g
	}
	if byId["b1"].Status != "running" || byId["b1"].ExitCode != nil {
		t.Fatalf("b1: %+v", byId["b1"])
	}
	if byId["b2"].Status != "exited" || byId["b2"].ExitCode == nil || *byId["b2"].ExitCode != 3 {
		t.Fatalf("b2: %+v", byId["b2"])
	}
	if byId["b3"].Status != "" {
		t.Fatalf("a block without a status has none: %+v", byId["b3"])
	}
	data, _ := json.Marshal(byId["b3"])
	if contains(string(data), `"status":`) || contains(string(data), `"exitcode"`) {
		t.Fatalf("empty status must not be written: %s", data)
	}
}

func TestAStatusChangeChangesTheRecordContent(t *testing.T) {
	a := fixture(t)
	a.Statuses = map[string]ProcStatus{"b1": {Status: "running"}}
	b := fixture(t)
	b.Statuses = map[string]ProcStatus{"b1": {Status: "exited"}}
	ja, _ := json.Marshal(BuildRecord(a).WithoutStamp())
	jb, _ := json.Marshal(BuildRecord(b).WithoutStamp())
	if string(ja) == string(jb) {
		t.Fatalf("a program that exited must change the content")
	}
}
