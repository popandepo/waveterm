// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// WebNavDenied is the one answer for every tile that the caller may not act on, whether it does not exist or is in another workspace.
const WebNavDenied = "that web tile is not available to this caller"

// checkWebNavCaller decides whether the caller of a web command (wsh web reload, back, forward, url) may act on a block. The caller is known by
// the block id in its token (the router recorded it when the link authenticated; a session cannot choose it), and may act only on a web tile in
// the SAME workspace as its own block. The server itself (the AI tools) is trusted. A caller that is not a block of this window's router (an
// unknown route, a remote connection behind another router) is refused.
func checkWebNavCaller(callerRoute string, callerBlockId string, callerWorkspace string, targetWorkspace string) error {
	if callerRoute == wshutil.DefaultRoute {
		return nil
	}
	if callerBlockId == "" {
		return fmt.Errorf("the caller of this web command is not a block of this window (its route %q has no block)", callerRoute)
	}
	// one text for a tile that does not exist and a tile in another workspace: the answer must not tell a caller which ids exist elsewhere
	if callerWorkspace == "" || targetWorkspace == "" || callerWorkspace != targetWorkspace {
		return fmt.Errorf(WebNavDenied)
	}
	return nil
}

var (
	webNavCallerBlockFn = func(route string) string { return wshutil.DefaultRouter.BlockIdForSourceRoute(route) }
	webNavWorkspaceOfFn = workspaceOfBlock
)

func workspaceOfBlock(ctx context.Context, blockId string) (string, error) {
	tabId, err := wstore.DBFindTabForBlockId(ctx, blockId)
	if err != nil {
		return "", err
	}
	return wstore.DBFindWorkspaceForTabId(ctx, tabId)
}

func (ws *WshServer) WebNavAuthorizeCommand(ctx context.Context, data wshrpc.CommandWebNavAuthorizeData) error {
	if data.CallerRoute == wshutil.DefaultRoute {
		return checkWebNavCaller(data.CallerRoute, "", "", "")
	}
	callerBlock := webNavCallerBlockFn(data.CallerRoute)
	callerWorkspace := ""
	if callerBlock != "" {
		callerWorkspace, _ = webNavWorkspaceOfFn(ctx, callerBlock)
	}
	targetWorkspace, _ := webNavWorkspaceOfFn(ctx, data.BlockId)
	return checkWebNavCaller(data.CallerRoute, callerBlock, callerWorkspace, targetWorkspace)
}
