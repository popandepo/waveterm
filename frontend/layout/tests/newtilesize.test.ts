// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, test } from "vitest";
import { newLayoutNode } from "../lib/layoutNode";
import { insertNode, splitHorizontal, splitVertical } from "../lib/layoutTree";
import { DefaultNodeSize, LayoutTreeActionType } from "../lib/types";
import { newLayoutTreeState } from "./model";

const shareOf = (sizes: number[], i: number) => sizes[i] / sizes.reduce((a, b) => a + b, 0);

test("a tile that is split in next to an untouched tile with weight 7 gets about 41 percent of the width", () => {
    const left = newLayoutNode(undefined, undefined, undefined, { blockId: "left" });
    const state = newLayoutTreeState(left);
    const added = newLayoutNode(undefined, 7, undefined, { blockId: "added" });
    splitHorizontal(state, { type: LayoutTreeActionType.SplitHorizontal, targetNodeId: left.id, newNode: added, position: "after" });
    const children = state.rootNode.children!;
    assert.deepEqual(
        children.map((c) => c.data?.blockId),
        ["left", "added"]
    );
    assert.deepEqual(
        children.map((c) => c.size),
        [DefaultNodeSize, 7]
    );
    const share = shareOf(
        children.map((c) => c.size),
        1
    );
    assert.isAbove(share, 0.4);
    assert.isBelow(share, 0.42);
});

test("without a weight the new tile is an equal half", () => {
    const left = newLayoutNode(undefined, undefined, undefined, { blockId: "left" });
    const state = newLayoutTreeState(left);
    const added = newLayoutNode(undefined, undefined, undefined, { blockId: "added" });
    splitHorizontal(state, { type: LayoutTreeActionType.SplitHorizontal, targetNodeId: left.id, newNode: added, position: "after" });
    const sizes = state.rootNode.children!.map((c) => c.size);
    assert.equal(shareOf(sizes, 1), 0.5);
});

test("a vertical split and a plain insert keep the weight of the new tile too", () => {
    const top = newLayoutNode(undefined, undefined, undefined, { blockId: "top" });
    const vstate = newLayoutTreeState(top);
    const below = newLayoutNode(undefined, 25, undefined, { blockId: "below" });
    splitVertical(vstate, { type: LayoutTreeActionType.SplitVertical, targetNodeId: top.id, newNode: below, position: "after" });
    assert.deepEqual(
        vstate.rootNode.children!.map((c) => c.size),
        [DefaultNodeSize, 25]
    );

    const first = newLayoutNode(undefined, undefined, undefined, { blockId: "first" });
    const istate = newLayoutTreeState(first);
    const second = newLayoutNode(undefined, 4, undefined, { blockId: "second" });
    insertNode(istate, { type: LayoutTreeActionType.InsertNode, node: second });
    assert.deepEqual(
        istate.rootNode.children!.map((c) => c.size),
        [DefaultNodeSize, 4]
    );
});
