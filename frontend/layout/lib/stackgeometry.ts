// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { isStackNode } from "./layoutNode";
import type { LayoutNode, LayoutNodeAdditionalProps, StackLayoutProps } from "./types";
import { setTransform } from "./utils";

export const StackStripHeightPx = 28;

// Only a stack whose members are all leaves is laid out as one. Anything else (a tree from an older build or a
// hand-edited layout) falls back to the normal row and column layout, so no member is ever hidden without a tab.
export function canLayoutAsStack(node: LayoutNode): boolean {
    return isStackNode(node) && node.children?.length > 0 && node.children.every((c) => !c.children && !!c.data);
}

export function stripsToShow(
    additionalPropsMap: Record<string, LayoutNodeAdditionalProps>
): [string, LayoutNodeAdditionalProps][] {
    return Object.entries(additionalPropsMap).filter(([, props]) => props.stack?.order?.length > 0);
}

export interface StackMemberLayout {
    rect: Dimensions;
    hidden: boolean;
}

export interface StackLayout {
    strip: Dimensions;
    activeId: string;
    order: string[];
    members: Record<string, StackMemberLayout>;
}

// Every member gets the same body rectangle, hidden or not: a terminal takes its rows and columns from the
// size of its element, so a hidden member that kept a different size would be resized when its tab is shown.
export function computeStackLayout(
    stack: LayoutNode,
    rect: Dimensions,
    stripHeightPx = StackStripHeightPx
): StackLayout {
    if (!isStackNode(stack)) {
        throw new Error("computeStackLayout needs a stack");
    }
    if (!stack.children?.length) {
        return { strip: { ...rect, height: 0 }, activeId: "", order: [], members: {} };
    }
    const stripHeight = Math.max(0, Math.min(stripHeightPx, rect.height));
    const body: Dimensions = {
        top: rect.top + stripHeight,
        left: rect.left,
        width: rect.width,
        height: Math.max(0, rect.height - stripHeight),
    };
    const order = stack.children.map((c) => c.id);
    const activeId = order.includes(stack.activeChildId) ? stack.activeChildId : order[0];
    const members: Record<string, StackMemberLayout> = {};
    for (const id of order) {
        members[id] = { rect: { ...body }, hidden: id !== activeId };
    }
    return {
        strip: { top: rect.top, left: rect.left, width: rect.width, height: stripHeight },
        activeId,
        order,
        members,
    };
}

// Fills the props map for a stack: its members, and the stack node itself with its strip. The stack must
// already have an entry in the map (its own rectangle comes from its parent).
export function applyStackLayout(
    stack: LayoutNode,
    rect: Dimensions,
    additionalPropsMap: Record<string, LayoutNodeAdditionalProps>
) {
    const layout = computeStackLayout(stack, rect);
    const own = additionalPropsMap[stack.id] ?? { treeKey: "0" };
    stack.children.forEach((child, i) => {
        const member = layout.members[child.id];
        additionalPropsMap[child.id] = {
            rect: member.rect,
            transform: setTransform(member.rect),
            treeKey: own.treeKey + i,
            isHidden: member.hidden,
            stackSize: layout.order.length,
        };
    });
    additionalPropsMap[stack.id] = {
        ...own,
        resizeHandles: [],
        stack: { strip: layout.strip, activeId: layout.activeId, order: layout.order },
    };
}

export interface StripTab {
    nodeId: string;
    blockId: string;
    active: boolean;
    focused: boolean;
}

export function buildStripTabs(stack: StackLayoutProps, leafs: LayoutNode[], focusedNodeId: string): StripTab[] {
    const tabs: StripTab[] = [];
    for (const id of stack.order) {
        const leaf = leafs.find((l) => l.id === id);
        if (!leaf?.data?.blockId) {
            continue;
        }
        tabs.push({
            nodeId: id,
            blockId: leaf.data.blockId,
            active: id === stack.activeId,
            focused: id === focusedNodeId,
        });
    }
    return tabs;
}

export interface StripTabRect {
    nodeId: string;
    left: number;
    right: number;
}

// Which tab of a strip a drag pointer is over, by its horizontal position (the strip is one row, so only x matters). Between two
// tabs the earlier one wins; left of the first tab it is the first; right of the last one (the empty end of the strip, or
// the + button) it is the last, which means "at the end". Nothing for a strip without tabs. Never "above the tile": while the
// pointer is over a strip the answer is always one of its tabs.
export function stripTabAt(tabs: StripTabRect[], x: number): string | undefined {
    if (!tabs?.length) {
        return undefined;
    }
    for (const tab of tabs) {
        if (x >= tab.left && x < tab.right) {
            return tab.nodeId;
        }
    }
    if (x < tabs[0].left) {
        return tabs[0].nodeId;
    }
    let before = tabs[0];
    for (const tab of tabs) {
        if (tab.right <= x) {
            before = tab;
        }
    }
    return before.nodeId;
}

// Whether the tab that is outlined for a drop belongs to the stack with these members: a strip clears the outline only when it is
// its own, so that moving from one strip onto another does not wipe the new one.
export function stripTargetBelongsTo(
    stackOrder: string[] | undefined,
    targetNodeId: string | null | undefined
): boolean {
    return targetNodeId != null && !!stackOrder?.includes(targetNodeId);
}
