// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, expect, test } from "vitest";
import {
    addChildAt,
    addStackMember,
    balanceNode,
    findNextInsertLocation,
    findNode,
    findParent,
    isStackNode,
    newLayoutNode,
    newStackNode,
    validateNode,
} from "../lib/layoutNode";
import {
    deleteNode,
    focusNode,
    insertNode,
    placementTargetId,
    setFocusedNode,
    splitHorizontal,
    stackNodes,
    unstackNode,
} from "../lib/layoutTree";
import { FlexDirection, LayoutNode, LayoutTreeActionType, LayoutTreeState } from "../lib/types";
import { newLayoutTreeState } from "./model";

function leaf(blockId: string, dir = FlexDirection.Column): LayoutNode {
    return newLayoutNode(dir, undefined, undefined, { blockId });
}

function treeWithThreeLeaves(): { state: LayoutTreeState; a: LayoutNode; b: LayoutNode; c: LayoutNode } {
    const a = leaf("a");
    const b = leaf("b");
    const c = leaf("c");
    const root = newLayoutNode(FlexDirection.Row, undefined, [a, b, c]);
    return { state: newLayoutTreeState(root), a, b, c };
}

describe("stack node basics", () => {
    test("a stack needs leaves and names an active member", () => {
        const a = leaf("a");
        const b = leaf("b");
        const stack = newStackNode([a, b], b.id);
        assert.isTrue(isStackNode(stack));
        assert.equal(stack.activeChildId, b.id);
        assert.isTrue(validateNode(stack));
        assert.throws(() => newStackNode([]), "A stack needs");
        assert.throws(() => newStackNode([newLayoutNode(FlexDirection.Row, undefined, [leaf("x")])]), "A stack needs");
    });

    test("an unknown active member falls back to the first", () => {
        const a = leaf("a");
        const stack = newStackNode([a, leaf("b")], "nope");
        assert.equal(stack.activeChildId, a.id);
    });

    test("a stack with a non-leaf child is invalid", () => {
        const stack = newStackNode([leaf("a"), leaf("b")]);
        stack.children![0] = newLayoutNode(FlexDirection.Row, undefined, [leaf("x")]);
        assert.isFalse(validateNode(stack));
    });

    test("layout JSON round trip keeps kind and activeChildId", () => {
        const stack = newStackNode([leaf("a"), leaf("b")]);
        const copy = JSON.parse(JSON.stringify(stack)) as LayoutNode;
        assert.equal(copy.kind, "stack");
        assert.equal(copy.activeChildId, stack.activeChildId);
    });

    test("old layouts without a kind are unaffected", () => {
        const { state } = treeWithThreeLeaves();
        assert.isFalse(isStackNode(state.rootNode));
        balanceNode(state.rootNode);
        assert.isUndefined(state.rootNode.kind);
    });
});

describe("the tree rewriters leave stacks alone", () => {
    test("balanceNode does not dissolve a stack wrapped in a single-child node", () => {
        const stack = newStackNode([leaf("a"), leaf("b")]);
        const wrapper = newLayoutNode(FlexDirection.Column, undefined, [stack]);
        const root = newLayoutNode(FlexDirection.Row, undefined, [wrapper, leaf("c")]);
        balanceNode(root);
        const found = root.children!.find((c) => isStackNode(c));
        assert.isDefined(found, "the stack must survive");
        assert.equal(found!.children!.length, 2);
    });

    test("balanceNode turns a one-member stack back into a plain leaf", () => {
        const a = leaf("a");
        const stack = newStackNode([a, leaf("b")]);
        const root = newLayoutNode(FlexDirection.Row, undefined, [stack, leaf("c")]);
        stack.children!.pop();
        balanceNode(root);
        const first = root.children![0];
        assert.isUndefined(first.kind);
        assert.isUndefined(first.activeChildId);
        assert.equal(first.data!.blockId, "a");
    });

    test("balanceNode repairs an active member that no longer exists", () => {
        const a = leaf("a");
        const stack = newStackNode([a, leaf("b")]);
        stack.activeChildId = "gone";
        const root = newLayoutNode(FlexDirection.Row, undefined, [stack, leaf("c")]);
        balanceNode(root);
        assert.equal(findNode(root, stack.id)!.activeChildId, a.id);
    });

    test("adding next to a stack places it beside the group, not inside", () => {
        const stack = newStackNode([leaf("a"), leaf("b")]);
        const root = newLayoutNode(FlexDirection.Row, undefined, [stack, leaf("c")]);
        addChildAt(stack, 1, leaf("n"));
        balanceNode(root);
        const stacks = [] as LayoutNode[];
        const walk = (n: LayoutNode) => {
            if (isStackNode(n)) stacks.push(n);
            n.children?.forEach(walk);
        };
        walk(root);
        assert.equal(stacks.length, 1);
        assert.equal(stacks[0].children!.length, 2, "the stack still has exactly its two members");
    });

    test("findNextInsertLocation treats a stack as a leaf", () => {
        const stack = newStackNode([leaf("a"), leaf("b")]);
        const loc = findNextInsertLocation(stack);
        assert.equal(loc.node, stack);
        assert.equal(loc.index, 1);
    });

    test("insertNode never adds a new block as a member of an existing stack", () => {
        const stack = newStackNode([leaf("a"), leaf("b")]);
        const state = newLayoutTreeState(stack);
        insertNode(state, { type: LayoutTreeActionType.InsertNode, node: leaf("n"), magnified: false, focused: true });
        balanceNode(state.rootNode);
        const stacks: LayoutNode[] = [];
        const walk = (n: LayoutNode) => {
            if (isStackNode(n)) stacks.push(n);
            n.children?.forEach(walk);
        };
        walk(state.rootNode);
        assert.equal(stacks.length, 1);
        assert.equal(stacks[0].children!.length, 2);
        assert.isDefined(findNode(state.rootNode, state.focusedNodeId!));
    });
});

describe("stackNodes and unstackNode", () => {
    test("two leaves become a stack in place, the moved one active and focused", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        balanceNode(state.rootNode);
        const stack = state.rootNode.children![0];
        assert.isTrue(isStackNode(stack));
        assert.deepEqual(
            stack.children!.map((x) => x.data!.blockId),
            ["a", "b"]
        );
        assert.equal(stack.activeChildId, b.id);
        assert.equal(state.focusedNodeId, b.id);
        assert.equal(state.rootNode.children!.length, 2, "root now holds the stack and c");
        assert.equal(state.rootNode.children![1].id, c.id);
    });

    test("a third leaf joins the existing stack after the target", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: c.id });
        balanceNode(state.rootNode);
        const root = state.rootNode;
        assert.isTrue(isStackNode(root), "everything is in one stack, which collapses to the root");
        assert.deepEqual(
            root.children!.map((x) => x.data!.blockId),
            ["a", "c", "b"]
        );
        assert.equal(root.activeChildId, c.id);
    });

    test("stacking onto the stack node itself works the same", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        const stack = findParent(state.rootNode, b.id)!;
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: stack.id, nodeId: c.id });
        assert.equal(stack.children!.length, 3);
    });

    test("refuses to stack a non-leaf or the same node", () => {
        const { state, a } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: a.id });
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: state.rootNode.id });
        assert.equal(state.rootNode.children!.length, 3);
        assert.isFalse(state.rootNode.children!.some((n) => isStackNode(n)));
    });

    test("unstack puts the member next to the stack and focuses it", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: b.id });
        balanceNode(state.rootNode);
        assert.deepEqual(
            state.rootNode.children!.map((x) => x.data?.blockId),
            ["a", "b", "c"],
            "the one-member stack collapsed back to a plain leaf"
        );
        assert.isFalse(state.rootNode.children!.some((n) => isStackNode(n)));
        assert.equal(state.focusedNodeId, b.id);
    });

    test("unstack works when the stack is the root", () => {
        const a = leaf("a");
        const b = leaf("b");
        const state = newLayoutTreeState(newStackNode([a, b]));
        unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: b.id });
        balanceNode(state.rootNode);
        assert.equal(state.rootNode.children!.length, 2);
        assert.equal(state.focusedNodeId, b.id);
    });
});

describe("focus brings a hidden member to the front", () => {
    function stackedState() {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        return { state, a, b, c, stack: findParent(state.rootNode, a.id)! };
    }

    test("focusNode on a hidden member makes it the active one", () => {
        const { state, a, b, stack } = stackedState();
        assert.equal(stack.activeChildId, b.id);
        focusNode(state, { type: LayoutTreeActionType.FocusNode, nodeId: a.id });
        assert.equal(stack.activeChildId, a.id);
        assert.equal(state.focusedNodeId, a.id);
    });

    test("setFocusedNode is what every focus path uses", () => {
        const { state, a, stack } = stackedState();
        setFocusedNode(state, a.id);
        assert.equal(stack.activeChildId, a.id);
    });

    test("deleting the active member activates its neighbour", () => {
        const { state, a, b, stack } = stackedState();
        deleteNode(state, { type: LayoutTreeActionType.DeleteNode, nodeId: b.id });
        assert.equal(stack.activeChildId, a.id);
        assert.isUndefined(state.focusedNodeId === b.id ? state.focusedNodeId : undefined);
    });
});

describe("placement targets", () => {
    test("a member as a drop or split target means the stack", () => {
        const { state, a, b } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        const stack = findParent(state.rootNode, a.id)!;
        assert.equal(placementTargetId(state.rootNode, a.id), stack.id);
        assert.equal(placementTargetId(state.rootNode, b.id), stack.id);
        assert.equal(placementTargetId(state.rootNode, state.rootNode.children![1].id), state.rootNode.children![1].id);
    });

    test("splitHorizontal on a member splits beside the group and never splices into it", () => {
        const { state, a, b } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        const stack = findParent(state.rootNode, a.id)!;
        splitHorizontal(state, {
            type: LayoutTreeActionType.SplitHorizontal,
            targetNodeId: a.id,
            newNode: leaf("n"),
            position: "after",
        });
        assert.equal(stack.children!.length, 2, "the stack kept exactly its members");
        assert.isTrue(state.rootNode.children!.some((n) => n.data?.blockId === "n"));
    });
});

test("addStackMember only accepts leaves", () => {
    const stack = newStackNode([leaf("a"), leaf("b")]);
    expect(() => addStackMember(stack, 0, newLayoutNode(FlexDirection.Row, undefined, [leaf("x")]))).toThrow();
    expect(() => addStackMember(leaf("p"), 0, leaf("q"))).toThrow();
    addStackMember(stack, 1, leaf("z"), true);
    assert.equal(stack.children![1].data!.blockId, "z");
    assert.equal(stack.activeChildId, stack.children![1].id);
});
