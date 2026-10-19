// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// Claude Code folds a long paste into a placeholder, and then the input box no longer shows the text this
	// check looks for. The live check on a throwaway session sets the real value; a text above it is refused.
	MaxSendChars = 4000

	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
	enterKey   = "\r"
	clearLine  = "\x15"

	lookAttempts = 25
	lookEvery    = 100 * time.Millisecond

	// the input box is at the bottom of the screen; older lines can hold anything Claude printed
	screenTail   = 30
	promptTail   = 12
	maxBoxLines  = 40
	minRuleRunes = 10
	footerLooks  = 3

	// first characters that make Claude's input box act instead of send
	boxCommandChars = "!/#@"
)

var (
	ErrRefusedText = errors.New("text refused")
	ErrBusy        = errors.New("busy")
	ErrOnPrompt    = errors.New("on a prompt")
	ErrNoInputBox  = errors.New("no input box")
	ErrNotRunning  = errors.New("not a running terminal")
	ErrNotSent     = errors.New("not sent")
	// ErrNotLoaded is a block whose terminal view is not in the front end: the screen only exists there, so it cannot be read.
	ErrNotLoaded = errors.New("the terminal view of this block is not loaded")
)

// the whole check depends on how Claude Code draws its input box; a layout this does not know is read as "no input
// box" and the send is refused, never guessed
var (
	promptRegexp = regexp.MustCompile(`^\s*│?\s*[>❯](?:\s(.*?))?\s*│?\s*$`)
	menuRegexp   = regexp.MustCompile(`(?i)esc to cancel|enter to confirm|do you want to|❯\s*\d+\.`)
	busyRegexp   = regexp.MustCompile(`(?i)esc to interrupt`)
	// the lines Claude Code writes under its input box; a menu's own footer (enter to select, esc to exit) is not one of them
	footerRegexp = regexp.MustCompile(`(?i)\? for shortcuts|shift\+tab|⏵⏵|⏸|accept edits|bypass permissions|plan mode|esc to interrupt|context left`)
	// a numbered row of a selection menu looks like a prompt line; text that starts with "1." after the prompt mark is
	// read as a menu too, which refuses the send rather than risk answering one
	optionRegexp = regexp.MustCompile(`❯\s*\d+\.`)
)

type SendRequest struct {
	BlockId       string `json:"blockid"`
	ExpectSession string `json:"expectsession"`
	Text          string `json:"text"`
}

type screenRequest struct {
	BlockId string `json:"blockid"`
}

type screenResponse struct {
	Lines []string `json:"lines"`
}

// screenView is what one look at the screen shows: whether there is an input box, what it holds, and whether a menu or a
// busy marker is showing outside of it (the box holds the text being sent, which may itself contain those words).
type screenView struct {
	hasBox bool
	box    string
	menu   bool
	busy   bool
}

func isRule(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "╰") || strings.Count(t, "─") >= minRuleRunes && strings.Trim(t, "─╭╮╰╯ ") == ""
}

// hasFooter looks at the few lines under the frame for one of Claude's own footer lines.
func hasFooter(below []string) bool {
	for i := 0; i < len(below) && i < footerLooks; i++ {
		if strings.TrimSpace(below[i]) == "" {
			continue
		}
		return footerRegexp.MatchString(below[i])
	}
	return false
}

func boxContent(line string) string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "│")
	t = strings.TrimSuffix(t, "│")
	return strings.TrimSpace(t)
}

func readScreen(lines []string) screenView {
	if len(lines) > screenTail {
		lines = lines[len(lines)-screenTail:]
	}
	start := -1
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-promptTail; i-- {
		if promptRegexp.MatchString(lines[i]) && !optionRegexp.MatchString(lines[i]) {
			start = i
			break
		}
	}
	var view screenView
	end := start
	if start >= 0 && start > 0 && isRule(lines[start-1]) {
		m := promptRegexp.FindStringSubmatch(lines[start])
		var parts []string
		if m[1] != "" {
			parts = append(parts, boxContent(m[1]))
		}
		for end = start + 1; end < len(lines) && end-start < maxBoxLines && !isRule(lines[end]); end++ {
			parts = append(parts, boxContent(lines[end]))
		}
		// The prompt mark alone is not an input box: a selection menu draws one too. The box is the whole frame, a rule above
		// and a rule below, with Claude's own footer under it; anything short of that is not recognized.
		if end < len(lines) && hasFooter(lines[end+1:]) {
			view.hasBox = true
			view.box = strings.Join(parts, " ")
			start--
		}
		end--
	}
	for i, l := range lines {
		if view.hasBox && i >= start && i <= end+1 {
			continue
		}
		view.menu = view.menu || menuRegexp.MatchString(l)
		view.busy = view.busy || busyRegexp.MatchString(l)
	}
	return view
}

func squash(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// CheckSendText refuses what must not reach the input box as a message: nothing, too much, anything that is not one
// plain line, and a first character that makes the box run a command, a slash command, a memory note or a file reference.
func CheckSendText(text string) error {
	if !utf8.ValidString(text) {
		return fmt.Errorf("%w: not valid text", ErrRefusedText)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: empty", ErrRefusedText)
	}
	if utf8.RuneCountInString(text) > MaxSendChars {
		return fmt.Errorf("%w: longer than %d characters", ErrRefusedText, MaxSendChars)
	}
	for _, r := range text {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return fmt.Errorf("%w: control character", ErrRefusedText)
		}
	}
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		if strings.ContainsRune(boxCommandChars, r) {
			return fmt.Errorf("%w: starts with a character the input box acts on", ErrRefusedText)
		}
		break
	}
	return nil
}

func (s *Service) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (s *Service) pause(d time.Duration) {
	if s.Pause != nil {
		s.Pause(d)
		return
	}
	time.Sleep(d)
}

func (s *Service) lockFor(blockId string) *sync.Mutex {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	if s.locks[blockId] == nil {
		s.locks[blockId] = &sync.Mutex{}
	}
	return s.locks[blockId]
}

func (s *Service) locateTerminal(ctx context.Context, blockId string) (*Block, error) {
	if err := checkId(blockId, true); err != nil {
		return nil, err
	}
	tree, err := s.B.Tree(ctx)
	if err != nil {
		return nil, err
	}
	loc, ok := findBlock(tree, blockId)
	if !ok {
		return nil, fmt.Errorf("%w: block %s", ErrNotFound, blockId)
	}
	return loc.block, nil
}

// ScreenOf is read only: it never writes to the block.
func (s *Service) ScreenOf(ctx context.Context, blockId string) ([]string, error) {
	blk, err := s.locateTerminal(ctx, blockId)
	if err != nil {
		return nil, err
	}
	if blk.View != "term" {
		return nil, fmt.Errorf("%w: block %s is not a terminal", ErrNotRunning, blockId)
	}
	return s.B.Screen(ctx, blockId)
}

// Send puts one message into the input box of the Claude session in a block and submits it. The checks and the writes
// are one sequence under the block's lock, and every write follows a look made in the same sequence: the text goes in as
// one bracketed paste, so a menu that opened after the look cannot take it as an answer, and Enter is written only after
// a second look shows the text in the box and nothing else going on. The block's session and state are read again right
// before Enter.
//
// Known limit: between the last look and the byte reaching the program a few milliseconds remain in which Claude's own screen
// can change (a menu opening after the look). The paste keeps that from answering a menu, but an Enter in that window is
// not prevented. The log holds the block id and the length only: a short text is recoverable from a short hash of it.
func (s *Service) Send(ctx context.Context, r SendRequest) (rtnErr error) {
	defer func() {
		result := "sent"
		if rtnErr != nil {
			result = rtnErr.Error()
		}
		s.logf("waveapi: send block=%s len=%d result=%q", truncateForError(r.BlockId), utf8.RuneCountInString(r.Text), truncateForError(result))
	}()
	if err := CheckSendText(r.Text); err != nil {
		return err
	}
	if err := checkId(r.BlockId, true); err != nil {
		return err
	}
	if err := checkId(r.ExpectSession, true); err != nil {
		return err
	}
	lock := s.lockFor(r.BlockId)
	lock.Lock()
	defer lock.Unlock()

	if err := s.checkTarget(ctx, r); err != nil {
		return err
	}

	lines, err := s.B.Screen(ctx, r.BlockId)
	if err != nil {
		return err
	}
	if err := checkIdle(readScreen(lines)); err != nil {
		return err
	}
	if err := s.B.Input(ctx, r.BlockId, []byte(pasteStart+r.Text+pasteEnd)); err != nil {
		return err
	}
	return s.submitWhenShown(ctx, r)
}

// checkTarget reads the block again: it must still run the expected session, as a running terminal.
func (s *Service) checkTarget(ctx context.Context, r SendRequest) error {
	blk, err := s.locateTerminal(ctx, r.BlockId)
	if err != nil {
		return err
	}
	if blk.SessionId == "" || blk.SessionId != r.ExpectSession {
		return ErrWrongSession
	}
	if blk.View != "term" || blk.Status != blockStatusRunning {
		return ErrNotRunning
	}
	return nil
}

func checkIdle(v screenView) error {
	switch {
	case v.menu:
		return ErrOnPrompt
	case !v.hasBox:
		return ErrNoInputBox
	case v.busy || v.box != "":
		return ErrBusy
	}
	return nil
}

// submitWhenShown waits for the pasted text to show in the input box. A menu stops everything with no key written: a
// key might answer it. The line is cleared only when the box holds our own text and the session is busy.
func (s *Service) submitWhenShown(ctx context.Context, r SendRequest) error {
	want := squash(r.Text)
	for i := 0; i < lookAttempts; i++ {
		s.pause(lookEvery)
		lines, err := s.B.Screen(ctx, r.BlockId)
		if err != nil {
			continue
		}
		v := readScreen(lines)
		if v.menu {
			return ErrNotSent
		}
		if !v.hasBox || squash(v.box) != want {
			continue
		}
		if v.busy {
			_ = s.B.Input(ctx, r.BlockId, []byte(clearLine))
			return ErrNotSent
		}
		// When the block changed after the first check, the box is not ours to touch: no Enter and no clearing.
		if err := s.checkTarget(ctx, r); err != nil {
			return err
		}
		return s.B.Input(ctx, r.BlockId, []byte(enterKey))
	}
	return ErrNotSent
}
