// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"log"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const preloadRequestTimeoutMs = 30000

// A block's controller starts, and its screen exists, only while its tab's page is loaded. A block made in a tab nobody
// has shown would otherwise sit unstarted until someone looks at the tab. The request is idempotent in the window (a loaded
// tab answers at once), so it is sent for every new block, and a failure is only logged: the block exists either way.
type preloadRequester struct {
	workspaceOf func(ctx context.Context, tabId string) (string, error)
	preload     func(data wshrpc.CommandElectronTabData) error
}

func makeLivePreloadRequester() *preloadRequester {
	return &preloadRequester{
		workspaceOf: wstore.DBFindWorkspaceForTabId,
		preload: func(data wshrpc.CommandElectronTabData) error {
			opts := &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: preloadRequestTimeoutMs}
			return wshclient.ElectronPreloadTabCommand(GetMainRpcClient(), data, opts)
		},
	}
}

func (p *preloadRequester) request(tabId string) {
	if tabId == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workspaceId, err := p.workspaceOf(ctx, tabId)
	if err != nil || workspaceId == "" {
		log.Printf("preload: no workspace found for tab %s: %v\n", tabId, err)
		return
	}
	if err := p.preload(wshrpc.CommandElectronTabData{WorkspaceId: workspaceId, TabId: tabId}); err != nil {
		log.Printf("preload: loading tab %s in the background failed: %v\n", tabId, err)
	}
}

// requestTabPageLoad runs in the background so creating a block never waits for the window.
func requestTabPageLoad(tabId string) {
	go makeLivePreloadRequester().request(tabId)
}
