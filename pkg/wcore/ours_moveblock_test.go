// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wcore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func initMoveTestStore(t *testing.T) context.Context {
	t.Helper()
	dataDir := t.TempDir()
	wavebase.DataHome_VarCache = dataDir
	if err := os.MkdirAll(filepath.Join(dataDir, wavebase.WaveDBDir), 0700); err != nil {
		t.Fatalf("db dir: %v", err)
	}
	if err := wstore.InitWStore(); err != nil {
		t.Fatalf("init wstore: %v", err)
	}
	t.Cleanup(func() { _ = wstore.CloseDB() })
	return context.Background()
}

type moveFixture struct {
	workspace string
	tabA      string
	tabB      string
	blocksA   []string
	blocksB   []string
}

func insertMoveFixture(t *testing.T, ctx context.Context, countA int, countB int) moveFixture {
	t.Helper()
	f := moveFixture{workspace: uuid.NewString()}
	makeTab := func(name string, count int) (string, []string) {
		layout := &waveobj.LayoutState{OID: uuid.NewString()}
		tab := &waveobj.Tab{OID: uuid.NewString(), Name: name, LayoutState: layout.OID}
		objs := []waveobj.WaveObj{layout}
		var ids []string
		for i := 0; i < count; i++ {
			block := &waveobj.Block{OID: uuid.NewString(), ParentORef: waveobj.MakeORef(waveobj.OType_Tab, tab.OID).String(), Meta: waveobj.MetaMapType{"view": "term"}}
			ids = append(ids, block.OID)
			objs = append(objs, block)
		}
		tab.BlockIds = ids
		objs = append(objs, tab)
		for _, obj := range objs {
			if err := wstore.DBInsert(ctx, obj); err != nil {
				t.Fatalf("insert: %v", err)
			}
		}
		return tab.OID, ids
	}
	f.tabA, f.blocksA = makeTab("A", countA)
	f.tabB, f.blocksB = makeTab("B", countB)
	ws := &waveobj.Workspace{OID: f.workspace, TabIds: []string{f.tabA, f.tabB}}
	if err := wstore.DBInsert(ctx, ws); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	return f
}

func pendingActions(t *testing.T, ctx context.Context, tabId string) []waveobj.LayoutActionData {
	t.Helper()
	layoutId, err := GetLayoutIdForTab(ctx, tabId)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := wstore.DBMustGet[*waveobj.LayoutState](ctx, layoutId)
	if err != nil {
		t.Fatal(err)
	}
	if layout.PendingBackendActions == nil {
		return nil
	}
	return *layout.PendingBackendActions
}

func TestMovingABlockToAnotherTabKeepsTheBlockAndMovesItsMembership(t *testing.T) {
	ctx := initMoveTestStore(t)
	f := insertMoveFixture(t, ctx, 2, 1)
	moved := f.blocksA[0]

	if err := MoveBlocksToTab(ctx, f.tabA, f.tabB, []string{moved}, f.blocksB[0], false); err != nil {
		t.Fatalf("move: %v", err)
	}

	block, err := wstore.DBGet[*waveobj.Block](ctx, moved)
	if err != nil || block == nil {
		t.Fatalf("the block must still exist after a move: %v", err)
	}
	if want := waveobj.MakeORef(waveobj.OType_Tab, f.tabB).String(); block.ParentORef != want {
		t.Fatalf("parent of the moved block = %q, want %q", block.ParentORef, want)
	}
	tabA, _ := wstore.DBMustGet[*waveobj.Tab](ctx, f.tabA)
	tabB, _ := wstore.DBMustGet[*waveobj.Tab](ctx, f.tabB)
	if strings.Join(tabA.BlockIds, ",") != f.blocksA[1] {
		t.Fatalf("tab A blocks = %v, want only %s", tabA.BlockIds, f.blocksA[1])
	}
	if strings.Join(tabB.BlockIds, ",") != f.blocksB[0]+","+moved {
		t.Fatalf("tab B blocks = %v, want %s then %s", tabB.BlockIds, f.blocksB[0], moved)
	}
	if found, err := wstore.DBFindTabForBlockId(ctx, moved); err != nil || found != f.tabB {
		t.Fatalf("the block must be found in tab B, got %q, %v", found, err)
	}
}

func TestMovingABlockQueuesALeafOnlyRemovalInTheSourceAndAnInsertInTheTarget(t *testing.T) {
	ctx := initMoveTestStore(t)
	f := insertMoveFixture(t, ctx, 2, 1)
	moved := f.blocksA[0]

	if err := MoveBlocksToTab(ctx, f.tabA, f.tabB, []string{moved}, f.blocksB[0], true); err != nil {
		t.Fatalf("move: %v", err)
	}

	src := pendingActions(t, ctx, f.tabA)
	if len(src) != 1 || src[0].ActionType != LayoutActionDataType_Detach || src[0].BlockId != moved {
		t.Fatalf("source tab must get exactly one detach of the block, got %+v", src)
	}
	for _, a := range src {
		if a.ActionType == LayoutActionDataType_Remove {
			t.Fatalf("a delete action would close the block and stop its process: %+v", a)
		}
	}
	dst := pendingActions(t, ctx, f.tabB)
	if len(dst) != 1 || dst[0].ActionType != LayoutActionDataType_StackInsert || dst[0].BlockId != moved || dst[0].TargetBlockId != f.blocksB[0] || !dst[0].Focused {
		t.Fatalf("target tab must get one stackinsert next to the target block, got %+v", dst)
	}
}

func TestMovingAGroupInsertsEachMemberIntoTheGroupOfTheFirst(t *testing.T) {
	ctx := initMoveTestStore(t)
	f := insertMoveFixture(t, ctx, 3, 1)
	members := []string{f.blocksA[0], f.blocksA[1]}

	if err := MoveBlocksToTab(ctx, f.tabA, f.tabB, members, f.blocksB[0], false); err != nil {
		t.Fatalf("move: %v", err)
	}

	dst := pendingActions(t, ctx, f.tabB)
	if len(dst) != 2 {
		t.Fatalf("one insert per member, got %+v", dst)
	}
	if dst[0].BlockId != members[0] || dst[0].TargetBlockId != f.blocksB[0] {
		t.Fatalf("first member goes next to the target: %+v", dst[0])
	}
	if dst[1].BlockId != members[1] || dst[1].TargetBlockId != members[0] {
		t.Fatalf("second member joins the group of the first: %+v", dst[1])
	}
	if len(pendingActions(t, ctx, f.tabA)) != 2 {
		t.Fatalf("one detach per member in the source tab")
	}
}

func TestAMoveThatCannotBeDoneChangesNothing(t *testing.T) {
	ctx := initMoveTestStore(t)
	f := insertMoveFixture(t, ctx, 2, 1)
	other := insertMoveFixture(t, ctx, 1, 1)

	cases := []struct {
		name    string
		from    string
		to      string
		blocks  []string
		target  string
		message string
	}{
		{"same tab", f.tabA, f.tabA, []string{f.blocksA[0]}, f.blocksA[1], "different tabs"},
		{"block not in the source tab", f.tabA, f.tabB, []string{f.blocksB[0]}, f.blocksB[0], "not in tab"},
		{"target not in the target tab", f.tabA, f.tabB, []string{f.blocksA[0]}, f.blocksA[1], "target block"},
		{"other workspace", f.tabA, other.tabB, []string{f.blocksA[0]}, other.blocksB[0], "workspace"},
		{"no blocks", f.tabA, f.tabB, nil, f.blocksB[0], "no block"},
		{"no target", f.tabA, f.tabB, []string{f.blocksA[0]}, "", "target block"},
	}
	for _, c := range cases {
		err := MoveBlocksToTab(ctx, c.from, c.to, c.blocks, c.target, false)
		if err == nil || !strings.Contains(err.Error(), c.message) {
			t.Errorf("%s: want an error containing %q, got %v", c.name, c.message, err)
		}
	}
	tabA, _ := wstore.DBMustGet[*waveobj.Tab](ctx, f.tabA)
	tabB, _ := wstore.DBMustGet[*waveobj.Tab](ctx, f.tabB)
	if len(tabA.BlockIds) != 2 || len(tabB.BlockIds) != 1 {
		t.Fatalf("refused moves must leave the tabs alone: A=%v B=%v", tabA.BlockIds, tabB.BlockIds)
	}
	if len(pendingActions(t, ctx, f.tabA)) != 0 || len(pendingActions(t, ctx, f.tabB)) != 0 {
		t.Fatalf("refused moves must queue nothing")
	}
}

func TestAMoveThatFailsAfterTheInsertWasQueuedChangesNothing(t *testing.T) {
	ctx := initMoveTestStore(t)
	f := insertMoveFixture(t, ctx, 1, 1)
	ghost := uuid.NewString()
	tabA, _ := wstore.DBMustGet[*waveobj.Tab](ctx, f.tabA)
	tabA.BlockIds = append(tabA.BlockIds, ghost)
	if err := wstore.DBUpdate(ctx, tabA); err != nil {
		t.Fatal(err)
	}

	err := MoveBlocksToTab(ctx, f.tabA, f.tabB, []string{f.blocksA[0], ghost}, f.blocksB[0], false)
	if err == nil {
		t.Fatalf("a block without a row cannot be moved")
	}
	if len(pendingActions(t, ctx, f.tabA)) != 0 || len(pendingActions(t, ctx, f.tabB)) != 0 {
		t.Fatalf("the failed transaction must take the queued inserts with it")
	}
	tabAAfter, _ := wstore.DBMustGet[*waveobj.Tab](ctx, f.tabA)
	if len(tabAAfter.BlockIds) != 2 {
		t.Fatalf("the failed transaction must leave tab A alone: %v", tabAAfter.BlockIds)
	}
}
