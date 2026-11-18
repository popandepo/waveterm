// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, test } from "vitest";
import { findParent, newLayoutNode, newStackNode, walkNodes } from "../lib/layoutNode";
import { computeMoveNode, placeholderTargetId, swapAction, swapNode } from "../lib/layoutTree";
import { DropDirection, FlexDirection, LayoutNode, LayoutTreeActionType, LayoutTreeState } from "../lib/types";
import { newLayoutTreeState } from "./model";

function leaf(blockId: string): LayoutNode {
    return newLayoutNode(FlexDirection.Column, undefined, undefined, { blockId });
}

function order(root: LayoutNode): string[] {
    const ids: string[] = [];
    walkNodes(root, (n) => {
        if (n.data?.blockId) ids.push(n.data.blockId);
    });
    return ids;
}

function threeTiles(): { state: LayoutTreeState; a: LayoutNode; b: LayoutNode; c: LayoutNode } {
    const a = leaf("a");
    const b = leaf("b");
    const c = leaf("c");
    return { state: newLayoutTreeState(newLayoutNode(FlexDirection.Row, undefined, [a, b, c])), a, b, c };
}

describe("swapAction", () => {
    test("two plain tiles swap as themselves", () => {
        const { state, a, c } = threeTiles();
        const action = swapAction(state.rootNode, a.id, c.id);
        assert.equal(action.type, LayoutTreeActionType.Swap);
        assert.deepEqual([action.node1Id, action.node2Id], [a.id, c.id]);
    });

    test("a member of a stack means its whole stack, on either side", () => {
        const a = leaf("a");
        const m1 = leaf("m1");
        const m2 = leaf("m2");
        const stack = newStackNode([m1, m2]);
        const state = newLayoutTreeState(newLayoutNode(FlexDirection.Row, undefined, [a, stack]));
        const action = swapAction(state.rootNode, a.id, m2.id);
        assert.deepEqual([action.node1Id, action.node2Id], [a.id, stack.id]);
        const reverse = swapAction(state.rootNode, m1.id, a.id);
        assert.deepEqual([reverse.node1Id, reverse.node2Id], [stack.id, a.id]);
    });

    test("nothing to swap: the same tile, two members of one stack, the root, an unknown id", () => {
        const a = leaf("a");
        const m1 = leaf("m1");
        const m2 = leaf("m2");
        const stack = newStackNode([m1, m2]);
        const root = newLayoutNode(FlexDirection.Row, undefined, [a, stack]);
        assert.isUndefined(swapAction(root, a.id, a.id));
        assert.isUndefined(swapAction(root, m1.id, m2.id));
        assert.isUndefined(swapAction(root, root.id, a.id));
        assert.isUndefined(swapAction(root, a.id, "nope"));
    });
});

describe("swapNode through swapAction", () => {
    test("the two tiles change places and the others stay", () => {
        const { state, a, c } = threeTiles();
        swapNode(state, swapAction(state.rootNode, a.id, c.id));
        assert.deepEqual(order(state.rootNode), ["c", "b", "a"]);
    });

    test("a plain tile and a whole stack change places", () => {
        const a = leaf("a");
        const stack = newStackNode([leaf("m1"), leaf("m2")]);
        const state = newLayoutTreeState(newLayoutNode(FlexDirection.Row, undefined, [a, stack]));
        swapNode(state, swapAction(state.rootNode, a.id, stack.children[1].id));
        assert.deepEqual(order(state.rootNode), ["m1", "m2", "a"]);
        assert.equal(findParent(state.rootNode, stack.children[0].id).id, stack.id);
    });
});

describe("the middle of a tile with the swap modifier", () => {
    const centre = (state: LayoutTreeState, nodeId: string, nodeToMoveId: string, swap?: boolean) =>
        computeMoveNode(state, {
            type: LayoutTreeActionType.ComputeMove,
            nodeId,
            nodeToMoveId,
            direction: DropDirection.Center,
            swap,
        } as any);

    test("swaps with the tile under the pointer", () => {
        const { state, a, c } = threeTiles();
        const action: any = centre(state, c.id, a.id, true);
        assert.equal(action.type, LayoutTreeActionType.Swap);
        assert.deepEqual(
            [action.node1Id, action.node2Id],
            [c.id, a.id],
            "node1 is the tile under the pointer: the outline is drawn over it"
        );
        assert.equal(placeholderTargetId(action), c.id);
    });

    test("without the modifier the middle still stacks", () => {
        const { state, a, c } = threeTiles();
        const action: any = centre(state, c.id, a.id, false);
        assert.equal(action.type, LayoutTreeActionType.Stack);
    });

    test("the modifier on an edge zone still moves", () => {
        const { state, a, c } = threeTiles();
        const action: any = computeMoveNode(state, {
            type: LayoutTreeActionType.ComputeMove,
            nodeId: c.id,
            nodeToMoveId: a.id,
            direction: DropDirection.Right,
            swap: true,
        } as any);
        assert.equal(action?.type, LayoutTreeActionType.Move);
    });
});
