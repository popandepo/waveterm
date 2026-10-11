// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// ClearAttentionIfCommand is the compare-and-clear of an attention mark: the window plans a clear for a mark it has seen, and by the time the request
// arrives a program may have written another one. The compare and the write are one transaction in the store, so only the mark that was planned for is
// removed. Returns whether it was cleared; a mark that has changed is not an error.
func (ws *WshServer) ClearAttentionIfCommand(ctx context.Context, data wshrpc.CommandClearAttentionIfData) (bool, error) {
	if data.BlockId == "" {
		return false, fmt.Errorf("blockid is required")
	}
	cleared, err := wstore.ClearBlockAttentionIf(ctx, data.BlockId, data.Level, data.Reason)
	if err != nil {
		return false, fmt.Errorf("error clearing the attention of the block: %w", err)
	}
	if cleared {
		wcore.SendWaveObjUpdate(waveobj.MakeORef(waveobj.OType_Block, data.BlockId))
	}
	return cleared, nil
}
