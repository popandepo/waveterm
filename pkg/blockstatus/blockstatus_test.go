// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import (
	"strings"
	"testing"
)

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

func TestDeleteRefusalIsSilentOnlyWhenTheAnswerIsKnown(t *testing.T) {
	silent := []struct{ view, status string }{
		{"term", "done"}, {"tsunami", "done"}, {"vdom", "done"},
		{"web", ""}, {"preview", ""}, {"waveai", ""}, {"sysinfo", ""}, {"launcher", ""}, {"processviewer", ""},
	}
	for _, c := range silent {
		if got := DeleteRefusal(c.view, c.status, true); got != "" {
			t.Errorf("%s/%s must be allowed, got %q", c.view, c.status, got)
		}
	}
	refused := []struct {
		name, view, status string
		have               bool
	}{
		{"running", "term", "running", true},
		{"starting", "term", "init", true},
		{"no controller", "term", "", true},
		{"unknown state", "term", "weird", true},
		{"unreadable state", "term", "", false},
		{"unreadable state even for done", "term", "done", false},
		{"unknown view", "somethingnew", "done", true},
		{"missing view", "", "done", true},
		{"mistyped view", "Web", "", true},
		{"running vdom", "vdom", "running", true},
	}
	for _, c := range refused {
		got := DeleteRefusal(c.view, c.status, c.have)
		if got == "" {
			t.Errorf("%s must be refused", c.name)
		}
		if !strings.Contains(got, "--force") {
			t.Errorf("%s: the refusal must say how to go on: %q", c.name, got)
		}
	}
}
