// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wcore

import (
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// This fork never opens the welcome screen, which is what used to apply the starter layout to the first tab of a fresh profile. The first tab must start
// like every other new tab: one terminal. (A fresh profile whose first tab is empty shows a blank window.)
func TestFirstTabOfAFreshProfileHasTheOrdinaryNewTabLayout(t *testing.T) {
	ctx := initTestWStore(t)
	wavebase.ConfigHome_VarCache = t.TempDir()
	firstLaunch, err := EnsureInitialData()
	if err != nil {
		t.Fatalf("EnsureInitialData: %v", err)
	}
	if !firstLaunch {
		t.Fatal("a fresh store must count as a first launch")
	}
	client, err := wstore.DBGetSingleton[*waveobj.Client](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.WindowIds) != 1 {
		t.Fatalf("windows: %d", len(client.WindowIds))
	}
	window, err := wstore.DBMustGet[*waveobj.Window](ctx, client.WindowIds[0])
	if err != nil {
		t.Fatal(err)
	}
	ws, err := wstore.DBMustGet[*waveobj.Workspace](ctx, window.WorkspaceId)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := wstore.DBMustGet[*waveobj.Tab](ctx, ws.ActiveTabId)
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.BlockIds) != 1 {
		t.Fatalf("the first tab of a fresh profile has %d blocks, want 1 (the ordinary new-tab layout)", len(tab.BlockIds))
	}
	block, err := wstore.DBMustGet[*waveobj.Block](ctx, tab.BlockIds[0])
	if err != nil {
		t.Fatal(err)
	}
	if view, _ := block.Meta[waveobj.MetaKey_View].(string); view != "term" {
		t.Fatalf("the first block is %q, want a terminal", view)
	}
	if ctl, _ := block.Meta[waveobj.MetaKey_Controller].(string); ctl != "shell" {
		t.Fatalf("the first block's controller is %q, want shell", ctl)
	}
}
