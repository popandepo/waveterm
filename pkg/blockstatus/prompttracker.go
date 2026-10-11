// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import (
	"bytes"
	"sync"
	"time"
)

// The shell's two private markers, both OSC 16162 commands of the shell integration protocol that the window ignores:
//
//	ESC ] 16162 ; P ; <token> BEL   the prompt function ran (the shell is about to wait for a line)
//	ESC ] 16162 ; B ; <token> BEL   the shell CONSUMED an Enter (its Enter key binding runs just before the line is accepted)
//	ESC ] 16162 ; Q ; <token> BEL   printed right after P when the shell's console input holds no key at that moment: nothing is queued for it
//	ESC ] 16162 ; N ; <token> BEL   the shell's Enter binding is no longer the one that reports B, or another key accepts a line: from now on this shell is never at prompt
//
// The token is made by wavesrv for each shell start and given to the shell in its environment, so that bytes which a program prints (or
// echoes, or a file holds) do not pass for a marker. A program that reads the environment of the shell to forge it is not defended against.
const (
	markerPrefix = "\x1b]16162;"
	markerEnd    = 7
	maxMarkerLen = 96
)

// PromptTracker says whether a shell waits at its prompt, from the two byte streams that pass wavesrv: what the shell prints and what is sent to
// it. It is built to fail towards "not at prompt":
//   - no token, no markers ever seen: never at prompt;
//   - every Enter (CR or LF byte) written to the shell counts as pending until the shell reports (marker B) that it consumed one. A prompt marker
//     only means "at prompt" when nothing is pending, so a type-ahead Enter that is still queued when the prompt of the earlier command is
//     printed does not count (ConPTY reorders and delays output, so no order of the bytes can be trusted, only the counts);
//   - a shell whose Enter binding was replaced (a profile) never reports B, so pending never returns to 0 and the shell never counts as at prompt
//     (and it says N at its next prompt, which ends it at once);
//   - Enters that the shell never consumes (typed into Read-Host, vim, a confirm prompt, Shift+Enter sent as a line feed) would keep pending above 0
//     for good, so pending is forgiven when the SHELL says that nothing is queued: marker Q, which its prompt function prints only when the console
//     input holds no key. A timer is not used: how long a queued Enter waits under load is not known. An Enter written less than QuietAge before the Q
//     arrives may still be on its way to the console input (so Q cannot have seen it), and is not forgiven;
//   - input that is not plain typing (function keys, custom chords, control characters) ends the state until the next prompt marker.
//
// Ctrl+C (byte 3), Escape, Backspace, Delete and the plain cursor keys keep it. Known gaps: a program that forges the token, and a command that
// starts without an Enter.
type PromptTracker struct {
	lock       sync.Mutex
	token      string
	atPrompt   bool
	broken     bool
	pending    int
	generation uint64
	lastEnter  time.Time // when the last Enter was written
	qWaiting   bool      // a Q arrived while the newest pending Enter was too young for it to speak for
	now        func() time.Time
	tail       []byte // the end of the output so far that may be the start of a marker split between two reads
}

// QuietAge is how long before a Q marker an Enter must have been written to count as seen by it (the Enter had time to reach the shell's console input).
const QuietAge = 300 * time.Millisecond

// InFlightWait: an Enter written less than QuietAge before a Q may have been on its way to the shell's console input when Q was decided, so Q cannot
// speak for it. If the shell has not reported consuming it (B) this long after it was written, it went to a program and is forgiven. Only these few Enters
// ever wait on a timer; the remaining risk is a shell that is stalled for longer than this at the very moment such an Enter arrives.
const InFlightWait = 1500 * time.Millisecond

func (p *PromptTracker) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// SetToken sets the token the shell was given ("" disables the tracker) and starts over.
func (p *PromptTracker) SetToken(token string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.token = token
	p.resetLocked()
}

// Reset is for a new shell process (the token stays).
func (p *PromptTracker) Reset() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.resetLocked()
}

func (p *PromptTracker) resetLocked() {
	p.atPrompt = false
	p.broken = false
	p.pending = 0
	p.lastEnter = time.Time{}
	p.qWaiting = false
	p.tail = p.tail[:0]
	p.generation++
}

// Output feeds what the shell printed.
func (p *PromptTracker) Output(b []byte) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.token == "" {
		return
	}
	data := append(append([]byte{}, p.tail...), b...)
	rest := data
	for {
		i := bytes.Index(rest, []byte(markerPrefix))
		if i < 0 {
			break
		}
		rest = rest[i:]
		end := bytes.IndexByte(rest, markerEnd)
		if end < 0 {
			if len(rest) > maxMarkerLen {
				rest = rest[len(markerPrefix):] // not a marker of ours: go on after the prefix
				continue
			}
			p.tail = append(p.tail[:0], rest...) // maybe the start of a marker; wait for the rest
			return
		}
		p.handleMarkerLocked(string(rest[len(markerPrefix):end]))
		rest = rest[end+1:]
	}
	// the end of the data may be the start of a prefix
	keep := len(markerPrefix) - 1
	if len(rest) < keep {
		keep = len(rest)
	}
	p.tail = append(p.tail[:0], rest[len(rest)-keep:]...)
}

func (p *PromptTracker) handleMarkerLocked(body string) {
	// body is "P;<token>" or "B;<token>"; anything else (other commands of the protocol, a wrong token) is ordinary output
	if len(body) < 2 || body[1] != ';' || body[2:] != p.token {
		return
	}
	switch body[0] {
	case 'P':
		p.generation++
		if p.broken {
			return
		}
		p.qWaiting = false
		p.atPrompt = p.pending == 0
	case 'Q':
		p.generation++
		p.qWaiting = false
		if p.broken {
			return
		}
		// the shell says its console input is empty: Enters that were pending went to a program, unless one was written so recently that it
		// may not have reached the console input yet (then it waits, see AtPrompt)
		if p.pending > 0 && !p.lastEnter.IsZero() {
			if p.clock().Sub(p.lastEnter) >= QuietAge {
				p.pending = 0
			} else {
				p.qWaiting = true
			}
		}
		p.atPrompt = p.pending == 0
	case 'B':
		if p.pending > 0 {
			p.pending--
		}
		p.qWaiting = false
		p.atPrompt = false
		p.generation++
	case 'N':
		p.broken = true
		p.qWaiting = false
		p.atPrompt = false
		p.generation++
	}
}

// Input feeds what was written to the shell (typed, pasted, or sent by a script).
func (p *PromptTracker) Input(b []byte) {
	enters, keeps := classifyInput(b)
	if enters == 0 && keeps {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.pending += enters
	if enters > 0 {
		p.lastEnter = p.clock()
	}
	p.qWaiting = false
	p.atPrompt = false
	p.generation++
}

// classifyInput counts the Enter bytes (CR and LF) and says whether the rest is plain typing and editing that cannot submit a line:
// printable characters, Tab, Backspace, Delete, Ctrl+C, Escape on its own, and the cursor, Home, End, Insert, Delete, PageUp and PageDown keys.
func classifyInput(b []byte) (enters int, keeps bool) {
	keeps = true
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == 13 || c == 10:
			enters++
		case c == 9 || c == 3 || c == 8 || c == 127 || (c >= 32 && c != 127):
			// Tab, Ctrl+C, Backspace, Delete, printable (and the bytes of a multi-byte character)
		case c == 27:
			n := plainKeyLength(b[i:])
			if n == 0 {
				keeps = false
			} else {
				i += n - 1
			}
		default:
			keeps = false
		}
	}
	return enters, keeps
}

// plainKeyLength is the length of an escape sequence for a plain editing key at the start of b, or 1 for a lone Escape (the end of the input or
// a printable character after it is Alt+key, which is not plain), or 0.
func plainKeyLength(b []byte) int {
	if len(b) == 1 {
		return 1
	}
	if b[1] == '[' {
		j := 2
		for j < len(b) && (b[j] >= '0' && b[j] <= '9' || b[j] == ';') {
			j++
		}
		if j >= len(b) {
			return 0
		}
		final := b[j]
		params := string(b[2:j])
		switch final {
		case 'A', 'B', 'C', 'D', 'H', 'F':
			return j + 1
		case '~':
			switch params {
			case "1", "2", "3", "4", "5", "6", "7", "8":
				return j + 1
			}
		}
		return 0
	}
	if b[1] == 'O' && len(b) >= 3 {
		switch b[2] {
		case 'A', 'B', 'C', 'D', 'H', 'F':
			return 3
		}
	}
	return 0
}

// AtPrompt says whether the shell waits at its prompt, and a generation number that changes with every event that could change the answer, so that
// a caller can ask before and after a slow look and refuse the answer if anything happened in between.
func (p *PromptTracker) AtPrompt() (bool, uint64) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.qWaiting && !p.broken && p.pending > 0 && p.clock().Sub(p.lastEnter) >= InFlightWait {
		p.pending = 0
		p.qWaiting = false
		p.atPrompt = true
		p.generation++
	}
	return p.atPrompt, p.generation
}
