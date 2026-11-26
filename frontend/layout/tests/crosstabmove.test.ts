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
vi.mock("@/app/store/wos", () => ({
    makeORef: (t: string, id: string) => `${t}:${id}`,
    getWaveObjectAtom: () => undefined,
    setObjectValue: () => {},
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

// the page of one tab, applying the queued actions to its tree; a delete takes the block out of existingBlocks
async function applyInTab(root: any, listed: string[], actions: any[], existingBlocks: Set<string>) {
    const store = createStore();
    const tabAtom = atom({ oid: "tab-1", layoutstate: "layout-1", blockids: listed } as any);
    const model: any = new LayoutModel(tabAtom, store.get, store.set);
    model.treeState = { rootNode: root, pendingBackendActions: [] };
    model.waveObjectAtom = atom({ pendingbackendactions: actions } as any);
    model.updateTree = () => {
        model.treeState.rootNode = balanceNode(model.treeState.rootNode, () => {});
        const nodes: any[] = [];
        walkNodes(model.treeState.rootNode, (n) => {
            if (n.data?.blockId) nodes.push(n);
        });
        store.set(model.leafs, nodes);
    };
    model.updateTree();
    model.blockExists = async (blockId: string) => existingBlocks.has(blockId);
    model.onNodeDelete = async (data: { blockId: string }) => {
        existingBlocks.delete(data.blockId);
    };
    await model.processPendingBackendActions();
    return blockIdsOf(model.treeState.rootNode);
}

// the model saves its tree on a debounce, which has to run out before the mocks go away
afterEach(() => new Promise((resolve) => setTimeout(resolve, 150)));

describe("a group moved to another tab", () => {
    test("leaves its old tab and joins the end of the target group with every block still there", async () => {
        const existingBlocks = new Set(["stays", "moved1", "moved2", "first", "second"]);
        const group = [leaf("first"), leaf("second")];

        const source = await applyInTab(
            newLayoutNode(undefined, undefined, [leaf("moved1"), leaf("moved2"), leaf("stays")]),
            ["stays"],
            [
                { actiontype: "detach", blockid: "moved1", actionid: "d1" },
                { actiontype: "detach", blockid: "moved2", actionid: "d2" },
            ],
            existingBlocks
        );
        const target = await applyInTab(
            newStackNode(group, group[0].id, 10),
            ["first", "second", "moved1", "moved2"],
            [
                { actiontype: "cleanuporphaned", actionid: "c1" },
                {
                    actiontype: "stackinsert",
                    blockid: "moved1",
                    targetblockid: "first",
                    position: "end",
                    actionid: "i1",
                },
                {
                    actiontype: "stackinsert",
                    blockid: "moved2",
                    targetblockid: "moved1",
                    position: "end",
                    actionid: "i2",
                },
            ],
            existingBlocks
        );

        assert.deepEqual(source, ["stays"]);
        assert.deepEqual(target, ["first", "second", "moved1", "moved2"]);
        assert.deepEqual([...existingBlocks].sort(), ["first", "moved1", "moved2", "second", "stays"]);
    });
});
