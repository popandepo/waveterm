// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { lazy } from "@/util/util";
import {
    addChildAt,
    addIntermediateNode,
    findInsertLocationFromIndexArr,
    findNextInsertLocation,
    findNode,
    findParent,
    removeChild,
} from "./layoutNode";
import {
    DefaultNodeSize,
    DropDirection,
    FlexDirection,
    LayoutNode,
    LayoutTreeAction,
    LayoutTreeActionType,
    LayoutTreeComputeMoveNodeAction,
    LayoutTreeDeleteNodeAction,
    LayoutTreeFocusNodeAction,
    LayoutTreeInsertNodeAction,
    LayoutTreeInsertNodeAtIndexAction,
    LayoutTreeMagnifyNodeToggleAction,
    LayoutTreeMoveNodeAction,
    LayoutTreeResizeNodeAction,
    LayoutTreeState,
    LayoutTreeSwapNodeAction,
    MoveOperation,
} from "./types";
import { reverseFlexDirection } from "./utils";

import { addStackMember, isStackNode, newLayoutNode, newStackNode } from "./layoutNode";
import {
    LayoutTreeReplaceNodeAction,
    LayoutTreeSplitHorizontalAction,
    LayoutTreeSplitVerticalAction,
    LayoutTreeStackAction,
    LayoutTreeStackInsertAction,
    LayoutTreeStackMoveAction,
    LayoutTreeUnstackAction,
} from "./types";

export const DEFAULT_MAX_CHILDREN = 5;

/**
 * Computes an operation for inserting a new node into the tree in the given direction relative to the specified node.
 *
 * @param layoutState The state of the tree.
 * @param computeInsertAction The operation to compute.
 */
/**
 * Focuses a node. If it is a member of a stack, the stack shows it: this is the single place where focus and the
 * active member of a stack are tied together, so every focus path (keyboard, click, wsh focusblock, new block,
 * focus fallback) brings a hidden member to the front.
 */
export function setFocusedNode(layoutState: LayoutTreeState, nodeId: string) {
    layoutState.focusedNodeId = nodeId;
    if (!nodeId || !layoutState.rootNode) {
        return;
    }
    const parent = findParent(layoutState.rootNode, nodeId);
    if (parent && isStackNode(parent)) {
        parent.activeChildId = nodeId;
    }
}

/**
 * Drop and split targets that are members of a stack mean the stack as a whole: a tile is placed next to the group,
 * never spliced into it.
 */
export function placementTargetId(rootNode: LayoutNode, nodeId: string): string {
    const parent = rootNode ? findParent(rootNode, nodeId) : undefined;
    return parent && isStackNode(parent) ? parent.id : nodeId;
}

// After a member leaves a stack, pick the neighbour that was next to it as the active one.
function activateNeighbourAfterRemoval(stack: LayoutNode, removedId: string, removedIndex: number) {
    if (!isStackNode(stack) || stack.activeChildId !== removedId || !stack.children?.length) {
        return;
    }
    stack.activeChildId = stack.children[Math.min(removedIndex, stack.children.length - 1)].id;
}

/**
 * What a drop on the middle of a tile does, or nothing. Nothing when the tile is dropped on itself or on its own
 * stack (no action means no outline). A whole stack that is dropped merges all its members into the target in order;
 * one member (or a plain tile) is stacked with the target.
 */
function centreDropAction(rootNode: LayoutNode, targetId: string, movedId: string): LayoutTreeAction | undefined {
    const moved = findNode(rootNode, movedId);
    if (!moved || targetId === movedId) {
        return undefined;
    }
    const movedParent = findParent(rootNode, movedId);
    const movedStack = isStackNode(moved) ? moved : movedParent && isStackNode(movedParent) ? movedParent : null;
    if (movedStack && (targetId === movedStack.id || movedStack.children?.some((c) => c.id === targetId))) {
        return undefined;
    }
    if (isStackNode(moved)) {
        const mergeAction: LayoutTreeStackMoveAction = {
            type: LayoutTreeActionType.StackMove,
            fromNodeId: movedId,
            toNodeId: targetId,
        };
        return mergeAction;
    }
    const stackAction: LayoutTreeStackAction = {
        type: LayoutTreeActionType.Stack,
        nodeId: movedId,
        targetNodeId: targetId,
    };
    return stackAction;
}

/**
 * The tile that the outline of a pending drop is drawn over.
 */
export function placeholderTargetId(action: LayoutTreeAction): string | undefined {
    switch (action?.type) {
        case LayoutTreeActionType.Swap:
            return (action as LayoutTreeSwapNodeAction).node1Id;
        case LayoutTreeActionType.Stack:
            return (action as LayoutTreeStackAction).targetNodeId;
        case LayoutTreeActionType.StackMove:
            return (action as LayoutTreeStackMoveAction).toNodeId;
    }
    return undefined;
}

export function computeMoveNode(layoutState: LayoutTreeState, computeInsertAction: LayoutTreeComputeMoveNodeAction) {
    const rootNode = layoutState.rootNode;
    const { nodeToMoveId, direction } = computeInsertAction;
    const nodeId = placementTargetId(rootNode, computeInsertAction.nodeId);
    if (!nodeId || !nodeToMoveId) {
        console.warn("either nodeId or nodeToMoveId not set", nodeId, nodeToMoveId);
        return;
    }
    if (direction === undefined) {
        console.warn("No direction provided for insertItemInDirection");
        return;
    }

    if (nodeId === nodeToMoveId) {
        console.warn("Cannot compute move node action since both nodes are equal");
        return;
    }

    let newMoveOperation: MoveOperation;
    const parent = lazy(() => findParent(rootNode, nodeId));
    const grandparent = lazy(() => findParent(rootNode, parent().id));
    const indexInParent = lazy(() => parent()?.children.findIndex((child) => nodeId === child.id));
    const indexInGrandparent = lazy(() => grandparent()?.children.findIndex((child) => parent().id === child.id));
    const nodeToMoveParent = lazy(() => findParent(rootNode, nodeToMoveId));
    const nodeToMoveIndexInParent = lazy(() =>
        nodeToMoveParent()?.children.findIndex((child) => nodeToMoveId === child.id)
    );
    const isRoot = rootNode.id === nodeId;

    // TODO: this should not be necessary. The drag layer is having trouble tracking changes to the LayoutNode fields, so I need to grab the node again here to get the latest data.
    const node = findNode(rootNode, nodeId);
    const nodeToMove = findNode(rootNode, nodeToMoveId);

    if (!node || !nodeToMove) {
        console.warn("node or nodeToMove not set", nodeId, nodeToMoveId);
        return;
    }

    switch (direction) {
        case DropDirection.OuterTop:
            if (node.flexDirection === FlexDirection.Column) {
                const grandparentNode = grandparent();
                if (grandparentNode) {
                    const index = indexInGrandparent();
                    newMoveOperation = {
                        parentId: grandparentNode.id,
                        node: nodeToMove,
                        index,
                    };
                    break;
                }
            }
        // falls through
        case DropDirection.Top:
            if (node.flexDirection === FlexDirection.Column) {
                newMoveOperation = { parentId: nodeId, index: 0, node: nodeToMove };
            } else {
                if (isRoot)
                    newMoveOperation = {
                        node: nodeToMove,
                        index: 0,
                        insertAtRoot: true,
                    };

                const parentNode = parent();
                if (parentNode)
                    newMoveOperation = {
                        parentId: parentNode.id,
                        index: indexInParent() ?? 0,
                        node: nodeToMove,
                    };
            }
            break;
        case DropDirection.OuterBottom:
            if (node.flexDirection === FlexDirection.Column) {
                const grandparentNode = grandparent();
                if (grandparentNode) {
                    const index = indexInGrandparent() + 1;
                    newMoveOperation = {
                        parentId: grandparentNode.id,
                        node: nodeToMove,
                        index,
                    };
                    break;
                }
            }
        // falls through
        case DropDirection.Bottom:
            if (node.flexDirection === FlexDirection.Column) {
                newMoveOperation = { parentId: nodeId, index: 1, node: nodeToMove };
            } else {
                if (isRoot)
                    newMoveOperation = {
                        node: nodeToMove,
                        index: 1,
                        insertAtRoot: true,
                    };

                const parentNode = parent();
                if (parentNode)
                    newMoveOperation = {
                        parentId: parentNode.id,
                        index: indexInParent() + 1,
                        node: nodeToMove,
                    };
            }
            break;
        case DropDirection.OuterLeft:
            if (node.flexDirection === FlexDirection.Row) {
                const grandparentNode = grandparent();
                if (grandparentNode) {
                    const index = indexInGrandparent();
                    newMoveOperation = {
                        parentId: grandparentNode.id,
                        node: nodeToMove,
                        index,
                    };
                    break;
                }
            }
        // falls through
        case DropDirection.Left:
            if (node.flexDirection === FlexDirection.Row) {
                newMoveOperation = { parentId: nodeId, index: 0, node: nodeToMove };
            } else {
                const parentNode = parent();
                if (parentNode)
                    newMoveOperation = {
                        parentId: parentNode.id,
                        index: indexInParent(),
                        node: nodeToMove,
                    };
            }
            break;
        case DropDirection.OuterRight:
            if (node.flexDirection === FlexDirection.Row) {
                const grandparentNode = grandparent();
                if (grandparentNode) {
                    const index = indexInGrandparent() + 1;
                    newMoveOperation = {
                        parentId: grandparentNode.id,
                        node: nodeToMove,
                        index,
                    };
                    break;
                }
            }
        // falls through
        case DropDirection.Right:
            if (node.flexDirection === FlexDirection.Row) {
                newMoveOperation = { parentId: nodeId, index: 1, node: nodeToMove };
            } else {
                const parentNode = parent();
                if (parentNode)
                    newMoveOperation = {
                        parentId: parentNode.id,
                        index: indexInParent() + 1,
                        node: nodeToMove,
                    };
            }
            break;
        case DropDirection.Center:
            // Fork rule: the middle of a tile means "stack with it", not "swap". swapNode stays in the code, unused.
            if (nodeId !== rootNode.id && nodeToMoveId !== rootNode.id) {
                return centreDropAction(rootNode, nodeId, nodeToMoveId);
            } else {
                console.warn("cannot stack with the root");
            }
            break;
        default:
            throw new Error(`Invalid direction: ${direction}`);
    }

    if (
        newMoveOperation?.parentId !== nodeToMoveParent()?.id ||
        (newMoveOperation.index !== nodeToMoveIndexInParent() &&
            newMoveOperation.index !== nodeToMoveIndexInParent() + 1)
    )
        return {
            type: LayoutTreeActionType.Move,
            ...newMoveOperation,
        } as LayoutTreeMoveNodeAction;
}

export function moveNode(layoutState: LayoutTreeState, action: LayoutTreeMoveNodeAction) {
    console.log("moveNode", layoutState, action);
    const rootNode = layoutState.rootNode;
    if (!action) {
        console.error("no move node action provided");
        return;
    }
    if (action.parentId && action.insertAtRoot) {
        console.error("parent and insertAtRoot cannot both be defined in a move node action");
        return;
    }

    const node = findNode(rootNode, action.node.id) ?? action.node;
    const parent = findNode(rootNode, action.parentId);
    const oldParent = findParent(rootNode, action.node.id);

    let startingIndex = 0;

    // If moving under the same parent, we need to make sure that we are removing the child from its old position, not its new one.
    // If the new index is before the old index, we need to start our search for the node to delete after the new index position.
    // If a node is being moved under the same parent, it can keep its size. Otherwise, it should get reset.
    if (oldParent && parent) {
        if (oldParent.id === parent.id) {
            const curIndexInParent = parent.children!.indexOf(node);
            if (curIndexInParent >= action.index) {
                startingIndex = action.index + 1;
            }
        } else {
            node.size = DefaultNodeSize;
        }
    }

    if (!parent && action.insertAtRoot) {
        if (!rootNode.children) {
            addIntermediateNode(rootNode);
        }
        addChildAt(rootNode, action.index, node);
    } else if (parent) {
        addChildAt(parent, action.index, node);
    } else {
        throw new Error("Invalid InsertOperation");
    }

    // Remove nodeToInsert from its old parent
    if (oldParent) {
        removeChild(oldParent, node, startingIndex);
    }
}

export function insertNode(layoutState: LayoutTreeState, action: LayoutTreeInsertNodeAction) {
    if (!action?.node) {
        console.error("insertNode cannot run, no insert node action provided");
        return;
    }
    if (!layoutState.rootNode) {
        layoutState.rootNode = action.node;
    } else {
        const insertLoc = findNextInsertLocation(layoutState.rootNode, DEFAULT_MAX_CHILDREN);
        addChildAt(insertLoc.node, insertLoc.index, action.node);
        if (action.magnified) {
            layoutState.magnifiedNodeId = action.node.id;
            setFocusedNode(layoutState, action.node.id);
        }
    }
    if (action.focused) {
        setFocusedNode(layoutState, action.node.id);
    }
}

export function insertNodeAtIndex(layoutState: LayoutTreeState, action: LayoutTreeInsertNodeAtIndexAction) {
    if (!action?.node || !action?.indexArr) {
        console.error("insertNodeAtIndex cannot run, either node or indexArr field is missing");
        return;
    }
    if (!layoutState.rootNode) {
        layoutState.rootNode = action.node;
    } else {
        const insertLoc = findInsertLocationFromIndexArr(layoutState.rootNode, action.indexArr);
        if (!insertLoc) {
            console.error("insertNodeAtIndex unable to find insert location");
            return;
        }
        addChildAt(insertLoc.node, insertLoc.index + 1, action.node);
        if (action.magnified) {
            layoutState.magnifiedNodeId = action.node.id;
            setFocusedNode(layoutState, action.node.id);
        }
    }
    if (action.focused) {
        setFocusedNode(layoutState, action.node.id);
    }
}

export function swapNode(layoutState: LayoutTreeState, action: LayoutTreeSwapNodeAction) {
    if (!action.node1Id || !action.node2Id) {
        console.error("invalid swapNode action, both node1 and node2 must be defined");
        return;
    }

    if (action.node1Id === layoutState.rootNode.id || action.node2Id === layoutState.rootNode.id) {
        console.error("invalid swapNode action, the root node cannot be swapped");
        return;
    }
    if (action.node1Id === action.node2Id) {
        console.error("invalid swapNode action, node1 and node2 are equal");
        return;
    }

    const parentNode1 = findParent(layoutState.rootNode, action.node1Id);
    const parentNode2 = findParent(layoutState.rootNode, action.node2Id);
    const parentNode1Index = parentNode1.children!.findIndex((child) => child.id === action.node1Id);
    const parentNode2Index = parentNode2.children!.findIndex((child) => child.id === action.node2Id);

    const node1 = parentNode1.children![parentNode1Index];
    const node2 = parentNode2.children![parentNode2Index];

    const node1Size = node1.size;
    node1.size = node2.size;
    node2.size = node1Size;

    parentNode1.children[parentNode1Index] = node2;
    parentNode2.children[parentNode2Index] = node1;
}

export function deleteNode(layoutState: LayoutTreeState, action: LayoutTreeDeleteNodeAction) {
    if (!action?.nodeId) {
        console.error("no delete node action provided");
        return;
    }
    if (!layoutState.rootNode) {
        console.error("no root node");
        return;
    }
    if (layoutState.rootNode.id === action.nodeId) {
        layoutState.rootNode = undefined;
    } else {
        const parent = findParent(layoutState.rootNode, action.nodeId);
        if (parent) {
            const node = parent.children.find((child) => child.id === action.nodeId);
            const removedIndex = parent.children.indexOf(node);
            removeChild(parent, node);
            activateNeighbourAfterRemoval(parent, node.id, removedIndex);
            if (layoutState.focusedNodeId === node.id) {
                layoutState.focusedNodeId = undefined;
            }
        } else {
            console.error("unable to delete node, not found in tree");
        }
    }
}

export function resizeNode(layoutState: LayoutTreeState, action: LayoutTreeResizeNodeAction) {
    if (!action.resizeOperations) {
        console.error("invalid resizeNode operation. nodeSizes array must be defined.");
    }
    for (const resize of action.resizeOperations) {
        if (!resize.nodeId || resize.size < 0 || resize.size > 100) {
            console.error("invalid resizeNode operation. nodeId must be defined and size must be between 0 and 100");
            return;
        }
        const node = findNode(layoutState.rootNode, resize.nodeId);
        node.size = resize.size;
    }
}

export function focusNode(layoutState: LayoutTreeState, action: LayoutTreeFocusNodeAction) {
    if (!action.nodeId) {
        console.error("invalid focusNode operation, nodeId must be defined.");
        return;
    }

    setFocusedNode(layoutState, action.nodeId);
}

export function magnifyNodeToggle(layoutState: LayoutTreeState, action: LayoutTreeMagnifyNodeToggleAction) {
    if (!action.nodeId) {
        console.error("invalid magnifyNodeToggle operation. nodeId must be defined.");
        return;
    }
    if (layoutState.rootNode.id === action.nodeId) {
        console.warn(`cannot toggle magnification of node ${action.nodeId} because it is the root node.`);
        return;
    }
    if (layoutState.magnifiedNodeId === action.nodeId) {
        layoutState.magnifiedNodeId = undefined;
    } else {
        layoutState.magnifiedNodeId = action.nodeId;
        setFocusedNode(layoutState, action.nodeId);
    }
}

export function clearTree(layoutState: LayoutTreeState) {
    layoutState.rootNode = undefined;
    layoutState.leafOrder = undefined;
    layoutState.focusedNodeId = undefined;
    layoutState.magnifiedNodeId = undefined;
}

export function replaceNode(layoutState: LayoutTreeState, action: LayoutTreeReplaceNodeAction) {
    const { targetNodeId, newNode } = action;
    if (layoutState.rootNode.id === targetNodeId) {
        newNode.size = layoutState.rootNode.size; // preserve size
        layoutState.rootNode = newNode;
    } else {
        const parent = findParent(layoutState.rootNode, targetNodeId);
        if (!parent) {
            console.error("replaceNode: Parent not found for", targetNodeId);
            return;
        }
        const index = parent.children.findIndex((child) => child.id === targetNodeId);
        if (index === -1) {
            console.error("replaceNode: Target node not found in parent's children", targetNodeId);
            return;
        }
        // Preserve the old node's size.
        const targetNode = parent.children[index];
        newNode.size = targetNode.size;
        parent.children[index] = newNode;
    }
    if (action.focused) {
        setFocusedNode(layoutState, newNode.id);
    }
}

// ─── SPLIT HORIZONTAL ─────────────────────────────────────────────────────────────

export function splitHorizontal(layoutState: LayoutTreeState, action: LayoutTreeSplitHorizontalAction) {
    const { newNode, position } = action;
    const targetNodeId = placementTargetId(layoutState.rootNode, action.targetNodeId);
    const targetNode = findNode(layoutState.rootNode, targetNodeId);
    if (!targetNode) {
        console.error("splitHorizontal: Target node not found", targetNodeId);
        return;
    }

    const parent = findParent(layoutState.rootNode, targetNodeId);
    if (parent && parent.flexDirection === FlexDirection.Row) {
        const index = parent.children.findIndex((child) => child.id === targetNodeId);
        if (index === -1) {
            console.error("splitHorizontal: Target node not found in parent's children", targetNodeId);
            return;
        }
        const insertIndex = position === "before" ? index : index + 1;
        // Directly splice in the new node instead of calling addChildAt (which may flatten nodes)
        parent.children.splice(insertIndex, 0, newNode);
    } else {
        // Otherwise, if no parent or parent's flexDirection is not Row, we need to wrap
        // Create a new group node with horizontal layout.
        // IMPORTANT: pass an initial children array so the new node is valid.
        const groupNode = newLayoutNode(FlexDirection.Row, targetNode.size, [targetNode], undefined);
        // Now decide the ordering based on the "position"
        groupNode.children = position === "before" ? [newNode, targetNode] : [targetNode, newNode];
        if (parent) {
            const index = parent.children.findIndex((child) => child.id === targetNodeId);
            if (index === -1) {
                console.error("splitHorizontal (wrap): Target node not found in parent's children", targetNodeId);
                return;
            }
            parent.children[index] = groupNode;
        } else {
            layoutState.rootNode = groupNode;
        }
    }
    if (action.focused) {
        setFocusedNode(layoutState, newNode.id);
    }
}

// ─── SPLIT VERTICAL ─────────────────────────────────────────────────────────────

export function splitVertical(layoutState: LayoutTreeState, action: LayoutTreeSplitVerticalAction) {
    const { newNode, position } = action;
    const targetNodeId = placementTargetId(layoutState.rootNode, action.targetNodeId);
    const targetNode = findNode(layoutState.rootNode, targetNodeId);
    if (!targetNode) {
        console.error("splitVertical: Target node not found", targetNodeId);
        return;
    }

    const parent = findParent(layoutState.rootNode, targetNodeId);
    if (parent && parent.flexDirection === FlexDirection.Column) {
        const index = parent.children.findIndex((child) => child.id === targetNodeId);
        if (index === -1) {
            console.error("splitVertical: Target node not found in parent's children", targetNodeId);
            return;
        }
        const insertIndex = position === "before" ? index : index + 1;
        // For vertical splits in an already vertical parent, splice directly.
        parent.children.splice(insertIndex, 0, newNode);
    } else {
        // Wrap target node in a new vertical group.
        // Create group node with an initial children array so that validation passes.
        const groupNode = newLayoutNode(FlexDirection.Column, targetNode.size, [targetNode], undefined);
        groupNode.children = position === "before" ? [newNode, targetNode] : [targetNode, newNode];
        if (parent) {
            const index = parent.children.findIndex((child) => child.id === targetNodeId);
            if (index === -1) {
                console.error("splitVertical (wrap): Target node not found in parent's children", targetNodeId);
                return;
            }
            parent.children[index] = groupNode;
        } else {
            layoutState.rootNode = groupNode;
        }
    }
    if (action.focused) {
        setFocusedNode(layoutState, newNode.id);
    }
}

// ─── STACK ──────────────────────────────────────────────────────────────────────

/**
 * Groups leaves into a stack (tab group): `nodeId` joins the stack that contains `targetNodeId`, or the two leaves
 * become a new stack where the target was. The moved leaf becomes the active member and takes focus, unless
 * `focused` is false: then nothing the user is looking at is hidden or refocused (the focused leaf stays the visible
 * member). Returns false, and leaves the tree untouched, when the action is invalid.
 */
export function stackNodes(layoutState: LayoutTreeState, action: LayoutTreeStackAction): boolean {
    const root = layoutState.rootNode;
    const { nodeId, targetNodeId } = action;
    if (!root || !nodeId || !targetNodeId || nodeId === targetNodeId) {
        console.error("stackNodes: need two different nodes");
        return false;
    }
    const node = findNode(root, nodeId);
    const target = findNode(root, targetNodeId);
    if (!node || !target) {
        console.error("stackNodes: node not found", nodeId, targetNodeId);
        return false;
    }
    if (node.children || !node.data) {
        console.error("stackNodes: only a leaf can be put into a stack", nodeId);
        return false;
    }
    const targetParent = findParent(root, targetNodeId);
    let stack: LayoutNode = isStackNode(target)
        ? target
        : targetParent && isStackNode(targetParent)
          ? targetParent
          : null;
    if (!stack && (target.children || !target.data)) {
        console.error("stackNodes: the target must be a leaf or a stack", targetNodeId);
        return false;
    }
    const oldParent = findParent(root, nodeId);
    if (!oldParent) {
        console.error("stackNodes: the root leaf cannot be moved into another node", nodeId);
        return false;
    }
    const wantFocus = action.focused ?? true;
    const nodeWasFocused = layoutState.focusedNodeId === nodeId;
    const activateMoved = wantFocus || nodeWasFocused;
    if (stack && oldParent === stack) {
        if (wantFocus) {
            setFocusedNode(layoutState, nodeId);
        }
        return true;
    }
    const removedIndex = oldParent.children.indexOf(node);
    removeChild(oldParent, node);
    activateNeighbourAfterRemoval(oldParent, nodeId, removedIndex);
    if (stack) {
        const after = stack.children.findIndex((c) => c.id === targetNodeId);
        addStackMember(stack, after >= 0 ? after + 1 : stack.children.length, node, activateMoved);
    } else {
        const newStack = newStackNode([target, node], activateMoved ? nodeId : targetNodeId, target.size);
        const parentNow = findParent(layoutState.rootNode, targetNodeId);
        if (parentNow) {
            const idx = parentNow.children.findIndex((c) => c.id === targetNodeId);
            newStack.flexDirection = target.flexDirection;
            parentNow.children[idx] = newStack;
        } else {
            layoutState.rootNode = newStack;
        }
        stack = newStack;
    }
    if (wantFocus) {
        setFocusedNode(layoutState, nodeId);
    }
    return true;
}

/**
 * Takes a leaf out of its stack and puts it next to the stack, focused unless `focused` is false.
 */
export function unstackNode(layoutState: LayoutTreeState, action: LayoutTreeUnstackAction): boolean {
    const root = layoutState.rootNode;
    const node = root ? findNode(root, action.nodeId) : undefined;
    const stack = node ? findParent(root, node.id) : undefined;
    if (!node || !stack || !isStackNode(stack)) {
        console.error("unstackNode: the node is not a member of a stack", action.nodeId);
        return false;
    }
    const removedIndex = stack.children.indexOf(node);
    removeChild(stack, node);
    activateNeighbourAfterRemoval(stack, node.id, removedIndex);
    node.size = DefaultNodeSize;
    const stackParent = findParent(root, stack.id);
    if (stackParent) {
        const idx = stackParent.children.indexOf(stack);
        node.flexDirection = reverseFlexDirection(stackParent.flexDirection);
        stackParent.children.splice(idx + 1, 0, node);
    } else {
        const group = newLayoutNode(FlexDirection.Row, stack.size, [stack, node]);
        stack.flexDirection = FlexDirection.Column;
        node.flexDirection = FlexDirection.Column;
        layoutState.rootNode = group;
    }
    if (action.focused ?? true) {
        setFocusedNode(layoutState, node.id);
    }
    return true;
}

/**
 * Puts a node that is not in the tree yet straight into the stack of the target leaf, or makes a stack of the
 * target and the new node. Nothing else moves, so a new terminal that is meant to be a tab never appears as a tile.
 */
export function insertNodeIntoStack(layoutState: LayoutTreeState, action: LayoutTreeStackInsertAction): boolean {
    const root = layoutState.rootNode;
    const { node, targetNodeId } = action;
    if (!root || !node || node.children || !node.data) {
        console.error("insertNodeIntoStack: the new node must be a leaf");
        return false;
    }
    const target = findNode(root, targetNodeId);
    if (!target || (target.children && !isStackNode(target)) || (!target.children && !target.data)) {
        console.error("insertNodeIntoStack: the target must be a leaf or a stack", targetNodeId);
        return false;
    }
    const wantFocus = action.focused ?? true;
    const targetParent = findParent(root, targetNodeId);
    const stack = isStackNode(target) ? target : targetParent && isStackNode(targetParent) ? targetParent : null;
    if (stack) {
        const after = stack.children.findIndex((c) => c.id === targetNodeId);
        addStackMember(stack, after >= 0 ? after + 1 : stack.children.length, node, wantFocus);
    } else {
        const newStack = newStackNode([target, node], wantFocus ? node.id : target.id, target.size);
        newStack.flexDirection = target.flexDirection;
        if (targetParent) {
            const idx = targetParent.children.findIndex((c) => c.id === targetNodeId);
            targetParent.children[idx] = newStack;
        } else {
            layoutState.rootNode = newStack;
        }
    }
    if (wantFocus) {
        setFocusedNode(layoutState, node.id);
    }
    return true;
}

/**
 * Shows a member of a stack without asking for keyboard focus. If the focused node is a member of the same stack the
 * focus moves with the tab, because keystrokes must never go to a terminal nobody can see.
 */
export function activateStackMember(layoutState: LayoutTreeState, nodeId: string): boolean {
    const root = layoutState.rootNode;
    const node = root ? findNode(root, nodeId) : undefined;
    const stack = node ? findParent(root, nodeId) : undefined;
    if (!node || !stack || !isStackNode(stack)) {
        return false;
    }
    stack.activeChildId = nodeId;
    if (layoutState.focusedNodeId && stack.children.some((c) => c.id === layoutState.focusedNodeId)) {
        layoutState.focusedNodeId = nodeId;
    }
    return true;
}

/**
 * Turns a plain tile into a stack of one: same place, same size. Stack mode then stays on (strip and plus) until
 * turnStackOff, even when members are closed down to one.
 */
export function turnIntoStack(layoutState: LayoutTreeState, nodeId: string): boolean {
    const root = layoutState.rootNode;
    const node = root ? findNode(root, nodeId) : undefined;
    if (!node || node.children || !node.data) {
        return false;
    }
    const parent = findParent(root, nodeId);
    if (parent && isStackNode(parent)) {
        return false;
    }
    const stack = newStackNode([node], node.id, node.size);
    stack.flexDirection = node.flexDirection;
    if (parent) {
        parent.children[parent.children.indexOf(node)] = stack;
    } else {
        layoutState.rootNode = stack;
    }
    return true;
}

/**
 * Gives the plain tile back when the stack has exactly one member. nodeId may be the stack or its member.
 */
export function turnStackOff(layoutState: LayoutTreeState, nodeId: string): boolean {
    const root = layoutState.rootNode;
    const node = root ? findNode(root, nodeId) : undefined;
    const stack = node && isStackNode(node) ? node : node ? findParent(root, nodeId) : undefined;
    if (!stack || !isStackNode(stack) || stack.children.length !== 1) {
        return false;
    }
    const only = stack.children[0];
    only.size = stack.size;
    only.flexDirection = stack.flexDirection;
    const parent = findParent(root, stack.id);
    if (parent) {
        parent.children[parent.children.indexOf(stack)] = only;
    } else {
        layoutState.rootNode = only;
    }
    return true;
}

/**
 * Moves everything from one place into the stack at another: fromNodeId is a tile or any member of a stack (then the
 * whole stack moves, in its order), toNodeId is a tile or a stack (or a member of one). All of it ends up as members
 * of the target stack, appended at the end; the old stack is left empty and removed by the next balance.
 */
export function moveIntoStack(layoutState: LayoutTreeState, action: LayoutTreeStackMoveAction): boolean {
    const root = layoutState.rootNode;
    const from = root ? findNode(root, action.fromNodeId) : undefined;
    const to = root ? findNode(root, action.toNodeId) : undefined;
    if (!from || !to || from.id === root.id || to.id === from.id) {
        return false;
    }
    const fromParent = findParent(root, from.id);
    const fromStack = isStackNode(from) ? from : fromParent && isStackNode(fromParent) ? fromParent : null;
    const members = fromStack ? [...fromStack.children] : [from];
    if (members.some((m) => m.children || !m.data)) {
        return false;
    }
    const toParent = findParent(root, to.id);
    const toStack = isStackNode(to) ? to : toParent && isStackNode(toParent) ? toParent : null;
    if (members.some((m) => m.id === to.id) || (toStack && fromStack && toStack.id === fromStack.id)) {
        return false;
    }
    if (!toStack && (to.children || !to.data)) {
        return false;
    }
    const activeMemberId = fromStack?.activeChildId ?? from.id;
    let targetId = toStack ? toStack.id : to.id;
    let moved = false;
    for (const member of members) {
        if (
            !stackNodes(layoutState, {
                type: LayoutTreeActionType.Stack,
                nodeId: member.id,
                targetNodeId: targetId,
                focused: false,
            })
        ) {
            break;
        }
        moved = true;
        const nowParent = findParent(layoutState.rootNode, member.id);
        if (nowParent && isStackNode(nowParent)) {
            targetId = nowParent.id;
        }
    }
    if (moved && (action.focused ?? false)) {
        setFocusedNode(layoutState, activeMemberId);
    }
    return moved;
}
