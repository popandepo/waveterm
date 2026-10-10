// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestTrimScreenLines(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		last  int
		want  []string
	}{
		{name: "trailing blanks removed", lines: []string{"a", "b", "", "  ", ""}, want: []string{"a", "b"}},
		{name: "inner blanks kept", lines: []string{"a", "", "b", ""}, want: []string{"a", "", "b"}},
		{name: "all blank", lines: []string{"", " ", ""}, want: []string{}},
		{name: "empty", lines: nil, want: []string{}},
		{name: "last n", lines: []string{"a", "b", "c", "d", "", ""}, last: 2, want: []string{"c", "d"}},
		{name: "last larger than content", lines: []string{"a", "b", ""}, last: 10, want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimScreenLines(tt.lines, tt.last)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeScreenLine(t *testing.T) {
	tests := map[string]string{
		"plain text":                  "plain text",
		"tab\tkept":                   "tab\tkept",
		"\x1b[31mred\x1b[0m":          "\\x1b[31mred\\x1b[0m",
		"bell\x07 back\x08":           "bell\\x07 back\\x08",
		"cr\rline":                    "cr\\x0dline",
		"bidi \u202e reversed":        "bidi \\u202e reversed",
		"c1 \u009b csi":               "c1 \\x9b csi",
		"emoji \U0001F451 and \u00e9": "emoji \U0001F451 and \u00e9",
	}
	for in, want := range tests {
		if got := sanitizeScreenLine(in); got != want {
			t.Errorf("sanitizeScreenLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadBlockScreenReadsTheRequestedBlock(t *testing.T) {
	screens := map[string][]string{
		"block-a": {"PS C:\\a>", ""},
		"block-b": {"Do you trust the files in this folder?", "> No, exit", ""},
	}
	var asked []string
	fetch := func(blockId string) ([]string, error) {
		asked = append(asked, blockId)
		lines, ok := screens[blockId]
		if !ok {
			return nil, fmt.Errorf("no route for feblock:%s", blockId)
		}
		return lines, nil
	}
	got, err := readBlockScreen("block-b", fetch, 0, DefaultScreenMaxBytes)
	if err != nil || !strings.Contains(got, "trust the files") || strings.Contains(got, "PS C:") {
		t.Fatalf("got %q, %v", got, err)
	}
	if !reflect.DeepEqual(asked, []string{"block-b"}) {
		t.Fatalf("asked for %q, want only block-b", asked)
	}
	if _, err := readBlockScreen("block-missing", fetch, 0, DefaultScreenMaxBytes); err == nil {
		t.Fatal("an unreadable block must be an error")
	}
}

func TestRenderScreenLimitsOutput(t *testing.T) {
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, fmt.Sprintf("line %04d", i))
	}
	got := renderScreen(lines, 0, 200)
	if len(got) > 400 || !strings.HasPrefix(got, "[wsh screen: output limited to 200 bytes") || !strings.HasSuffix(got, "line 0999") {
		t.Fatalf("unexpected output: %q", got)
	}
	if strings.Contains(renderScreen(lines[:3], 0, 200), "limited") {
		t.Fatal("short output must not report a cut")
	}
	if got := renderScreen([]string{"a very long single line that exceeds the limit"}, 0, 5); !strings.Contains(got, "a very long") {
		t.Fatalf("the newest line must survive: %q", got)
	}
}
