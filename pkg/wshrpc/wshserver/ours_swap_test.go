// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func swapRequest(a string, b string) wshrpc.CommandStackBlockData {
	return wshrpc.CommandStackBlockData{BlockId: a, TargetBlockId: b, Action: wshrpc.StackBlockAction_Swap}
}

func TestPlanSwapQueuesASwapOfTwoBlocksOfOneTab(t *testing.T) {
	tabId, action, err := planStackBlock(swapRequest("b1", "b2"), fakeTabLookup(twoBlocksOneTab), inStackFn(false), sameStackFn(false))
	if err != nil {
		t.Fatal(err)
	}
	if tabId != "t1" || action.ActionType != "swap" || action.BlockId != "b1" || action.TargetBlockId != "b2" {
		t.Fatalf("tab %s action %+v", tabId, action)
	}
	if action.Focused {
		t.Fatalf("a swap never takes keyboard focus")
	}
}

func TestPlanSwapRefusesWhatCannotBeSwapped(t *testing.T) {
	cases := []struct {
		name    string
		data    wshrpc.CommandStackBlockData
		same    bool
		message string
	}{
		{"itself", swapRequest("b1", "b1"), false, "itself"},
		{"no second block", swapRequest("b1", ""), false, "second block"},
		{"other tab", swapRequest("b1", "b3"), false, "same tab"},
		{"same group", swapRequest("b1", "b2"), true, "same group"},
	}
	for _, c := range cases {
		_, _, err := planStackBlock(c.data, fakeTabLookup(twoBlocksOneTab), inStackFn(false), sameStackFn(c.same))
		if err == nil || !strings.Contains(err.Error(), c.message) {
			t.Errorf("%s: want an error containing %q, got %v", c.name, c.message, err)
		}
	}
}
