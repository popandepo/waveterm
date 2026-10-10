// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

func TestSplitSessionCommand(t *testing.T) {
	tests := []struct {
		name     string
		dash     []string
		file     string
		wantCmd  string
		wantArgs []string
		wantErr  bool
	}{
		{name: "after dashes", dash: []string{"claude", "--model", "x", "a; b & c \"d\""}, wantCmd: "claude", wantArgs: []string{"--model", "x", "a; b & c \"d\""}},
		{name: "from a file", file: `["claude","-p","one;two"]`, wantCmd: "claude", wantArgs: []string{"-p", "one;two"}},
		{name: "both", dash: []string{"a"}, file: `["b"]`, wantErr: true},
		{name: "neither", wantErr: true},
		{name: "not an array", file: `{"a":1}`, wantErr: true},
		{name: "empty command", dash: []string{""}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data []byte
			if tt.file != "" {
				data = []byte(tt.file)
			}
			cmd, args, err := splitSessionCommand(tt.dash, data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if cmd != tt.wantCmd || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("got %q %q, want %q %q", cmd, args, tt.wantCmd, tt.wantArgs)
			}
		})
	}
}

func TestParseSessionEnv(t *testing.T) {
	got, err := parseSessionEnv([]string{"A=1", "B=x=y", "C="})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"A": "1", "B": "x=y", "C": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, err := parseSessionEnv([]string{"novalue"}); err == nil {
		t.Fatal("expected an error for a pair without =")
	}
	if _, err := parseSessionEnv([]string{"=x"}); err == nil {
		t.Fatal("expected an error for an empty key")
	}
}

func TestBuildSessionBlockMetaPassesArgumentsUntouched(t *testing.T) {
	prompt := "a; b & c | d \"quoted\" %PATH% ^x `tick` $(cmd)\nsecond line"
	meta := buildSessionBlockMeta("pwsh", []string{"-File", "s.ps1", "-Prompt", prompt}, `C:\work`, map[string]any{"K": "v"}, "")
	if meta["cmd:shell"] != false {
		t.Fatalf("cmd:shell must be false so that no shell re-parses the arguments, got %v", meta["cmd:shell"])
	}
	args, ok := meta["cmd:args"].([]string)
	if !ok || !reflect.DeepEqual(args, []string{"-File", "s.ps1", "-Prompt", prompt}) {
		t.Fatalf("arguments changed: %q", meta["cmd:args"])
	}
	if meta["cmd"] != "pwsh" || meta["controller"] != "cmd" || meta["view"] != "term" || meta["cmd:cwd"] != `C:\work` {
		t.Fatalf("unexpected meta %v", meta)
	}
	if meta["cmd:runonce"] != true || meta["cmd:runonstart"] != true {
		t.Fatalf("the program must start once on creation: %v", meta)
	}
	if _, has := meta["connection"]; has {
		t.Fatal("no connection unless one is given")
	}
	if got := buildSessionBlockMeta("x", nil, "c", nil, "ssh:host")["connection"]; got != "ssh:host" {
		t.Fatalf("connection not set: %v", got)
	}
}

func TestParseSessionMetaAcceptsOnlySessionKeys(t *testing.T) {
	got, err := parseSessionMeta([]string{"session:id=abc-123", "session:model=claude-sonnet-5-5", "session:name=tether notes"})
	if err != nil {
		t.Fatal(err)
	}
	if got["session:id"] != "abc-123" || got["session:model"] != "claude-sonnet-5-5" || got["session:name"] != "tether notes" || len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	for _, bad := range []string{"cmd=evil", "view=web", "controller=shell", "session:=x", "novalue", "=x", "session:id", "cmd:runonce=true", "connection=other"} {
		if _, err := parseSessionMeta([]string{bad}); err == nil {
			t.Fatalf("%q should be refused: only session:* keys with a value may be set", bad)
		}
	}
	if _, err := parseSessionMeta([]string{"session:id=a", "session:id=b"}); err == nil {
		t.Fatalf("the same key twice should be refused")
	}
	if _, err := parseSessionMeta([]string{"session:id=a\nb"}); err == nil {
		t.Fatalf("a newline in a value should be refused")
	}
}

func TestRestartModeMakesTheProgramStartAgainWithWave(t *testing.T) {
	meta := buildSessionBlockMeta("pwsh", []string{"-File", "x.ps1"}, "C:/w", nil, "")
	if meta[waveobj.MetaKey_CmdRunOnce] != true {
		t.Fatalf("the default stays run once")
	}
	applySessionOptions(meta, true, map[string]any{"session:id": "abc"})
	if meta[waveobj.MetaKey_CmdRunOnce] != false || meta[waveobj.MetaKey_CmdRunOnStart] != true {
		t.Fatalf("restart mode must start the program at every start of Wave: %v", meta)
	}
	if meta[waveobj.MetaKey_CmdClearOnStart] != false {
		t.Fatalf("restart mode keeps the earlier output")
	}
	if meta["session:id"] != "abc" {
		t.Fatalf("the session meta is carried on the block")
	}
	plain := buildSessionBlockMeta("pwsh", nil, "C:/w", nil, "")
	applySessionOptions(plain, false, nil)
	if plain[waveobj.MetaKey_CmdRunOnce] != true || plain[waveobj.MetaKey_CmdClearOnStart] != true {
		t.Fatalf("without --restart nothing changes: %v", plain)
	}
}

func TestMetaKeysCannotCarryControlCharacters(t *testing.T) {
	for _, bad := range []string{
		"session:id\ncmd:runonce=true=x",
		"session:a b=x",
		"session:id\r=x",
		"session:\x00x=x",
		"session:id/../x=x",
		"session:=x",
	} {
		if _, err := parseSessionMeta([]string{bad}); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
	if _, err := parseSessionMeta([]string{"session:model.name_1-x=v"}); err != nil {
		t.Fatalf("plain keys must pass: %v", err)
	}
}

func TestRestartRefusesAProgramThatCannotResume(t *testing.T) {
	for _, argv := range [][]string{
		{"claude", "--session-id", "11111111-2222-3333-4444-555555555555"},
		{"claude-personal", "--session-id=11111111-2222-3333-4444-555555555555", "--model", "x"},
	} {
		_, err := validateSessionStart(true, argv[0], argv[1:], nil)
		if err == nil || !strings.Contains(err.Error(), "--session-id") || !strings.Contains(err.Error(), "resume") {
			t.Fatalf("%v: want a refusal that explains the resume rule, got %v", argv, err)
		}
	}
	if _, err := validateSessionStart(false, "claude", []string{"--session-id", "x"}, nil); err != nil {
		t.Fatalf("without --restart the program runs once and the flag is fine: %v", err)
	}
	if _, err := validateSessionStart(true, "pwsh", []string{"-File", "Start-ResumableSession.ps1", "-SessionId", "x"}, nil); err != nil {
		t.Fatalf("a resumable starter is the supported way: %v", err)
	}
}

func TestInvalidSessionOptionsAreRefusedBeforeAnyTabIsCreated(t *testing.T) {
	t.Setenv("WAVETERM_WORKSPACEID", "11111111-1111-1111-1111-111111111111")
	oldMeta, oldRestart, oldName := sessionMeta, sessionRestart, sessionName
	oldFile := sessionArgsFile
	defer func() {
		sessionMeta, sessionRestart, sessionName, sessionArgsFile = oldMeta, oldRestart, oldName, oldFile
	}()
	sessionArgsFile = ""
	sessionName = "t"
	sessionMeta = []string{"cmd=evil"}
	sessionRestart = false
	err := sessionStartRun(sessionStartCmd, []string{"pwsh", "-NoProfile"})
	if err == nil || !strings.Contains(err.Error(), "--meta") {
		t.Fatalf("want the --meta error before any tab or RPC work, got %v", err)
	}
}

func TestMetaValuesRefuseControlCharacters(t *testing.T) {
	for _, bad := range []string{"session:id=a\x1b[31m", "session:id=a\x00b", "session:id=a\tb", "session:id=a\x7fb", "session:id=a\rb"} {
		if _, err := parseSessionMeta([]string{bad}); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
	if m, err := parseSessionMeta([]string{"session:name=tether notes (x) æ"}); err != nil || m["session:name"] != "tether notes (x) æ" {
		t.Fatalf("spaces and ordinary text must pass: %v %v", m, err)
	}
}

func TestRestartRefusesShellFormsAndHiddenSessionIdFlags(t *testing.T) {
	cases := [][]string{
		{"pwsh", "-Command", "claude --session-id 11111111-2222-3333-4444-555555555555"},
		{"pwsh", "-NoProfile", "-c", "claude-personal", "--resume"},
		{"cmd", "/c", "claude --session-id x"},
		{"bash", "-lc", "claude --session-id x"},
		{"powershell", "-EncodedCommand", "AAAA"},
		{"claude", "--model", "x --session-id y"},
	}
	for _, argv := range cases {
		if _, err := validateSessionStart(true, argv[0], argv[1:], nil); err == nil {
			t.Fatalf("%v should be refused with --restart", argv)
		}
	}
	for _, argv := range cases[:5] {
		if _, err := validateSessionStart(false, argv[0], argv[1:], nil); err != nil {
			t.Fatalf("%v without --restart must stay allowed: %v", argv, err)
		}
	}
	if _, err := validateSessionStart(true, "pwsh", []string{"-NoProfile", "-File", "start.ps1", "-SessionId", "x"}, nil); err != nil {
		t.Fatalf("a script starter is the supported form: %v", err)
	}
}

func TestValidateReturnsTheParsedMetaSoItIsParsedOnce(t *testing.T) {
	meta, err := validateSessionStart(false, "pwsh", nil, []string{"session:id=a", "session:model=m"})
	if err != nil || meta["session:id"] != "a" || meta["session:model"] != "m" {
		t.Fatalf("got %v %v", meta, err)
	}
}

func TestRestartAlsoRefusesTheCommonInlineInterpreterForms(t *testing.T) {
	for _, argv := range [][]string{
		{"node", "-e", "require('child_process')"},
		{"node", "--eval", "x"},
		{"node", "-p", "1"},
		{"python", "-c", "import os"},
		{"python3", "-c", "x"},
		{"py", "-c", "x"},
		{"wsl", "claude", "--resume", "x"},
		{"wsl.exe", "-e", "claude"},
		{"pwsh", "-CommandWithArgs", "x"},
		{"pwsh", "-cwa", "x"},
		{"ruby", "-e", "x"},
		{"perl", "-e", "x"},
		{`C:\Program Files\nodejs\node.exe`, "-e", "x"},
	} {
		if _, err := validateSessionStart(true, argv[0], argv[1:], nil); err == nil {
			t.Fatalf("%v should be refused with --restart", argv)
		}
		if _, err := validateSessionStart(false, argv[0], argv[1:], nil); err != nil {
			t.Fatalf("%v without --restart stays allowed: %v", argv, err)
		}
	}
	for _, argv := range [][]string{
		{"node", "start-session.js"},
		{"python", "start_session.py", "--id", "x"},
		{"pwsh", "-File", "x.ps1"},
	} {
		if _, err := validateSessionStart(true, argv[0], argv[1:], nil); err != nil {
			t.Fatalf("a script file is the supported form: %v %v", argv, err)
		}
	}
}

func TestMetaValuesRefuseUnicodeControlCharactersToo(t *testing.T) {
	for _, bad := range []string{"session:id=a\u0085b", "session:id=a\u009bb", "session:id=a\u2028b", "session:id=a\u2029b", "session:id=a\u0080"} {
		if _, err := parseSessionMeta([]string{bad}); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
}
