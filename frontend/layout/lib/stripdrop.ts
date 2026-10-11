// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { tabDropAction } from "./layoutTree";
import { StripTabRect, stripTabAt } from "./stackgeometry";
import type { LayoutNode, LayoutTreeAction } from "./types";

export type StripDropSide = "before" | "after";
export interface StripDropTarget {
    nodeId: string;
    side: StripDropSide;
}

export interface StripHover {
    action: LayoutTreeAction;
    target: StripDropTarget;
}

// What a drag over a strip means at pointer position x: the action a drop would do and the tab to outline, with the side of that tab on
// which the dragged tab will land. Nothing (null) when no drop is possible there (no tabs, or dropping a tab on itself).
//   - a tab of the same stack is moved to the place of the target: it lands AFTER the target when it comes from the left of it, BEFORE when
//     it comes from the right (the others shift over);
//   - a tab from outside joins the stack right after the target.
export function stripHover(
    rootNode: LayoutNode,
    draggedNodeId: string | undefined,
    tabs: StripTabRect[],
    x: number | undefined
): StripHover | null {
    if (draggedNodeId == null || x == null) {
        return null;
    }
    const targetNodeId = stripTabAt(tabs, x);
    if (!targetNodeId) {
        return null;
    }
    const action = tabDropAction(rootNode, draggedNodeId, targetNodeId);
    if (!action) {
        return null;
    }
    let side: StripDropSide = "after";
    const from = tabs.findIndex((t) => t.nodeId === draggedNodeId);
    const to = tabs.findIndex((t) => t.nodeId === targetNodeId);
    if (from >= 0 && to >= 0 && from > to) {
        side = "before";
    }
    return { action, target: { nodeId: targetNodeId, side } };
}

export interface StripHoverSink {
    setPending: (action: LayoutTreeAction) => void;
    clearPending: () => void;
    setTarget: (target: StripDropTarget | null) => void;
}

// One hover over a strip. Either the pending action and the outline are set from the pointer, or (nothing can be dropped here) the pending
// action is CLEARED, so that the "insert above" of the tile body that the pointer left can never stay on screen over a strip.
export function applyStripHover(
    sink: StripHoverSink,
    rootNode: LayoutNode,
    draggedNodeId: string | undefined,
    tabs: StripTabRect[],
    x: number | undefined
): StripHover | null {
    const hover = stripHover(rootNode, draggedNodeId, tabs, x);
    if (hover == null) {
        sink.clearPending();
        return null;
    }
    sink.setPending(hover.action);
    sink.setTarget(hover.target);
    return hover;
}
