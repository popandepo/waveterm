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
func planStackBlock(data wshrpc.CommandStackBlockData, findTab func(blockId string) (string, error)) (string, waveobj.LayoutActionData, error) {
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
	case wshrpc.StackBlockAction_Unstack:
		return tabId, waveobj.LayoutActionData{
			ActionType: wcore.LayoutActionDataType_Unstack,
			BlockId:    data.BlockId,
			Focused:    data.Focused,
		}, nil
	case wshrpc.StackBlockAction_Activate:
		return tabId, waveobj.LayoutActionData{
			ActionType: wcore.LayoutActionDataType_Activate,
			BlockId:    data.BlockId,
		}, nil
	}
	return "", none, fmt.Errorf("unknown stack action %q (stack, unstack or activate)", data.Action)
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
