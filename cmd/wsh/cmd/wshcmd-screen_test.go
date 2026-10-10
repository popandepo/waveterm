// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
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
