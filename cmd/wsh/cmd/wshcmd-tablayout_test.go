// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func TestTabLayoutPrintsTheTilesAsJsonAndAsksOnlyForThatTab(t *testing.T) {
	old := tabLayoutRPC
	t.Cleanup(func() { tabLayoutRPC = old; tabLayoutTab = "" })
	var asked []string
	tabLayoutRPC = func(data wshrpc.CommandGetTabLayoutData) (wshrpc.TabLayoutData, error) {
		asked = append(asked, data.TabId)
		return wshrpc.TabLayoutData{TabId: data.TabId, Tiles: []wshrpc.TabLayoutTile{{BlockIds: []string{"a"}}, {BlockIds: []string{"b", "c"}, Active: "c"}}, PendingActions: 1}, nil
	}
	tabLayoutTab = "tab-1"

	text, err := tabLayoutText()
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "tab-1" {
		t.Fatalf("asked %v", asked)
	}
	var got wshrpc.TabLayoutData
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("the output must be JSON: %v\n%s", err, text)
	}
	if len(got.Tiles) != 2 || got.Tiles[1].Active != "c" || got.PendingActions != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestTabLayoutDefaultsToTheTabOfTheCaller(t *testing.T) {
	t.Setenv("WAVETERM_BLOCKID", "")
	t.Setenv("WAVETERM_TABID", "tab-from-env")
	old := tabLayoutRPC
	t.Cleanup(func() { tabLayoutRPC = old; tabLayoutTab = "" })
	var asked string
	tabLayoutRPC = func(data wshrpc.CommandGetTabLayoutData) (wshrpc.TabLayoutData, error) {
		asked = data.TabId
		return wshrpc.TabLayoutData{TabId: data.TabId}, nil
	}
	tabLayoutTab = ""
	if _, err := tabLayoutText(); err != nil {
		t.Fatal(err)
	}
	if asked != "tab-from-env" {
		t.Fatalf("asked %q", asked)
	}
}

func TestTabLayoutWithoutATabSaysHowToGiveOne(t *testing.T) {
	t.Setenv("WAVETERM_BLOCKID", "")
	t.Setenv("WAVETERM_TABID", "")
	tabLayoutTab = ""
	if _, err := tabLayoutText(); err == nil || !strings.Contains(err.Error(), "--tab") {
		t.Fatalf("want a message naming --tab, got %v", err)
	}
}

func TestTabLayoutIsReadOnlyAndHasNoFocusFlag(t *testing.T) {
	if tabLayoutCmd.Flags().Lookup("focus") != nil || tabLayoutCmd.Flags().Lookup("activate") != nil {
		t.Fatalf("a read-back must not offer to change focus")
	}
}
