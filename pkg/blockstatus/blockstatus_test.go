// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import "testing"

func TestTheThreeWords(t *testing.T) {
	if s, c := FromShellProc("running", 0); s != Running || c != nil {
		t.Fatalf("running: %q %v", s, c)
	}
	if s, c := FromShellProc("done", 3); s != Exited || c == nil || *c != 3 {
		t.Fatalf("done keeps the exit code: %q %v", s, c)
	}
	if s, c := FromShellProc("done", 0); s != Exited || c == nil || *c != 0 {
		t.Fatalf("a clean exit is still an exit with code 0, not no code: %q %v", s, c)
	}
	if s, c := FromShellProc("init", 0); s != NotStarted || c != nil {
		t.Fatalf("init: %q %v", s, c)
	}
}

func TestUnknownIsNotRunning(t *testing.T) {
	for _, in := range []string{"", "weird", "Running", "RUNNING"} {
		if s, c := FromShellProc(in, 0); s != "" || c != nil {
			t.Fatalf("%q gives %q %v, want no status", in, s, c)
		}
	}
}
