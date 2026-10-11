// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import (
	"testing"
	"time"
)

const (
	cr   = 13
	lf   = 10
	ctrC = 3
	esc  = 27
	tok  = "tok123"
)

func mark(kind byte, token string) []byte {
	return append(append([]byte(markerPrefix), kind, ';'), append([]byte(token), 7)...)
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func newTracker() (*PromptTracker, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	p := &PromptTracker{now: clk.Now}
	p.SetToken(tok)
	return p, clk
}

func at(p *PromptTracker) bool {
	v, _ := p.AtPrompt()
	return v
}

func TestNoTokenOrNoMarkerIsNeverAtPrompt(t *testing.T) {
	var p PromptTracker
	p.Output(mark('P', tok))
	if at(&p) {
		t.Fatalf("without a token the tracker is off")
	}
	q, _ := newTracker()
	q.Output([]byte("PS C:\\> "))
	if at(q) {
		t.Fatalf("output without the marker must not say at prompt")
	}
}

func TestOnlyTheRightTokenCounts(t *testing.T) {
	p, _ := newTracker()
	p.Output(mark('P', "other"))
	p.Output(mark('P', ""))
	p.Output([]byte{0x1b, ']', '1', '6', '1', '6', '2', ';', 'A', 7}) // the existing prompt command of the protocol
	if at(p) {
		t.Fatalf("a marker without the shell's token is ordinary output (a printed or echoed marker)")
	}
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("the right token counts")
	}
}

func TestTheMarkerIsFoundWhenSplitBetweenReadsAndAmongOtherOutput(t *testing.T) {
	m := mark('P', tok)
	for split := 0; split <= len(m); split++ {
		p, _ := newTracker()
		p.Output(append([]byte("output before "), m[:split]...))
		if split < len(m) && at(p) {
			t.Fatalf("split %d: half a marker is not a marker", split)
		}
		p.Output(append(append([]byte{}, m[split:]...), []byte("\x1b[2J\x1b[HPS> \x1b[?25h")...))
		if !at(p) {
			t.Fatalf("split %d: a marker split between two reads must be found", split)
		}
	}
	p, _ := newTracker()
	p.Output(append(append([]byte("a"), mark('P', "other")...), append(mark('P', tok), []byte("tail")...)...))
	if !at(p) {
		t.Fatalf("two markers in one read: the second is ours")
	}
}

func TestEnterWrittenIsPendingUntilTheShellSaysItConsumedIt(t *testing.T) {
	p, _ := newTracker()
	p.Output(mark('P', tok))
	p.Input([]byte("Start-Sleep 20\r"))
	if at(p) {
		t.Fatalf("an Enter ends at-prompt at once")
	}
	p.Output(mark('B', tok)) // the shell consumed it
	if at(p) {
		t.Fatalf("a running command is not at prompt")
	}
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("the prompt after the command, nothing pending, is at prompt")
	}
}

func TestTypeAheadIsNotAtPromptWhileTheQueuedEnterIsStillPending(t *testing.T) {
	// the real sequence from a ConPTY run: "Start-Sleep 2" CR, then "Get-Date" CR typed ahead; the first prompt's marker arrives long after
	// the second Enter was written, and the B of the second Enter 12 ms after it
	p, _ := newTracker()
	p.Output(mark('P', tok))
	p.Input([]byte("Start-Sleep 2\r"))
	p.Output(mark('B', tok))
	p.Input([]byte("Get-Date\r")) // typed ahead while the first runs
	p.Output(mark('P', tok))      // the first command's prompt
	if at(p) {
		t.Fatalf("a stale prompt marker with an Enter still queued must not say at prompt")
	}
	p.Output(mark('B', tok)) // the queued Get-Date accepted
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("after both commands the prompt is at prompt")
	}
}

func TestTwoLinesInOneWriteAndAPasteGiveTwoPendingEnters(t *testing.T) {
	p, _ := newTracker()
	p.Output(mark('P', tok))
	p.Input([]byte("Get-Date\rGet-Date\r"))
	p.Output(mark('B', tok))
	p.Output(mark('P', tok))
	if at(p) {
		t.Fatalf("one of two Enters is still pending")
	}
	p.Output(mark('B', tok))
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("both consumed, then the prompt")
	}
}

func TestAReplacedEnterBindingIsNeverAtPrompt(t *testing.T) {
	// no B ever comes: pending stays above 0 after the first Enter. Within the resync wait it stays not-at-prompt.
	p, clk := newTracker()
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("a shell that has not been used yet is at prompt")
	}
	p.Input([]byte("ls\r"))
	p.Output(mark('P', tok))
	if at(p) {
		t.Fatalf("an Enter nobody reported must keep the shell not-at-prompt")
	}
	clk.t = clk.t.Add(ResyncWait - time.Millisecond)
	if at(p) {
		t.Fatalf("not before the resync wait")
	}
	// the shell then says its binding is not ours any more: broken for good, also after the wait
	p.Output(mark('N', tok))
	clk.t = clk.t.Add(time.Hour)
	if at(p) {
		t.Fatalf("a broken shell is never at prompt")
	}
	p.Output(mark('P', tok))
	if at(p) {
		t.Fatalf("a prompt marker does not repair a broken shell")
	}
}

func TestABrokenShellWithNothingPendingIsStillNeverAtPrompt(t *testing.T) {
	p, _ := newTracker()
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("setup: at prompt")
	}
	p.Output(mark('N', tok))
	if at(p) {
		t.Fatalf("N ends at-prompt at once")
	}
	p.Output(mark('P', tok))
	if at(p) {
		t.Fatalf("with nothing pending, a prompt marker still must not repair a shell that reported N")
	}
	p.SetToken(tok) // a new shell start
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("a new shell start begins unbroken")
	}
}

func TestEntersThatAProgramConsumedAreForgivenAfterThePromptStoodUnanswered(t *testing.T) {
	p, clk := newTracker()
	p.Output(mark('P', tok))
	p.Input([]byte("Read-Host\r"))
	p.Output(mark('B', tok))
	p.Input([]byte("answer\r")) // typed into the program: the shell never consumes this one
	p.Input([]byte("vim\r"))    // also not
	p.Output(mark('P', tok))    // the command is done, the prompt is back
	if at(p) {
		t.Fatalf("right after the prompt, pending Enters still count")
	}
	clk.t = clk.t.Add(ResyncWait - time.Millisecond)
	if at(p) {
		t.Fatalf("not before the wait")
	}
	clk.t = clk.t.Add(2 * time.Millisecond)
	if !at(p) {
		t.Fatalf("a prompt that stood for the wait with no B after it means nothing is queued: at prompt")
	}
}

func TestInputDuringTheWaitCancelsTheForgiveness(t *testing.T) {
	p, clk := newTracker()
	p.Output(mark('P', tok))
	p.Input([]byte("answer\r"))
	p.Output(mark('P', tok))
	clk.t = clk.t.Add(time.Second)
	p.Input([]byte("x")) // plain typing keeps the state: the wait goes on
	clk.t = clk.t.Add(600 * time.Millisecond)
	if !at(p) {
		t.Fatalf("typing is not an Enter and does not cancel the wait")
	}
	p2, clk2 := newTracker()
	p2.Output(mark('P', tok))
	p2.Input([]byte("answer\r"))
	p2.Output(mark('P', tok))
	clk2.t = clk2.t.Add(time.Second)
	p2.Input([]byte("\r"))
	clk2.t = clk2.t.Add(2 * time.Second)
	if at(p2) {
		t.Fatalf("a new Enter during the wait starts the count again: not at prompt")
	}
}

func TestAGenerationChangeAcrossASlowLookIsVisible(t *testing.T) {
	p, _ := newTracker()
	p.Output(mark('P', tok))
	_, g1 := p.AtPrompt()
	p.Input([]byte("\r")) // an Enter during the process look
	_, g2 := p.AtPrompt()
	if g1 == g2 {
		t.Fatalf("an Enter must change the generation so that the caller can refuse an answer it got before")
	}
	p.Output(mark('B', tok))
	p.Output(mark('P', tok))
	_, g3 := p.AtPrompt()
	if g3 == g2 {
		t.Fatalf("markers change the generation too")
	}
}

func TestTypingCancellingAndPlainEditingKeysKeepAtPrompt(t *testing.T) {
	keys := map[string][]byte{
		"printable":     []byte("Get-Chi"),
		"Ctrl+C":        {ctrC},
		"Escape":        {esc},
		"Backspace":     {127, 8},
		"Tab":           {9},
		"arrows":        []byte("\x1b[A\x1b[B\x1b[C\x1b[D"),
		"Home and End":  []byte("\x1b[H\x1b[F\x1b[1~\x1b[4~"),
		"Delete":        []byte("\x1b[3~"),
		"ctrl+arrow":    []byte("\x1b[1;5C"),
		"SS3 arrows":    []byte("\x1bOA\x1bOH"),
		"multibyte":     []byte("caf\xc3\xa9"),
		"PageUp/Down":   []byte("\x1b[5~\x1b[6~"),
		"Insert":        []byte("\x1b[2~"),
		"Escape+typing": append([]byte{esc}, []byte("[")...),
	}
	for name, input := range keys {
		if name == "Escape+typing" {
			continue // an Escape followed by a character is Alt+key, covered below
		}
		p, _ := newTracker()
		p.Output(mark('P', tok))
		p.Input(input)
		if !at(p) {
			t.Errorf("%s must keep at-prompt", name)
		}
	}
}

func TestOtherKeysEndAtPromptUntilTheNextPrompt(t *testing.T) {
	keys := map[string][]byte{
		"F5":           []byte("\x1b[15~"),
		"F1 (SS3)":     []byte("\x1bOP"),
		"Alt+x":        []byte("\x1bx"),
		"Ctrl+A":       {1},
		"Ctrl+L":       {12},
		"unknown CSI":  []byte("\x1b[200~"),
		"focus report": []byte("\x1b[I"),
	}
	for name, input := range keys {
		p, _ := newTracker()
		p.Output(mark('P', tok))
		p.Input(input)
		if at(p) {
			t.Errorf("%s may be a custom accept key and must end at-prompt", name)
		}
		p.Output(mark('P', tok))
		if !at(p) {
			t.Errorf("%s: the next prompt marker puts it back", name)
		}
	}
}

func TestCtrlCAtThePromptKeepsItAndTheRedrawnPromptBringsItAgain(t *testing.T) {
	p, _ := newTracker()
	p.Output(mark('P', tok))
	p.Input([]byte{ctrC})
	p.Output(mark('P', tok)) // PSReadLine prints a new prompt after a cancelled line
	if !at(p) {
		t.Fatalf("Ctrl+C at the prompt is still at prompt")
	}
	p.Input([]byte("Start-Sleep 30\r"))
	p.Output(mark('B', tok))
	p.Input([]byte{ctrC}) // stops the command
	if at(p) {
		t.Fatalf("Ctrl+C alone is not a prompt")
	}
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("the prompt after the stopped command")
	}
}

func TestAFullScreenProgramKeepsItBusyAndTheShellsPromptReturnsIt(t *testing.T) {
	p, clk := newTracker()
	p.Output(mark('P', tok))
	p.Input([]byte("vim x.txt\r"))
	p.Output(mark('B', tok))
	p.Output(append([]byte("\x1b[?1049h"), []byte("screen contents")...))
	p.Input([]byte(":q\r")) // Enter inside the program: pending, the shell never consumes it
	p.Output([]byte("\x1b[?1049l"))
	if at(p) {
		t.Fatalf("a program that has taken over the screen is not a prompt, also after it left the screen")
	}
	p.Output(mark('P', tok))
	clk.t = clk.t.Add(ResyncWait)
	if !at(p) {
		t.Fatalf("the shell's prompt after the program exits, with the wait passed, is at prompt")
	}
}

func TestResetStartsOverButKeepsTheToken(t *testing.T) {
	p, _ := newTracker()
	p.Output(mark('P', tok)[:5])
	p.Reset()
	p.Output(mark('P', tok)[5:])
	if at(p) {
		t.Fatalf("a reset forgets half a marker")
	}
	p.Output(mark('P', tok))
	if !at(p) {
		t.Fatalf("the token is kept")
	}
	p.Reset()
	if at(p) {
		t.Fatalf("a reset ends at-prompt")
	}
}
