// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { treeActionForBackendAction } from "../lib/backendaction";
import { newLayoutNode } from "../lib/layoutNode";
import { insertNode, splitHorizontal, splitVertical } from "../lib/layoutTree";
import { DefaultNodeSize, LayoutTreeActionType } from "../lib/types";
import { newLayoutTreeState } from "./model";

// The weight that wsh --size asks for travels in the layout action that the server queues (nodesize). These tests send such an action through the
// front end's action translation (the one the layout model uses) and apply the result to a layout tree: if the front end stopped reading nodesize
// from the action, they fail.
const sizesOf = (state: any) => state.rootNode.children.map((c: any) => c.size);

describe("a layout action with a nodesize gives the new tile that weight", () => {
    test("a split to the right of a tile", () => {
        const left = newLayoutNode(undefined, undefined, undefined, { blockId: "left" });
        const state = newLayoutTreeState(left);
        const res = treeActionForBackendAction(
            { actiontype: "splithorizontal", blockid: "added", targetblockid: "left", position: "after", nodesize: 7 } as any,
            (id) => (id === "left" ? left : undefined)
        );
        assert.isNotNull(res);
        assert.property(res, "action");
        splitHorizontal(state, (res as any).action);
        assert.deepEqual(sizesOf(state), [DefaultNodeSize, 7]);
    });

    test("a split below a tile", () => {
        const top = newLayoutNode(undefined, undefined, undefined, { blockId: "top" });
        const state = newLayoutTreeState(top);
        const res = treeActionForBackendAction(
            { actiontype: "splitvertical", blockid: "added", targetblockid: "top", position: "after", nodesize: 25 } as any,
            (id) => (id === "top" ? top : undefined)
        );
        splitVertical(state, (res as any).action);
        assert.deepEqual(sizesOf(state), [DefaultNodeSize, 25]);
    });

    test("a plain insert (no target): this one used to drop the weight", () => {
        const first = newLayoutNode(undefined, undefined, undefined, { blockId: "first" });
        const state = newLayoutTreeState(first);
        const res = treeActionForBackendAction({ actiontype: "insert", blockid: "second", nodesize: 4 } as any, () => undefined);
        assert.property(res, "action");
        assert.equal((res as any).action.type, LayoutTreeActionType.InsertNode);
        insertNode(state, (res as any).action);
        assert.deepEqual(sizesOf(state), [DefaultNodeSize, 4]);
    });

    test("without a nodesize the new tile has the default weight (an equal half)", () => {
        const left = newLayoutNode(undefined, undefined, undefined, { blockId: "left" });
        const state = newLayoutTreeState(left);
        const res = treeActionForBackendAction(
            { actiontype: "splithorizontal", blockid: "added", targetblockid: "left", position: "after" } as any,
            () => left
        );
        splitHorizontal(state, (res as any).action);
        assert.deepEqual(sizesOf(state), [DefaultNodeSize, DefaultNodeSize]);
    });

    test("a missing target and a bad position are errors, nothing is applied", () => {
        assert.property(treeActionForBackendAction({ actiontype: "splithorizontal", blockid: "a", targetblockid: "nope", position: "after" } as any, () => undefined), "error");
        const t = newLayoutNode(undefined, undefined, undefined, { blockId: "t" });
        assert.property(treeActionForBackendAction({ actiontype: "splitvertical", blockid: "a", targetblockid: "t", position: "sideways" } as any, () => t), "error");
    });
});

describe("the layout model uses the translation", () => {
    const src = readFileSync(path.join(__dirname, "..", "lib", "layoutModel.ts"), "utf8");
    test("insert and both splits go through treeActionForBackendAction", () => {
        assert.equal(src.split("treeActionForBackendAction(action,").length - 1, 3);
    });
    test("no handler builds a node for a backend action without the weight", () => {
        const body = src.slice(src.indexOf("private async handleBackendAction"));
        const handler = body.slice(0, body.indexOf("case LayoutTreeActionType.Stack"));
        const calls = handler.split("newLayoutNode(").slice(1);
        for (const call of calls) {
            assert.match(call.slice(0, 40), /^undefined, action\.nodesize/, call.slice(0, 60));
        }
    });
});
