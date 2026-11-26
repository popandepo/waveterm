// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func TestAStackMoveBetweenTabsMovesTheBlockAndKeepsItsRow(t *testing.T) {
	ctx := initDeleteTestStore(t)
	blockA := insertBlockInTab(t, ctx, "term")
	blockB := insertBlockInTab(t, ctx, "term")
	tabA, _ := wstore.DBFindTabForBlockId(ctx, blockA)
	tabB, _ := wstore.DBFindTabForBlockId(ctx, blockB)
	ws := &waveobj.Workspace{OID: uuid.NewString(), TabIds: []string{tabA, tabB}}
	if err := wstore.DBInsert(ctx, ws); err != nil {
		t.Fatal(err)
	}

	server := &WshServer{}
	err := server.StackBlockCommand(ctx, wshrpc.CommandStackBlockData{BlockId: blockA, TargetBlockId: blockB, Action: wshrpc.StackBlockAction_Move})
	if err != nil {
		t.Fatalf("a move between tabs of one workspace must be accepted: %v", err)
	}
	if !blockExists(t, ctx, blockA) {
		t.Fatalf("the moved block must still exist")
	}
	if now, err := wstore.DBFindTabForBlockId(ctx, blockA); err != nil || now != tabB {
		t.Fatalf("the moved block must be in tab B, got %q, %v", now, err)
	}
}
