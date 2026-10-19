// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import "testing"

// The question of a permission menu as Claude Code draws it (live, 19 Oct 2026: " Do you want to create second.txt?") is a menu on its own,
// without any numbered row: the send is refused. This is the refusal the anchored term must keep.
func TestTheQuestionOfAMenuAtTheStartOfALineIsAMenu(t *testing.T) {
	for _, row := range []string{
		" Do you want to create second.txt?",
		"Do you want to proceed?",
		"   DO YOU WANT TO make this edit to a.txt?",
		"│ Do you want to run this command? │",
		"│Do you want to proceed?",
	} {
		v := readScreen([]string{" Create file", " second.txt", row, " Esc to go back"})
		if v.hasBox || !v.menu {
			t.Fatalf("%q: view %+v", row, v)
		}
		// and above an idle box, with nothing else on the screen saying menu
		w := readScreen(append([]string{row}, liveIdleScreen("")...))
		if !w.menu {
			t.Fatalf("%q above an idle box: view %+v", row, w)
		}
		if err := checkIdle(w); err == nil {
			t.Fatalf("%q above an idle box: the idle check passed", row)
		}
	}
}

// A sentence that only mentions the question in the middle of a line, above an idle box, is output and not a menu: the send is not refused.
func TestTheQuestionInTheMiddleOfALineIsNotAMenu(t *testing.T) {
	for _, output := range []string{
		"● I asked: do you want to see the result?",
		"  and then asked whether do you want to continue is a bad question",
		"the prompt reads \"Do you want to proceed?\" in the permission dialog",
		"> Do you want to know more?",
	} {
		v := readScreen(append([]string{output}, liveIdleScreen("")...))
		if v.menu || !v.hasBox {
			t.Fatalf("%q: view %+v", output, v)
		}
		if err := checkIdle(v); err != nil {
			t.Fatalf("%q: an idle box was refused: %v", output, err)
		}
	}
}

// Known limit, pinned so that it is not a surprise: a line of output that itself begins with the question is read as a menu, and the send is refused
// (it fails safe).
func TestALineOfOutputThatStartsWithTheQuestionStillRefuses(t *testing.T) {
	v := readScreen(append([]string{"Do you want to see the result?"}, liveIdleScreen("")...))
	if !v.menu {
		t.Fatalf("view %+v", v)
	}
}

// The footers are not anchored: they come after other words on their line.
func TestAFooterAfterOtherWordsIsStillAMenu(t *testing.T) {
	for _, row := range []string{" Enter to select · ↑/↓ to navigate · Esc to cancel", "  Press Enter to confirm", " Tab to amend · esc to cancel"} {
		v := readScreen([]string{" Pick one", " ❯ Default", row})
		if v.hasBox || !v.menu {
			t.Fatalf("%q: view %+v", row, v)
		}
	}
}
