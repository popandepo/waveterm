// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// tabLayoutTiles reads the stored layout tree (decoded JSON) and returns its tiles in layout order: a leaf is one tile,
// a stack is one tile with its members in order.
func tabLayoutTiles(node any) []wshrpc.TabLayoutTile {
	tiles := []wshrpc.TabLayoutTile{}
	var walk func(n any)
	walk = func(n any) {
		m, ok := n.(map[string]any)
		if !ok {
			return
		}
		children, _ := m["children"].([]any)
		if m["kind"] == "stack" {
			tile := wshrpc.TabLayoutTile{BlockIds: []string{}}
			activeChild, _ := m["activeChildId"].(string)
			for _, c := range children {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				data, _ := cm["data"].(map[string]any)
				blockId, _ := data["blockId"].(string)
				if blockId == "" {
					continue
				}
				tile.BlockIds = append(tile.BlockIds, blockId)
				if cm["id"] == activeChild {
					tile.Active = blockId
				}
			}
			if len(tile.BlockIds) > 0 {
				tiles = append(tiles, tile)
			}
			return
		}
		if len(children) == 0 {
			data, _ := m["data"].(map[string]any)
			if blockId, _ := data["blockId"].(string); blockId != "" {
				tiles = append(tiles, wshrpc.TabLayoutTile{BlockIds: []string{blockId}})
			}
			return
		}
		for _, c := range children {
			walk(c)
		}
	}
	walk(node)
	return tiles
}

// GetTabLayoutCommand is read-only: it changes nothing and never touches focus or the shown tab.
func (ws *WshServer) GetTabLayoutCommand(ctx context.Context, data wshrpc.CommandGetTabLayoutData) (wshrpc.TabLayoutData, error) {
	var none wshrpc.TabLayoutData
	if data.TabId == "" {
		return none, fmt.Errorf("a tab id is required")
	}
	layoutId, err := wcore.GetLayoutIdForTab(ctx, data.TabId)
	if err != nil {
		return none, err
	}
	layout, err := wstore.DBGet[*waveobj.LayoutState](ctx, layoutId)
	if err != nil {
		return none, fmt.Errorf("reading the layout of tab %s: %w", data.TabId, err)
	}
	if layout == nil {
		return none, fmt.Errorf("tab %s has no layout object", data.TabId)
	}
	rtn := wshrpc.TabLayoutData{TabId: data.TabId, Tiles: tabLayoutTiles(layout.RootNode)}
	if layout.PendingBackendActions != nil {
		rtn.PendingActions = len(*layout.PendingBackendActions)
	}
	return rtn, nil
}
