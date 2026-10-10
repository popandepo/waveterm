// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"encoding/json"
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
	tabId, action, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b2", Action: wshrpc.StackBlockAction_Stack, Focused: true}, fakeTabLookup(twoBlocksOneTab), inStackFn(true))
	if err != nil {
		t.Fatal(err)
	}
	if tabId != "t1" || action.ActionType != "stack" || action.BlockId != "b1" || action.TargetBlockId != "b2" || !action.Focused {
		t.Fatalf("tab %s action %+v", tabId, action)
	}
}

func TestPlanStackBlockDefaultsToNoFocus(t *testing.T) {
	_, action, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b2", Action: wshrpc.StackBlockAction_Stack}, fakeTabLookup(twoBlocksOneTab), inStackFn(true))
	if err != nil {
		t.Fatal(err)
	}
	if action.Focused {
		t.Fatalf("an unset focused field must not take focus")
	}
}

func TestPlanStackBlockUnstackAndActivate(t *testing.T) {
	_, un, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", Action: wshrpc.StackBlockAction_Unstack}, fakeTabLookup(twoBlocksOneTab), inStackFn(true))
	if err != nil || un.ActionType != "unstack" || un.BlockId != "b1" {
		t.Fatalf("unstack: %+v %v", un, err)
	}
	_, act, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", Action: wshrpc.StackBlockAction_Activate, Focused: true}, fakeTabLookup(twoBlocksOneTab), inStackFn(true))
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
		_, _, err := planStackBlock(c.data, fakeTabLookup(twoBlocksOneTab), inStackFn(true))
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

func inStackFn(v bool) func(string, string) (bool, error) {
	return func(tabId string, blockId string) (bool, error) { return v, nil }
}

func TestUnstackAndActivateNeedAGroupMember(t *testing.T) {
	for _, action := range []string{wshrpc.StackBlockAction_Unstack, wshrpc.StackBlockAction_Activate} {
		_, _, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", Action: action}, fakeTabLookup(twoBlocksOneTab), inStackFn(false))
		if err == nil || !strings.Contains(err.Error(), "not in a group") {
			t.Fatalf("%s on an ordinary block must be refused, got %v", action, err)
		}
	}
	if _, _, err := planStackBlock(wshrpc.CommandStackBlockData{BlockId: "b1", TargetBlockId: "b2", Action: wshrpc.StackBlockAction_Stack}, fakeTabLookup(twoBlocksOneTab), inStackFn(false)); err != nil {
		t.Fatalf("stack does not need an existing group: %v", err)
	}
}

const layoutJson = `{"id":"root","children":[
 {"id":"s1","kind":"stack","activeChildId":"n2","children":[{"id":"n1","data":{"blockId":"b1"}},{"id":"n2","data":{"blockId":"b2"}}]},
 {"id":"n3","data":{"blockId":"b3"}},
 {"id":"r2","children":[{"id":"n4","data":{"blockId":"b4"}},{"id":"n5","data":{"blockId":"b5"}}]}]}`

func TestBlockInStack(t *testing.T) {
	var root any
	if err := json.Unmarshal([]byte(layoutJson), &root); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"b1": true, "b2": true, "b3": false, "b4": false, "b5": false, "zz": false}
	for blockId, expect := range want {
		if got := blockInStack(root, blockId); got != expect {
			t.Errorf("%s: got %v want %v", blockId, got, expect)
		}
	}
	if blockInStack(nil, "b1") {
		t.Errorf("no layout means no group")
	}
}

func TestCreateInStackTargetMustBeInTheSameTab(t *testing.T) {
	find := fakeTabLookup(twoBlocksOneTab)
	if err := checkStackTargetInTab("t1", "b2", find); err != nil {
		t.Fatalf("same tab: %v", err)
	}
	if err := checkStackTargetInTab("t1", "b3", find); err == nil || !strings.Contains(err.Error(), "same tab") {
		t.Fatalf("other tab: %v", err)
	}
	if err := checkStackTargetInTab("t1", "zz", find); err == nil {
		t.Fatalf("unknown target must be refused")
	}
	if err := checkStackTargetInTab("t1", "", find); err == nil {
		t.Fatalf("empty target must be refused")
	}
}
