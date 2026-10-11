// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import (
	"bytes"
	"sync"
)

// PromptMarker is what the PowerShell start-up script prints each time its prompt function runs: ESC ] 16162 ; P BEL. It is a private
// command of the shell integration protocol (the window ignores commands it does not know).
var PromptMarker = []byte{0x1b, ']', '1', '6', '1', '6', '2', ';', 'P', 0x07}

// PromptTracker says whether a shell is waiting at its prompt, from the two byte streams that pass wavesrv: what the shell prints and what is
// sent to it. It is deliberately simple and errs towards "not at prompt":
//   - "at prompt" starts only when the prompt marker has been seen in the output;
//   - ANY input that contains a carriage return or a line feed (a typed Enter, a paste with a newline, a line sent by a script) ends it, whatever
//     the shell does with it;
//   - a shell that never prints the marker (integration off, an old shell, cmd.exe, a profile that replaced the prompt) is never at prompt.
//
// Ctrl+C is the byte 0x03: it does not end the state (a cancelled line, or a stopped command, makes the shell print its prompt again, and the
// marker with it). Known gap: input that was sent before the previous prompt's marker was read can leave "at prompt" set while that input runs.
type PromptTracker struct {
	lock     sync.Mutex
	atPrompt bool
	tail     []byte // the end of the last output, in case the marker is split between two reads
}

func (p *PromptTracker) Output(b []byte) {
	p.lock.Lock()
	defer p.lock.Unlock()
	data := append(append([]byte{}, p.tail...), b...)
	if i := bytes.LastIndex(data, PromptMarker); i >= 0 {
		p.atPrompt = true
	}
	keep := len(PromptMarker) - 1
	if len(data) < keep {
		keep = len(data)
	}
	p.tail = append(p.tail[:0], data[len(data)-keep:]...)
}

func (p *PromptTracker) Input(b []byte) {
	if bytes.IndexByte(b, 13) < 0 && bytes.IndexByte(b, 10) < 0 {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.atPrompt = false
}

func (p *PromptTracker) AtPrompt() bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.atPrompt
}

// Reset is for a new shell process.
func (p *PromptTracker) Reset() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.atPrompt = false
	p.tail = p.tail[:0]
}
