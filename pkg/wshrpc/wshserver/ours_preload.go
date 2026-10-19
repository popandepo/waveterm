// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"log"
	"time"

	"github.com/wavetermdev/waveterm/pkg/blockcontroller"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const (
	preloadRequestTimeoutMs = 30000
	preloadMaxAttempts      = 5
	preloadLookupTimeout    = 5 * time.Second
)

// A block's controller starts, and its screen exists, only while its tab's page is loaded. A block made in a tab nobody
// has shown would otherwise sit unstarted until someone looks at the tab. The request is idempotent in the window (a loaded
// tab answers at once). Whether the page really picked the new block up is not assumed: the block's controller is looked
// at after each request, and the request is repeated, a few times with growing waits, while a block that should have one has
// none. A failure is only logged: the block exists either way.
type preloadRequester struct {
	workspaceOf func(ctx context.Context, tabId string) (string, error)
	preload     func(data wshrpc.CommandElectronTabData) error
	needsStart  func(ctx context.Context, blockId string) bool
	wait        func(attempt int)
}

func makeLivePreloadRequester() *preloadRequester {
	return &preloadRequester{
		workspaceOf: wstore.DBFindWorkspaceForTabId,
		preload: func(data wshrpc.CommandElectronTabData) error {
			opts := &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: preloadRequestTimeoutMs}
			return wshclient.ElectronPreloadTabCommand(GetMainRpcClient(), data, opts)
		},
		needsStart: blockWantsAControllerAndHasNone,
		wait:       func(attempt int) { time.Sleep(time.Duration(attempt+1) * time.Second) },
	}
}

// blockWantsAControllerAndHasNone is false when the block cannot be read: nothing is then known to be missing.
func blockWantsAControllerAndHasNone(ctx context.Context, blockId string) bool {
	blk, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
	if err != nil || blk == nil {
		return false
	}
	controller, _ := blk.Meta[waveobj.MetaKey_Controller].(string)
	return controller != "" && blockcontroller.GetBlockControllerRuntimeStatus(blockId) == nil
}

func (p *preloadRequester) request(tabId string, blockId string) {
	if tabId == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), preloadLookupTimeout)
	workspaceId, err := p.workspaceOf(ctx, tabId)
	cancel()
	if err != nil || workspaceId == "" {
		log.Printf("preload: no workspace found for tab %s: %v\n", tabId, err)
		return
	}
	data := wshrpc.CommandElectronTabData{WorkspaceId: workspaceId, TabId: tabId}
	for attempt := 0; attempt < preloadMaxAttempts; attempt++ {
		if err := p.preload(data); err != nil {
			log.Printf("preload: loading tab %s in the background failed (attempt %d): %v\n", tabId, attempt+1, err)
		}
		p.wait(attempt)
		lookCtx, lookCancel := context.WithTimeout(context.Background(), preloadLookupTimeout)
		missing := blockId != "" && p.needsStart(lookCtx, blockId)
		lookCancel()
		if !missing {
			return
		}
	}
	log.Printf("preload: block %s in tab %s has no controller after %d requests\n", blockId, tabId, preloadMaxAttempts)
}

// requestTabPageLoad runs in the background so creating a block never waits for the window.
func requestTabPageLoad(tabId string, blockId string) {
	go makeLivePreloadRequester().request(tabId, blockId)
}
