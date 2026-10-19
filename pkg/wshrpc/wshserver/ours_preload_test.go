// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

type preloadFake struct {
	requests    []wshrpc.CommandElectronTabData
	waits       []int
	missing     int // how many looks still say the block has no controller
	looks       int
	preloadErr  error
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
			f.requests = append(f.requests, d)
			return f.preloadErr
		},
		needsStart: func(ctx context.Context, blockId string) bool {
			f.looks++
			if f.missing > 0 {
				f.missing--
				return true
			}
			return false
		},
		wait: func(attempt int) { f.waits = append(f.waits, attempt) },
	}
}

func TestPreloadIsRequestedForTheTabsWorkspaceAndStopsOnceTheBlockHasAController(t *testing.T) {
	f := &preloadFake{}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != 1 || f.requests[0].TabId != "tab1" || f.requests[0].WorkspaceId != "ws-of-tab1" || f.requests[0].Force {
		t.Fatalf("requests: %+v", f.requests)
	}
	if f.looks != 1 {
		t.Fatalf("the block must be looked at once after the request, looks=%d", f.looks)
	}
}

func TestAPageThatMissedTheBlockIsAskedAgainUntilItStarts(t *testing.T) {
	f := &preloadFake{missing: 2}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != 3 {
		t.Fatalf("two misses mean three requests, got %d", len(f.requests))
	}
	if len(f.waits) != 3 || f.waits[0] != 0 || f.waits[2] != 2 {
		t.Fatalf("each request is followed by a longer wait: %v", f.waits)
	}
}

func TestARequestThatNeverTakesIsGivenUpAfterTheLimit(t *testing.T) {
	f := &preloadFake{missing: 1000}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != preloadMaxAttempts {
		t.Fatalf("requests %d, limit %d", len(f.requests), preloadMaxAttempts)
	}
}

func TestAFailingRequestIsRetriedAndNeverPanics(t *testing.T) {
	f := &preloadFake{preloadErr: errors.New("window is gone"), missing: 1000}
	f.requester().request("tab1", "blk1")
	if len(f.requests) != preloadMaxAttempts {
		t.Fatalf("a failing request is repeated up to the limit, got %d", len(f.requests))
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
}

func TestWithoutABlockOnlyTheRequestIsMade(t *testing.T) {
	f := &preloadFake{missing: 1000}
	f.requester().request("tab1", "")
	if len(f.requests) != 1 || f.looks != 0 {
		t.Fatalf("requests %d looks %d", len(f.requests), f.looks)
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
