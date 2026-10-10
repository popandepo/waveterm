// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { newLayoutNode, newStackNode } from "../lib/layoutNode";
import { applyStackLayout, buildStripTabs, computeStackLayout, StackStripHeightPx } from "../lib/stackgeometry";
import { FlexDirection, type LayoutNodeAdditionalProps } from "../lib/types";

function leaf(blockId: string) {
    return newLayoutNode(FlexDirection.Row, undefined, undefined, { blockId });
}

const rect = { top: 100, left: 50, width: 800, height: 600 };

describe("stack geometry", () => {
    test("every member gets the same body rectangle below the strip", () => {
        const [a, b, c] = [leaf("a"), leaf("b"), leaf("c")];
        const stack = newStackNode([a, b, c], b.id);
        const out = computeStackLayout(stack, rect);
        const body = { top: 100 + StackStripHeightPx, left: 50, width: 800, height: 600 - StackStripHeightPx };
        expect(out.strip).toEqual({ top: 100, left: 50, width: 800, height: StackStripHeightPx });
        for (const id of [a.id, b.id, c.id]) {
            expect(out.members[id].rect).toEqual(body);
        }
    });

    test("exactly one member is visible: the active one", () => {
        const [a, b, c] = [leaf("a"), leaf("b"), leaf("c")];
        const stack = newStackNode([a, b, c], c.id);
        const out = computeStackLayout(stack, rect);
        expect(out.members[a.id].hidden).toBe(true);
        expect(out.members[b.id].hidden).toBe(true);
        expect(out.members[c.id].hidden).toBe(false);
    });

    test("an unknown active member falls back to the first one, never to none", () => {
        const [a, b] = [leaf("a"), leaf("b")];
        const stack = newStackNode([a, b], a.id);
        stack.activeChildId = "gone";
        const out = computeStackLayout(stack, rect);
        expect(out.activeId).toBe(a.id);
        expect(out.members[a.id].hidden).toBe(false);
        expect(out.members[b.id].hidden).toBe(true);
    });

    test("hidden members keep the visible size so a terminal is not resized when its tab is hidden", () => {
        const [a, b] = [leaf("a"), leaf("b")];
        const stack = newStackNode([a, b], a.id);
        const first = computeStackLayout(stack, rect);
        stack.activeChildId = b.id;
        const second = computeStackLayout(stack, rect);
        expect(second.members[a.id].rect).toEqual(first.members[a.id].rect);
        expect(second.members[b.id].rect).toEqual(first.members[b.id].rect);
    });

    test("a rectangle shorter than the strip gives a strip that fits and a body of zero, never negative", () => {
        const [a] = [leaf("a")];
        const stack = newStackNode([a], a.id);
        const out = computeStackLayout(stack, { top: 0, left: 0, width: 300, height: 10 });
        expect(out.strip.height).toBe(10);
        expect(out.members[a.id].rect.height).toBe(0);
    });

    test("the order of the members is the order of the children", () => {
        const [a, b, c] = [leaf("a"), leaf("b"), leaf("c")];
        const stack = newStackNode([a, b, c], a.id);
        expect(computeStackLayout(stack, rect).order).toEqual([a.id, b.id, c.id]);
    });

    test("a node that is not a stack is refused", () => {
        expect(() => computeStackLayout(leaf("a"), rect)).toThrow();
    });
});

describe("applying a stack layout to the props map", () => {
    test("members get rect, transform and hidden flag, the stack gets its strip and no resize handles", () => {
        const [a, b] = [leaf("a"), leaf("b")];
        const stack = newStackNode([a, b], b.id);
        const map: Record<string, LayoutNodeAdditionalProps> = { [stack.id]: { treeKey: "0", rect } };
        applyStackLayout(stack, rect, map);
        expect(map[a.id].isHidden).toBe(true);
        expect(map[b.id].isHidden).toBe(false);
        expect(map[a.id].rect).toEqual(map[b.id].rect);
        expect(map[b.id].transform.width).toBe(`${rect.width}px`);
        expect(map[b.id].transform.height).toBe(`${rect.height - StackStripHeightPx}px`);
        expect(map[stack.id].resizeHandles).toEqual([]);
        expect(map[stack.id].stack.activeId).toBe(b.id);
        expect(map[stack.id].stack.order).toEqual([a.id, b.id]);
        expect(map[stack.id].stack.strip.height).toBe(StackStripHeightPx);
        expect(map[stack.id].treeKey).toBe("0");
    });

    test("tree keys of members follow their position so a switch does not remount them", () => {
        const [a, b] = [leaf("a"), leaf("b")];
        const stack = newStackNode([a, b], a.id);
        const map: Record<string, LayoutNodeAdditionalProps> = { [stack.id]: { treeKey: "01", rect } };
        applyStackLayout(stack, rect, map);
        expect(map[a.id].treeKey).toBe("010");
        expect(map[b.id].treeKey).toBe("011");
        stack.activeChildId = b.id;
        applyStackLayout(stack, rect, map);
        expect(map[a.id].treeKey).toBe("010");
        expect(map[b.id].treeKey).toBe("011");
    });
});

describe("tab strip model", () => {
    test("one tab per member in order with the block id, the active one marked", () => {
        const [a, b, c] = [leaf("blk-a"), leaf("blk-b"), leaf("blk-c")];
        const stack = newStackNode([a, b, c], b.id);
        const map: Record<string, LayoutNodeAdditionalProps> = { [stack.id]: { treeKey: "0", rect } };
        applyStackLayout(stack, rect, map);
        const tabs = buildStripTabs(map[stack.id].stack, [c, a, b], null);
        expect(tabs.map((t) => t.blockId)).toEqual(["blk-a", "blk-b", "blk-c"]);
        expect(tabs.map((t) => t.active)).toEqual([false, true, false]);
        expect(tabs.map((t) => t.nodeId)).toEqual([a.id, b.id, c.id]);
    });

    test("the focused member is marked even when it is not the active one being shown first", () => {
        const [a, b] = [leaf("blk-a"), leaf("blk-b")];
        const stack = newStackNode([a, b], a.id);
        const map: Record<string, LayoutNodeAdditionalProps> = { [stack.id]: { treeKey: "0", rect } };
        applyStackLayout(stack, rect, map);
        const tabs = buildStripTabs(map[stack.id].stack, [a, b], b.id);
        expect(tabs.map((t) => t.focused)).toEqual([false, true]);
    });

    test("a member that is missing from the leaf list is left out, not shown as an empty tab", () => {
        const [a, b] = [leaf("blk-a"), leaf("blk-b")];
        const stack = newStackNode([a, b], a.id);
        const map: Record<string, LayoutNodeAdditionalProps> = { [stack.id]: { treeKey: "0", rect } };
        applyStackLayout(stack, rect, map);
        expect(buildStripTabs(map[stack.id].stack, [a], null).map((t) => t.blockId)).toEqual(["blk-a"]);
    });
});
