// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import "testing"

const (
	cr   = 13
	lf   = 10
	ctrC = 3
	esc  = 27
)

func TestNoMarkerIsNeverAtPrompt(t *testing.T) {
	var p PromptTracker
	if p.AtPrompt() {
		t.Fatalf("a new tracker is not at prompt")
	}
	p.Output([]byte("PS C:\\> "))
	p.Output([]byte{esc, ']', '1', '6', '1', '6', '2', ';', 'A', 7}) // the existing "prompt" command of the protocol is not our marker
	if p.AtPrompt() {
		t.Fatalf("output without the marker must not say at prompt")
	}
}

func TestTheMarkerSetsAtPromptAlsoWhenSplitBetweenReads(t *testing.T) {
	for split := 0; split <= len(PromptMarker); split++ {
		var p PromptTracker
		p.Output(append([]byte("output before "), PromptMarker[:split]...))
		if split < len(PromptMarker) && p.AtPrompt() {
			t.Fatalf("split %d: half a marker is not a marker", split)
		}
		p.Output(append(append([]byte{}, PromptMarker[split:]...), []byte("PS C:\\> ")...))
		if !p.AtPrompt() {
			t.Fatalf("split %d: a marker split between two reads must be found", split)
		}
	}
}

func TestEnterEndsAtPromptWhateverTheShellDoesWithIt(t *testing.T) {
	cases := map[string][]byte{
		"a typed Enter":                 {cr},
		"a line sent by a script":       []byte("Get-Date\r"),
		"a line feed":                   {lf},
		"a paste with a newline":        []byte("a\nb\n"),
		"a bracketed paste with a line": append(append([]byte{esc, '[', '2', '0', '0', '~'}, []byte("x\ry")...), esc, '[', '2', '0', '1', '~'),
	}
	for name, input := range cases {
		var p PromptTracker
		p.Output(PromptMarker)
		p.Input(input)
		if p.AtPrompt() {
			t.Errorf("%s must end at-prompt", name)
		}
	}
}

func TestTypingWithoutEnterAndCancellingKeepAtPrompt(t *testing.T) {
	var p PromptTracker
	p.Output(PromptMarker)
	p.Input([]byte("Get-Chi"))
	p.Input([]byte{ctrC}) // a cancelled line: the shell prints its prompt again, with the marker
	p.Input([]byte{esc})  // Escape clears the line
	if !p.AtPrompt() {
		t.Fatalf("typing, Ctrl+C and Escape never start a command")
	}
	p.Output(PromptMarker)
	if !p.AtPrompt() {
		t.Fatalf("the prompt printed again after Ctrl+C is still at prompt")
	}
}

func TestACommandThatRunsKeepsItBusyUntilTheNextPrompt(t *testing.T) {
	var p PromptTracker
	p.Output(PromptMarker)
	p.Input([]byte("Start-Sleep 20\r"))
	p.Output([]byte("some output of the command\r\n"))
	if p.AtPrompt() {
		t.Fatalf("a running command is not at prompt")
	}
	// a paste with newlines while it runs stays busy
	p.Input([]byte("next\nline\n"))
	if p.AtPrompt() {
		t.Fatalf("a paste while a command runs stays busy")
	}
	// Ctrl+C stops it, the prompt returns with its marker
	p.Input([]byte{ctrC})
	if p.AtPrompt() {
		t.Fatalf("Ctrl+C alone is not a prompt")
	}
	p.Output(PromptMarker)
	if !p.AtPrompt() {
		t.Fatalf("the prompt after the command is at prompt")
	}
}

func TestAFullScreenProgramLeavesAtPromptOnlyWhenTheShellPromptsAgain(t *testing.T) {
	var p PromptTracker
	p.Output(PromptMarker)
	p.Input([]byte("vim x.txt\r"))
	p.Output(append([]byte{esc, '[', '?', '1', '0', '4', '9', 'h'}, []byte("screen contents")...)) // alternate screen on
	p.Input([]byte(":q\r"))                                                                        // Enter inside the program
	p.Output([]byte{esc, '[', '?', '1', '0', '4', '9', 'l'})                                       // alternate screen off
	if p.AtPrompt() {
		t.Fatalf("a program that has taken over the screen is not a prompt, also after it left the screen")
	}
	p.Output(PromptMarker)
	if !p.AtPrompt() {
		t.Fatalf("the shell's prompt after the program exits is at prompt")
	}
}

func TestResetStartsOver(t *testing.T) {
	var p PromptTracker
	p.Output(PromptMarker[:5])
	p.Reset()
	p.Output(PromptMarker[5:])
	if p.AtPrompt() {
		t.Fatalf("a reset forgets half a marker")
	}
	p.Output(PromptMarker)
	p.Reset()
	if p.AtPrompt() {
		t.Fatalf("a reset ends at-prompt")
	}
}
