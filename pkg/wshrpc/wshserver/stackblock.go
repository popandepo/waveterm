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
// the same tab; a "move" between tabs is handled before this by moveAcrossTabs.
func planStackBlock(
	data wshrpc.CommandStackBlockData,
	findTab func(blockId string) (string, error),
	inStack func(tabId string, blockId string) (bool, error),
	sameStack func(tabId string, blockA string, blockB string) (bool, error),
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
	case wshrpc.StackBlockAction_Stack, wshrpc.StackBlockAction_Move:
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
		same, err := sameStack(tabId, data.BlockId, data.TargetBlockId)
		if err != nil {
			return "", none, fmt.Errorf("looking up the groups: %w", err)
		}
		if same {
			return "", none, fmt.Errorf("both blocks are already in the same group")
		}
		actionType := wcore.LayoutActionDataType_Stack
		if data.Action == wshrpc.StackBlockAction_Move {
			actionType = wcore.LayoutActionDataType_StackMove
		}
		return tabId, waveobj.LayoutActionData{
			ActionType:    actionType,
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

// createTargetNeedsSameTab: a new block that goes into a group, or replaces a block, acts on a target block that must be in the tab the block
// is created in. The server decides this at the moment of the request, so it holds whatever the caller looked up earlier.
func createTargetNeedsSameTab(action string, targetBlockId string) bool {
	if action == wshrpc.CreateBlockAction_Stack {
		return true
	}
	return action == wshrpc.CreateBlockAction_Replace && targetBlockId != ""
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

// stackIdOf reads the stored layout tree (decoded JSON) and returns the id of the stack the block is a member of,
// or "" for a block that is not in a stack. The page saves the tree 100 ms after a change, so this can lag a very
// recent change by that much.
func stackIdOf(node any, blockId string) string {
	m, ok := node.(map[string]any)
	if !ok {
		return ""
	}
	children, _ := m["children"].([]any)
	isStack := m["kind"] == "stack"
	for _, c := range children {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if data, ok := cm["data"].(map[string]any); ok && isStack && data["blockId"] == blockId {
			id, _ := m["id"].(string)
			return id
		}
		if id := stackIdOf(c, blockId); id != "" {
			return id
		}
	}
	return ""
}

// stackMembersOf returns the block ids of the stack that holds blockId, in order, or just blockId when it is not in one.
func stackMembersOf(node any, blockId string) []string {
	stackId := stackIdOf(node, blockId)
	if stackId == "" {
		return []string{blockId}
	}
	var members []string
	var walk func(n any)
	walk = func(n any) {
		m, ok := n.(map[string]any)
		if !ok {
			return
		}
		children, _ := m["children"].([]any)
		if m["id"] == stackId {
			for _, c := range children {
				if cm, ok := c.(map[string]any); ok {
					if data, ok := cm["data"].(map[string]any); ok {
						if id, ok := data["blockId"].(string); ok {
							members = append(members, id)
						}
					}
				}
			}
			return
		}
		for _, c := range children {
			walk(c)
		}
	}
	walk(node)
	if len(members) == 0 {
		return []string{blockId}
	}
	return members
}

func blockInStack(node any, blockId string) bool {
	return stackIdOf(node, blockId) != ""
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

// moveAcrossTabs handles a stack move whose two blocks are in different tabs: the block, or every member of its group,
// joins the group at the target in the target tab. Processes keep running because only the layout and the tab lists change.
func moveAcrossTabs(ctx context.Context, data wshrpc.CommandStackBlockData, fromTab string, toTab string) error {
	layoutId, err := wcore.GetLayoutIdForTab(ctx, fromTab)
	if err != nil {
		return err
	}
	layoutObj, err := wstore.DBGet[*waveobj.LayoutState](ctx, layoutId)
	if err != nil {
		return err
	}
	members := stackMembersOf(layoutObj.RootNode, data.BlockId)
	return wcore.MoveBlocksToTab(ctx, fromTab, toTab, members, data.TargetBlockId, data.Focused)
}

func (ws *WshServer) StackBlockCommand(ctx context.Context, data wshrpc.CommandStackBlockData) error {
	ctx = waveobj.ContextWithUpdates(ctx)
	if data.Action == wshrpc.StackBlockAction_Move && data.BlockId != "" && data.TargetBlockId != "" {
		fromTab, fromErr := wstore.DBFindTabForBlockId(ctx, data.BlockId)
		toTab, toErr := wstore.DBFindTabForBlockId(ctx, data.TargetBlockId)
		if fromErr == nil && toErr == nil && fromTab != toTab {
			if err := moveAcrossTabs(ctx, data, fromTab, toTab); err != nil {
				return err
			}
			wps.Broker.SendUpdateEvents(waveobj.ContextGetUpdatesRtn(ctx))
			return nil
		}
	}
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
	}, func(tabId string, blockA string, blockB string) (bool, error) {
		layoutId, err := wcore.GetLayoutIdForTab(ctx, tabId)
		if err != nil {
			return false, err
		}
		layoutObj, err := wstore.DBGet[*waveobj.LayoutState](ctx, layoutId)
		if err != nil {
			return false, err
		}
		a := stackIdOf(layoutObj.RootNode, blockA)
		return a != "" && a == stackIdOf(layoutObj.RootNode, blockB), nil
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
