// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// Deleting the only block of a tab deletes the tab too. The layout action the command queues afterwards must not
// dereference the tab that is gone (it answered "panic in handleRequest: invalid memory address" although the tab did go away).
func TestDeletingTheOnlyBlockOfATabDoesNotPanic(t *testing.T) {
	ctx := initDeleteTestStore(t)
	ws := &WshServer{}
	workspace := &waveobj.Workspace{OID: uuid.NewString(), Name: "W"}
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "T1"}
	other := &waveobj.Tab{OID: uuid.NewString(), Name: "Home"}
	workspace.TabIds = []string{other.OID, tab.OID}
	workspace.ActiveTabId = other.OID
	block := &waveobj.Block{OID: uuid.NewString(), ParentORef: waveobj.MakeORef(waveobj.OType_Tab, tab.OID).String(), Meta: waveobj.MetaMapType{"view": "web"}}
	tab.BlockIds = []string{block.OID}
	layout := &waveobj.LayoutState{OID: uuid.NewString()}
	tab.LayoutState = layout.OID
	for _, obj := range []waveobj.WaveObj{workspace, layout, other, tab, block} {
		if err := wstore.DBInsert(ctx, obj); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	if err := ws.DeleteBlockCommand(ctx, wshrpc.CommandDeleteBlockData{BlockId: block.OID}); err != nil {
		t.Fatalf("deleting the last block of a tab must succeed: %v", err)
	}
	if blockExists(t, ctx, block.OID) {
		t.Fatalf("the block must be gone")
	}
	gone, err := wstore.DBGet[*waveobj.Tab](context.Background(), tab.OID)
	if err != nil || gone != nil {
		t.Fatalf("the empty tab must be gone too (tab=%v err=%v)", gone, err)
	}
}
