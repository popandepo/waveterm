// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package waveapi is a small local API over what wavesrv already knows: list workspaces, tabs and blocks,
// focus a block or tab, and go to an address. It never reaches the LAN: the transport is a unix socket in a
// folder that only the current user can open, and every call carries a token read from a file in that folder.
package waveapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sync"
	"time"
)

var (
	ErrBadId     = errors.New("bad id")
	ErrNotFound  = errors.New("not found")
	ErrMismatch  = errors.New("address parts do not belong together")
	ErrNotActive = errors.New("the tab of this block is not the active tab; use goto")

	ErrWrongSession = errors.New("the block does not run the expected session")
)

const blockStatusRunning = "running"

var idRegexp = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Block struct {
	BlockId    string `json:"blockid"`
	View       string `json:"view,omitempty"`
	Slot       string `json:"slot,omitempty"`
	Controller string `json:"controller,omitempty"`
	Connection string `json:"connection,omitempty"`
	Title      string `json:"title,omitempty"`
	Url        string `json:"url,omitempty"`
	// Status is running, exited or notstarted for a block that has a program; empty when it has none or when it is not known.
	Status   string `json:"status,omitempty"`
	ExitCode *int   `json:"exitcode,omitempty"`
	// SessionId is the Claude session the block was opened for (block meta session:id), only when it is a lowercase uuid.
	SessionId string `json:"sessionid,omitempty"`
}

type Tab struct {
	TabId  string  `json:"tabid"`
	Name   string  `json:"name"`
	Blocks []Block `json:"blocks"`
}

type Workspace struct {
	WorkspaceId string `json:"workspaceid"`
	WindowId    string `json:"windowid"`
	Name        string `json:"name"`
	ActiveTabId string `json:"activetabid"`
	Tabs        []Tab  `json:"tabs"`
}

type Address struct {
	WorkspaceId string `json:"workspaceid,omitempty"`
	TabId       string `json:"tabid,omitempty"`
	BlockId     string `json:"blockid,omitempty"`
}

// Backend is everything the service needs from wavesrv; tests use a fake.
type Backend interface {
	Tree(ctx context.Context) ([]Workspace, error)
	FocusWindow(ctx context.Context, windowId string) error
	SetActiveTab(ctx context.Context, workspaceId string, tabId string) error
	SetBlockFocus(ctx context.Context, tabId string, blockId string) error
	// Screen returns the lines of one terminal block as its view shows them.
	Screen(ctx context.Context, blockId string) ([]string, error)
	// Input writes raw bytes to the program of one terminal block.
	Input(ctx context.Context, blockId string, data []byte) error
}

type Service struct {
	B Backend
	// Pause and Logf are replaced in tests; nil means time.Sleep and the standard logger.
	Pause func(time.Duration)
	Logf  func(format string, args ...any)

	lockMu sync.Mutex
	locks  map[string]*sync.Mutex
}

func metaString(meta map[string]any, key string) string {
	s, _ := meta[key].(string)
	return s
}

// Only these keys leave the process: block meta also holds command lines, environments and working
// directories. A url is cut down to scheme and host (and port): paths and queries can hold reset links
// and tokens, and a url without a host (about:blank, file:) is left out.
func BlockFromMeta(blockId string, meta map[string]any) Block {
	rtn := Block{
		BlockId:    blockId,
		View:       metaString(meta, "view"),
		Slot:       metaString(meta, "slot"),
		Controller: metaString(meta, "controller"),
		Connection: metaString(meta, "connection"),
		Title:      metaString(meta, "frame:title"),
	}
	if id := metaString(meta, "session:id"); idRegexp.MatchString(id) {
		rtn.SessionId = id
	}
	u, err := url.Parse(metaString(meta, "url"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return rtn
	}
	rtn.Url = u.Scheme + "://" + u.Host
	return rtn
}

func checkId(id string, required bool) error {
	if id == "" {
		if required {
			return fmt.Errorf("%w: missing", ErrBadId)
		}
		return nil
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("%w: %q is not a lowercase uuid", ErrBadId, truncateForError(id))
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Workspace, error) {
	return s.B.Tree(ctx)
}

type located struct {
	ws    Workspace
	tab   Tab
	block *Block
}

func findWorkspace(tree []Workspace, workspaceId string) (Workspace, bool) {
	for _, ws := range tree {
		if ws.WorkspaceId == workspaceId {
			return ws, true
		}
	}
	return Workspace{}, false
}

func findTab(tree []Workspace, tabId string) (located, bool) {
	for _, ws := range tree {
		for _, tab := range ws.Tabs {
			if tab.TabId == tabId {
				return located{ws: ws, tab: tab}, true
			}
		}
	}
	return located{}, false
}

func findBlock(tree []Workspace, blockId string) (located, bool) {
	for _, ws := range tree {
		for _, tab := range ws.Tabs {
			for i := range tab.Blocks {
				if tab.Blocks[i].BlockId == blockId {
					return located{ws: ws, tab: tab, block: &tab.Blocks[i]}, true
				}
			}
		}
	}
	return located{}, false
}

// Focus never switches workspace or window. A block must already be in the active tab of its workspace;
// a tab address makes that tab the active one.
func (s *Service) Focus(ctx context.Context, a Address) error {
	if err := checkId(a.WorkspaceId, false); err != nil {
		return err
	}
	if err := checkId(a.TabId, false); err != nil {
		return err
	}
	if err := checkId(a.BlockId, false); err != nil {
		return err
	}
	if a.BlockId == "" && a.TabId == "" {
		return fmt.Errorf("%w: give a block id or a tab id", ErrBadId)
	}
	tree, err := s.B.Tree(ctx)
	if err != nil {
		return err
	}
	if a.BlockId != "" {
		loc, ok := findBlock(tree, a.BlockId)
		if !ok {
			return fmt.Errorf("%w: block %s", ErrNotFound, a.BlockId)
		}
		if (a.TabId != "" && a.TabId != loc.tab.TabId) || (a.WorkspaceId != "" && a.WorkspaceId != loc.ws.WorkspaceId) {
			return ErrMismatch
		}
		if loc.ws.ActiveTabId != loc.tab.TabId {
			return ErrNotActive
		}
		return s.B.SetBlockFocus(ctx, loc.tab.TabId, a.BlockId)
	}
	loc, ok := findTab(tree, a.TabId)
	if !ok {
		return fmt.Errorf("%w: tab %s", ErrNotFound, a.TabId)
	}
	if a.WorkspaceId != "" && a.WorkspaceId != loc.ws.WorkspaceId {
		return ErrMismatch
	}
	if loc.ws.ActiveTabId == loc.tab.TabId {
		return nil
	}
	return s.B.SetActiveTab(ctx, loc.ws.WorkspaceId, loc.tab.TabId)
}

// Goto checks the whole address against the tree before the first change. The OS window is only
// raised when window is true: this is what keeps a jump from taking keyboard focus away from the user.
func (s *Service) Goto(ctx context.Context, a Address, window bool) error {
	if err := checkId(a.WorkspaceId, true); err != nil {
		return err
	}
	if err := checkId(a.TabId, true); err != nil {
		return err
	}
	if err := checkId(a.BlockId, false); err != nil {
		return err
	}
	tree, err := s.B.Tree(ctx)
	if err != nil {
		return err
	}
	ws, ok := findWorkspace(tree, a.WorkspaceId)
	if !ok {
		return fmt.Errorf("%w: workspace %s", ErrNotFound, a.WorkspaceId)
	}
	var tab *Tab
	for i := range ws.Tabs {
		if ws.Tabs[i].TabId == a.TabId {
			tab = &ws.Tabs[i]
		}
	}
	if tab == nil {
		return fmt.Errorf("%w: tab %s is not in workspace %s", ErrMismatch, a.TabId, a.WorkspaceId)
	}
	if a.BlockId != "" {
		found := false
		for _, b := range tab.Blocks {
			found = found || b.BlockId == a.BlockId
		}
		if !found {
			return fmt.Errorf("%w: block %s is not in tab %s", ErrMismatch, a.BlockId, a.TabId)
		}
	}
	if window {
		if err := s.B.FocusWindow(ctx, ws.WindowId); err != nil {
			return err
		}
	}
	if ws.ActiveTabId != a.TabId {
		if err := s.B.SetActiveTab(ctx, ws.WorkspaceId, a.TabId); err != nil {
			return err
		}
	}
	if a.BlockId == "" {
		return nil
	}
	return s.B.SetBlockFocus(ctx, a.TabId, a.BlockId)
}
