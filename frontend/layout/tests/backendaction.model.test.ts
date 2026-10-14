// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom, createStore } from "jotai";
import { assert, describe, test, vi } from "vitest";

// The layout model reads the store only through these modules; the stand-ins keep it in memory.
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
import { newLayoutNode } from "../lib/layoutNode";
import { DefaultNodeSize } from "../lib/types";

function modelWithOneTile() {
    const store = createStore();
    const tabAtom = atom({ oid: "tab-1", layoutstate: "layout-1" } as any);
    const model = new LayoutModel(tabAtom, store.get, store.set);
    const left = newLayoutNode(undefined, undefined, undefined, { blockId: "left" });
    (model as any).treeState = { rootNode: left, pendingBackendActions: [] };
    // the model finds a target through its list of leaves, which the page keeps up to date
    store.set((model as any).leafs, [left]);
    (model as any).persistToBackend = () => {};
    return model;
}

function weights(model: any): number[] {
    const root = model.treeState.rootNode;
    return (root.children ?? [root]).map((n: any) => n.size);
}

describe("the layout model applies the weight of a queued backend action", () => {
    for (const actiontype of ["splithorizontal", "splitvertical"]) {
        test(`${actiontype} with nodesize 4 gives the new tile weight 4 next to a default tile`, async () => {
            const model: any = modelWithOneTile();
            await model.handleBackendAction({
                actiontype,
                actionid: "a1",
                blockid: "added",
                targetblockid: "left",
                position: "after",
                nodesize: 4,
            });
            assert.deepEqual(weights(model), [DefaultNodeSize, 4]);
        });
    }

    test("without a nodesize the new tile has the default weight", async () => {
        const model: any = modelWithOneTile();
        await model.handleBackendAction({
            actiontype: "splithorizontal",
            actionid: "a2",
            blockid: "added",
            targetblockid: "left",
            position: "after",
        });
        assert.deepEqual(weights(model), [DefaultNodeSize, DefaultNodeSize]);
    });

    // the plain insert is where the weight used to be dropped (the handler built the node without it)
    test("a plain insert with nodesize 4 gives the new tile weight 4", async () => {
        const model: any = modelWithOneTile();
        await model.handleBackendAction({ actiontype: "insert", actionid: "a3", blockid: "added", nodesize: 4 });
        assert.deepEqual(weights(model), [DefaultNodeSize, 4]);
    });

    test("a plain insert without a nodesize gives the default weight", async () => {
        const model: any = modelWithOneTile();
        await model.handleBackendAction({ actiontype: "insert", actionid: "a4", blockid: "added" });
        assert.deepEqual(weights(model), [DefaultNodeSize, DefaultNodeSize]);
    });
});
