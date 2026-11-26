// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func decodeTree(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestStackMembersOfReturnsTheGroupInOrderOrTheBlockAlone(t *testing.T) {
	tree := decodeTree(t, `{"id":"root","children":[
		{"id":"s1","kind":"stack","children":[{"id":"l1","data":{"blockId":"a"}},{"id":"l2","data":{"blockId":"b"}}]},
		{"id":"l3","data":{"blockId":"c"}}]}`)
	members := func(node any, blockId string) string {
		ids, _ := stackMembersOf(node, blockId)
		return strings.Join(ids, ",")
	}
	if got := members(tree, "b"); got != "a,b" {
		t.Fatalf("members of the group of b = %q, want a,b", got)
	}
	if got := members(tree, "c"); got != "c" {
		t.Fatalf("a block outside a group moves alone, got %q", got)
	}
	if got := members(nil, "z"); got != "z" {
		t.Fatalf("an unreadable tree moves the block alone, got %q", got)
	}
}

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

func TestAStackMoveBetweenTabsOfTwoWorkspacesIsRefused(t *testing.T) {
	ctx := initDeleteTestStore(t)
	blockA := insertBlockInTab(t, ctx, "term")
	blockB := insertBlockInTab(t, ctx, "term")
	tabA, _ := wstore.DBFindTabForBlockId(ctx, blockA)
	tabB, _ := wstore.DBFindTabForBlockId(ctx, blockB)
	for _, tab := range []string{tabA, tabB} {
		if err := wstore.DBInsert(ctx, &waveobj.Workspace{OID: uuid.NewString(), TabIds: []string{tab}}); err != nil {
			t.Fatal(err)
		}
	}
	err := (&WshServer{}).StackBlockCommand(ctx, wshrpc.CommandStackBlockData{BlockId: blockA, TargetBlockId: blockB, Action: wshrpc.StackBlockAction_Move})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("want a workspace refusal, got %v", err)
	}
	if now, _ := wstore.DBFindTabForBlockId(ctx, blockA); now != tabA {
		t.Fatalf("a refused move must leave the block in tab A")
	}
}
