// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { isStackNode } from "./layoutNode";
import type { LayoutNode, LayoutNodeAdditionalProps } from "./types";
import { setTransform } from "./utils";

export const StackStripHeightPx = 28;

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
    if (!isStackNode(stack) || !stack.children?.length) {
        throw new Error("computeStackLayout needs a stack with members");
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
        };
    });
    additionalPropsMap[stack.id] = {
        ...own,
        resizeHandles: [],
        stack: { strip: layout.strip, activeId: layout.activeId, order: layout.order },
    };
}
