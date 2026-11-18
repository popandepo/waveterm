// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wcore

import (
	"context"
	"fmt"

	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// MoveBlocksToTab moves existing blocks from one tab to another without touching the blocks themselves: the block
// rows and their controllers stay, so a running process keeps running. Everything is checked before anything is
// written. The layout changes are queued for the pages of the two tabs: the target gets the inserts first, then the
// membership changes, then the source gets a leaf-only removal. The order matters: the clean-up of orphaned blocks
// deletes a block that its tab lists but its layout lacks, so the insert must be queued before the tab lists the block.
func MoveBlocksToTab(ctx context.Context, fromTabId string, toTabId string, blockIds []string, targetBlockId string, focused bool) error {
	if err := checkMoveBlocksToTab(ctx, fromTabId, toTabId, blockIds, targetBlockId); err != nil {
		return err
	}
	inserts := make([]waveobj.LayoutActionData, len(blockIds))
	removals := make([]waveobj.LayoutActionData, len(blockIds))
	target := targetBlockId
	for i, blockId := range blockIds {
		inserts[i] = waveobj.LayoutActionData{
			ActionType:    LayoutActionDataType_StackInsert,
			BlockId:       blockId,
			TargetBlockId: target,
			Focused:       focused && i == 0,
		}
		removals[i] = waveobj.LayoutActionData{ActionType: LayoutActionDataType_Detach, BlockId: blockId}
		target = blockId
	}
	if err := QueueLayoutActionForTab(ctx, toTabId, inserts...); err != nil {
		return fmt.Errorf("queuing the insert in the target tab: %w", err)
	}
	if err := moveBlockMembership(ctx, fromTabId, toTabId, blockIds); err != nil {
		// the inserts are already queued: take the leaves back out so the block is not shown in two tabs
		undo := make([]waveobj.LayoutActionData, len(blockIds))
		for i, blockId := range blockIds {
			undo[i] = waveobj.LayoutActionData{ActionType: LayoutActionDataType_Detach, BlockId: blockId}
		}
		if undoErr := QueueLayoutActionForTab(ctx, toTabId, undo...); undoErr != nil {
			return fmt.Errorf("%w (and the queued inserts could not be taken back: %v)", err, undoErr)
		}
		return err
	}
	if err := QueueLayoutActionForTab(ctx, fromTabId, removals...); err != nil {
		return fmt.Errorf("queuing the removal in the source tab: %w", err)
	}
	return nil
}

func checkMoveBlocksToTab(ctx context.Context, fromTabId string, toTabId string, blockIds []string, targetBlockId string) error {
	if len(blockIds) == 0 {
		return fmt.Errorf("no block to move")
	}
	if fromTabId == toTabId {
		return fmt.Errorf("a move between tabs needs two different tabs")
	}
	fromWorkspace, err := wstore.DBFindWorkspaceForTabId(ctx, fromTabId)
	if err != nil {
		return fmt.Errorf("finding the workspace of tab %s: %w", fromTabId, err)
	}
	toWorkspace, err := wstore.DBFindWorkspaceForTabId(ctx, toTabId)
	if err != nil {
		return fmt.Errorf("finding the workspace of tab %s: %w", toTabId, err)
	}
	if fromWorkspace != toWorkspace {
		return fmt.Errorf("both tabs must be in the same workspace")
	}
	fromTab, err := wstore.DBMustGet[*waveobj.Tab](ctx, fromTabId)
	if err != nil {
		return fmt.Errorf("source tab: %w", err)
	}
	toTab, err := wstore.DBMustGet[*waveobj.Tab](ctx, toTabId)
	if err != nil {
		return fmt.Errorf("target tab: %w", err)
	}
	if targetBlockId == "" || utilfn.SliceIdx(toTab.BlockIds, targetBlockId) < 0 {
		return fmt.Errorf("target block %q is not in tab %s", targetBlockId, toTabId)
	}
	seen := map[string]bool{}
	for _, blockId := range blockIds {
		if seen[blockId] {
			return fmt.Errorf("block %s is named twice", blockId)
		}
		seen[blockId] = true
		if utilfn.SliceIdx(fromTab.BlockIds, blockId) < 0 {
			return fmt.Errorf("block %s is not in tab %s", blockId, fromTabId)
		}
	}
	return nil
}

func moveBlockMembership(ctx context.Context, fromTabId string, toTabId string, blockIds []string) error {
	return wstore.WithTx(ctx, func(tx *wstore.TxWrap) error {
		fromTab, err := wstore.DBMustGet[*waveobj.Tab](tx.Context(), fromTabId)
		if err != nil {
			return fmt.Errorf("source tab: %w", err)
		}
		toTab, err := wstore.DBMustGet[*waveobj.Tab](tx.Context(), toTabId)
		if err != nil {
			return fmt.Errorf("target tab: %w", err)
		}
		newParent := waveobj.MakeORef(waveobj.OType_Tab, toTabId).String()
		for _, blockId := range blockIds {
			block, err := wstore.DBMustGet[*waveobj.Block](tx.Context(), blockId)
			if err != nil {
				return fmt.Errorf("block %s: %w", blockId, err)
			}
			block.ParentORef = newParent
			if err := wstore.DBUpdate(tx.Context(), block); err != nil {
				return fmt.Errorf("updating block %s: %w", blockId, err)
			}
			fromTab.BlockIds = utilfn.RemoveElemFromSlice(fromTab.BlockIds, blockId)
			toTab.BlockIds = append(toTab.BlockIds, blockId)
		}
		if err := wstore.DBUpdate(tx.Context(), fromTab); err != nil {
			return fmt.Errorf("updating the source tab: %w", err)
		}
		return wstore.DBUpdate(tx.Context(), toTab)
	})
}
