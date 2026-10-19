// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"log"
	"sync"
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
// tab answers at once). Whether the page really picked the new blocks up is not assumed: after each request the controller of
// every block still waiting is looked at, and the request is repeated, a few times with growing waits, while one that should
// have a controller has none. A failed request counts as "not picked up" whatever the look says. A failure is only logged:
// the block exists either way.
//
// One worker per tab: blocks made in a tab while a worker is on it join that worker's list (and give it a fresh set of
// attempts), so a burst of new blocks in a hidden tab is one series of requests, not one per block.
type preloadRequester struct {
	workspaceOf func(ctx context.Context, tabId string) (string, error)
	preload     func(data wshrpc.CommandElectronTabData) error
	needsStart  func(ctx context.Context, blockId string) bool
	wait        func(attempt int)

	lock     sync.Mutex
	inflight map[string]*tabLoad
}

type tabLoad struct {
	pending  map[string]bool
	attempts int
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
		inflight:   map[string]*tabLoad{},
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

// join returns true when the caller is now the tab's worker; false when a worker already has the tab and took the block over.
func (p *preloadRequester) join(tabId string, blockId string) bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	if tl, ok := p.inflight[tabId]; ok {
		if blockId != "" {
			tl.pending[blockId] = true
			tl.attempts = 0
		}
		return false
	}
	tl := &tabLoad{pending: map[string]bool{}}
	if blockId != "" {
		tl.pending[blockId] = true
	}
	p.inflight[tabId] = tl
	return true
}

func (p *preloadRequester) release(tabId string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	delete(p.inflight, tabId)
}

// nextRound reserves the next attempt and returns the blocks waiting; ok is false when the attempts are used up.
func (p *preloadRequester) nextRound(tabId string) (attempt int, ids []string, ok bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	tl := p.inflight[tabId]
	if tl.attempts >= preloadMaxAttempts {
		delete(p.inflight, tabId)
		return 0, nil, false
	}
	attempt = tl.attempts
	tl.attempts++
	for id := range tl.pending {
		ids = append(ids, id)
	}
	return attempt, ids, true
}

// settle drops the blocks that started and ends the worker when none is left waiting. New blocks that joined during the round stay.
func (p *preloadRequester) settle(tabId string, started []string) (done bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	tl := p.inflight[tabId]
	for _, id := range started {
		delete(tl.pending, id)
	}
	if len(tl.pending) == 0 {
		delete(p.inflight, tabId)
		return true
	}
	return false
}

func (p *preloadRequester) request(tabId string, blockId string) {
	if tabId == "" || !p.join(tabId, blockId) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), preloadLookupTimeout)
	workspaceId, err := p.workspaceOf(ctx, tabId)
	cancel()
	if err != nil || workspaceId == "" {
		log.Printf("preload: no workspace found for tab %s: %v\n", tabId, err)
		p.release(tabId)
		return
	}
	data := wshrpc.CommandElectronTabData{WorkspaceId: workspaceId, TabId: tabId}
	for {
		attempt, ids, ok := p.nextRound(tabId)
		if !ok {
			log.Printf("preload: blocks in tab %s still have no controller after %d requests\n", tabId, preloadMaxAttempts)
			return
		}
		requestErr := p.preload(data)
		if requestErr != nil {
			log.Printf("preload: loading tab %s in the background failed (attempt %d): %v\n", tabId, attempt+1, requestErr)
		}
		p.wait(attempt)
		var started []string
		if requestErr == nil {
			for _, id := range ids {
				lookCtx, lookCancel := context.WithTimeout(context.Background(), preloadLookupTimeout)
				missing := p.needsStart(lookCtx, id)
				lookCancel()
				if !missing {
					started = append(started, id)
				}
			}
		}
		if p.settle(tabId, started) {
			return
		}
	}
}

var livePreload = makeLivePreloadRequester()

// requestTabPageLoad runs in the background so creating a block never waits for the window.
func requestTabPageLoad(tabId string, blockId string) {
	go livePreload.request(tabId, blockId)
}
