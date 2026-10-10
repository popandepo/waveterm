// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
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
