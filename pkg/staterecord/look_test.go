// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package staterecord

import (
	"strings"
	"testing"
)

func cp(codes ...rune) string { return string(codes) }

func TestColorValueAcceptsWhatTheWindowDraws(t *testing.T) {
	for _, good := range []string{"#fff", "#FFF8", "#2D7DD2", "#2d7dd2cc", "red", "RebeccaPurple", "transparent", "currentcolor",
		"rgb(1,2,3)", "rgb(1 2 3 / 50%)", "hsl(120, 50%, 50%)", "oklch(0.7 0.1 200)", "  #fff  "} {
		if got := ColorValue(good); got != strings.TrimSpace(good) {
			t.Errorf("%q must pass as %q, got %q", good, strings.TrimSpace(good), got)
		}
	}
}

func TestColorValueRefusesEverythingElse(t *testing.T) {
	bad := []string{
		"red;background:url(http://x/a.png)", "url(http://x/a.png)", "url(javascript:alert(1))", "expression(alert(1))", "rgb(0,0,0);",
		"rgb(0 0 0) url(x)", "rgb(var(--x))", "rgb(calc(1) 2 3)", "rgb(0,0,0)}body{display:none", "#fff /* x */", "#fff;", "#ffff00ff00",
		"var(--accent-color)", "env(x)", "attr(title)", "image-set(url(x) 1x)", "color-mix(in srgb, red, blue)", "color(display-p3 1 0 0)",
		"linear-gradient(red, blue)", "javascript:alert(1)", "\\72 ed", "re" + cp(0x200b) + "d", cp(0x202e) + "red", "re" + cp(0xa) + "d",
		"rgb(1," + cp(0x2028) + "2,3)", cp(0xff52, 0xff45, 0xff44), strings.Repeat("r", 31), "rgb(" + strings.Repeat("1", 70) + ")", "<b>red</b>",
		`"red"`, "red blue", "-moz-initial", "--x", "", "   ",
		"notacolour", "inherit", "initial", "unset", "revert", "banana",
	}
	for _, b := range bad {
		if got := ColorValue(b); got != "" {
			t.Errorf("%q must not pass, got %q", b, got)
		}
	}
}

func TestSymbolValueAcceptsIconNamesAndShortText(t *testing.T) {
	for _, good := range []string{"rocket", "solid@rocket", "regular@bell", "brands@github", "custom@thing", "rocket+spin", "rocket+beat+fw", "a",
		cp(0x1F680), "AB", cp(0x1F468, 0x200d, 0x1F469, 0x200d, 0x1F467), cp(0x1F44D, 0x1F3FD), "  rocket  "} {
		if got := SymbolValue(good); got != strings.TrimSpace(good) {
			t.Errorf("%q must pass as %q, got %q", good, strings.TrimSpace(good), got)
		}
	}
}

func TestSymbolValueRefusesMarkupHiddenCharactersAndLongValues(t *testing.T) {
	bad := []string{
		"", "   ", "<img src=x onerror=alert(1)>", "rocket;background:red", "a b", "rocket" + cp(7), "ro" + cp(0x202e) + "cket", "a" + cp(0x200b) + "b",
		"a" + cp(0x2028) + "b", "a" + cp(0x2066) + "b", cp(0x200d) + "a", "a" + cp(0x200d), "a" + cp(0x200d, 0x200d) + "b", "a " + cp(0x200d) + "b",
		strings.Repeat("a", 41), "ABC", cp(0x1F680, 0x1F680, 0x1F680), "solid@" + strings.Repeat("a", 40), "Rocket", "rocket+blink", "../x", "a\nb",
		strings.Repeat("x", 100000),
	}
	for _, b := range bad {
		if got := SymbolValue(b); got != "" {
			t.Errorf("%q must not pass, got %q", b, got)
		}
	}
}
