// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package wavesrvapi connects the local API in pkg/waveapi to wavesrv: it reads the object store and
// sends the same RPC calls that wsh sends. It is kept apart so the wsh binary does not link wavesrv.
package wavesrvapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/blockcontroller"
	"github.com/wavetermdev/waveterm/pkg/blockstatus"
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
	ApiDirName      = "api"
	rpcTimeoutMs    = 3000
	screenTimeoutMs = 5000
	tabRoutePrefix  = "tab:"
	StartWarnAfter  = 10 * time.Second
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
				apiBlock := waveapi.BlockFromMeta(blockId, blk.Meta)
				apiBlock.Status, apiBlock.ExitCode = statusOf(blockId)
				apiTab.Blocks = append(apiTab.Blocks, apiBlock)
			}
			ws.Tabs = append(ws.Tabs, apiTab)
		}
		rtn = append(rtn, ws)
	}
	return rtn, nil
}

// statusOf reads the state of the block's program from its controller; a block without a controller has no status.
func statusOf(blockId string) (string, *int) {
	rt := blockcontroller.GetBlockControllerRuntimeStatus(blockId)
	if rt == nil {
		return "", nil
	}
	return blockstatus.FromShellProc(rt.ShellProcStatus, rt.ShellProcExitCode)
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

// Screen reads through the terminal view, so it fails when the block's tab was never shown.
func (b *Backend) Screen(ctx context.Context, blockId string) ([]string, error) {
	opts := &wshrpc.RpcOpts{Route: wshutil.MakeFeBlockRouteId(blockId), Timeout: screenTimeoutMs}
	result, err := wshclient.TermGetScrollbackLinesCommand(wshserver.GetMainRpcClient(), wshrpc.CommandTermGetScrollbackLinesData{}, opts)
	if err != nil {
		if isNoViewRoute(err, blockId) {
			return nil, fmt.Errorf("%w: block %s", waveapi.ErrNotLoaded, blockId)
		}
		return nil, fmt.Errorf("could not read block %s: %w", blockId, err)
	}
	return result.Lines, nil
}

// isNoViewRoute is true only for the router's answer that this block's own front-end route does not exist. The error crosses
// the RPC as text, so any other failure mentioning a route (another block, another kind of route) stays an internal error.
func isNoViewRoute(err error, blockId string) bool {
	return strings.Contains(err.Error(), fmt.Sprintf("no route for %q", wshutil.MakeFeBlockRouteId(blockId)))
}

func (b *Backend) Input(ctx context.Context, blockId string, data []byte) error {
	input := wshrpc.CommandBlockInputData{BlockId: blockId, InputData64: base64.StdEncoding.EncodeToString(data)}
	return wshclient.ControllerInputCommand(wshserver.GetMainRpcClient(), input, &wshrpc.RpcOpts{Timeout: rpcTimeoutMs})
}

// The local API is optional, so it must never hold wavesrv back: it starts in the background, and a start
// that takes longer than StartWarnAfter is logged. A failed start is only logged too.
func Start() {
	startInBackground(func() (*waveapi.Server, error) {
		dir := filepath.Join(wavebase.GetWaveDataDir(), ApiDirName)
		srv, err := waveapi.StartServer(dir, &waveapi.Service{B: MakeBackend()})
		if err == nil {
			log.Printf("local api listening in %s\n", dir)
		}
		return srv, err
	}, StartWarnAfter)
}

func startInBackground(start func() (*waveapi.Server, error), warnAfter time.Duration) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("local api start panicked: %v\n", r)
			}
		}()
		warn := time.AfterFunc(warnAfter, func() {
			log.Printf("local api is slow to start (more than %v); wavesrv is not waiting for it\n", warnAfter)
		})
		defer warn.Stop()
		if _, err := start(); err != nil {
			log.Printf("local api not started: %v\n", err)
		}
	}()
	return done
}
