// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func stubWebText(t *testing.T) (*[]wshrpc.CommandWebTextData, *bool) {
	t.Helper()
	oldAuth, oldInfo, oldRPC, oldResolve, oldSel, oldMax, oldBlockArg := webNavAuthorize, webNavBlockInfo, webTextRPC, resolveBlockRef, webTextSelector, webTextMax, blockArg
	t.Cleanup(func() {
		webNavAuthorize, webNavBlockInfo, webTextRPC, resolveBlockRef, webTextSelector, webTextMax, blockArg = oldAuth, oldInfo, oldRPC, oldResolve, oldSel, oldMax, oldBlockArg
	})
	var sent []wshrpc.CommandWebTextData
	read := false
	webNavAuthorize = func(blockId string) error { return nil }
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { read = true; return webInfo("web"), nil }
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	webTextRPC = func(d wshrpc.CommandWebTextData) (*wshrpc.CommandWebTextRtnData, error) {
		sent = append(sent, d)
		return &wshrpc.CommandWebTextRtnData{Text: "hello"}, nil
	}
	webTextSelector, webTextMax, blockArg = "", maxWebTextBytes, ""
	return &sent, &read
}

func TestWebTextSendsTheRequestForTheBlocksOwnTab(t *testing.T) {
	sent, _ := stubWebText(t)
	webTextSelector, webTextMax = "main h1", 1000
	if err := webTextCmd.RunE(webTextCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 1 {
		t.Fatalf("want one request, got %+v", *sent)
	}
	d := (*sent)[0]
	if d.TabId != "tab-other" || d.BlockId != webBlockId || d.Selector != "main h1" || d.MaxBytes != 1000 {
		t.Fatalf("bad request: %+v", d)
	}
}

func TestWebTextRefusesAForeignTileBeforeAnythingIsReadAndSendsNothing(t *testing.T) {
	sent, read := stubWebText(t)
	webNavAuthorize = func(blockId string) error { return errString("no") }
	err := webTextCmd.RunE(webTextCmd, nil)
	if err == nil || err.Error() != webNavDenied {
		t.Fatalf("want the one denial text, got %v", err)
	}
	if *read || len(*sent) != 0 {
		t.Fatalf("nothing may be read or sent: read=%v sent=%v", *read, *sent)
	}
}

func TestWebTextRefusesATileThatIsNotAWebTile(t *testing.T) {
	sent, _ := stubWebText(t)
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { return webInfo("term"), nil }
	if err := webTextCmd.RunE(webTextCmd, nil); err == nil || !strings.Contains(err.Error(), "not a web block") {
		t.Fatalf("got %v", err)
	}
	if len(*sent) != 0 {
		t.Fatal("nothing may be sent")
	}
}

func TestWebTextTheWindowsRefusalComesBackAsItIs(t *testing.T) {
	stubWebText(t)
	webTextRPC = func(d wshrpc.CommandWebTextData) (*wshrpc.CommandWebTextRtnData, error) {
		return nil, errString("this web tile uses the shared default sign-in store of Wave, so its text is not given out")
	}
	if err := webTextCmd.RunE(webTextCmd, nil); err == nil || !strings.Contains(err.Error(), "shared default sign-in store") {
		t.Fatalf("got %v", err)
	}
	webTextRPC = func(d wshrpc.CommandWebTextData) (*wshrpc.CommandWebTextRtnData, error) {
		return nil, errString("rpc: " + webNavDenied)
	}
	if err := webTextCmd.RunE(webTextCmd, nil); err == nil || err.Error() != webNavDenied {
		t.Fatalf("a denial from the window is the same one text, got %v", err)
	}
}

func TestWebTextOptionsAreCheckedBeforeAnyRequest(t *testing.T) {
	sent, read := stubWebText(t)
	for _, c := range []struct {
		sel string
		max int
	}{{"", 0}, {"", -1}, {"", maxWebTextBytes + 1}, {strings.Repeat("a", 513), 10}, {"a" + string(rune(10)) + "b", 10}, {"a" + string(rune(27)) + "b", 10}} {
		webTextSelector, webTextMax = c.sel, c.max
		if err := webTextCmd.RunE(webTextCmd, nil); err == nil {
			t.Errorf("selector %q max %d must be refused", c.sel, c.max)
		}
	}
	if *read || len(*sent) != 0 {
		t.Fatal("a refused option must not read or send anything")
	}
}

func TestSafeTextBlockKeepsLinesAndTabsAndShowsTheRest(t *testing.T) {
	keep := "a" + string(rune(10)) + "b" + string(rune(9)) + "c"
	if got := safeTextBlock(keep); got != keep {
		t.Fatalf("got %q", got)
	}
	bad := []rune{27, 13, 0, 0x202e, 0x200b}
	got := safeTextBlock("x" + string(bad) + "y")
	for _, r := range bad {
		if strings.ContainsRune(got, r) {
			t.Errorf("%U must not reach the terminal: %q", r, got)
		}
	}
	for _, want := range []string{"<U+001B>", "<U+000D>", "<U+0000>", "<U+202E>", "<U+200B>"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in %q", want, got)
		}
	}
}
