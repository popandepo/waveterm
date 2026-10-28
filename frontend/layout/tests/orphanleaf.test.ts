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

// The saved layout is what survives a restart, so the tests drive the real path (processPendingBackendActions) and watch the save.
function modelWith(root: any, blockids: string[] | undefined, existing: string[] = []) {
    const store = createStore();
    const tabAtom = atom({ oid: "tab-1", layoutstate: "layout-1", blockids } as any);
    const model: any = new LayoutModel(tabAtom, store.get, store.set);
    model.setTabBlockIds = (ids: string[]) => store.set(tabAtom, { oid: "tab-1", layoutstate: "layout-1", blockids: ids } as any);
    model.treeState = { rootNode: root, pendingBackendActions: [] };
    model.waveObjectAtom = atom({ pendingbackendactions: [{ actiontype: "cleanuporphaned", actionid: "c1" }] } as any);
    const saved: any[] = [];
    // the real persistToBackend runs; what it hands to the store is what is recorded
    setObjectValue.mockReset();
    setObjectValue.mockImplementation((obj: any) => {
        const copy = structuredClone(obj.rootnode);
        copy.savedLeafOrder = structuredClone(obj.leaforder);
        saved.push(copy);
    });
    // like the real updateTreeHelper, the layout callback reads every child of the node it is given
    model.updateTree = () => {
        const leafs: any[] = [];
        model.treeState.rootNode = balanceNode(model.treeState.rootNode, (n) => {
            n.children?.forEach((c) => c.id);
            if (!n.children?.length) leafs.push({ nodeid: n.id, blockid: n.data?.blockId });
        });
        model.treeState.leafOrder = leafs;
    };
    model.blockExists = async (id: string) => existing.includes(id);
    return { model, saved };
}

describe("the cleanuporphaned action", () => {
    test("removes the leaf whose block is gone, and the stack it leaves empty, and saves the result", async () => {
        const { root, orphan, status, manager } = homeTree();
        const { model, saved } = modelWith(root, ["c5b03b0c", "a008b469"]);
        await model.processPendingBackendActions();
        await new Promise((resolve) => setTimeout(resolve, 150));
        assert.equal(findNode(model.treeState.rootNode, orphan.id), undefined);
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), [status.data.blockId, manager.data.blockId].sort());
        assert.equal(model.treeState.rootNode.children.length, 2);
        assert.equal(saved.length, 1);
        assert.deepEqual(blockIdsOf(saved[0]).sort(), [status.data.blockId, manager.data.blockId].sort());
    });

    test("the saved tree has no hole where the empty stack was, and its leaf order no longer names the leaf", async () => {
        const { root, status, manager } = homeTree();
        const { model, saved } = modelWith(root, ["c5b03b0c", "a008b469"]);
        await model.processPendingBackendActions();
        await new Promise((resolve) => setTimeout(resolve, 150));
        assert.equal(saved.length, 1);
        assert.ok(saved[0].children.every((c: any) => c != null));
        assert.deepEqual(
            saved[0].savedLeafOrder.map((e: any) => e.blockid).sort(),
            [status.data.blockId, manager.data.blockId].sort()
        );
    });

    test("keeps a leaf the tab list does not name yet when its block exists (a block just created)", async () => {
        const { root } = homeTree();
        const { model } = modelWith(root, ["c5b03b0c", "a008b469"], ["7193ba65"]);
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["7193ba65", "a008b469", "c5b03b0c"]);
    });

    test("keeps the leaf when the backend cannot be asked", async () => {
        const { root } = homeTree();
        const { model } = modelWith(root, ["c5b03b0c", "a008b469"]);
        model.blockExists = async () => undefined;
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["7193ba65", "a008b469", "c5b03b0c"]);
    });

    test("a block created and listed while the backend is asked keeps its leaf, though the answer says it was missing", async () => {
        const { root, orphan } = homeTree();
        const { model } = modelWith(root, ["c5b03b0c", "a008b469"]);
        model.blockExists = async () => {
            model.setTabBlockIds(["c5b03b0c", "a008b469", "7193ba65"]);
            return false;
        };
        await model.processPendingBackendActions();
        assert.ok(findNode(model.treeState.rootNode, orphan.id));
    });

    test("a leaf that was replaced while the backend was asked is not removed", async () => {
        const { root, orphan } = homeTree();
        const { model } = modelWith(root, ["c5b03b0c", "a008b469"]);
        model.blockExists = async () => {
            orphan.data = { blockId: "other" };
            return false;
        };
        await model.processPendingBackendActions();
        assert.ok(findNode(model.treeState.rootNode, orphan.id));
    });

    test("leaves a tree alone when every leaf has its block", async () => {
        const { root } = homeTree();
        const { model } = modelWith(root, ["7193ba65", "c5b03b0c", "a008b469"]);
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["7193ba65", "a008b469", "c5b03b0c"]);
    });

    test("does not touch the tree while the tab's block list is not loaded", async () => {
        const { root } = homeTree();
        const { model } = modelWith(root, undefined);
        await model.processPendingBackendActions();
        assert.deepEqual(blockIdsOf(model.treeState.rootNode).sort(), ["7193ba65", "a008b469", "c5b03b0c"]);
    });
});

describe("balanceNode and holes", () => {
    test("an empty stack is dropped before the callback sees the node's children", () => {
        const { root } = homeTree();
        root.children[0].children = [];
        const seen: boolean[] = [];
        balanceNode(root, (n) => n.children?.forEach((c) => seen.push(c != null)));
        assert.ok(seen.every((v) => v));
        assert.equal(root.children.length, 2);
    });

    test("a saved tree with a null child is loaded without throwing, and the null is dropped", () => {
        const { root } = homeTree();
        root.children[0] = null;
        const balanced = balanceNode(root, (n) => n.children?.forEach((c) => c.id));
        assert.equal(balanced.children.length, 2);
        assert.ok(balanced.children.every((c: any) => c != null));
    });
});
