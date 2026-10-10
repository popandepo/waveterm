// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildTabStyleMeta(t *testing.T) {
	tests := []struct {
		name    string
		color   string
		symbol  string
		clear   bool
		want    map[string]any
		wantErr bool
	}{
		{name: "colour name and icon", color: "gold", symbol: "crown", want: map[string]any{"tab:flagcolor": "gold", "tab:symbol": "crown"}},
		{name: "hex colour only", color: "#ff8800", want: map[string]any{"tab:flagcolor": "#ff8800"}},
		{name: "hex colour with alpha", color: "#ff880080", want: map[string]any{"tab:flagcolor": "#ff880080"}},
		{name: "emoji symbol", symbol: "\U0001F451", want: map[string]any{"tab:symbol": "\U0001F451"}},
		{name: "prefixed icon", symbol: "solid@star", want: map[string]any{"tab:symbol": "solid@star"}},
		{name: "clear removes both", clear: true, want: map[string]any{"tab:flagcolor": nil, "tab:symbol": nil}},
		{name: "nothing given", wantErr: true},
		{name: "clear with colour", color: "red", clear: true, wantErr: true},
		{name: "bad colour name", color: "notacolour", wantErr: true},
		{name: "bad hex", color: "#12345", wantErr: true},
		{name: "markup in symbol", symbol: "<img src=x>", wantErr: true},
		{name: "long text symbol", symbol: "abcdefghijk!", wantErr: true},
		{name: "three letters is more than two graphemes", symbol: "ABC", wantErr: true},
		{name: "two letters", symbol: "AB", want: map[string]any{"tab:symbol": "AB"}},
		{name: "accented letter is one grapheme", symbol: "e\u0301", want: map[string]any{"tab:symbol": "e\u0301"}},
		{name: "flag is one grapheme", symbol: "\U0001F1E7\U0001F1FB", want: map[string]any{"tab:symbol": "\U0001F1E7\U0001F1FB"}},
		{name: "lone joiner", symbol: "\u200d", wantErr: true},
		{name: "joiner at the end", symbol: "a\u200d", wantErr: true},
		{name: "double joiner", symbol: "\U0001F468\u200d\u200d\U0001F469", wantErr: true},
		{name: "bidi override", symbol: "a\u202eb", wantErr: true},
		{name: "bidi isolate", symbol: "\u2066x", wantErr: true},
		{name: "control char", symbol: "a\x07", wantErr: true},
		{name: "line separator", symbol: "a\u2028", wantErr: true},
		{name: "combining marks pile", symbol: "e" + strings.Repeat("\u0301", 40), wantErr: true},
		{name: "long icon name", symbol: strings.Repeat("a", 41), wantErr: true},
		{name: "zwj emoji family", symbol: "\U0001F468\u200d\U0001F469\u200d\U0001F467", want: map[string]any{"tab:symbol": "\U0001F468\u200d\U0001F469\u200d\U0001F467"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildTabStyleMeta(tt.color, tt.symbol, tt.clear)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
