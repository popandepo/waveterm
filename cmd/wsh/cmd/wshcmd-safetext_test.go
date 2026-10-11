// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"
	"unicode"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func controlRunes() map[string]rune {
	return map[string]rune{
		"escape":                    0x1b,
		"NUL":                       0x00,
		"bell":                      0x07,
		"newline":                   0x0a,
		"carriage return":           0x0d,
		"C1 control sequence intro": 0x9b,
		"DEL":                       0x7f,
		"bidi override":             0x202e,
		"zero width space":          0x200b,
		"line separator":            0x2028,
		"paragraph separator":       0x2029,
		"byte order mark":           0xfeff,
		"replacement":               0xfffd,
	}
}

func TestSafeTextShowsEveryControlAsAVisibleCode(t *testing.T) {
	for name, r := range controlRunes() {
		in := "a" + string(r) + "b"
		out := safeText(in, 100)
		if strings.ContainsRune(out, r) {
			t.Errorf("%s: the character itself must not be printed: %q", name, out)
		}
		if !strings.HasPrefix(out, "a<U+") || !strings.HasSuffix(out, ">b") {
			t.Errorf("%s: want a visible code between a and b, got %q", name, out)
		}
	}
	for _, r := range safeText("x\x1b[2J\x1b]0;evil\x07y", 100) {
		if unicode.IsControl(r) {
			t.Fatalf("a terminal sequence survived: %q", safeText("x\x1b[2J\x1b]0;evil\x07y", 100))
		}
	}
}

func TestSafeTextKeepsOrdinaryTextAndCutsLongText(t *testing.T) {
	if got := safeText("Wave - résumé 中文 \U0001F680", 100); got != "Wave - résumé 中文 \U0001F680" {
		t.Fatalf("ordinary text, accents, CJK and emoji pass unchanged, got %q", got)
	}
	long := strings.Repeat("x", 500)
	got := safeText(long, 200)
	if got != strings.Repeat("x", 200)+"..." {
		t.Fatalf("long text is cut at the limit and says so, got %d characters", len([]rune(got)))
	}
	if safeText(strings.Repeat("y", 200), 200) != strings.Repeat("y", 200) {
		t.Fatalf("text exactly at the limit is not cut")
	}
}

func TestWhatTheCommandPrintsIsSafeAndCappedInTextAndInJson(t *testing.T) {
	hostile := &wshrpc.CommandWebNavRtnData{Url: "http://x/" + strings.Repeat("u", 5000), Title: "t\x1b[31mred\x1b]0;title\x07" + strings.Repeat("z", 1000), CanGoBack: true}
	for _, asJson := range []bool{false, true} {
		for _, action := range []string{"url", "reload"} {
			out, err := formatWebNav(action, hostile, asJson)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range out {
				if unicode.IsControl(r) && r != '\n' {
					t.Errorf("json=%v action=%s: a control character is printed: %q", asJson, action, r)
				}
			}
			if strings.Contains(out, strings.Repeat("u", maxPrintedUrlRunes+10)) || strings.Contains(out, strings.Repeat("z", maxPrintedTitleRunes+10)) {
				t.Errorf("json=%v action=%s: the output is not capped", asJson, action)
			}
		}
	}
	text, _ := formatWebNav("url", hostile, false)
	if !strings.Contains(text, "<U+001B>") {
		t.Fatalf("the escape must be shown as a code, got %q", text)
	}
	if hostile.Title == "" || !strings.Contains(hostile.Title, "\x1b") {
		t.Fatalf("the caller's data must not be changed")
	}
}
