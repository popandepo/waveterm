// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	maxPrintedTitleRunes = 200
	maxPrintedUrlRunes   = 2048
)

// safeText makes text from a web page (its title and address) fit to be printed to a terminal and read by a script: every control character (the escape
// that starts a terminal sequence, the C1 controls, NUL, line breaks), format character and separator is shown as <U+XXXX> instead of being sent to the
// terminal, and text longer than max characters is cut and ends in "...". A program that wrote the title controls it completely, so none of it is
// printed as it is.
func safeText(s string, max int) string {
	var b strings.Builder
	count := 0
	for _, r := range s {
		if count >= max {
			b.WriteString("...")
			break
		}
		count++
		if isUnprintableRune(r) {
			b.WriteString(fmt.Sprintf("<U+%04X>", r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isUnprintableRune is what a page never gets to send to the terminal as it is: controls, format characters, line and paragraph separators, the
// replacement character (a broken sequence), and the default-ignorable code points (variation selectors, Hangul fillers, Mongolian free variation
// selectors and the like), which draw nothing and can hide text as format characters do.
func isUnprintableRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) || r == unicode.ReplacementChar ||
		unicode.Is(unicode.Variation_Selector, r) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r)
}
