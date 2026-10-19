// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
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
	preloadMaxAttempts      = 5  // requests made while one block waits
	preloadMaxRounds        = 15 // requests one worker makes in its life, however many blocks join
	preloadLookupTimeout    = 5 * time.Second
)

var errNoWorkspace = errors.New("no workspace for the tab")

// A block's controller starts, and its screen exists, only while its tab's page is loaded. A block made in a tab nobody
// has shown would otherwise sit unstarted until someone looks at the tab. The request is idempotent in the window (a loaded
// tab answers at once). Whether the page really picked the new blocks up is not assumed: after each request the controller of
// every block still waiting is looked at, and the request is repeated, with growing waits, while one that should have a
// controller has none. A failed request, or a failed lookup of the tab's workspace, counts as "not picked up" whatever the
// look says. A failure is only logged: the block exists either way.
//
// One worker per tab: blocks made in a tab while a worker is on it join that worker's list. Limits: each block gets at most
// preloadMaxAttempts requests while it waits (a block that joins late does not give the others more), and one worker makes at
// most preloadMaxRounds requests however many blocks join, so a busy tab cannot keep a worker alive for good.
type preloadRequester struct {
	workspaceOf func(ctx context.Context, tabId string) (string, error)
	preload     func(data wshrpc.CommandElectronTabData) error
	needsStart  func(ctx context.Context, blockId string) bool
	wait        func(round int)

	lock     sync.Mutex
	inflight map[string]*tabLoad
}

type tabLoad struct {
	pending map[string]int // block id -> requests made while it waited
	rounds  int
}

func makeLivePreloadRequester() *preloadRequester {
	return &preloadRequester{
		workspaceOf: wstore.DBFindWorkspaceForTabId,
		preload: func(data wshrpc.CommandElectronTabData) error {
			opts := &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: preloadRequestTimeoutMs}
			return wshclient.ElectronPreloadTabCommand(GetMainRpcClient(), data, opts)
		},
		needsStart: blockWantsAControllerAndHasNone,
		wait:       func(round int) { time.Sleep(time.Duration(round+1) * time.Second) },
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
		if _, waiting := tl.pending[blockId]; blockId != "" && !waiting {
			tl.pending[blockId] = 0
		}
		return false
	}
	tl := &tabLoad{pending: map[string]int{}}
	if blockId != "" {
		tl.pending[blockId] = 0
	}
	p.inflight[tabId] = tl
	return true
}

// nextRound reserves the next request and returns the blocks that are still waiting for it; gaveUp are the blocks that have
// used their requests. ok is false when the worker is done: the round limit is reached, or every block left has used its requests.
func (p *preloadRequester) nextRound(tabId string) (round int, ids []string, gaveUp []string, ok bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	tl := p.inflight[tabId]
	if tl.rounds >= preloadMaxRounds {
		for id := range tl.pending {
			gaveUp = append(gaveUp, id)
		}
		delete(p.inflight, tabId)
		return 0, nil, gaveUp, false
	}
	round = tl.rounds
	tl.rounds++
	for id, n := range tl.pending {
		if n >= preloadMaxAttempts {
			gaveUp = append(gaveUp, id)
			delete(tl.pending, id)
			continue
		}
		tl.pending[id] = n + 1
		ids = append(ids, id)
	}
	if round > 0 && len(ids) == 0 {
		delete(p.inflight, tabId)
		return 0, nil, gaveUp, false
	}
	return round, ids, gaveUp, true
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
	workspaceId := ""
	for {
		round, ids, gaveUp, ok := p.nextRound(tabId)
		for _, id := range gaveUp {
			log.Printf("preload: block %s in tab %s still has no controller after its requests\n", id, tabId)
		}
		if !ok {
			return
		}
		var requestErr error
		if workspaceId == "" {
			ctx, cancel := context.WithTimeout(context.Background(), preloadLookupTimeout)
			workspaceId, requestErr = p.workspaceOf(ctx, tabId)
			cancel()
			if requestErr == nil && workspaceId == "" {
				requestErr = errNoWorkspace
			}
		}
		if requestErr == nil {
			requestErr = p.preload(wshrpc.CommandElectronTabData{WorkspaceId: workspaceId, TabId: tabId})
		}
		if requestErr != nil {
			log.Printf("preload: loading tab %s in the background failed (request %d): %v\n", tabId, round+1, requestErr)
		}
		p.wait(min(round, preloadMaxAttempts-1))
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
