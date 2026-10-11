// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func initDeleteTestStore(t *testing.T) context.Context {
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

func insertBlock(t *testing.T, ctx context.Context, view string) string {
	t.Helper()
	block := &waveobj.Block{OID: uuid.NewString(), Meta: waveobj.MetaMapType{"view": view}}
	if err := wstore.DBInsert(ctx, block); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return block.OID
}

func TestTheServerRefusesARunningOrUnknownBlockBeforeDeletingIt(t *testing.T) {
	ctx := initDeleteTestStore(t)
	old := deleteStatusOf
	t.Cleanup(func() { deleteStatusOf = old })
	state := "running"
	deleteStatusOf = func(string) (string, bool) { return state, true }

	term := insertBlock(t, ctx, "term")
	if got := deleteRefusalForBlock(ctx, term); !strings.Contains(got, "--force") {
		t.Fatalf("a running terminal must be refused with a way forward, got %q", got)
	}
	state = "done"
	if got := deleteRefusalForBlock(ctx, term); got != "" {
		t.Fatalf("an ended terminal may go, got %q", got)
	}
	state = ""
	if got := deleteRefusalForBlock(ctx, term); got == "" {
		t.Fatalf("a terminal without a controller state must be refused")
	}
	deleteStatusOf = func(string) (string, bool) { return "", false }
	state = "done"
	if got := deleteRefusalForBlock(ctx, term); got == "" {
		t.Fatalf("a state that cannot be read must be refused")
	}
	web := insertBlock(t, ctx, "web")
	if got := deleteRefusalForBlock(ctx, web); got != "" {
		t.Fatalf("a web tile has no program, got %q", got)
	}
	unknown := insertBlock(t, ctx, "somethingnew")
	if got := deleteRefusalForBlock(ctx, unknown); got == "" {
		t.Fatalf("an unknown view must be refused")
	}
	if got := deleteRefusalForBlock(ctx, uuid.NewString()); got == "" {
		t.Fatalf("a block that cannot be read must be refused")
	}
}

func TestDeleteBlockCommandChecksOnlyWhenAskedAndBeforeDeleting(t *testing.T) {
	src, err := os.ReadFile("wshserver.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	fn := text[strings.Index(text, "func (ws *WshServer) DeleteBlockCommand"):]
	fn = fn[:strings.Index(fn, "\n}\n")]
	check := strings.Index(fn, "if data.RefuseRunning {")
	del := strings.Index(fn, "wcore.DeleteBlock(ctx, data.BlockId, true)")
	if check < 0 || del < 0 || check > del {
		t.Fatalf("the check (%d) must come before the delete (%d), and only for a request that asks for it", check, del)
	}
	var data wshrpc.CommandDeleteBlockData
	if data.RefuseRunning {
		t.Fatalf("a request that does not ask must not be checked: other callers (the slot command, the block controller) keep their behaviour")
	}
}

// The real command against a real store, as a caller of the RPC sees it: a running program is refused and the block is still there; an ended one,
// or a request that does not ask for the check, deletes.
func insertBlockInTab(t *testing.T, ctx context.Context, view string) (blockId string) {
	t.Helper()
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "T1"}
	block := &waveobj.Block{OID: uuid.NewString(), ParentORef: waveobj.MakeORef(waveobj.OType_Tab, tab.OID).String(), Meta: waveobj.MetaMapType{"view": view}}
	// a second block keeps the tab alive when the first one is deleted (deleting the last block of a tab deletes the tab)
	keeper := &waveobj.Block{OID: uuid.NewString(), ParentORef: block.ParentORef, Meta: waveobj.MetaMapType{"view": "web"}}
	tab.BlockIds = []string{block.OID, keeper.OID}
	// the delete queues a layout action for the tab, which needs the tab's layout object
	layout := &waveobj.LayoutState{OID: uuid.NewString()}
	tab.LayoutState = layout.OID
	for _, obj := range []waveobj.WaveObj{layout, tab, block, keeper} {
		if err := wstore.DBInsert(ctx, obj); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return block.OID
}

func blockExists(t *testing.T, ctx context.Context, blockId string) bool {
	t.Helper()
	b, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return b != nil
}

func TestDeleteBlockCommandRefusesARunningBlockAndKeepsIt(t *testing.T) {
	ctx := initDeleteTestStore(t)
	old := deleteStatusOf
	t.Cleanup(func() { deleteStatusOf = old })
	state := "running"
	deleteStatusOf = func(string) (string, bool) { return state, true }
	ws := &WshServer{}

	running := insertBlockInTab(t, ctx, "term")
	err := ws.DeleteBlockCommand(ctx, wshrpc.CommandDeleteBlockData{BlockId: running, RefuseRunning: true})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("a running block must be refused with a way forward, got %v", err)
	}
	if !blockExists(t, ctx, running) {
		t.Fatalf("a refused block must still be there")
	}
	if err := ws.DeleteBlockCommand(ctx, wshrpc.CommandDeleteBlockData{BlockId: running}); err != nil {
		t.Fatalf("a request that does not ask for the check deletes as before: %v", err)
	}
	if blockExists(t, ctx, running) {
		t.Fatalf("the block must be gone after an unchecked delete")
	}

	state = "done"
	ended := insertBlockInTab(t, ctx, "term")
	if err := ws.DeleteBlockCommand(ctx, wshrpc.CommandDeleteBlockData{BlockId: ended, RefuseRunning: true}); err != nil {
		t.Fatalf("an ended program may be deleted: %v", err)
	}
	if blockExists(t, ctx, ended) {
		t.Fatalf("the ended block must be gone")
	}

	state = "none"
	unknown := insertBlockInTab(t, ctx, "term")
	if err := ws.DeleteBlockCommand(ctx, wshrpc.CommandDeleteBlockData{BlockId: unknown, RefuseRunning: true}); err == nil {
		t.Fatalf("a terminal without a controller state must be refused")
	}
	if !blockExists(t, ctx, unknown) {
		t.Fatalf("a refused block must still be there")
	}
}

func TestTheStateIsReadFromTheControllerRegistryNotFixed(t *testing.T) {
	status, have := deleteStatusOf("00000000-0000-0000-0000-00000000dead")
	if status == "done" || !have {
		t.Fatalf("a block without a controller has no state (not \"done\"), got %q have=%v", status, have)
	}
}
