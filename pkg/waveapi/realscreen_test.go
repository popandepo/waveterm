// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The lines of a live Claude Code session as the local API returned them (19 Oct 2026): the top rule carries the session's name;
// the prompt mark is followed by a no-break space in an empty box, and by NO separator once text is typed ("❯Reply with only ...").
var nbsp = string(rune(noBreakSpace))

func liveIdleScreen(prompt string) []string {
	promptLine := "❯" + nbsp
	if prompt != "" {
		promptLine = "❯" + prompt
	}
	return []string{
		"✻ Worked for 2s · done 21:24",
		"",
		strings.Repeat("─", 102) + " throwaway-chat3 ─",
		promptLine,
		strings.Repeat("─", 120),
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

// Found by the first live idle send on the swapped Wave: it answered 409 not sent because the box with text in it was not recognized.
// Every layout of the prompt line seen or imagined reads the same text: no separator (live), a plain space, a no-break space.
func TestTheTextIsReadWhateverFollowsThePromptMark(t *testing.T) {
	const text = "Reply with only the word pong."
	for name, mark := range map[string]string{"no separator": "❯", "plain space": "❯ ", "no-break space": "❯" + nbsp, "greater-than": "> "} {
		lines := liveIdleScreen("")
		lines[3] = mark + text
		v := readScreen(lines)
		if !v.hasBox || v.box != text {
			t.Fatalf("%s: view %+v", name, v)
		}
	}
}

// With no separator after the mark a numbered row still is a menu row and never a box.
func TestAMenuRowWithNoSeparatorIsStillAMenu(t *testing.T) {
	v := readScreen([]string{" Pick one", " ❯1. Yes", "   2. No"})
	if v.hasBox || !v.menu {
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

// The comparison after the paste removes every white space on both sides, so a no-break space inside the text is found whether the box shows it
// as a no-break space or as a plain space.
func TestTextWithAnInnerNoBreakSpaceIsFoundInTheBox(t *testing.T) {
	text := "ok" + nbsp + "there"
	for name, shown := range map[string]string{"as sent": text, "as a plain space": "ok there", "as two spaces": "ok  there"} {
		svc, fb, _ := newSendSvc(func(w []string) []string {
			if len(w) == 0 {
				return boxLines("")
			}
			return boxLines(shown)
		})
		if err := svc.Send(context.Background(), req(text)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(fb.writes) != 2 || fb.writes[0] != pasteOf(text) || fb.writes[1] != enterKey {
			t.Fatalf("%s: writes %q", name, fb.writes)
		}
	}
}

// The text rule's lower edge: one character is refused (it can answer an open menu), two are the shortest that may be sent.
func TestTheShortestTextThatMayBeSentIsTwoCharacters(t *testing.T) {
	for text, refused := range map[string]bool{"1": true, "y": true, " 1 ": true, "12": false, "ok": false, "1 ": true, "\U0001F600": true, "\U0001F600\U0001F600": false} {
		err := CheckSendText(text)
		if refused != errors.Is(err, ErrRefusedText) {
			t.Fatalf("%q: refused=%v, want %v (%v)", text, errors.Is(err, ErrRefusedText), refused, err)
		}
	}
}

func TestAPromptMarkWithoutTheFrameStillIsNotABox(t *testing.T) {
	lines := []string{"output", "❯" + nbsp + "something", "no footer here"}
	if v := readScreen(lines); v.hasBox {
		t.Fatalf("view %+v", v)
	}
}
