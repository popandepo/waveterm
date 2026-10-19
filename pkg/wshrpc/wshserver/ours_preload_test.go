// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

type preloadFake struct {
	lock        sync.Mutex
	requests    []wshrpc.CommandElectronTabData
	waits       []int
	missing     map[string]int // looks per block that still say it has no controller
	looked      []string
	preloadErr  error
	preloadGate func() // called inside the request, to hold it
	workspaceOf func(ctx context.Context, tabId string) (string, error)
}

func (f *preloadFake) requester() *preloadRequester {
	workspaceOf := f.workspaceOf
	if workspaceOf == nil {
		workspaceOf = func(ctx context.Context, tabId string) (string, error) { return "ws-of-" + tabId, nil }
	}
	return &preloadRequester{
		workspaceOf: workspaceOf,
		preload: func(d wshrpc.CommandElectronTabData) error {
			f.lock.Lock()
			f.requests = append(f.requests, d)
			err := f.preloadErr
			gate := f.preloadGate
			f.lock.Unlock()
			if gate != nil {
				gate()
			}
			return err
		},
		needsStart: func(ctx context.Context, blockId string) bool {
			f.lock.Lock()
			defer f.lock.Unlock()
			f.looked = append(f.looked, blockId)
			if f.missing[blockId] > 0 {
				f.missing[blockId]--
				return true
			}
			return false
		},
		wait:     func(attempt int) { f.lock.Lock(); f.waits = append(f.waits, attempt); f.lock.Unlock() },
		inflight: map[string]*tabLoad{},
	}
}

func TestPreloadIsRequestedForTheTabsWorkspaceAndStopsOnceTheBlockHasAController(t *testing.T) {
	f := &preloadFake{}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != 1 || f.requests[0].TabId != "tab1" || f.requests[0].WorkspaceId != "ws-of-tab1" || f.requests[0].Force {
		t.Fatalf("requests: %+v", f.requests)
	}
	if len(f.looked) != 1 || f.looked[0] != "blk1" {
		t.Fatalf("the block must be looked at once after the request, looked=%v", f.looked)
	}
}

func TestAPageThatMissedTheBlockIsAskedAgainUntilItStarts(t *testing.T) {
	f := &preloadFake{missing: map[string]int{"blk1": 2}}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != 3 {
		t.Fatalf("two misses mean three requests, got %d", len(f.requests))
	}
	if len(f.waits) != 3 || f.waits[0] != 0 || f.waits[2] != 2 {
		t.Fatalf("each request is followed by a longer wait: %v", f.waits)
	}
}

func TestARequestThatNeverTakesIsGivenUpAfterTheLimit(t *testing.T) {
	f := &preloadFake{missing: map[string]int{"blk1": 1000}}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != preloadMaxAttempts {
		t.Fatalf("requests %d, limit %d", len(f.requests), preloadMaxAttempts)
	}
}

// The look says "nothing is missing" (a block that cannot be read counts as not missing), yet the request itself failed:
// the page was not loaded, so the request must be repeated.
func TestAFailedRequestIsRepeatedEvenWhenTheLookSaysNothingIsMissing(t *testing.T) {
	f := &preloadFake{preloadErr: errors.New("window is gone")}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != preloadMaxAttempts {
		t.Fatalf("a failing request is repeated up to the limit, got %d", len(f.requests))
	}
	if len(f.looked) != 0 {
		t.Fatalf("after a failed request there is nothing to look at, looked=%v", f.looked)
	}
}

func TestARequestThatFailsOnceThenWorksStopsWhenTheBlockStarted(t *testing.T) {
	f := &preloadFake{}
	r := f.requester()
	calls := 0
	inner := r.preload
	r.preload = func(d wshrpc.CommandElectronTabData) error {
		calls++
		if err := inner(d); err != nil {
			return err
		}
		if calls == 1 {
			return errors.New("window not ready")
		}
		return nil
	}
	r.request("tab1", "blk1")
	if len(f.requests) != 2 || len(f.looked) != 1 {
		t.Fatalf("one failed request, then one good one and a look: requests %d, looked %v", len(f.requests), f.looked)
	}
}

func TestPreloadNeedsATabAndAWorkspace(t *testing.T) {
	f := &preloadFake{workspaceOf: func(ctx context.Context, tabId string) (string, error) {
		if tabId == "orphan" {
			return "", errors.New("not found")
		}
		return "ws", nil
	}}
	r := f.requester()
	r.request("", "blk1")
	r.request("orphan", "blk1")
	if len(f.requests) != 0 {
		t.Fatalf("a missing tab or workspace must not reach the window, requests=%d", len(f.requests))
	}
	if len(r.inflight) != 0 {
		t.Fatalf("a tab without a workspace must not stay marked as being worked on: %v", r.inflight)
	}
}

func TestWithoutABlockOnlyOneRequestIsMade(t *testing.T) {
	f := &preloadFake{preloadErr: errors.New("window is gone")}
	f.requester().request("tab1", "")
	if len(f.requests) != 1 || len(f.looked) != 0 {
		t.Fatalf("requests %d looked %v", len(f.requests), f.looked)
	}
}

// Blocks made in a tab while a worker is already on it join that worker: one series of requests, every block looked at.
func TestBlocksMadeWhileAWorkerIsOnTheTabJoinItInsteadOfStartingOthers(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	f := &preloadFake{preloadGate: func() {
		once.Do(func() { close(entered) })
		<-release
	}}
	r := f.requester()
	done := make(chan struct{})
	go func() { r.request("tab1", "blk1"); close(done) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first request never reached the window")
	}
	returned := make(chan struct{})
	go func() { r.request("tab1", "blk2"); r.request("tab1", "blk3"); close(returned) }()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("a block made while a worker is on the tab must hand itself over at once, not wait or start a worker")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker never finished")
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	if len(f.requests) != 2 {
		t.Fatalf("the first request, then one more round for the blocks that joined: got %d", len(f.requests))
	}
	seen := map[string]bool{}
	for _, id := range f.looked {
		seen[id] = true
	}
	var ids []string
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if strings.Join(ids, ",") != "blk1,blk2,blk3" {
		t.Fatalf("every block must be looked at: %v", ids)
	}
	if len(r.inflight) != 0 {
		t.Fatalf("the tab is released at the end: %v", r.inflight)
	}
}

func TestDifferentTabsAreWorkedOnSeparately(t *testing.T) {
	f := &preloadFake{}
	r := f.requester()
	r.request("tab1", "blk1")
	r.request("tab2", "blk2")
	if len(f.requests) != 2 || f.requests[0].TabId != "tab1" || f.requests[1].TabId != "tab2" {
		t.Fatalf("requests: %+v", f.requests)
	}
}

func TestCreateBlockAsksForThePageOnlyAfterTheBlockIsLaidOut(t *testing.T) {
	src, err := os.ReadFile("wshserver.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (ws *WshServer) CreateBlockCommand(")
	end := strings.Index(text, "func (ws *WshServer) CreateSubBlockCommand(")
	if start < 0 || end < start {
		t.Fatal("CreateBlockCommand not found")
	}
	body := text[start:end]
	queue := strings.Index(body, "wcore.QueueLayoutActionForTab(")
	send := strings.Index(body, "wps.Broker.SendUpdateEvents(updates)")
	load := strings.Index(body, "requestTabPageLoad(tabId, blockData.OID)")
	if queue < 0 || send < 0 || load < 0 || !(queue < send && send < load) {
		t.Fatalf("the page load must be requested after the layout is queued and the updates are sent (queue %d, send %d, load %d)", queue, send, load)
	}
}
