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

func TestPreloadIsRequestedForTheTabsWorkspace(t *testing.T) {
	var got []wshrpc.CommandElectronTabData
	p := &preloadRequester{
		workspaceOf: func(ctx context.Context, tabId string) (string, error) { return "ws-of-" + tabId, nil },
		preload: func(d wshrpc.CommandElectronTabData) error {
			got = append(got, d)
			return nil
		},
	}
	p.request("tab1")
	if len(got) != 1 || got[0].TabId != "tab1" || got[0].WorkspaceId != "ws-of-tab1" || got[0].Force {
		t.Fatalf("requests: %+v", got)
	}
}

func TestPreloadNeedsATabAndAWorkspaceAndSurvivesFailures(t *testing.T) {
	calls := 0
	p := &preloadRequester{
		workspaceOf: func(ctx context.Context, tabId string) (string, error) {
			if tabId == "orphan" {
				return "", errors.New("not found")
			}
			return "ws", nil
		},
		preload: func(d wshrpc.CommandElectronTabData) error {
			calls++
			return errors.New("window is gone")
		},
	}
	p.request("")
	p.request("orphan")
	if calls != 0 {
		t.Fatalf("a missing tab or workspace must not reach the window, calls=%d", calls)
	}
	p.request("tab1")
	if calls != 1 {
		t.Fatalf("a failing request is logged and does not panic or repeat, calls=%d", calls)
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
	load := strings.Index(body, "requestTabPageLoad(tabId)")
	if queue < 0 || send < 0 || load < 0 || !(queue < send && send < load) {
		t.Fatalf("the page load must be requested after the layout is queued and the updates are sent (queue %d, send %d, load %d)", queue, send, load)
	}
}
