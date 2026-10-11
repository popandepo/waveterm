// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package attention holds the rules for the "this tile needs the owner" marks that bubble up from a tile to its stack tab, its tile and its
// top tab: the three levels, the check of the reason text, and the highest level among several. The same rules are written in
// frontend/app/store/attention.ts; both are tested against pkg/attention/testdata/vectors.json.
package attention

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	LevelNone   = ""
	LevelInfo   = "info"
	LevelNeed   = "need"
	LevelUrgent = "urgent"

	MaxReasonRunes = 120

	MetaKeyLevel  = "attention:level"
	MetaKeyReason = "attention:reason"
)

// Rank orders the levels; an unknown level ranks as none.
func Rank(level string) int {
	switch level {
	case LevelInfo:
		return 1
	case LevelNeed:
		return 2
	case LevelUrgent:
		return 3
	}
	return 0
}

// Highest is the highest level among the given ones (a container shows the highest level of what it holds); unknown levels count as none.
func Highest(levels ...string) string {
	best := LevelNone
	for _, l := range levels {
		if Rank(l) > Rank(best) {
			best = l
		}
	}
	return best
}

// NeedsReason: the two orange levels must say why.
func NeedsReason(level string) bool {
	return level == LevelNeed || level == LevelUrgent
}

func hasHiddenRune(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
			return true
		}
	}
	return false
}

// Check validates a level and a reason as they would be written on a block. It returns the cleaned reason (trimmed). A reason is required
// for need and urgent, optional for info; it has no control, format or line characters and is at most MaxReasonRunes long.
func Check(level string, reason string) (string, error) {
	if Rank(level) == 0 {
		return "", fmt.Errorf("unknown attention level %q (info, need or urgent)", level)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		if NeedsReason(level) {
			return "", fmt.Errorf("attention level %q needs a reason (--reason \"...\"), it is shown to the owner", level)
		}
		return "", nil
	}
	if hasHiddenRune(reason) {
		return "", fmt.Errorf("the reason has control or hidden characters")
	}
	if utf8.RuneCountInString(reason) > MaxReasonRunes {
		return "", fmt.Errorf("the reason is longer than %d characters", MaxReasonRunes)
	}
	return reason, nil
}

// Of reads the attention of a block from its meta values, with the same rules as Check; anything that does not pass counts as no attention.
func Of(level any, reason any) (string, string) {
	l, _ := level.(string)
	r, _ := reason.(string)
	cleaned, err := Check(l, r)
	if err != nil {
		return LevelNone, ""
	}
	return l, cleaned
}
