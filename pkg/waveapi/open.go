// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

const MaxTitleChars = 80

var (
	// Neither error names the url: an address handed to open can hold a one-time entry token.
	ErrBadUrl   = errors.New("bad url: give an http or https address with a host and no user part")
	ErrBadTitle = errors.New("bad title: at most 80 characters on one line")
)

// OpenRequest makes a web block in one tab. Focus false leaves every window, tab and block focus as it was.
type OpenRequest struct {
	TabId string `json:"tabid"`
	Url   string `json:"url"`
	Title string `json:"title,omitempty"`
	Focus bool   `json:"focus,omitempty"`
}

var openKeys = []string{"tabid", "url", "title", "focus"}

type openResponse struct {
	BlockId string `json:"blockid"`
}

// breaksLine is a control character or a Unicode line or paragraph separator (U+2028, U+2029), which a title bar or a log line
// would show as a new line.
func breaksLine(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp)
}

func checkOpenUrl(raw string) error {
	if raw == "" || strings.IndexFunc(raw, breaksLine) >= 0 {
		return ErrBadUrl
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ErrBadUrl
	}
	return nil
}

func checkTitle(title string) error {
	if len([]rune(title)) > MaxTitleChars || strings.IndexFunc(title, breaksLine) >= 0 {
		return ErrBadTitle
	}
	return nil
}

// Open checks everything before it makes the block, so a refused call changes nothing.
func (s *Service) Open(ctx context.Context, req OpenRequest) (string, error) {
	if err := checkId(req.TabId, true); err != nil {
		return "", err
	}
	if err := checkOpenUrl(req.Url); err != nil {
		return "", err
	}
	if err := checkTitle(req.Title); err != nil {
		return "", err
	}
	tree, err := s.B.Tree(ctx)
	if err != nil {
		return "", err
	}
	loc, ok := findTab(tree, req.TabId)
	if !ok {
		return "", fmt.Errorf("%w: tab %s", ErrNotFound, req.TabId)
	}
	blockId, err := s.B.CreateWebBlock(ctx, req.TabId, req.Url, req.Title)
	if err != nil {
		return "", err
	}
	if !req.Focus {
		return blockId, nil
	}
	// The tab was looked up before the block was made, so the tree is not read again: a new block may not be listed yet.
	switched, err := s.moveFocus(ctx, loc.ws, req.TabId, blockId, true)
	if err == nil {
		return blockId, nil
	}
	// Take back what this call did, so a failed open leaves no block and the tab as it was. A raised window stays raised.
	if switched {
		_ = s.B.SetActiveTab(ctx, loc.ws.WorkspaceId, loc.ws.ActiveTabId)
	}
	_ = s.B.DeleteBlock(ctx, blockId)
	return "", err
}
