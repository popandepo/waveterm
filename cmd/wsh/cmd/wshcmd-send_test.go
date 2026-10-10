// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
	"testing"
)

func TestEncodeSendTokens(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []string
		want    []string
		wantErr bool
	}{
		{name: "down then enter are separate chunks", tokens: []string{"key:Down", "key:Enter"}, want: []string{"\x1b[B", "\r"}},
		{name: "key names are case-insensitive", tokens: []string{"key:ESCAPE", "key:tab", "key:BackTab"}, want: []string{"\x1b", "\t", "\x1b[Z"}},
		{name: "ctrl c is the interrupt byte", tokens: []string{"key:Ctrl+C"}, want: []string{"\x03"}},
		{name: "ctrl letters", tokens: []string{"key:ctrl+a", "key:Ctrl+Z"}, want: []string{"\x01", "\x1a"}},
		{name: "text prefix and bare text", tokens: []string{"text:hello", "world; quit"}, want: []string{"hello", "world; quit"}},
		{name: "text may contain the key prefix after text", tokens: []string{"text:key:Down"}, want: []string{"key:Down"}},
		{name: "arrows", tokens: []string{"key:Up", "key:Left", "key:Right"}, want: []string{"\x1b[A", "\x1b[D", "\x1b[C"}},
		{name: "unknown key", tokens: []string{"key:Hyper+Q"}, wantErr: true},
		{name: "ctrl needs a letter", tokens: []string{"key:Ctrl+1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeSendTokens(tt.tokens)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
