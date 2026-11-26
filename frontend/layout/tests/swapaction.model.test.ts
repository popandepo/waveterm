// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom, createStore } from "jotai";
import { afterEach, assert, describe, test, vi } from "vitest";

vi.mock("@/app/store/global", () => {
    const { atom } = require("jotai");
    return {
        getSettingsKeyAtom: () => atom(undefined),
        WOS: { makeORef: (t: string, id: string) => `${t}:${id}`, getWaveObjectAtom: () => atom(undefined) },
    };
});
const { setObjectValue } = vi.hoisted(() => ({ setObjectValue: vi.fn() }));
vi.mock("@/app/store/wos", () => ({
    makeORef: (t: string, id: string) => `${t}:${id}`,
    getWaveObjectAtom: () => undefined,
    setObjectValue,
}));
vi.mock("@/app/store/services", () => ({ BlockService: {}, ObjectService: {} }));
vi.mock("@/app/store/focusManager", () => ({
    FocusManager: { getInstance: () => ({ requestNodeFocus: () => {}, setBlockFocus: () => {} }) },
}));

import { LayoutModel } from "../lib/layoutModel";
import { balanceNode, newLayoutNode, newStackNode, walkNodes } from "../lib/layoutNode";

function leaf(blockId: string) {
    return newLayoutNode(undefined, undefined, undefined, { blockId });
}

function blockIdsOf(root: any): string[] {
    const ids: string[] = [];
    walkNodes(root, (n) => {
        if (n.data?.blockId) ids.push(n.data.blockId);
    });
    return ids;
}

function modelWith(root: any, blockids: string[], actions: any[]) {
    const store = createStore();
    const tabAtom = atom({ oid: "tab-1", layoutstate: "layout-1", blockids } as any);
    const model: any = new LayoutModel(tabAtom, store.get, store.set);
    model.treeState = { rootNode: root, pendingBackendActions: [] };
    model.waveObjectAtom = atom({ pendingbackendactions: actions } as any);
    setObjectValue.mockReset();
    model.updateTree = () => {
        model.treeState.rootNode = balanceNode(model.treeState.rootNode, (n) => {
            n.children?.forEach((c) => c.id);
        });
        const nodes: any[] = [];
        walkNodes(model.treeState.rootNode, (n) => {
            if (n.data?.blockId) nodes.push(n);
        });
        store.set(model.leafs, nodes);
        model.treeState.leafOrder = nodes.map((n) => ({ nodeid: n.id, blockid: n.data.blockId }));
    };
    model.updateTree();
    model.onNodeDelete = vi.fn(async () => {});
    return model;
}

afterEach(() => new Promise((resolve) => setTimeout(resolve, 150)));

describe("the swap action queued by the server", () => {
    test("two tiles change places and no block is closed", async () => {
        const model = modelWith(
            newLayoutNode(undefined, undefined, [leaf("a"), leaf("b"), leaf("c")]),
            ["a", "b", "c"],
            [{ actiontype: "swap", blockid: "a", targetblockid: "c", actionid: "s1" }]
        );
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode), ["c", "b", "a"]);
        assert.equal(model.onNodeDelete.mock.calls.length, 0);
    });

    test("a block that is a member of a group swaps as the whole group", async () => {
        const stack = newStackNode([leaf("m1"), leaf("m2")]);
        const model = modelWith(
            newLayoutNode(undefined, undefined, [leaf("a"), stack]),
            ["a", "m1", "m2"],
            [{ actiontype: "swap", blockid: "a", targetblockid: "m2", actionid: "s1" }]
        );
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode), ["m1", "m2", "a"]);
    });

    test("two members of one group are left alone", async () => {
        const stack = newStackNode([leaf("m1"), leaf("m2")]);
        const model = modelWith(
            newLayoutNode(undefined, undefined, [leaf("a"), stack]),
            ["a", "m1", "m2"],
            [{ actiontype: "swap", blockid: "m1", targetblockid: "m2", actionid: "s1" }]
        );
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode), ["a", "m1", "m2"]);
    });

    test("a block that is not in the tree changes nothing", async () => {
        const model = modelWith(
            newLayoutNode(undefined, undefined, [leaf("a"), leaf("b")]),
            ["a", "b"],
            [{ actiontype: "swap", blockid: "a", targetblockid: "gone", actionid: "s1" }]
        );
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode), ["a", "b"]);
    });
});
