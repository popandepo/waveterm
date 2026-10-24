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
function setup(
    over: {
        rect?: any;
        image?: any;
        images?: any[];
        info?: any;
        calls?: string[];
        windowGone?: boolean;
        tabLoaded?: boolean;
        parked?: boolean;
        becomesActive?: boolean;
    } = {}
) {
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
            if (over.becomesActive) {
                view.isActiveTab = true;
            }
            if (over.images) {
                return over.images.shift();
            }
            return over.image === undefined ? fakeImage() : over.image;
        },
        focus: () => {
            throw new Error("focus must not be called");
        },
    };
    const moved = () => {
        throw new Error("an on-screen view must not be moved");
    };
    const view: any = {
        webContents: wc,
        isActiveTab: !over.parked,
        isOnScreen: () => !over.parked,
        positionTabOnScreen: over.parked
            ? () => {
                  calls.push("on-screen");
              }
            : moved,
        positionTabOffScreen: over.parked
            ? () => {
                  calls.push("off-screen");
              }
            : moved,
        setBounds: () => {
            throw new Error("setBounds is only reached through positionTabOnScreen/OffScreen");
        },
    };
    const ww = {
        allLoadedTabViews: new Map<string, any>(over.tabLoaded === false ? [] : [[TabId, view]]),
        getContentBounds: () => ({ x: 0, y: 0, width: 800, height: 600 }),
        contentView: {
            addChildView: (v: any, index: number) => {
                calls.push("below:" + index);
                if (v !== view) {
                    throw new Error("only the target view is reordered");
                }
            },
        },
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
        sleep: async () => {
            calls.push("sleep");
        },
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

    test("a parked tab is put inside the window BEHIND the others for the capture, then parked again, and nothing is shown or focused", async () => {
        const s = setup({ parked: true });
        const res = await blockShot(Data, s.deps);
        expect(res.width).toBe(100);
        expect(s.calls).toEqual(["authorize", "info", "below:0", "on-screen", "sleep", "rect", "authorize", "capture", "off-screen"]);
    });

    test("a parked tab is parked again also when the capture fails", async () => {
        const s = setup({ parked: true, image: fakeImage({ empty: true }) });
        await expect(blockShot(Data, s.deps)).rejects.toThrow("empty");
        expect(s.calls[s.calls.length - 1]).toBe("off-screen");
    });

    test("an empty first picture of a parked tab is asked for again, a few times, then it is an error", async () => {
        const later = setup({ parked: true, images: [fakeImage({ empty: true }), fakeImage({ empty: true }), fakeImage()] });
        await expect(blockShot(Data, later.deps)).resolves.toBeTruthy();
        expect(later.calls.filter((c) => c === "capture").length).toBe(3);
        const never = setup({ parked: true, images: Array.from({ length: 10 }, () => fakeImage({ empty: true })) });
        await expect(blockShot(Data, never.deps)).rejects.toThrow("empty");
        expect(never.calls.filter((c) => c === "capture").length).toBe(4);
    });

    test("a tab that became the active tab during the capture is left where the window put it", async () => {
        const s = setup({ parked: true, becomesActive: true });
        await blockShot(Data, s.deps);
        expect(s.calls).not.toContain("off-screen");
    });

    test("two captures of the same tab at once do not move it at the same time", async () => {
        const s = setup({ parked: true });
        await Promise.all([blockShot(Data, s.deps), blockShot(Data, s.deps)]);
        const seq = s.calls.filter((c) => c === "on-screen" || c === "off-screen");
        expect(seq).toEqual(["on-screen", "off-screen", "on-screen", "off-screen"]);
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
