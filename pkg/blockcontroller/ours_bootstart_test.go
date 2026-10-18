// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func insertTabBlock(t *testing.T, ctx context.Context, tabId string, meta waveobj.MetaMapType) string {
	t.Helper()
	block := &waveobj.Block{OID: uuid.NewString(), ParentORef: waveobj.MakeORef(waveobj.OType_Tab, tabId).String(), Meta: meta}
	if err := wstore.DBInsert(ctx, block); err != nil {
		t.Fatal(err)
	}
	return block.OID
}

// A restart block (runonstart, not run once) in a tab that nobody ever shows must be started by the backend itself at
// start. Other blocks wait for their view, as before.
func TestRestartBlocksStartAtBackendStartWithoutTheirTabBeingShown(t *testing.T) {
	dataDir := t.TempDir()
	old := wavebase.DataHome_VarCache
	wavebase.DataHome_VarCache = dataDir
	t.Cleanup(func() { wavebase.DataHome_VarCache = old })
	if err := os.MkdirAll(filepath.Join(dataDir, wavebase.WaveDBDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := wstore.InitWStore(); err != nil {
		t.Fatalf("init wstore: %v", err)
	}
	t.Cleanup(func() { _ = wstore.CloseDB() })
	ctx := context.Background()

	hiddenTab := uuid.NewString()
	restart := insertTabBlock(t, ctx, hiddenTab, waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": "x", "cmd:runonstart": true})
	insertTabBlock(t, ctx, hiddenTab, waveobj.MetaMapType{"view": "term", "controller": "shell"})
	insertTabBlock(t, ctx, hiddenTab, waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": "x", "cmd:runonstart": true, "cmd:runonce": true})
	insertTabBlock(t, ctx, hiddenTab, waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": "x", "cmd:runonstart": false})
	insertTabBlock(t, ctx, hiddenTab, waveobj.MetaMapType{"view": "web", "cmd:runonstart": true})

	type call struct{ tabId, blockId string }
	var calls []call
	oldResync := resyncForBootStart
	t.Cleanup(func() { resyncForBootStart = oldResync })
	resyncForBootStart = func(ctx context.Context, tabId string, blockId string, rtOpts *waveobj.RuntimeOpts, force bool) error {
		calls = append(calls, call{tabId, blockId})
		return nil
	}

	StartRestartBlocks(ctx)

	sort.Slice(calls, func(i, j int) bool { return calls[i].blockId < calls[j].blockId })
	if len(calls) != 1 || calls[0].blockId != restart || calls[0].tabId != hiddenTab {
		t.Fatalf("exactly the restart block must be started in its tab %s, got %+v", hiddenTab, calls)
	}
}
