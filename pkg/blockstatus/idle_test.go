// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import "testing"

func TestShellIsIdle(t *testing.T) {
	cases := []struct {
		name   string
		names  []string
		failed bool
		want   bool
	}{
		{"no children", nil, false, true},
		{"only the console host", []string{"conhost.exe"}, false, true},
		{"console helpers in any case", []string{"OpenConsole.exe", "CONHOST.EXE"}, false, true},
		{"a program is running", []string{"ping.exe"}, false, false},
		{"a program beside the console host", []string{"conhost.exe", "node.exe"}, false, false},
		{"a child that could not be named", []string{"?"}, false, false},
		{"the look failed with no names", nil, true, false},
		{"the look failed and says console host", []string{"conhost.exe"}, true, false},
	}
	for _, c := range cases {
		if got := ShellIsIdle(c.names, c.failed); got != c.want {
			t.Errorf("%s: ShellIsIdle(%v, %v) = %v, want %v", c.name, c.names, c.failed, got, c.want)
		}
	}
}
