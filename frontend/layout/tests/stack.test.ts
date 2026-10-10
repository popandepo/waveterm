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
    activateStackMember,
    computeMoveNode,
    deleteNode,
    focusNode,
    insertNode,
    insertNodeIntoStack,
    placementTargetId,
    setFocusedNode,
    splitHorizontal,
    stackNodes,
    turnIntoStack,
    turnStackOff,
    unstackNode,
} from "../lib/layoutTree";
import { DropDirection, FlexDirection, LayoutNode, LayoutTreeActionType, LayoutTreeState } from "../lib/types";
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

    test("balanceNode keeps a one-member stack: stack mode ends only when the user turns it off", () => {
        const a = leaf("a");
        const stack = newStackNode([a, leaf("b")]);
        const root = newLayoutNode(FlexDirection.Row, undefined, [stack, leaf("c")]);
        stack.children!.pop();
        balanceNode(root);
        const first = root.children![0];
        assert.equal(first.kind, "stack");
        assert.equal(first.activeChildId, a.id);
        assert.equal(first.children![0].data!.blockId, "a");
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
            state.rootNode.children!.map((x) => x.data?.blockId ?? x.children![0].data!.blockId),
            ["a", "b", "c"],
            "a stays in its stack of one: stack mode stays on until it is turned off"
        );
        assert.isTrue(isStackNode(state.rootNode.children![0]));
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

describe("stacking without taking focus", () => {
    test("stacking onto the focused leaf with focused=false keeps that leaf visible and focused", () => {
        const { state, a, b } = treeWithThreeLeaves();
        state.focusedNodeId = a.id;
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id, focused: false });
        const stack = findParent(state.rootNode, a.id)!;
        assert.isTrue(isStackNode(stack));
        assert.equal(stack.activeChildId, a.id, "the leaf he is looking at stays the visible member");
        assert.equal(state.focusedNodeId, a.id);
    });

    test("stacking two leaves away from the focused one leaves focus alone", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        state.focusedNodeId = c.id;
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id, focused: false });
        assert.equal(state.focusedNodeId, c.id);
    });

    test("moving the focused leaf into a stack with focused=false keeps it active and focused", () => {
        const { state, a, b } = treeWithThreeLeaves();
        state.focusedNodeId = b.id;
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id, focused: false });
        const stack = findParent(state.rootNode, b.id)!;
        assert.equal(stack.activeChildId, b.id);
        assert.equal(state.focusedNodeId, b.id);
    });

    test("joining an existing stack with focused=false does not change its active member", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        setFocusedNode(state, a.id);
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: c.id, focused: false });
        const stack = findParent(state.rootNode, a.id)!;
        assert.equal(stack.children!.length, 3);
        assert.equal(stack.activeChildId, a.id);
        assert.equal(state.focusedNodeId, a.id);
    });

    test("default (no focused field) still activates and focuses the moved leaf", () => {
        const { state, a, b } = treeWithThreeLeaves();
        state.focusedNodeId = a.id;
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        assert.equal(findParent(state.rootNode, b.id)!.activeChildId, b.id);
        assert.equal(state.focusedNodeId, b.id);
    });

    test("unstack with focused=false leaves focus where it was", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        state.focusedNodeId = c.id;
        unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: b.id, focused: false });
        assert.equal(state.focusedNodeId, c.id);
    });
});

describe("stack and unstack report whether they changed the tree", () => {
    test("valid actions return true", () => {
        const { state, a, b } = treeWithThreeLeaves();
        assert.isTrue(stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id }));
        assert.isTrue(unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: b.id }));
    });

    test("invalid stack actions return false and change nothing, so the reducer requests no focus", () => {
        const { state, a } = treeWithThreeLeaves();
        const before = JSON.stringify(state.rootNode);
        const focusBefore = state.focusedNodeId;
        const bad = [
            { targetNodeId: a.id, nodeId: a.id },
            { targetNodeId: a.id, nodeId: "missing" },
            { targetNodeId: "missing", nodeId: a.id },
            { targetNodeId: a.id, nodeId: state.rootNode.id },
            { targetNodeId: "", nodeId: a.id },
        ];
        for (const b of bad) {
            assert.isFalse(stackNodes(state, { type: LayoutTreeActionType.Stack, ...b }), JSON.stringify(b));
        }
        assert.equal(JSON.stringify(state.rootNode), before);
        assert.equal(state.focusedNodeId, focusBefore);
    });

    test("invalid unstack actions return false and change nothing", () => {
        const { state, a } = treeWithThreeLeaves();
        const before = JSON.stringify(state.rootNode);
        assert.isFalse(
            unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: a.id }),
            "a leaf in a plain row"
        );
        assert.isFalse(unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: "missing" }));
        assert.equal(JSON.stringify(state.rootNode), before);
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

describe("inserting a new leaf straight into a stack", () => {
    test("a new leaf next to a plain leaf makes a stack of the two, the new one active when focused", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        const fresh = leaf("fresh");
        assert.isTrue(
            insertNodeIntoStack(state, {
                type: LayoutTreeActionType.StackInsert,
                node: fresh,
                targetNodeId: b.id,
            } as any)
        );
        const stack = findParent(state.rootNode, b.id);
        assert.isTrue(isStackNode(stack));
        assert.deepEqual(
            stack.children.map((n) => n.data.blockId),
            ["b", "fresh"]
        );
        assert.equal(stack.activeChildId, fresh.id);
        assert.equal(state.focusedNodeId, fresh.id);
        assert.equal(state.rootNode.children.length, 3, "the stack replaces b, nothing else moves");
        assert.equal(state.rootNode.children[0].id, a.id);
        assert.equal(state.rootNode.children[2].id, c.id);
    });

    test("a new leaf next to a member of an existing stack joins that stack after it", () => {
        const { state, a, b } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, nodeId: b.id, targetNodeId: a.id } as any);
        const stack = findParent(state.rootNode, a.id);
        const fresh = leaf("fresh");
        assert.isTrue(
            insertNodeIntoStack(state, {
                type: LayoutTreeActionType.StackInsert,
                node: fresh,
                targetNodeId: a.id,
                focused: false,
            } as any)
        );
        assert.deepEqual(
            stack.children.map((n) => n.data.blockId),
            ["a", "fresh", "b"]
        );
    });

    test("without focus the active member and the focus stay where they were", () => {
        const { state, a, b } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, nodeId: b.id, targetNodeId: a.id } as any);
        const stack = findParent(state.rootNode, a.id);
        const activeBefore = stack.activeChildId;
        const focusBefore = state.focusedNodeId;
        const fresh = leaf("fresh");
        insertNodeIntoStack(state, {
            type: LayoutTreeActionType.StackInsert,
            node: fresh,
            targetNodeId: a.id,
            focused: false,
        } as any);
        assert.equal(stack.activeChildId, activeBefore);
        assert.equal(state.focusedNodeId, focusBefore);
    });

    test("an unknown target or a target that is not a leaf changes nothing and returns false", () => {
        const { state } = treeWithThreeLeaves();
        const before = JSON.stringify(state.rootNode);
        assert.isFalse(
            insertNodeIntoStack(state, {
                type: LayoutTreeActionType.StackInsert,
                node: leaf("x"),
                targetNodeId: "nope",
            } as any)
        );
        assert.isFalse(
            insertNodeIntoStack(state, {
                type: LayoutTreeActionType.StackInsert,
                node: leaf("x"),
                targetNodeId: state.rootNode.id,
            } as any)
        );
        assert.equal(JSON.stringify(state.rootNode), before);
    });

    test("the new node must be a leaf", () => {
        const { state, b } = treeWithThreeLeaves();
        const notLeaf = newLayoutNode(FlexDirection.Row, undefined, [leaf("p"), leaf("q")]);
        assert.isFalse(
            insertNodeIntoStack(state, {
                type: LayoutTreeActionType.StackInsert,
                node: notLeaf,
                targetNodeId: b.id,
            } as any)
        );
    });
});

describe("activating a member without taking focus", () => {
    test("another stack member is shown and the focus is left alone when it was elsewhere", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, {
            type: LayoutTreeActionType.Stack,
            nodeId: b.id,
            targetNodeId: a.id,
            focused: false,
        } as any);
        setFocusedNode(state, c.id);
        assert.isTrue(activateStackMember(state, b.id));
        assert.equal(findParent(state.rootNode, a.id).activeChildId, b.id);
        assert.equal(state.focusedNodeId, c.id);
    });

    test("focus follows when the focused member of that stack would be hidden", () => {
        const { state, a, b } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, nodeId: b.id, targetNodeId: a.id } as any);
        setFocusedNode(state, a.id);
        assert.isTrue(activateStackMember(state, b.id));
        assert.equal(findParent(state.rootNode, a.id).activeChildId, b.id);
        assert.equal(state.focusedNodeId, b.id, "keystrokes must never go to a hidden terminal");
    });

    test("a leaf that is not in a stack or an unknown node returns false and changes nothing", () => {
        const { state, a } = treeWithThreeLeaves();
        const before = JSON.stringify(state);
        assert.isFalse(activateStackMember(state, a.id));
        assert.isFalse(activateStackMember(state, "nope"));
        assert.equal(JSON.stringify(state), before);
    });
});

describe("stack mode on a single tile", () => {
    test("turning a tile into a stack keeps its place and size and makes a one-member stack", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        b.size = 33;
        assert.isTrue(turnIntoStack(state, b.id));
        const stack = state.rootNode.children![1];
        assert.isTrue(isStackNode(stack));
        assert.deepEqual(
            stack.children!.map((x) => x.id),
            [b.id]
        );
        assert.equal(stack.activeChildId, b.id);
        assert.equal(stack.size, 33);
        assert.equal(state.rootNode.children![0].id, a.id);
        assert.equal(state.rootNode.children![2].id, c.id);
    });

    test("a one-member stack survives balancing: stack mode stays on until it is turned off", () => {
        const { state, b } = treeWithThreeLeaves();
        turnIntoStack(state, b.id);
        balanceNode(state.rootNode);
        const stack = findParent(state.rootNode, b.id)!;
        assert.isTrue(isStackNode(stack));
        assert.equal(stack.children!.length, 1);
    });

    test("closing the second to last member leaves a one-member stack, closing the last removes it", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        deleteNode(state, { type: LayoutTreeActionType.DeleteNode, nodeId: b.id });
        balanceNode(state.rootNode);
        const stack = findParent(state.rootNode, a.id)!;
        assert.isTrue(isStackNode(stack));
        assert.equal(stack.activeChildId, a.id);
        deleteNode(state, { type: LayoutTreeActionType.DeleteNode, nodeId: a.id });
        balanceNode(state.rootNode);
        assert.isNull(findNode(state.rootNode, a.id) ?? null);
        assert.equal(state.rootNode.data!.blockId, "c", "only c is left, so it is the root leaf and no stack remains");
    });

    test("turning stack mode off works only with one member left and gives the plain tile back", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        turnIntoStack(state, b.id);
        assert.isTrue(turnStackOff(state, b.id));
        balanceNode(state.rootNode);
        assert.isFalse(state.rootNode.children!.some((n) => isStackNode(n)));
        assert.deepEqual(
            state.rootNode.children!.map((x) => x.id),
            [a.id, b.id, c.id]
        );
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        assert.isFalse(turnStackOff(state, a.id), "two members: nothing is turned off");
        assert.isTrue(isStackNode(findParent(state.rootNode, a.id)!));
    });

    test("a tile that is already a member, a non-leaf and an unknown id are refused", () => {
        const { state, a, b } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        assert.isFalse(turnIntoStack(state, a.id));
        assert.isFalse(turnIntoStack(state, state.rootNode.id));
        assert.isFalse(turnIntoStack(state, "nope"));
        assert.isFalse(turnStackOff(state, "nope"));
        assert.isFalse(turnStackOff(state, state.rootNode.id));
    });

    test("a root leaf can be turned into a stack and back", () => {
        const only = leaf("only");
        const state = newLayoutTreeState(only);
        assert.isTrue(turnIntoStack(state, only.id));
        assert.isTrue(isStackNode(state.rootNode));
        assert.isTrue(turnStackOff(state, only.id));
        assert.isFalse(isStackNode(state.rootNode));
        assert.equal(state.rootNode.data!.blockId, "only");
    });
});

describe("dropping a tile on the middle of another makes a stack", () => {
    test("the centre zone computes a stack action, not a swap", () => {
        const { state, a, b } = treeWithThreeLeaves();
        const action = computeMoveNode(state, {
            type: LayoutTreeActionType.ComputeMove,
            nodeId: a.id,
            nodeToMoveId: b.id,
            direction: DropDirection.Center,
        });
        assert.equal(action?.type, LayoutTreeActionType.Stack);
        assert.equal((action as any).nodeId, b.id);
        assert.equal((action as any).targetNodeId, a.id);
    });

    test("a drop on a member means its stack, so the dragged tile joins the stack", () => {
        const { state, a, b, c } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        const stack = findParent(state.rootNode, a.id)!;
        const action = computeMoveNode(state, {
            type: LayoutTreeActionType.ComputeMove,
            nodeId: b.id,
            nodeToMoveId: c.id,
            direction: DropDirection.Center,
        });
        assert.equal((action as any).targetNodeId, stack.id);
        stackNodes(state, action as any);
        assert.equal(stack.children!.length, 3);
    });

    test("dropping the root or onto itself is refused and the edge zones still move tiles", () => {
        const { state, a, b } = treeWithThreeLeaves();
        const rootDrop = computeMoveNode(state, {
            type: LayoutTreeActionType.ComputeMove,
            nodeId: state.rootNode.id,
            nodeToMoveId: a.id,
            direction: DropDirection.Center,
        });
        assert.notEqual(rootDrop?.type, LayoutTreeActionType.Stack, "the root is never stacked with");
        const edge = computeMoveNode(state, {
            type: LayoutTreeActionType.ComputeMove,
            nodeId: a.id,
            nodeToMoveId: b.id,
            direction: DropDirection.Left,
        });
        assert.equal(edge?.type, LayoutTreeActionType.Move);

describe("leaving and entering stack mode never replaces the tile object", () => {
    test("take out, turn into a stack and turn off keep the same node (so the view is not remounted)", () => {
        const { state, a, b } = treeWithThreeLeaves();
        const keep = (id: string) => findNode(state.rootNode, id);
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        assert.strictEqual(keep(b.id), b);
        unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: b.id });
        balanceNode(state.rootNode);
        assert.strictEqual(keep(b.id), b, "take out of stack");
        assert.strictEqual(keep(a.id), a);
        turnIntoStack(state, b.id);
        assert.strictEqual(keep(b.id), b, "turn into a stack");
        turnStackOff(state, b.id);
        balanceNode(state.rootNode);
        assert.strictEqual(keep(b.id), b, "turn stack off");
    });

    test("take out of stack leaves focus on the member that was taken out", () => {
        const { state, a, b } = treeWithThreeLeaves();
        stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: a.id, nodeId: b.id });
        unstackNode(state, { type: LayoutTreeActionType.Unstack, nodeId: b.id });
        assert.equal(state.focusedNodeId, b.id);
    });
});
