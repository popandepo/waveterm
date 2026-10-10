// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func fakeTabLookup(tabs map[string]string) func(string) (string, error) {
	return func(blockId string) (string, error) {
		tab, ok := tabs[blockId]
		if !ok {
			return "", fmt.Errorf("no tab for %s", blockId)
		}
		return tab, nil
	}
}

var twoBlocksOneTab = map[string]string{"b1": "t1", "b2": "t1", "b3": "t2"}

func TestPlanStackBlockStack(t *testing.T) {
	tabId, action, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b2", Action: wshrpc.StackBlockAction_Stack, Focused: true}, fakeTabLookup(twoBlocksOneTab))
	if err != nil {
		t.Fatal(err)
	}
	if tabId != "t1" || action.ActionType != "stack" || action.BlockId != "b1" || action.TargetBlockId != "b2" || !action.Focused {
		t.Fatalf("tab %s action %+v", tabId, action)
	}
}

func TestPlanStackBlockDefaultsToNoFocus(t *testing.T) {
	_, action, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b2", Action: wshrpc.StackBlockAction_Stack}, fakeTabLookup(twoBlocksOneTab))
	if err != nil {
		t.Fatal(err)
	}
	if action.Focused {
		t.Fatalf("an unset focused field must not take focus")
	}
}

func TestPlanStackBlockUnstackAndActivate(t *testing.T) {
	_, un, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", Action: wshrpc.StackBlockAction_Unstack}, fakeTabLookup(twoBlocksOneTab))
	if err != nil || un.ActionType != "unstack" || un.BlockId != "b1" {
		t.Fatalf("unstack: %+v %v", un, err)
	}
	_, act, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", Action: wshrpc.StackBlockAction_Activate, Focused: true}, fakeTabLookup(twoBlocksOneTab))
	if err != nil || act.ActionType != "activate" || act.BlockId != "b1" {
		t.Fatalf("activate: %+v %v", act, err)
	}
	if act.Focused {
		t.Fatalf("activate never asks for keyboard focus; the layout moves focus only when it must")
	}
}

func TestPlanStackBlockRefusals(t *testing.T) {
	cases := []struct {
		name string
		data wshrpc.CommandStackBlockData
		want string
	}{
		{"no block", wshrpc.CommandStackBlockData{Action: wshrpc.StackBlockAction_Stack, TargetBlockId: "b2"}, "block id"},
		{"no target", wshrpc.CommandStackBlockData{BlockId: "b1", Action: wshrpc.StackBlockAction_Stack}, "target"},
		{"same block", wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b1", Action: wshrpc.StackBlockAction_Stack}, "itself"},
		{"other tab", wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b3", Action: wshrpc.StackBlockAction_Stack}, "same tab"},
		{"unknown block", wshrpc.CommandStackBlockData{BlockId: "zz", TargetBlockId: "b2", Action: wshrpc.StackBlockAction_Stack}, "no tab"},
		{"unknown action", wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b2", Action: "merge"}, "action"},
		{"no action", wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b2"}, "action"},
	}
	for _, c := range cases {
		_, _, err := planStackBlock(c.data, fakeTabLookup(twoBlocksOneTab))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: want error containing %q, got %v", c.name, c.want, err)
		}
	}
}

func TestPlanCreateInStack(t *testing.T) {
	action, err := planCreateInStack("new1", "b2", true)
	if err != nil {
		t.Fatal(err)
	}
	if action.ActionType != "stackinsert" || action.BlockId != "new1" || action.TargetBlockId != "b2" || !action.Focused {
		t.Fatalf("action %+v", action)
	}
	if _, err := planCreateInStack("new1", "", true); err == nil {
		t.Fatalf("a stack target is required")
	}
}
