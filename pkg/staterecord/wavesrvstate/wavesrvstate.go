// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package wavesrvstate connects the state record to wavesrv: it reads the object store into a Snapshot and starts a writer
// that reacts to object updates. It is kept apart so that the record itself needs no database.
package wavesrvstate

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/wavetermdev/waveterm/pkg/staterecord"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wps"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const (
	StateDirName  = "state"
	StateFileName = "wave-state.json"
	debounce      = 300 * time.Millisecond
	loadTimeout   = 5 * time.Second
)

func loadSnapshot() (staterecord.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	defer cancel()
	snap := staterecord.Snapshot{
		WaveVersion: wavebase.WaveVersion,
		Tabs:        map[string]*waveobj.Tab{},
		Blocks:      map[string]*waveobj.Block{},
		Layouts:     map[string]*waveobj.LayoutState{},
	}
	entries, err := wcore.ListAllWorkspaces(ctx)
	if err != nil {
		return snap, fmt.Errorf("listing workspaces: %w", err)
	}
	// A read that fails is an error, so that the writer keeps the last good record: an incomplete record would look like closed
	// sessions. An object that is simply not there (deleted) is left out.
	for _, entry := range entries {
		ws, err := wcore.GetWorkspace(ctx, entry.WorkspaceId)
		if err != nil {
			return snap, fmt.Errorf("workspace %s: %w", entry.WorkspaceId, err)
		}
		if ws == nil {
			continue
		}
		snap.Workspaces = append(snap.Workspaces, staterecord.WorkspaceSource{WindowId: entry.WindowId, Workspace: ws})
		for _, tabId := range ws.TabIds {
			tab, err := wstore.DBGet[*waveobj.Tab](ctx, tabId)
			if err != nil {
				return snap, fmt.Errorf("tab %s: %w", tabId, err)
			}
			if tab == nil {
				continue
			}
			snap.Tabs[tabId] = tab
			if tab.LayoutState != "" {
				layout, err := wstore.DBGet[*waveobj.LayoutState](ctx, tab.LayoutState)
				if err != nil {
					return snap, fmt.Errorf("layout %s: %w", tab.LayoutState, err)
				}
				if layout != nil {
					snap.Layouts[tab.LayoutState] = layout
				}
			}
			for _, blockId := range tab.BlockIds {
				block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
				if err != nil {
					return snap, fmt.Errorf("block %s: %w", blockId, err)
				}
				if block != nil {
					snap.Blocks[blockId] = block
				}
			}
		}
	}
	return snap, nil
}

// StartWith writes the record once and then again after every change to the objects it is built from.
func StartWith(path string, build func() (staterecord.Record, error), debounceFor time.Duration) *staterecord.Writer {
	w := staterecord.MakeWriter(path, build, debounceFor)
	wps.AddObserver(func(event wps.WaveEvent) {
		if staterecord.IsRelevantUpdate(event.Event, event.Scopes) {
			w.Notify()
		}
	})
	w.Notify()
	return w
}

// Start uses the object store. A failure is logged and never stops wavesrv.
func Start() *staterecord.Writer {
	path := filepath.Join(wavebase.GetWaveDataDir(), StateDirName, StateFileName)
	w := StartWith(path, func() (staterecord.Record, error) {
		snap, err := loadSnapshot()
		if err != nil {
			return staterecord.Record{}, err
		}
		return staterecord.BuildRecord(snap), nil
	}, debounce)
	log.Printf("state record: %s\n", path)
	return w
}
