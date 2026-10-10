// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wps"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// planStackBlock checks a stack request and turns it into the layout action that is queued for the tab. Nothing
// is changed here, so every refusal leaves the layout as it was. The two blocks of a "stack" request must be in
// the same tab: moving a block between tabs is a different job.
func planStackBlock(
	data wshrpc.CommandStackBlockData,
	findTab func(blockId string) (string, error),
	inStack func(tabId string, blockId string) (bool, error),
) (string, waveobj.LayoutActionData, error) {
	var none waveobj.LayoutActionData
	if data.BlockId == "" {
		return "", none, fmt.Errorf("a block id is required")
	}
	tabId, err := findTab(data.BlockId)
	if err != nil {
		return "", none, fmt.Errorf("block %s: %w", data.BlockId, err)
	}
	switch data.Action {
	case wshrpc.StackBlockAction_Stack:
		if data.TargetBlockId == "" {
			return "", none, fmt.Errorf("a target block is required to stack")
		}
		if data.TargetBlockId == data.BlockId {
			return "", none, fmt.Errorf("a block cannot be stacked with itself")
		}
		targetTab, err := findTab(data.TargetBlockId)
		if err != nil {
			return "", none, fmt.Errorf("target block %s: %w", data.TargetBlockId, err)
		}
		if targetTab != tabId {
			return "", none, fmt.Errorf("both blocks must be in the same tab to stack them")
		}
		return tabId, waveobj.LayoutActionData{
			ActionType:    wcore.LayoutActionDataType_Stack,
			BlockId:       data.BlockId,
			TargetBlockId: data.TargetBlockId,
			Focused:       data.Focused,
		}, nil
	case wshrpc.StackBlockAction_Unstack, wshrpc.StackBlockAction_Activate:
		member, err := inStack(tabId, data.BlockId)
		if err != nil {
			return "", none, fmt.Errorf("looking up the group of block %s: %w", data.BlockId, err)
		}
		if !member {
			return "", none, fmt.Errorf("block %s is not in a group", data.BlockId)
		}
		if data.Action == wshrpc.StackBlockAction_Activate {
			return tabId, waveobj.LayoutActionData{
				ActionType: wcore.LayoutActionDataType_Activate,
				BlockId:    data.BlockId,
			}, nil
		}
		return tabId, waveobj.LayoutActionData{
			ActionType: wcore.LayoutActionDataType_Unstack,
			BlockId:    data.BlockId,
			Focused:    data.Focused,
		}, nil
	}
	return "", none, fmt.Errorf("unknown stack action %q (stack, unstack or activate)", data.Action)
}

// checkStackTargetInTab runs before anything is created: a new block that is meant for a group must have its
// target in the same tab, or the layout action could not find it.
func checkStackTargetInTab(tabId string, targetBlockId string, findTab func(blockId string) (string, error)) error {
	if targetBlockId == "" {
		return fmt.Errorf("a target block is required to create a block in a stack")
	}
	targetTab, err := findTab(targetBlockId)
	if err != nil {
		return fmt.Errorf("target block %s: %w", targetBlockId, err)
	}
	if targetTab != tabId {
		return fmt.Errorf("the target block must be in the same tab as the new block")
	}
	return nil
}

// blockInStack reads the stored layout tree (decoded JSON) and reports whether the block is a member of a stack.
// The page saves the tree 100 ms after a change, so this can lag a very recent change by that much.
func blockInStack(node any, blockId string) bool {
	m, ok := node.(map[string]any)
	if !ok {
		return false
	}
	children, _ := m["children"].([]any)
	isStack := m["kind"] == "stack"
	for _, c := range children {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if data, ok := cm["data"].(map[string]any); ok && isStack && data["blockId"] == blockId {
			return true
		}
		if blockInStack(c, blockId) {
			return true
		}
	}
	return false
}

func planCreateInStack(newBlockId string, targetBlockId string, focused bool) (*waveobj.LayoutActionData, error) {
	if targetBlockId == "" {
		return nil, fmt.Errorf("a target block is required to create a block in a stack")
	}
	return &waveobj.LayoutActionData{
		ActionType:    wcore.LayoutActionDataType_StackInsert,
		BlockId:       newBlockId,
		TargetBlockId: targetBlockId,
		Focused:       focused,
	}, nil
}

func (ws *WshServer) StackBlockCommand(ctx context.Context, data wshrpc.CommandStackBlockData) error {
	ctx = waveobj.ContextWithUpdates(ctx)
	tabId, action, err := planStackBlock(data, func(blockId string) (string, error) {
		return wstore.DBFindTabForBlockId(ctx, blockId)
	}, func(tabId string, blockId string) (bool, error) {
		layoutId, err := wcore.GetLayoutIdForTab(ctx, tabId)
		if err != nil {
			return false, err
		}
		layoutObj, err := wstore.DBGet[*waveobj.LayoutState](ctx, layoutId)
		if err != nil {
			return false, err
		}
		return blockInStack(layoutObj.RootNode, blockId), nil
	})
	if err != nil {
		return err
	}
	if err := wcore.QueueLayoutActionForTab(ctx, tabId, action); err != nil {
		return fmt.Errorf("error queuing layout action: %w", err)
	}
	wps.Broker.SendUpdateEvents(waveobj.ContextGetUpdatesRtn(ctx))
	return nil
}
