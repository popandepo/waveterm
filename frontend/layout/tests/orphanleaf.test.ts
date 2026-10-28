// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom, createStore } from "jotai";
import { assert, describe, test, vi } from "vitest";

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
}));
vi.mock("@/app/store/services", () => ({ BlockService: {}, ObjectService: {} }));
vi.mock("@/app/store/focusManager", () => ({
    FocusManager: { getInstance: () => ({ requestNodeFocus: () => {}, setBlockFocus: () => {} }) },
}));

import { LayoutModel } from "../lib/layoutModel";
import { balanceNode, findNode, newLayoutNode, newStackNode, orphanLeafIds, walkNodes } from "../lib/layoutNode";

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

// the Home tab as found on 28 Oct 2026: a stack of one whose block row was gone, a stack with a live block, a plain tile
function homeTree() {
    const orphan = leaf("7193ba65");
    const status = leaf("c5b03b0c");
    const manager = leaf("a008b469");
    const root = newLayoutNode(undefined, undefined, [newStackNode([orphan]), newStackNode([status]), manager]);
    return { root, orphan, status, manager };
}

describe("orphanLeafIds", () => {
    test("names the leaf whose block is not in the tab, and only that one", () => {
        const { root, orphan } = homeTree();
        assert.deepEqual(orphanLeafIds(root, ["c5b03b0c", "a008b469"]), [orphan.id]);
    });

    test("names nothing when every leaf has its block", () => {
        const { root } = homeTree();
        assert.deepEqual(orphanLeafIds(root, ["7193ba65", "c5b03b0c", "a008b469"]), []);
    });

    test("a tab whose block list is not loaded names nothing (an unknown list is not an empty one)", () => {
        const { root } = homeTree();
        assert.deepEqual(orphanLeafIds(root, undefined), []);
    });

    test("no tree names nothing", () => {
        assert.deepEqual(orphanLeafIds(undefined, ["x"]), []);
    });
});

function modelWith(root: any, blockids: string[] | undefined) {
    const store = createStore();
    const tabAtom = atom({ oid: "tab-1", layoutstate: "layout-1", blockids } as any);
    const model: any = new LayoutModel(tabAtom, store.get, store.set);
    model.treeState = { rootNode: root, pendingBackendActions: [] };
    model.persistToBackend = () => {};
    model.updateTree = () => {};
    return model;
}

describe("the cleanuporphaned action", () => {
    test("removes the leaf whose block is gone, and the stack it leaves empty", async () => {
        const { root, orphan, status, manager } = homeTree();
        const model = modelWith(root, ["c5b03b0c", "a008b469"]);
        await model.handleBackendAction({ actiontype: "cleanuporphaned", actionid: "c1" });
        assert.equal(findNode(model.treeState.rootNode, orphan.id), undefined);
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), [status.data.blockId, manager.data.blockId].sort());
        // updateTree balances the tree after the action; it is stubbed here, so the step is run by hand
        balanceNode(model.treeState.rootNode);
        assert.equal(model.treeState.rootNode.children.length, 2);
        assert.ok(model.treeState.rootNode.children.every((c: any) => c.children?.length !== 0));
    });

    test("leaves a tree alone when every leaf has its block", async () => {
        const { root } = homeTree();
        const model = modelWith(root, ["7193ba65", "c5b03b0c", "a008b469"]);
        await model.handleBackendAction({ actiontype: "cleanuporphaned", actionid: "c2" });
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["7193ba65", "a008b469", "c5b03b0c"]);
    });

    test("does not touch the tree while the tab's block list is not loaded", async () => {
        const { root } = homeTree();
        const model = modelWith(root, undefined);
        await model.handleBackendAction({ actiontype: "cleanuporphaned", actionid: "c3" });
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["7193ba65", "a008b469", "c5b03b0c"]);
    });
});
