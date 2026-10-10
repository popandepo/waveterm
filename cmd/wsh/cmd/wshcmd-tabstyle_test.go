// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
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
