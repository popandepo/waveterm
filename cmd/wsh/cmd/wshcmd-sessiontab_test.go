// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// fakeSessionOps records the order of the calls; the tab-creation part is the fakeTabCreator of the tab tests.
type fakeSessionOps struct {
	fakeTabCreator
	blockErr error
	styleErr error
	closeErr error
}

func (f *fakeSessionOps) CloseTab(wsId string, tabId string) error {
	f.calls = append(f.calls, "close "+tabId)
	return f.closeErr
}

func (f *fakeSessionOps) SetTabStyle(tabId string, meta map[string]any) error {
	f.calls = append(f.calls, "style "+tabId)
	return f.styleErr
}

func (f *fakeSessionOps) CreateBlock(tabId string, meta map[string]any) (string, error) {
	f.calls = append(f.calls, "block "+tabId)
	if f.blockErr != nil {
		return "", f.blockErr
	}
	return "block-new", nil
}

// The page of a tab queues "clean up orphaned blocks" when it first loads. A block created after that is in the tab's
// block list before its layout insert has run, and the cleanup deletes it (the session vanished and a bare shell was left).
// So the block must exist, with its layout insert queued, before the page is loaded.
func TestSessionTabLoadsItsPageOnlyAfterTheBlockExists(t *testing.T) {
	f := &fakeSessionOps{}
	tabId, blockId, err := createSessionTab(f, "ws1", "s1", map[string]any{"tab:color": "#fff"}, map[string]any{"view": "term"}, false)
	if err != nil || tabId != "tab-new" || blockId != "block-new" {
		t.Fatalf("got %q %q %v", tabId, blockId, err)
	}
	want := []string{"create ws=ws1 name=s1 empty=true", "style tab-new", "block tab-new", "preload tab-new"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls %v, want %v", f.calls, want)
	}
}

func TestSessionTabActivatesOnlyWhenAskedAndAfterTheLoad(t *testing.T) {
	f := &fakeSessionOps{}
	if _, _, err := createSessionTab(f, "ws1", "s1", nil, map[string]any{"view": "term"}, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"create ws=ws1 name=s1 empty=true", "block tab-new", "preload tab-new", "activate tab-new"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls %v, want %v", f.calls, want)
	}
}

func TestSessionTabFailuresNameTheTabThatWasLeft(t *testing.T) {
	f := &fakeSessionOps{blockErr: fmt.Errorf("boom")}
	_, _, err := createSessionTab(f, "ws1", "s1", nil, map[string]any{"view": "term"}, false)
	if err == nil || !strings.Contains(err.Error(), "tab-new") {
		t.Fatalf("a failed block must name the tab, got %v", err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "preload") {
			t.Fatalf("a tab without its block must not be loaded: %v", f.calls)
		}
	}
	f = &fakeSessionOps{fakeTabCreator: fakeTabCreator{preloadErr: fmt.Errorf("slow")}}
	tabId, blockId, err := createSessionTab(f, "ws1", "s1", nil, map[string]any{"view": "term"}, false)
	if err == nil || tabId != "tab-new" || blockId != "block-new" || !strings.Contains(err.Error(), "tab-new") {
		t.Fatalf("a failed load must name the tab and keep the block id, got %q %q %v", tabId, blockId, err)
	}
}

// A tab this call made must not be left behind empty when styling it or creating its block fails.
func TestAFailedStyleOrBlockClosesTheTabJustCreated(t *testing.T) {
	for name, f := range map[string]*fakeSessionOps{
		"block": {blockErr: fmt.Errorf("boom")},
		"style": {styleErr: fmt.Errorf("boom")},
	} {
		tabId, _, err := createSessionTab(f, "ws1", "s1", map[string]any{"tab:color": "#fff"}, map[string]any{"view": "term"}, false)
		if err == nil || !strings.Contains(err.Error(), "tab-new") {
			t.Fatalf("%s: the error must name the tab, got %v", name, err)
		}
		if tabId != "" {
			t.Fatalf("%s: a closed tab must not be handed back as a leftover, got %q", name, tabId)
		}
		if f.calls[len(f.calls)-1] != "close tab-new" {
			t.Fatalf("%s: the tab just created must be closed last, calls %v", name, f.calls)
		}
	}
}

func TestIfTheLeftoverTabCannotBeClosedTheErrorSaysSoAndKeepsItsId(t *testing.T) {
	f := &fakeSessionOps{blockErr: fmt.Errorf("boom"), closeErr: fmt.Errorf("nope")}
	tabId, _, err := createSessionTab(f, "ws1", "s1", nil, map[string]any{"view": "term"}, false)
	if err == nil || tabId != "tab-new" || !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("both failures must be named and the tab id kept, got %q %v", tabId, err)
	}
}

// A load failure leaves a tab WITH its block: nothing is closed, the caller gets both ids.
func TestAFailedLoadDoesNotCloseATabThatHasItsBlock(t *testing.T) {
	f := &fakeSessionOps{fakeTabCreator: fakeTabCreator{preloadErr: fmt.Errorf("slow")}}
	_, _, _ = createSessionTab(f, "ws1", "s1", nil, map[string]any{"view": "term"}, false)
	for _, c := range f.calls {
		if strings.HasPrefix(c, "close") {
			t.Fatalf("a tab with its block must be kept: %v", f.calls)
		}
	}
}

func TestSessionStartUsesTheSessionTabOrder(t *testing.T) {
	src, err := os.ReadFile("wshcmd-session.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	fn := text[strings.Index(text, "func sessionStartRun"):]
	fn = fn[:strings.Index(fn, "\n}\n")]
	if !strings.Contains(fn, "createSessionTab(") || strings.Contains(fn, "createBackgroundTab(") {
		t.Fatalf("session start must create the block before the page is loaded (createSessionTab), not load an empty tab first")
	}
}
