// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	sessA = "0a1b2c3d-0000-4000-8000-0123456789ab"
	sessB = "0a1b2c3d-0000-4000-8000-0123456789ac"
)

func pasteOf(text string) string { return "\x1b[200~" + text + "\x1b[201~" }

func boxLines(text string) []string {
	return []string{
		"  earlier output",
		"╭────────────────────────────────────╮",
		"│ > " + text + strings.Repeat(" ", 20) + "│",
		"╰────────────────────────────────────╯",
		"  ? for shortcuts",
	}
}

func menuLines() []string {
	return []string{
		" Bash command",
		"   rm thing",
		" Do you want to proceed?",
		" ❯ 1. Yes",
		"   2. No",
		" Esc to cancel",
	}
}

func busyLines() []string {
	return append(boxLines(""), "  ✻ Working (12s · esc to interrupt)")
}

// what the input box would show after these writes: the text of a paste that nothing has submitted or cleared yet
func typedAfter(writes []string) string {
	if len(writes) == 0 {
		return ""
	}
	last := writes[len(writes)-1]
	if strings.HasPrefix(last, "\x1b[200~") && strings.HasSuffix(last, "\x1b[201~") {
		return strings.TrimSuffix(strings.TrimPrefix(last, "\x1b[200~"), "\x1b[201~")
	}
	return ""
}

func followsPaste(writes []string) []string { return boxLines(typedAfter(writes)) }

func sendTree(view string, status string, session string) []Workspace {
	return []Workspace{{WorkspaceId: ws1, WindowId: win1, Name: "main", ActiveTabId: tabA, Tabs: []Tab{
		{TabId: tabA, Name: "A", Blocks: []Block{{BlockId: blk1, View: view, Controller: "shell", Status: status, SessionId: session}}},
	}}}
}

func newSendSvc(screenFn func([]string) []string) (*Service, *fakeBackend, *[]string) {
	fb := &fakeBackend{tree: sendTree("term", "running", sessA), screenFn: screenFn}
	var logs []string
	var mu sync.Mutex
	svc := &Service{B: fb, Pause: func(time.Duration) {}, Logf: func(f string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(f, a...))
	}}
	return svc, fb, &logs
}

func req(text string) SendRequest {
	return SendRequest{BlockId: blk1, ExpectSession: sessA, Text: text}
}

func wantNoWrites(t *testing.T, fb *fakeBackend) {
	t.Helper()
	if len(fb.writes) != 0 {
		t.Fatalf("bytes were written: %q", fb.writes)
	}
}

func TestSendWritesOnePasteThenEnter(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	if err := svc.Send(context.Background(), req("hello")); err != nil {
		t.Fatal(err)
	}
	want := []string{pasteOf("hello"), "\r"}
	if fmt.Sprintf("%q", fb.writes) != fmt.Sprintf("%q", want) {
		t.Fatalf("writes %q, want %q", fb.writes, want)
	}
}

func TestSendKeepsNonAsciiTextInOnePaste(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	if err := svc.Send(context.Background(), req("héllo wörld 日本")); err != nil {
		t.Fatal(err)
	}
	if len(fb.writes) != 2 || fb.writes[0] != pasteOf("héllo wörld 日本") {
		t.Fatalf("writes %q", fb.writes)
	}
}

func TestSendRefusesWrongSessionOrNoSession(t *testing.T) {
	for _, tc := range []struct{ name, block, expect string }{
		{"other session", sessB, sessA},
		{"block without session", "", sessA},
		{"no expected session", sessA, ""},
		{"expected session not a guid", sessA, "x"},
	} {
		svc, fb, _ := newSendSvc(followsPaste)
		fb.tree = sendTree("term", "running", tc.block)
		err := svc.Send(context.Background(), SendRequest{BlockId: blk1, ExpectSession: tc.expect, Text: "hi"})
		if err == nil {
			t.Fatalf("%s: was sent", tc.name)
		}
		wantNoWrites(t, fb)
	}
}

func TestSendRefusesBlockThatIsNotARunningTerminal(t *testing.T) {
	for _, tc := range []struct{ view, status string }{{"term", "exited"}, {"term", "notstarted"}, {"term", ""}, {"web", "running"}} {
		svc, fb, _ := newSendSvc(followsPaste)
		fb.tree = sendTree(tc.view, tc.status, sessA)
		if err := svc.Send(context.Background(), req("hi")); err == nil {
			t.Fatalf("%+v: was sent", tc)
		}
		wantNoWrites(t, fb)
	}
}

func TestSendRefusesUnknownBlockAndBadId(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	err := svc.Send(context.Background(), SendRequest{BlockId: blk2, ExpectSession: sessA, Text: "hi"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown block: %v", err)
	}
	err = svc.Send(context.Background(), SendRequest{BlockId: "nope", ExpectSession: sessA, Text: "hi"})
	if !errors.Is(err, ErrBadId) {
		t.Fatalf("bad id: %v", err)
	}
	wantNoWrites(t, fb)
}

func TestSendRefusesWhatTheFirstLookDoesNotShowAsIdle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  error
	}{
		{"permission menu", menuLines(), ErrOnPrompt},
		{"busy", busyLines(), ErrBusy},
		{"user is typing", boxLines("half a sentence"), ErrBusy},
		{"no input box", []string{"PS C:\\> "}, ErrNoInputBox},
		{"empty screen", nil, ErrNoInputBox},
	} {
		svc, fb, _ := newSendSvc(func([]string) []string { return tc.lines })
		err := svc.Send(context.Background(), req("hi"))
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, err, tc.want)
		}
		wantNoWrites(t, fb)
	}
}

func TestSendLookFailureWritesNothing(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	fb.screenErr = errors.New("terminal view is not loaded")
	if err := svc.Send(context.Background(), req("hi")); err == nil {
		t.Fatal("was sent")
	}
	wantNoWrites(t, fb)
}

func TestSendMenuAfterPasteGetsNoEnterAndNoKey(t *testing.T) {
	svc, fb, _ := newSendSvc(func(w []string) []string {
		if len(w) == 0 {
			return boxLines("")
		}
		return menuLines()
	})
	err := svc.Send(context.Background(), req("1 ok"))
	if !errors.Is(err, ErrNotSent) {
		t.Fatalf("got %v", err)
	}
	if len(fb.writes) != 1 || fb.writes[0] != pasteOf("1 ok") {
		t.Fatalf("anything besides the paste reached the program: %q", fb.writes)
	}
}

func TestSendMenuBesideTheTextGetsNoEnterAndNoKey(t *testing.T) {
	svc, fb, _ := newSendSvc(func(w []string) []string {
		if len(w) == 0 {
			return boxLines("")
		}
		return append(boxLines(typedAfter(w)), menuLines()...)
	})
	if err := svc.Send(context.Background(), req("1 ok")); !errors.Is(err, ErrNotSent) {
		t.Fatalf("got %v", err)
	}
	if len(fb.writes) != 1 {
		t.Fatalf("a key reached the program while a menu showed: %q", fb.writes)
	}
}

func TestSendTextThatSoundsLikeAMenuIsNotAMenu(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	if err := svc.Send(context.Background(), req("Do you want to proceed? esc to interrupt, esc to cancel")); err != nil {
		t.Fatal(err)
	}
	if len(fb.writes) != 2 || fb.writes[1] != "\r" {
		t.Fatalf("writes %q", fb.writes)
	}
}

func TestSendTextNeverShownGetsNoEnter(t *testing.T) {
	svc, fb, _ := newSendSvc(func([]string) []string { return boxLines("") })
	if err := svc.Send(context.Background(), req("hi")); !errors.Is(err, ErrNotSent) {
		t.Fatalf("got %v", err)
	}
	if len(fb.writes) != 1 {
		t.Fatalf("writes %q", fb.writes)
	}
	if fb.looks < 3 {
		t.Fatalf("it gave up after %d looks; a slow redraw needs more", fb.looks)
	}
}

func TestSendDifferentTextInBoxGetsNoEnterAndNoClear(t *testing.T) {
	svc, fb, _ := newSendSvc(func(w []string) []string {
		if len(w) == 0 {
			return boxLines("")
		}
		return boxLines("something else typed meanwhile")
	})
	if err := svc.Send(context.Background(), req("hi")); !errors.Is(err, ErrNotSent) {
		t.Fatalf("got %v", err)
	}
	for _, w := range fb.writes[1:] {
		t.Fatalf("a key followed the paste although the box held other text: %q", w)
	}
}

func TestSendBusyAfterPasteStillSubmitsNothingElse(t *testing.T) {
	svc, fb, _ := newSendSvc(func(w []string) []string {
		if len(w) == 0 {
			return boxLines("")
		}
		return append(boxLines(typedAfter(w)), "  ✻ Working (1s · esc to interrupt)")
	})
	if err := svc.Send(context.Background(), req("hi")); err == nil {
		t.Fatal("was sent while the session turned busy")
	}
	for _, w := range fb.writes {
		if w == "\r" {
			t.Fatalf("Enter written: %q", fb.writes)
		}
	}
}

func TestSendClearsOnlyTextOfItsOwnThatCouldNotBeSubmitted(t *testing.T) {
	// the text sits in the box and the second look shows a state that stops the Enter: busy
	svc, fb, _ := newSendSvc(func(w []string) []string {
		if len(w) == 0 {
			return boxLines("")
		}
		return append(boxLines(typedAfter(w)), "  ✻ Working (1s · esc to interrupt)")
	})
	_ = svc.Send(context.Background(), req("hi"))
	if len(fb.writes) != 2 || fb.writes[1] != "\x15" {
		t.Fatalf("want paste then Ctrl+U, got %q", fb.writes)
	}
}

func TestSendInputFailureStopsTheSequence(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	fb.inputErr = errors.New("pipe closed")
	if err := svc.Send(context.Background(), req("hi")); err == nil {
		t.Fatal("was sent")
	}
	wantNoWrites(t, fb)
}

func TestSendWrapsLongTextInBox(t *testing.T) {
	long := strings.Repeat("word ", 30)
	svc, fb, _ := newSendSvc(func(w []string) []string {
		text := typedAfter(w)
		if text == "" {
			return boxLines("")
		}
		lines := []string{"╭────────────────────────────────────╮", "│ > " + text[:30] + "│"}
		for rest := text[30:]; rest != ""; {
			n := min(30, len(rest))
			lines = append(lines, "│   "+rest[:n]+"│")
			rest = rest[n:]
		}
		return append(lines, "╰────────────────────────────────────╯", "  ? for shortcuts")
	})
	if err := svc.Send(context.Background(), req(strings.TrimSpace(long))); err != nil {
		t.Fatal(err)
	}
	if len(fb.writes) != 2 || fb.writes[1] != "\r" {
		t.Fatalf("writes %q", fb.writes)
	}
}

func TestSendBorderlessBoxIsRead(t *testing.T) {
	rule := strings.Repeat("─", 40)
	screen := func(text string) []string { return []string{"  output", rule, "> " + text, rule, "  ? for shortcuts"} }
	svc, fb, _ := newSendSvc(func(w []string) []string { return screen(typedAfter(w)) })
	if err := svc.Send(context.Background(), req("hi there")); err != nil {
		t.Fatal(err)
	}
	if len(fb.writes) != 2 || fb.writes[1] != "\r" {
		t.Fatalf("writes %q", fb.writes)
	}
}

func TestSendRefusesText(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"spaces only":   "   ",
		"bang":          "!echo x",
		"bang indented": " !echo x",
		"slash":         "/clear",
		"slash spaced":  "  /clear",
		"hash":          "#note",
		"at":            "@file",
		"newline":       "a\nb",
		"carriage":      "a\rb",
		"tab":           "a\tb",
		"escape":        "a\x1b[201~b",
		"nul":           "a\x00b",
		"delete":        "a\x7fb",
		"c1":            "a\u0085b",
		"line sep":      "a\u2028b",
		"too long":      strings.Repeat("x", MaxSendChars+1),
		"one digit":     "1",
		"one letter":    "y",
		"padded digit":  "  2 ",
		"one emoji":     "\U0001F600",
		"unicode space": "\u00a0!echo x",
	}
	for name, text := range cases {
		svc, fb, _ := newSendSvc(followsPaste)
		err := svc.Send(context.Background(), req(text))
		if !errors.Is(err, ErrRefusedText) {
			t.Fatalf("%s: got %v", name, err)
		}
		wantNoWrites(t, fb)
		if fb.looks != 0 {
			t.Fatalf("%s: the screen was read for a text that is refused", name)
		}
	}
}

func TestSendAcceptsTextAtTheCapAndPlainSymbols(t *testing.T) {
	for _, text := range []string{strings.Repeat("x", MaxSendChars), "what about 100% of it? a!b c/d e#f g@h", "ok 1", "1 ok", "ok"} {
		svc, fb, _ := newSendSvc(followsPaste)
		if err := svc.Send(context.Background(), req(text)); err != nil {
			t.Fatalf("%.20q: %v", text, err)
		}
		if len(fb.writes) != 2 {
			t.Fatalf("writes %d", len(fb.writes))
		}
	}
}

func TestSendCountsCharactersNotBytes(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	if err := svc.Send(context.Background(), req(strings.Repeat("é", MaxSendChars))); err != nil {
		t.Fatalf("a text of exactly the cap in characters was refused: %v", err)
	}
	if len(fb.writes) != 2 {
		t.Fatalf("writes %d", len(fb.writes))
	}
}

// a screen is an input box only when its whole frame and Claude's footer are there; everything else is refused, whatever it looks like
func TestSendRefusesALayoutThatIsNotPositivelyAnInputBox(t *testing.T) {
	full := boxLines("")
	cases := map[string][]string{
		"prompt mark alone at the bottom": {"Pick one:", "  alpha", "❯"},
		"a choice row without a number":   {" Allow this tool?", " ❯ Yes", "   No"},
		"frame without the footer":        full[:4],
		"footer without the frame":        {"> ", "  ? for shortcuts"},
		"a menu's footer under the frame": append(append([]string{}, full[:4]...), "  Enter to select · Esc to exit"),
		"no top border":                   {"  out", "│ >                  │", "╰────────────────────────────────────╯", "  ? for shortcuts"},
		"no bottom border":                {"  out", "╭────────────────────────────────────╮", "│ >                  │", "  ? for shortcuts"},
		"footer far below the frame":      append(append([]string{}, full[:4]...), "", "", "", "", "  ? for shortcuts"),
		"a list menu with a prompt mark":  {"  Select a model", "╭────────────────────────────────────╮", "│ ❯                                  │", "╰────────────────────────────────────╯", "  Enter to select"},
	}
	for name, lines := range cases {
		lines := lines
		svc, fb, _ := newSendSvc(func([]string) []string { return lines })
		if err := svc.Send(context.Background(), req("1 ok")); err == nil {
			t.Fatalf("%s: was sent", name)
		}
		wantNoWrites(t, fb)
	}
}

func TestSendNoEnterWhenTheSecondLookIsNotPositivelyAnInputBox(t *testing.T) {
	svc, fb, _ := newSendSvc(func(w []string) []string {
		if len(w) == 0 {
			return boxLines("")
		}
		return []string{"> " + typedAfter(w)}
	})
	if err := svc.Send(context.Background(), req("hi")); !errors.Is(err, ErrNotSent) {
		t.Fatalf("got %v", err)
	}
	if len(fb.writes) != 1 {
		t.Fatalf("a key followed the paste: %q", fb.writes)
	}
}

func TestSendNoEnterWhenTheBlockChangedBeforeEnter(t *testing.T) {
	for name, change := range map[string]func(*fakeBackend){
		"the program exited": func(fb *fakeBackend) { fb.tree = sendTree("term", "exited", sessA) },
		"another session":    func(fb *fakeBackend) { fb.tree = sendTree("term", "running", sessB) },
		"the block is gone":  func(fb *fakeBackend) { fb.tree = []Workspace{} },
	} {
		var fb *fakeBackend
		var svc *Service
		svc, fb, _ = newSendSvc(func(w []string) []string {
			if len(w) > 0 {
				change(fb)
			}
			return followsPaste(w)
		})
		if err := svc.Send(context.Background(), req("hi")); err == nil {
			t.Fatalf("%s: was sent", name)
		}
		for _, w := range fb.writes {
			if w == "\r" {
				t.Fatalf("%s: Enter written", name)
			}
		}
	}
}

func TestSendLogsLengthNeverTheTextOrAHashOfIt(t *testing.T) {
	svc, _, logs := newSendSvc(followsPaste)
	secret := "hunter2-private-words"
	if err := svc.Send(context.Background(), req(secret)); err != nil {
		t.Fatal(err)
	}
	_ = svc.Send(context.Background(), req("!"+secret))
	if len(*logs) == 0 {
		t.Fatal("nothing logged")
	}
	for _, l := range *logs {
		if strings.Contains(l, secret) || strings.Contains(l, "hunter") {
			t.Fatalf("the text was logged: %q", l)
		}
	}
	if !strings.Contains((*logs)[0], fmt.Sprintf("len=%d", len(secret))) {
		t.Fatalf("length missing: %q", (*logs)[0])
	}
	// a short text is recoverable from a short hash of it
	for _, l := range *logs {
		if strings.Contains(strings.ToLower(l), "hash") {
			t.Fatalf("a hash of the text was logged: %q", l)
		}
	}
}

func TestSendSerializesPerBlock(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := svc.Send(context.Background(), req(fmt.Sprintf("m%d", i))); err != nil {
				t.Errorf("send %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if len(fb.writes) != 16 {
		t.Fatalf("writes %d", len(fb.writes))
	}
	for i := 0; i < 16; i += 2 {
		if !strings.HasPrefix(fb.writes[i], "\x1b[200~") || fb.writes[i+1] != "\r" {
			t.Fatalf("two sends interleaved: %q", fb.writes)
		}
	}
}

func TestScreenIsReadOnlyAndNeedsATerminal(t *testing.T) {
	svc, fb, _ := newSendSvc(func([]string) []string { return boxLines("abc") })
	lines, err := svc.ScreenOf(context.Background(), blk1)
	if err != nil || len(lines) == 0 {
		t.Fatalf("lines %v err %v", lines, err)
	}
	wantNoWrites(t, fb)
	fb.tree = sendTree("web", "", "")
	if _, err := svc.ScreenOf(context.Background(), blk1); err == nil {
		t.Fatal("a web block was read as a screen")
	}
}

func TestUnloadedViewIsAConflictNotAnInternalError(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	fb.screenErr = fmt.Errorf("%w: block %s", ErrNotLoaded, blk1)
	h := MakeHandler(testToken, svc)
	bodies := map[string]string{
		"/v1/screen": `{"blockid":"` + blk1 + `"}`,
		"/v1/send":   `{"blockid":"` + blk1 + `","expectsession":"` + sessA + `","text":"hi"}`,
	}
	for path, body := range bodies {
		rec := doCall(t, h, "POST", path, testToken, body)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not loaded") {
			t.Fatalf("%s: status %d: %s", path, rec.Code, rec.Body.String())
		}
	}
	wantNoWrites(t, fb)
}

func TestSendHandlerMapsErrorsAndNeverReturnsScreen(t *testing.T) {
	for _, tc := range []struct {
		name   string
		screen func([]string) []string
		body   string
		status int
	}{
		{"ok", followsPaste, `{"blockid":"` + blk1 + `","expectsession":"` + sessA + `","text":"hi"}`, http.StatusOK},
		{"refused", followsPaste, `{"blockid":"` + blk1 + `","expectsession":"` + sessA + `","text":"/clear"}`, http.StatusBadRequest},
		{"busy", func([]string) []string { return busyLines() }, `{"blockid":"` + blk1 + `","expectsession":"` + sessA + `","text":"hi"}`, http.StatusConflict},
		{"mismatch", followsPaste, `{"blockid":"` + blk1 + `","expectsession":"` + sessB + `","text":"hi"}`, http.StatusConflict},
		{"unknown block", followsPaste, `{"blockid":"` + blk2 + `","expectsession":"` + sessA + `","text":"hi"}`, http.StatusNotFound},
		{"unknown key", followsPaste, `{"blockid":"` + blk1 + `","expectsession":"` + sessA + `","text":"hi","keys":"x"}`, http.StatusBadRequest},
		{"duplicate key", followsPaste, `{"blockid":"` + blk1 + `","text":"a","text":"b","expectsession":"` + sessA + `"}`, http.StatusBadRequest},
	} {
		svc, _, _ := newSendSvc(tc.screen)
		h := MakeHandler(testToken, svc)
		rec := doCall(t, h, "POST", "/v1/send", testToken, tc.body)
		if rec.Code != tc.status {
			t.Fatalf("%s: status %d: %s", tc.name, rec.Code, rec.Body.String())
		}
		for _, leak := range []string{"earlier output", "for shortcuts", "Working", "Do you want"} {
			if strings.Contains(rec.Body.String(), leak) {
				t.Fatalf("%s: the answer holds screen text: %s", tc.name, rec.Body.String())
			}
		}
	}
}

func TestSendAndScreenNeedTheToken(t *testing.T) {
	svc, fb, _ := newSendSvc(followsPaste)
	h := MakeHandler(testToken, svc)
	for _, path := range []string{"/v1/send", "/v1/screen"} {
		rec := doCall(t, h, "POST", path, "", `{"blockid":"`+blk1+`","expectsession":"`+sessA+`","text":"hi"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	wantNoWrites(t, fb)
	if rec := doCall(t, h, "GET", "/v1/send", testToken, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
}

func TestScreenHandlerReturnsLines(t *testing.T) {
	svc, _, _ := newSendSvc(func([]string) []string { return boxLines("abc") })
	h := MakeHandler(testToken, svc)
	rec := doCall(t, h, "POST", "/v1/screen", testToken, `{"blockid":"`+blk1+`"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "lines") {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
}
