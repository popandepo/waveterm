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

import { insertedBlockIds } from "../lib/backendaction";
import { LayoutModel } from "../lib/layoutModel";
import { balanceNode, newLayoutNode, walkNodes } from "../lib/layoutNode";

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
        const leafs: any[] = [];
        model.treeState.rootNode = balanceNode(model.treeState.rootNode, (n) => {
            n.children?.forEach((c) => c.id);
            if (!n.children?.length) leafs.push({ nodeid: n.id, blockid: n.data?.blockId });
        });
        model.treeState.leafOrder = leafs;
        const nodes: any[] = [];
        walkNodes(model.treeState.rootNode, (n) => {
            if (n.data?.blockId) nodes.push(n);
        });
        store.set(model.leafs, nodes);
    };
    model.updateTree();
    model.blockExists = async () => true;
    model.onNodeDelete = vi.fn(async () => {});
    return model;
}

afterEach(() => new Promise((resolve) => setTimeout(resolve, 150)));

describe("insertedBlockIds", () => {
    test("names the blocks of insert actions only", () => {
        const ids = insertedBlockIds([
            { actiontype: "stackinsert", blockid: "a", actionid: "1" },
            { actiontype: "insert", blockid: "b", actionid: "2" },
            { actiontype: "delete", blockid: "c", actionid: "3" },
            { actiontype: "detach", blockid: "d", actionid: "4" },
            { actiontype: "cleanuporphaned", actionid: "5" },
        ] as any);
        assert.deepEqual([...ids].sort(), ["a", "b"]);
    });
});

describe("the detach action (a block that moved to another tab)", () => {
    test("removes the leaf and never deletes the block", async () => {
        const moved = leaf("moved");
        const stays = leaf("stays");
        const model = modelWith(
            newLayoutNode(undefined, undefined, [moved, stays]),
            ["stays"],
            [{ actiontype: "detach", blockid: "moved", actionid: "d1" }]
        );
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode), ["stays"]);
        assert.equal(model.onNodeDelete.mock.calls.length, 0);
    });

    test("a delete action for the same leaf does call onNodeDelete (the difference that matters)", async () => {
        const moved = leaf("moved");
        const stays = leaf("stays");
        const model = modelWith(
            newLayoutNode(undefined, undefined, [moved, stays]),
            ["stays"],
            [{ actiontype: "delete", blockid: "moved", actionid: "d1" }]
        );
        await model.processPendingBackendActions();
        assert.equal(model.onNodeDelete.mock.calls.length, 1);
    });
});

describe("a block that arrives from another tab", () => {
    test("is not cleaned up as an orphan while its insert is still queued after the clean-up", async () => {
        const target = leaf("target");
        const model = modelWith(
            newLayoutNode(undefined, undefined, [target]),
            ["target", "arriving"],
            [
                { actiontype: "cleanuporphaned", actionid: "c1" },
                { actiontype: "stackinsert", blockid: "arriving", targetblockid: "target", actionid: "i1" },
            ]
        );
        await model.processPendingBackendActions();
        assert.equal(model.onNodeDelete.mock.calls.length, 0);
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["arriving", "target"]);
    });

    test("a listed block with no leaf and no queued insert is still cleaned up", async () => {
        const target = leaf("target");
        const model = modelWith(
            newLayoutNode(undefined, undefined, [target]),
            ["target", "stray"],
            [{ actiontype: "cleanuporphaned", actionid: "c1" }]
        );
        await model.processPendingBackendActions();
        assert.deepEqual(model.onNodeDelete.mock.calls, [[{ blockId: "stray" }]]);
    });
});

describe("a block that arrives from another tab, more cases", () => {
    test("an insert that was queued after the batch was read still protects the block from the clean-up", async () => {
        const target = leaf("target");
        const model = modelWith(
            newLayoutNode(undefined, undefined, [target]),
            ["target", "arriving"],
            [{ actiontype: "stackinsert", blockid: "arriving", targetblockid: "target", actionid: "late" }]
        );
        await model.cleanupOrphanedBlocks();
        assert.equal(model.onNodeDelete.mock.calls.length, 0);
    });

    test("an insert whose target is gone puts the block into the tree on its own instead of leaving it to be deleted", async () => {
        const other = leaf("other");
        const model = modelWith(
            newLayoutNode(undefined, undefined, [other]),
            ["other", "arriving"],
            [
                { actiontype: "stackinsert", blockid: "arriving", targetblockid: "closed-meanwhile", actionid: "i1" },
                { actiontype: "cleanuporphaned", actionid: "c1" },
            ]
        );
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["arriving", "other"]);
        assert.equal(model.onNodeDelete.mock.calls.length, 0);
    });
});
