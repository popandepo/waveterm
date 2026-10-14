// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// An error from the window can carry text the page wrote (a throwing innerText getter); it reaches the terminal only made printable.
func TestWebTextAnErrorFromTheWindowIsPrintedSafely(t *testing.T) {
	stubWebText(t)
	webTextRPC = func(d wshrpc.CommandWebTextData) (*wshrpc.CommandWebTextRtnData, error) {
		return nil, errString("the page refused the selector: \x1b]0;owned\x07\x1b[2J\u202egnp.exe\r\nfake prompt$ ")
	}
	err := webTextCmd.RunE(webTextCmd, nil)
	if err == nil {
		t.Fatal("the error must come back")
	}
	msg := err.Error()
	for _, r := range msg {
		if r == 0x1b || r == 0x07 || r == '\r' || r == '\n' || r == 0x202e {
			t.Fatalf("a control or format character reached the error text: %q", msg)
		}
	}
	if !strings.Contains(msg, "the page refused the selector") || !strings.Contains(msg, "<U+001B>") {
		t.Fatalf("the text stays readable, the escapes are shown: %q", msg)
	}
}

// The cap is on what is printed: escaping can make the text up to eight times longer, so the cut comes after it.
func TestWebTextOutputIsCappedAfterEscaping(t *testing.T) {
	text := strings.Repeat("\x01", 1000)
	out, cut := webTextOutput(text, 1000)
	if len(out) > 1000 {
		t.Fatalf("the printed text is %d bytes, more than the cap of 1000", len(out))
	}
	if !cut {
		t.Fatal("the cut must be reported")
	}
	if strings.Count(out, "<U+0001>") != 1000/len("<U+0001>") || strings.HasSuffix(out, "<U+") {
		t.Fatalf("whole escapes only, never half of one: %q", out[len(out)-12:])
	}
	if !utf8.ValidString(out) {
		t.Fatal("never a split character")
	}

	multi := strings.Repeat("\u00e9", 600) // 1200 bytes
	out, cut = webTextOutput(multi, 1001)
	if len(out) != 1000 || !cut || !utf8.ValidString(out) {
		t.Fatalf("cut at a character boundary: %d bytes, cut=%v", len(out), cut)
	}

	out, cut = webTextOutput("line one\n\tline two", 1000)
	if out != "line one\n\tline two" || cut {
		t.Fatalf("short text is printed as it is: %q %v", out, cut)
	}
}

// Runs the command with the real printer and returns what reached stdout and stderr.
func runWebTextCaptured(t *testing.T, max int, pageText string) (string, string) {
	t.Helper()
	stubWebText(t)
	webTextMax = max
	webTextRPC = func(d wshrpc.CommandWebTextData) (*wshrpc.CommandWebTextRtnData, error) {
		return &wshrpc.CommandWebTextRtnData{Text: pageText}, nil
	}
	oldOut, oldErr := WrappedStdout, WrappedStderr
	t.Cleanup(func() { WrappedStdout, WrappedStderr = oldOut, oldErr })
	var out, errOut bytes.Buffer
	WrappedStdout, WrappedStderr = &out, &errOut
	if err := webTextCmd.RunE(webTextCmd, nil); err != nil {
		t.Fatal(err)
	}
	return out.String(), errOut.String()
}

func TestWebTextRunPrintsAtMostMaxBytesNewlineIncluded(t *testing.T) {
	out, errOut := runWebTextCaptured(t, 64, strings.Repeat("\x1b", 64))
	if len(out) > 64 || !strings.HasPrefix(out, "<U+001B>") || !strings.HasSuffix(out, "\n") {
		t.Fatalf("printed %d bytes: %q", len(out), out)
	}
	if !strings.Contains(errOut, "cut") {
		t.Fatalf("the cut is said on stderr: %q", errOut)
	}
	out, errOut = runWebTextCaptured(t, 1, "a")
	if out != "\n" || !strings.Contains(errOut, "cut") {
		t.Fatalf("--max-bytes 1 with \"a\": at most 1 byte on stdout, the newline counted: %q %q", out, errOut)
	}
	out, errOut = runWebTextCaptured(t, 2, "a")
	if out != "a\n" || errOut != "" {
		t.Fatalf("text and newline that fit are printed whole: %q %q", out, errOut)
	}
}
