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

func checkOpenUrl(raw string) error {
	if raw == "" || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return ErrBadUrl
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ErrBadUrl
	}
	return nil
}

func checkTitle(title string) error {
	if len([]rune(title)) > MaxTitleChars || strings.IndexFunc(title, unicode.IsControl) >= 0 {
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
	return blockId, s.Goto(ctx, Address{WorkspaceId: loc.ws.WorkspaceId, TabId: req.TabId, BlockId: blockId}, true)
}
