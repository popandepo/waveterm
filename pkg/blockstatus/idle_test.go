// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import "testing"

const (
	sys32 = `C:\Windows\System32`
	wave  = `C:\Users\x\AppData\Local\Programs\WaveFork\abc`
)

var trusted = []string{sys32, wave}

func TestShellIsIdle(t *testing.T) {
	conhost := ChildProc{Name: "conhost.exe", Exe: sys32 + `\conhost.exe`}
	cases := []struct {
		name   string
		below  []ChildProc
		failed bool
		want   bool
	}{
		{"nothing below", nil, false, true},
		{"only the console host from the system folder", []ChildProc{conhost}, false, true},
		{"the console host in other case and slashes", []ChildProc{{Name: "CONHOST.EXE", Exe: `c:/windows/system32/ConHost.exe`}}, false, true},
		{"OpenConsole from Wave's own folder", []ChildProc{{Name: "OpenConsole.exe", Exe: wave + `\OpenConsole.exe`}}, false, true},
		{"a program is running", []ChildProc{{Name: "ping.exe", Exe: sys32 + `\PING.EXE`}}, false, false},
		{"a program beside the console host", []ChildProc{conhost, {Name: "node.exe", Exe: `C:\n\node.exe`}}, false, false},
		{"a program NAMED conhost.exe in another folder", []ChildProc{{Name: "conhost.exe", Exe: `C:\Users\x\evil\conhost.exe`}}, false, false},
		{"a program named conhost.exe whose path is unknown", []ChildProc{{Name: "conhost.exe"}}, false, false},
		{"a folder that only starts like the trusted one", []ChildProc{{Name: "conhost.exe", Exe: `C:\Windows\System32x\conhost.exe`}}, false, false},
		{"the look failed", nil, true, false},
		{"the look failed and shows only a console host", []ChildProc{conhost}, true, false},
	}
	for _, c := range cases {
		if got := ShellIsIdle(c.below, c.failed, trusted); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNoTrustedFolderMeansNoHelperIsTrusted(t *testing.T) {
	if ShellIsIdle([]ChildProc{{Name: "conhost.exe", Exe: sys32 + `\conhost.exe`}}, false, nil) {
		t.Fatalf("with no trusted folder every child is a program")
	}
}
