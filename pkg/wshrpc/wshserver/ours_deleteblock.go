// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"

	"github.com/wavetermdev/waveterm/pkg/blockcontroller"
	"github.com/wavetermdev/waveterm/pkg/blockstatus"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// the state of a block's program; a variable so that a test can stand in for a program that is running
var deleteStatusOf = func(blockId string) (string, bool) {
	rt := blockcontroller.GetBlockControllerRuntimeStatus(blockId)
	if rt == nil {
		return "", true
	}
	return rt.ShellProcStatus, true
}

// deleteRefusalForBlock reads the block's view and program state and asks blockstatus.DeleteRefusal. A block that cannot be read is refused.
func deleteRefusalForBlock(ctx context.Context, blockId string) string {
	block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
	if err != nil || block == nil {
		return "the block could not be read before it was deleted; use --force to delete it anyway"
	}
	view, _ := block.Meta[waveobj.MetaKey_View].(string)
	status, have := deleteStatusOf(blockId)
	return blockstatus.DeleteRefusal(view, status, have)
}
