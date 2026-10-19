// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"strings"
	"testing"
)

// The lines of a live Claude Code session as the local API returned them (19 Oct 2026): the prompt mark is followed by a
// no-break space, and the top rule carries the session's name.
var nbsp = string(rune(noBreakSpace))

func liveIdleScreen(prompt string) []string {
	return []string{
		"✻ Worked for 5s · done 14:02",
		"",
		strings.Repeat("─", 24) + " throwaway-chat ─",
		"❯" + nbsp + prompt,
		strings.Repeat("─", 41),
		"  ⏵⏵ auto mode on (shift+tab to cycle)",
	}
}

func TestTheLiveIdleBoxIsRecognized(t *testing.T) {
	v := readScreen(liveIdleScreen(""))
	if !v.hasBox || v.box != "" || v.menu || v.busy {
		t.Fatalf("view %+v", v)
	}
}

func TestTextInTheLiveBoxIsRead(t *testing.T) {
	v := readScreen(liveIdleScreen("Reply with only the word pong."))
	if !v.hasBox || v.box != "Reply with only the word pong." {
		t.Fatalf("view %+v", v)
	}
}

func TestTheLiveBoxWhileWorkingIsBusy(t *testing.T) {
	lines := append([]string{"✻ Working (3s · esc to interrupt)"}, liveIdleScreen("")...)
	v := readScreen(lines)
	if !v.hasBox || !v.busy {
		t.Fatalf("view %+v", v)
	}
}

// A menu row that is drawn with a no-break space after the mark must still count as a menu: reading it as plain text would let a send answer it.
func TestAMenuRowWithANoBreakSpaceIsAMenu(t *testing.T) {
	lines := []string{
		" Do you want to create hello.txt?",
		" ❯" + nbsp + "1. Yes",
		"   2. No",
		" Esc to cancel",
	}
	v := readScreen(lines)
	if v.hasBox || !v.menu {
		t.Fatalf("view %+v", v)
	}
}

// Nothing else on the screen says "menu" here: only the numbered row with the no-break space does.
func TestABareMenuRowWithANoBreakSpaceIsAMenu(t *testing.T) {
	v := readScreen([]string{" Pick one", " ❯" + nbsp + "1. Yes", "   2. No"})
	if v.hasBox || !v.menu {
		t.Fatalf("view %+v", v)
	}
}

func TestOnlyAShortLabelInAnOtherwiseUnbrokenRuleCountsAsARule(t *testing.T) {
	rule := strings.Repeat("─", 24)
	for _, tc := range []struct {
		name string
		line string
		want bool
	}{
		{"plain", strings.Repeat("─", 41), true},
		{"labeled", rule + " throwaway-chat ─", true},
		{"label too long", rule + " " + strings.Repeat("x", 80) + " ─", false},
		{"rule inside text", "see " + rule + " there ─", false},
		{"no trailing rule", rule + " throwaway-chat", false},
		{"label holds a rule character", rule + " a ─ b ─", false},
		{"short run", "──── name ─", false},
	} {
		if got := isRule(tc.line); got != tc.want {
			t.Fatalf("%s: isRule(%q) = %v, want %v", tc.name, tc.line, got, tc.want)
		}
	}
}

func TestAPromptMarkWithoutTheFrameStillIsNotABox(t *testing.T) {
	lines := []string{"output", "❯" + nbsp + "something", "no footer here"}
	if v := readScreen(lines); v.hasBox {
		t.Fatalf("view %+v", v)
	}
}
