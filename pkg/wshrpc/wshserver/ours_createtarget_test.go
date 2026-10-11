// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func TestCreateTargetNeedsSameTab(t *testing.T) {
	cases := []struct {
		action string
		target string
		want   bool
	}{
		{wshrpc.CreateBlockAction_Stack, "b", true},
		{wshrpc.CreateBlockAction_Stack, "", true},
		{wshrpc.CreateBlockAction_Replace, "b", true},
		{wshrpc.CreateBlockAction_Replace, "", false},
		{"", "", false},
		{"", "b", false},
	}
	for _, c := range cases {
		if got := createTargetNeedsSameTab(c.action, c.target); got != c.want {
			t.Errorf("createTargetNeedsSameTab(%q, %q) = %v, want %v", c.action, c.target, got, c.want)
		}
	}
}

func TestACrossTabReplaceIsRefusedByTheServerAndCreatesNothing(t *testing.T) {
	ctx := initDeleteTestStore(t)
	blockInOne := insertBlockInTab(t, ctx, "term")
	blockInTwo := insertBlockInTab(t, ctx, "term")
	tabOne, err := wstore.DBFindTabForBlockId(ctx, blockInOne)
	if err != nil {
		t.Fatal(err)
	}
	tabTwo, err := wstore.DBFindTabForBlockId(ctx, blockInTwo)
	if err != nil {
		t.Fatal(err)
	}
	before, err := wstore.DBMustGet[*waveobj.Tab](ctx, tabOne)
	if err != nil {
		t.Fatal(err)
	}
	ws := &WshServer{}
	_, err = ws.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{
		TabId:         tabOne,
		BlockDef:      &waveobj.BlockDef{Meta: waveobj.MetaMapType{"view": "web"}},
		TargetBlockId: blockInTwo,
		TargetAction:  wshrpc.CreateBlockAction_Replace,
	})
	if err == nil || !strings.Contains(err.Error(), "same tab") {
		t.Fatalf("a replace of a block in tab %s from tab %s must be refused, got %v", tabTwo, tabOne, err)
	}
	after, err := wstore.DBMustGet[*waveobj.Tab](ctx, tabOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.BlockIds) != len(before.BlockIds) {
		t.Fatalf("a refused request must create no block: %d blocks before, %d after", len(before.BlockIds), len(after.BlockIds))
	}
	if !blockExists(t, ctx, blockInTwo) {
		t.Fatalf("the target block must be untouched")
	}
}
