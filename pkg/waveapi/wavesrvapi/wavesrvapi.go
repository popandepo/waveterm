// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package wavesrvapi connects the local API in pkg/waveapi to wavesrv: it reads the object store and
// sends the same RPC calls that wsh sends. It is kept apart so the wsh binary does not link wavesrv.
package wavesrvapi

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/wavetermdev/waveterm/pkg/waveapi"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshserver"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const (
	ApiDirName     = "api"
	rpcTimeoutMs   = 3000
	tabRoutePrefix = "tab:"
)

type Backend struct{}

func MakeBackend() *Backend {
	return &Backend{}
}

func (b *Backend) Tree(ctx context.Context) ([]waveapi.Workspace, error) {
	entries, err := wcore.ListAllWorkspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	var rtn []waveapi.Workspace
	for _, entry := range entries {
		wsData, err := wcore.GetWorkspace(ctx, entry.WorkspaceId)
		if err != nil {
			return nil, fmt.Errorf("getting workspace %s: %w", entry.WorkspaceId, err)
		}
		ws := waveapi.Workspace{
			WorkspaceId: entry.WorkspaceId,
			WindowId:    entry.WindowId,
			Name:        wsData.Name,
			ActiveTabId: wsData.ActiveTabId,
			Tabs:        []waveapi.Tab{},
		}
		for _, tabId := range wsData.TabIds {
			tab, err := wstore.DBMustGet[*waveobj.Tab](ctx, tabId)
			if err != nil {
				return nil, fmt.Errorf("getting tab %s: %w", tabId, err)
			}
			apiTab := waveapi.Tab{TabId: tabId, Name: tab.Name, Blocks: []waveapi.Block{}}
			for _, blockId := range tab.BlockIds {
				blk, err := wstore.DBMustGet[*waveobj.Block](ctx, blockId)
				if err != nil {
					return nil, fmt.Errorf("getting block %s: %w", blockId, err)
				}
				apiTab.Blocks = append(apiTab.Blocks, waveapi.BlockFromMeta(blockId, blk.Meta))
			}
			ws.Tabs = append(ws.Tabs, apiTab)
		}
		rtn = append(rtn, ws)
	}
	return rtn, nil
}

func electronOpts() *wshrpc.RpcOpts {
	return &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: rpcTimeoutMs}
}

func (b *Backend) FocusWindow(ctx context.Context, windowId string) error {
	return wshclient.FocusWindowCommand(wshserver.GetMainRpcClient(), windowId, electronOpts())
}

func (b *Backend) SetActiveTab(ctx context.Context, workspaceId string, tabId string) error {
	data := wshrpc.CommandElectronTabData{WorkspaceId: workspaceId, TabId: tabId}
	return wshclient.ElectronSetActiveTabCommand(wshserver.GetMainRpcClient(), data, electronOpts())
}

func (b *Backend) SetBlockFocus(ctx context.Context, tabId string, blockId string) error {
	opts := &wshrpc.RpcOpts{Route: tabRoutePrefix + tabId, Timeout: rpcTimeoutMs}
	return wshclient.SetBlockFocusCommand(wshserver.GetMainRpcClient(), blockId, opts)
}

// Start is not fatal when it fails: wavesrv keeps running without the local API.
func Start() *waveapi.Server {
	dir := filepath.Join(wavebase.GetWaveDataDir(), ApiDirName)
	srv, err := waveapi.StartServer(dir, &waveapi.Service{B: MakeBackend()})
	if err != nil {
		log.Printf("local api not started: %v\n", err)
		return nil
	}
	log.Printf("local api listening in %s\n", dir)
	return srv
}
