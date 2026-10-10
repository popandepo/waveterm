// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"reflect"
	"testing"
	"time"
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
			got, err := encodeSendTokens(tt.tokens, false)
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

func TestEncodeSendTokensRefusesControlText(t *testing.T) {
	bad := []string{"a\x1b[Ab", "line1\nline2", "x\ry", "bs\x08", "del\x7f", "bidi\u202eRTL", "text:esc\x1b"}
	for _, text := range bad {
		if _, err := encodeSendTokens([]string{text}, false); err == nil {
			t.Fatalf("expected %q to be refused", text)
		}
	}
	got, err := encodeSendTokens([]string{"a\x1b[Ab"}, true)
	if err != nil || len(got) != 1 || got[0] != "a\x1b[Ab" {
		t.Fatalf("--raw should pass the text through, got %q, %v", got, err)
	}
	if _, err := encodeSendTokens([]string{"plain text, with ; & | \" and unicode \u00e9\U0001F451"}, false); err != nil {
		t.Fatalf("plain text must be accepted: %v", err)
	}
}

func TestDeliverChunksTargetsTheRequestedBlock(t *testing.T) {
	type call struct{ block, data string }
	var calls []call
	var slept []time.Duration
	send := func(blockId string, data []byte) error {
		calls = append(calls, call{blockId, string(data)})
		return nil
	}
	sleep := func(d time.Duration) { slept = append(slept, d) }
	n, bytes, err := deliverChunks("block-unfocused-123", []string{"\x1b[B", "", "\r", "hi"}, 40*time.Millisecond, send, sleep)
	if err != nil || n != 3 || bytes != 6 {
		t.Fatalf("got %d chunks %d bytes err %v", n, bytes, err)
	}
	want := []call{{"block-unfocused-123", "\x1b[B"}, {"block-unfocused-123", "\r"}, {"block-unfocused-123", "hi"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
	if len(slept) != 2 {
		t.Fatalf("expected a pause between chunks only, got %v", slept)
	}
}

func TestDeliverChunksEmptyAndErrors(t *testing.T) {
	never := func(blockId string, data []byte) error { t.Fatal("must not send"); return nil }
	if _, _, err := deliverChunks("b", []string{"", ""}, 0, never, func(time.Duration) {}); err == nil {
		t.Fatal("an all-empty send must be an error")
	}
	count := 0
	failing := func(blockId string, data []byte) error {
		count++
		if count == 2 {
			return fmt.Errorf("no controller found for block %s", blockId)
		}
		return nil
	}
	n, _, err := deliverChunks("b", []string{"a", "b", "c"}, 0, failing, func(time.Duration) {})
	if err == nil || n != 1 || count != 2 {
		t.Fatalf("expected to stop at the failing chunk: n=%d count=%d err=%v", n, count, err)
	}
}
