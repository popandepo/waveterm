// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestDeleteBlockRefusesAProgramThatMayStillRun(t *testing.T) {
	cases := []struct {
		name    string
		view    string
		status  string
		err     error
		force   bool
		refused bool
	}{
		{"running terminal", "term", "running", nil, false, true},
		{"starting terminal", "term", "init", nil, false, true},
		{"terminal without a state", "term", "none", nil, false, true},
		{"terminal with an unknown state", "term", "weird", nil, false, true},
		{"terminal whose state cannot be read", "term", "", errors.New("timeout"), false, true},
		{"tsunami app running", "tsunami", "running", nil, false, true},
		{"ended terminal", "term", "done", nil, false, false},
		{"web tile", "web", "", nil, false, false},
		{"preview", "preview", "", nil, false, false},
		{"running terminal with force", "term", "running", nil, true, false},
		{"unreadable terminal with force", "term", "", errors.New("timeout"), true, false},
	}
	for _, c := range cases {
		got := deleteBlockRefusal(c.view, c.status, c.err, c.force)
		if (got != "") != c.refused {
			t.Errorf("%s: refused=%v, got %q", c.name, got != "", got)
		}
		if c.refused && !strings.Contains(got, "--force") {
			t.Errorf("%s: the refusal must say how to go on: %q", c.name, got)
		}
	}
}
