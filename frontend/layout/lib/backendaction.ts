// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { newLayoutNode } from "./layoutNode";
import {
    LayoutNode,
    LayoutTreeAction,
    LayoutTreeActionType,
    LayoutTreeInsertNodeAction,
    LayoutTreeSplitHorizontalAction,
    LayoutTreeSplitVerticalAction,
} from "./types";

export type BackendActionResult = { action?: LayoutTreeAction; error?: string };

// The tree action for a layout action that the server queued (a block was created): a plain insert or a split. The new tile's weight (nodesize, from wsh
// --size) goes into the new node in each of them; without one the node gets the default weight. Kept apart from the layout model so that it can be tested
// without a window.
export function treeActionForBackendAction(
    action: LayoutActionData,
    nodeByBlockId: (blockId: string) => LayoutNode | undefined
): BackendActionResult | null {
    switch (action.actiontype) {
        case LayoutTreeActionType.InsertNode: {
            const insertNodeAction: LayoutTreeInsertNodeAction = {
                type: LayoutTreeActionType.InsertNode,
                node: newLayoutNode(undefined, action.nodesize, undefined, { blockId: action.blockid }),
                magnified: action.magnified,
                focused: action.focused,
            };
            return { action: insertNodeAction };
        }
        case LayoutTreeActionType.SplitHorizontal:
        case LayoutTreeActionType.SplitVertical: {
            const name = action.actiontype === LayoutTreeActionType.SplitHorizontal ? "SplitHorizontal" : "SplitVertical";
            const targetNode = nodeByBlockId(action.targetblockid);
            if (!targetNode) {
                return { error: `Cannot apply eventbus layout action ${name}, could not find target node with blockId ${action.targetblockid}` };
            }
            if (action.position != "before" && action.position != "after") {
                return { error: `Cannot apply eventbus layout action ${name}, invalid position ${action.position}` };
            }
            const newNode = newLayoutNode(undefined, action.nodesize, undefined, { blockId: action.blockid });
            if (action.actiontype === LayoutTreeActionType.SplitHorizontal) {
                const splitAction: LayoutTreeSplitHorizontalAction = {
                    type: LayoutTreeActionType.SplitHorizontal,
                    targetNodeId: targetNode.id,
                    newNode,
                    position: action.position,
                };
                return { action: splitAction };
            }
            const splitAction: LayoutTreeSplitVerticalAction = {
                type: LayoutTreeActionType.SplitVertical,
                targetNodeId: targetNode.id,
                newNode,
                position: action.position,
            };
            return { action: splitAction };
        }
        default:
            return null;
    }
}
