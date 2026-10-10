// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
	"testing"
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
