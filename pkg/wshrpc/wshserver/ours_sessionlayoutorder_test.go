// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func pendingActionTypes(t *testing.T, ctx context.Context, layoutId string) []string {
	t.Helper()
	layout, err := wstore.DBMustGet[*waveobj.LayoutState](ctx, layoutId)
	if err != nil {
		t.Fatal(err)
	}
	var rtn []string
	if layout.PendingBackendActions != nil {
		for _, a := range *layout.PendingBackendActions {
			rtn = append(rtn, a.ActionType)
		}
	}
	return rtn
}

func insertLayoutTab(t *testing.T, ctx context.Context) (tabId string, layoutId string) {
	t.Helper()
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "s"}
	layout := &waveobj.LayoutState{OID: uuid.NewString()}
	tab.LayoutState = layout.OID
	for _, obj := range []waveobj.WaveObj{layout, tab} {
		if err := wstore.DBInsert(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
	return tab.OID, layout.OID
}

// The page of a tab queues a clean-up of orphaned blocks when it loads (BlockService.CleanupOrphanedBlocks queues exactly this
// action). The frontend runs the queue in order, so a block whose insert is queued BEHIND the clean-up can be deleted as an orphan.
// With the block created first, its insert is ahead of the clean-up; this shows both orders in the real store and layout queue.
func TestTheBlocksInsertIsQueuedAheadOfThePagesCleanupOnlyWhenTheBlockCameFirst(t *testing.T) {
	ctx := initDeleteTestStore(t)
	ws := &WshServer{}
	cleanup := waveobj.LayoutActionData{ActionType: wcore.LayoutActionDataType_CleanupOrphaned, ActionId: uuid.NewString()}
	def := &waveobj.BlockDef{Meta: waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": "x"}}

	blockFirstTab, blockFirstLayout := insertLayoutTab(t, ctx)
	if _, err := ws.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{TabId: blockFirstTab, BlockDef: def}); err != nil {
		t.Fatal(err)
	}
	if err := wcore.QueueLayoutActionForTab(ctx, blockFirstTab, cleanup); err != nil {
		t.Fatal(err)
	}
	got := pendingActionTypes(t, ctx, blockFirstLayout)
	if len(got) != 2 || got[0] == wcore.LayoutActionDataType_CleanupOrphaned || got[1] != wcore.LayoutActionDataType_CleanupOrphaned {
		t.Fatalf("block first: the insert must come before the clean-up, got %v", got)
	}

	pageFirstTab, pageFirstLayout := insertLayoutTab(t, ctx)
	if err := wcore.QueueLayoutActionForTab(ctx, pageFirstTab, cleanup); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{TabId: pageFirstTab, BlockDef: def}); err != nil {
		t.Fatal(err)
	}
	got = pendingActionTypes(t, ctx, pageFirstLayout)
	if len(got) != 2 || got[0] != wcore.LayoutActionDataType_CleanupOrphaned {
		t.Fatalf("page first (the old order): the clean-up is ahead of the insert, which is the loss, got %v", got)
	}
}
