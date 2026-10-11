// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wcore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/util/shellutil"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// Deleting a block removes its per-block PowerShell history file, and only that one. The store is closed again at the end so that the
// temporary folder can be removed on Windows.
func TestDeletingABlockRemovesItsHistoryFileAndOnlyThat(t *testing.T) {
	ctx := initTestWStore(t)
	t.Cleanup(func() { _ = wstore.CloseDB() })
	dataDir := wavebase.GetWaveDataDir()
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "T1"}
	mine := &waveobj.Block{OID: uuid.NewString(), ParentORef: waveobj.MakeORef(waveobj.OType_Tab, tab.OID).String(), Meta: waveobj.MetaMapType{"view": "term"}}
	other := &waveobj.Block{OID: uuid.NewString(), ParentORef: waveobj.MakeORef(waveobj.OType_Tab, tab.OID).String(), Meta: waveobj.MetaMapType{"view": "term"}}
	tab.BlockIds = []string{mine.OID, other.OID}
	for _, obj := range []waveobj.WaveObj{tab, mine, other} {
		if err := wstore.DBInsert(ctx, obj); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dataDir, shellutil.HistoryDirName), 0700); err != nil {
		t.Fatal(err)
	}
	mineFile := shellutil.BlockHistoryFile(dataDir, mine.OID)
	otherFile := shellutil.BlockHistoryFile(dataDir, other.OID)
	for _, f := range []string{mineFile, otherFile} {
		if err := os.WriteFile(f, []byte("echo hi\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := DeleteBlock(context.Background(), mine.OID, false); err != nil {
		t.Fatalf("DeleteBlock: %v", err)
	}
	if _, err := os.Stat(mineFile); !os.IsNotExist(err) {
		t.Fatalf("the history file of the deleted block must be gone: %v", err)
	}
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("another block's history file must stay: %v", err)
	}
}
