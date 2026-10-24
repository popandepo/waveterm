// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { buildRectScript, blockShot, BlockShotDeps } from "./emain-blockshot";

const BlockId = "11111111-2222-3333-4444-555555555555";
const TabId = "66666666-7777-8888-9999-000000000000";
const WorkspaceId = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee";
const Data = { workspaceid: WorkspaceId, tabid: TabId, blockid: BlockId };

function fakeImage(opts: { empty?: boolean; bytes?: number } = {}) {
    return {
        isEmpty: () => !!opts.empty,
        toPNG: () => Buffer.alloc(opts.bytes ?? 3, 1),
        getSize: () => ({ width: 100, height: 50 }),
        resize: () => fakeImage(opts),
    };
}

// A window whose every way of showing, focusing or raising a view throws: the capture must never reach for them.
function setup(over: { rect?: any; image?: any; info?: any; calls?: string[]; windowGone?: boolean; tabLoaded?: boolean } = {}) {
    const calls = over.calls ?? [];
    const captured: any[] = [];
    const scripts: string[] = [];
    const wc = {
        executeJavaScript: async (script: string) => {
            calls.push("rect");
            scripts.push(script);
            return over.rect === undefined ? { x: 10.4, y: 20.6, width: 300.2, height: 120.7 } : over.rect;
        },
        capturePage: async (rect: any, opts: any) => {
            calls.push("capture");
            captured.push({ rect, opts });
            return over.image === undefined ? fakeImage() : over.image;
        },
        focus: () => {
            throw new Error("focus must not be called");
        },
    };
    const view = {
        webContents: wc,
        setBounds: () => {
            throw new Error("a view must not be moved");
        },
    };
    const ww = {
        allLoadedTabViews: new Map<string, any>(over.tabLoaded === false ? [] : [[TabId, view]]),
        show: () => {
            throw new Error("show must not be called");
        },
        focus: () => {
            throw new Error("window focus must not be called");
        },
    };
    const deps: BlockShotDeps = {
        authorize: async () => {
            calls.push("authorize");
        },
        blockInfo: async () => {
            calls.push("info");
            return over.info === undefined ? { blockid: BlockId, tabid: TabId, workspaceid: WorkspaceId, block: { meta: { view: "term" } } } : over.info;
        },
        getWindow: () => (over.windowGone ? null : (ww as any)),
    };
    return { deps, calls, captured, scripts };
}

describe("blockShot", () => {
    test("takes the block's rectangle from the loaded tab and captures exactly it, awake and without showing anything", async () => {
        const s = setup();
        const res = await blockShot(Data, s.deps);
        expect(s.calls).toEqual(["authorize", "info", "rect", "authorize", "capture"]);
        expect(s.captured[0].rect).toEqual({ x: 10, y: 21, width: 300, height: 121 });
        expect(s.captured[0].opts).toEqual({ stayAwake: true });
        expect(res.width).toBe(100);
        expect(res.height).toBe(50);
        expect(Buffer.from(res.png, "base64").length).toBe(3);
    });

    test("works for a block that is not a web tile (a terminal)", async () => {
        const s = setup({ info: { blockid: BlockId, tabid: TabId, workspaceid: WorkspaceId, block: { meta: { view: "term" } } } });
        await expect(blockShot(Data, s.deps)).resolves.toBeTruthy();
    });

    test("refuses missing ids and a short block id before anything is asked", async () => {
        for (const bad of [{ ...Data, tabid: "" }, { ...Data, blockid: "" }, { ...Data, workspaceid: "" }, { ...Data, blockid: "11111111" }]) {
            const s = setup();
            await expect(blockShot(bad, s.deps)).rejects.toThrow();
            expect(s.calls).toEqual([]);
        }
    });

    test("a caller that is not allowed stops everything", async () => {
        const s = setup();
        s.deps.authorize = async () => {
            throw new Error("that web tile is not available to this caller");
        };
        await expect(blockShot(Data, s.deps)).rejects.toThrow("not available to this caller");
        expect(s.calls).toEqual([]);
    });

    test("refuses a block record that is not the one named, not in the tab named, or not in the workspace named", async () => {
        for (const info of [
            { blockid: "other", tabid: TabId, workspaceid: WorkspaceId, block: {} },
            { blockid: BlockId, tabid: "other", workspaceid: WorkspaceId, block: {} },
            { blockid: BlockId, tabid: TabId, workspaceid: "other", block: {} },
            { blockid: BlockId, tabid: TabId, workspaceid: WorkspaceId },
            null,
        ]) {
            const s = setup({ info });
            await expect(blockShot(Data, s.deps)).rejects.toThrow();
            expect(s.calls).not.toContain("rect");
            expect(s.calls).not.toContain("capture");
        }
    });

    test("no window, or a tab that is not loaded, is an error that says to wait for the tab to load and never loads or shows it", async () => {
        const gone = setup({ windowGone: true });
        await expect(blockShot(Data, gone.deps)).rejects.toThrow("no window");
        const unloaded = setup({ tabLoaded: false });
        await expect(blockShot(Data, unloaded.deps)).rejects.toThrow("not loaded");
        expect(unloaded.calls).not.toContain("capture");
    });

    test("a block with no size (not drawn, or behind another stack member) is an error and nothing is captured", async () => {
        for (const rect of [null, undefined, { x: 0, y: 0, width: 0, height: 100 }, { x: 0, y: 0, width: 100, height: 0 }, { x: "a", y: 0, width: 5, height: 5 }, "nope"]) {
            const s = setup({ rect: rect === undefined ? null : rect });
            await expect(blockShot(Data, s.deps)).rejects.toThrow();
            expect(s.calls).not.toContain("capture");
        }
    });

    test("an empty picture is an error, never a blank file", async () => {
        const s = setup({ image: fakeImage({ empty: true }) });
        await expect(blockShot(Data, s.deps)).rejects.toThrow("empty");
        const none = setup({ image: null });
        await expect(blockShot(Data, none.deps)).rejects.toThrow("empty");
    });

    test("the script gets the block id as a JSON string and looks the block up by data-blockid", () => {
        const script = buildRectScript(BlockId);
        expect(script).toContain(JSON.stringify(BlockId));
        expect(script).toContain("data-blockid");
        expect(script).toContain("getBoundingClientRect");
        // an id with a quote cannot break out of the string
        expect(buildRectScript('x"); alert(1); ("')).toContain(JSON.stringify('x"); alert(1); ("'));
    });
});
