// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { buildRectScript, blockShot, BlockShotDeps } from "./emain-blockshot";

const BlockId = "11111111-2222-3333-4444-555555555555";
const TabId = "66666666-7777-8888-9999-000000000000";
const WorkspaceId = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee";
const Data = { workspaceid: WorkspaceId, tabid: TabId, blockid: BlockId };

const Encoded = { n: 0 };

function fakeImage(opts: { empty?: boolean; bytes?: number } = {}) {
    return {
        isEmpty: () => !!opts.empty,
        toPNG: () => {
            Encoded.n++;
            return Buffer.alloc(opts.bytes ?? 3, 1);
        },
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
        rects?: any[];
        info?: any;
        calls?: string[];
        windowGone?: boolean;
        tabLoaded?: boolean;
        parked?: boolean;
        becomesActive?: boolean;
        positionThrows?: boolean;
        throttleThrows?: boolean;
        offThrows?: boolean;
        hangCapture?: boolean;
        hangRect?: boolean;
        blocks?: any[];
        overlay?: boolean;
    } = {}
) {
    const calls = over.calls ?? [];
    const releases: (() => void)[] = [];
    const captured: any[] = [];
    const scripts: string[] = [];
    const wc = {
        executeJavaScript: async (script: string) => {
            calls.push("rect");
            if (over.hangRect) {
                return new Promise(() => {});
            }
            scripts.push(script);
            if (over.rects) {
                const r = over.rects.length > 1 ? over.rects.shift() : over.rects[0];
            return over.overlay && r ? { ...r, overlay: true } : r;
            }
            const r = over.rect === undefined ? { x: 10.4, y: 20.6, width: 300.2, height: 120.7 } : over.rect;
            return over.overlay && r ? { ...r, overlay: true } : r;
        },
        setBackgroundThrottling: (on: boolean) => {
            if (!over.parked) {
                throw new Error("a shown tab's throttling must not be touched");
            }
            calls.push("throttle:" + on);
            if (over.throttleThrows && on === false) {
                throw new Error("throttle failed");
            }
        },
        capturePage: async (rect: any, opts: any) => {
            if (rect === undefined) {
                calls.push("kick");
                return fakeImage();
            }
            calls.push("capture");
            captured.push({ rect, opts });
            if (over.hangCapture) {
                return new Promise((resolve) => {
                    releases.push(() => resolve(fakeImage()));
                });
            }
            if (over.becomesActive) {
                view.isActiveTab = true;
            }
            if (over.images) {
                const next = over.images.shift();
                if (next instanceof Error) {
                    throw next;
                }
                return next;
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
                  if (over.positionThrows) {
                      throw new Error("position failed");
                  }
              }
            : moved,
        positionTabOffScreen: over.parked
            ? () => {
                  calls.push("off-screen");
                  if (over.offThrows) {
                      throw new Error("park failed");
                  }
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
        tabBlocks: async () => over.blocks ?? [{ blockid: BlockId, meta: { view: "term" } }],
        getWindow: () => (over.windowGone ? null : (ww as any)),
        sleep: async () => {
            calls.push("sleep");
        },
    };
    return { deps, calls, captured, scripts, releases, over };
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
        expect(s.calls).toEqual(["authorize", "info", "below:0", "on-screen", "throttle:false", "sleep", "rect", "authorize", "capture", "off-screen", "throttle:true"]);
    });

    test("a parked tab is parked again also when the capture fails", async () => {
        const s = setup({ parked: true, image: fakeImage({ empty: true }) });
        await expect(blockShot(Data, s.deps)).rejects.toThrow("empty");
        expect(s.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
    });

    test("an empty first picture of a parked tab is asked for again, a few times, then it is an error", async () => {
        const later = setup({ parked: true, images: [fakeImage({ empty: true }), fakeImage({ empty: true }), fakeImage()] });
        await expect(blockShot(Data, later.deps)).resolves.toBeTruthy();
        expect(later.calls.filter((c) => c === "capture").length).toBe(3);
        const never = setup({ parked: true, images: Array.from({ length: 10 }, () => fakeImage({ empty: true })) });
        await expect(blockShot(Data, never.deps)).rejects.toThrow("empty");
        expect(never.calls.filter((c) => c === "capture").length).toBe(4);
    });

    test("a transient compositor error (UnknownVizError, seen right after the move) is retried for a parked tab, not for a shown one", async () => {
        const viz = () => new Error("UnknownVizError");
        const parked = setup({ parked: true, images: [viz(), viz(), fakeImage()] });
        await expect(blockShot(Data, parked.deps)).resolves.toBeTruthy();
        expect(parked.calls.filter((c) => c === "capture").length).toBe(3);
        const shown = setup({ images: [viz(), fakeImage()] });
        await expect(blockShot(Data, shown.deps)).rejects.toThrow("UnknownVizError");
        expect(shown.calls.filter((c) => c === "capture").length).toBe(1);
        const always = setup({ parked: true, images: Array.from({ length: 10 }, viz) });
        await expect(blockShot(Data, always.deps)).rejects.toThrow("UnknownVizError");
        expect(always.calls.filter((c) => c === "capture").length).toBe(4);
    });

    test("a parked tab whose page has not laid the block out yet (the layout minimum) is polled until it has, and the captured rectangle is the laid-out one", async () => {
        const tiny = { x: 0, y: 0, width: 29, height: 61 };
        const big = { x: 5, y: 6, width: 974, height: 1161 };
        const s = setup({ parked: true, rects: [tiny, tiny, null, big] });
        await blockShot(Data, s.deps);
        expect(s.calls.filter((c) => c === "rect").length).toBe(4);
        expect(s.captured[0].rect).toEqual(big);
        expect(s.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
    });

    test("a page that stays at the layout minimum gets a throwaway capture to wake it, once at the third look and again at the eighth", async () => {
        const tiny = { x: 0, y: 0, width: 29, height: 61 };
        const big = { x: 5, y: 6, width: 974, height: 1161 };
        const woken = setup({ parked: true, rects: [tiny, tiny, tiny, big] });
        await blockShot(Data, woken.deps);
        expect(woken.calls.filter((c) => c === "kick").length).toBe(1);
        const later = setup({ parked: true, rects: [...Array.from({ length: 9 }, () => tiny), big] });
        await blockShot(Data, later.deps);
        expect(later.calls.filter((c) => c === "kick").length).toBe(2);
        const quick = setup({ parked: true });
        await blockShot(Data, quick.deps);
        expect(quick.calls).not.toContain("kick");
    });

    test("a parked tab that never gets laid out is an error that says so, nothing is captured, and the tab is parked again", async () => {
        const s = setup({ parked: true, rects: [{ x: 0, y: 0, width: 29, height: 61 }] });
        await expect(blockShot(Data, s.deps)).rejects.toThrow("laid out");
        expect(s.calls).not.toContain("capture");
        expect(s.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
    });

    test("a shown tab is not polled: its rectangle is taken as it is, small or not", async () => {
        const s = setup({ rects: [{ x: 0, y: 0, width: 29, height: 61 }] });
        const res = await blockShot(Data, s.deps);
        expect(s.calls.filter((c) => c === "rect").length).toBe(1);
        expect(s.captured[0].rect).toEqual({ x: 0, y: 0, width: 29, height: 61 });
        expect(res.width).toBe(100);
    });

    test("a failure while moving the view in is cleaned up: the view is parked again", async () => {
        const pos = setup({ parked: true, positionThrows: true });
        await expect(blockShot(Data, pos.deps)).rejects.toThrow("position failed");
        expect(pos.calls).toContain("off-screen");
        expect(pos.calls).not.toContain("throttle:true");
        const thr = setup({ parked: true, throttleThrows: true });
        await expect(blockShot(Data, thr.deps)).rejects.toThrow("throttle failed");
        expect(thr.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
    });

    test("every cleanup step runs even when another one throws", async () => {
        const s = setup({ parked: true, offThrows: true });
        await expect(blockShot(Data, s.deps)).resolves.toBeTruthy();
        expect(s.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
    });

    test("a hung capture ends at the deadline, goes through the cleanup, a late answer is dropped unencoded, and the same tab then works for the next caller", async () => {
        const s = setup({ parked: true, hangCapture: true });
        s.deps.deadlineMs = 400;
        Encoded.n = 0;
        let settled = false;
        const first = blockShot(Data, s.deps).then(
            () => (settled = true),
            () => (settled = true)
        );
        // the capture is really pending (reached, not answered) before the deadline can fire
        for (let i = 0; i < 100 && s.releases.length === 0; i++) {
            await new Promise((r) => setTimeout(r, 2));
        }
        expect(s.releases.length).toBe(1);
        expect(s.calls).toContain("capture");
        expect(settled).toBe(false);
        await first;
        expect(settled).toBe(true);
        expect(s.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
        const before = s.calls.length;
        s.releases.forEach((r) => r());
        await new Promise((r) => setTimeout(r, 20));
        expect(Encoded.n).toBe(0);
        expect(s.calls.length).toBe(before);
        // the same view, tab and lock: the next caller gets its picture
        s.over.hangCapture = false;
        s.deps.deadlineMs = 5000;
        await expect(blockShot(Data, s.deps)).resolves.toBeTruthy();
        expect(Encoded.n).toBe(1);
    });

    test("a hung page script (the rectangle) ends at the deadline too, and nothing is captured late", async () => {
        const s = setup({ parked: true, hangRect: true });
        s.deps.deadlineMs = 30;
        await expect(blockShot(Data, s.deps)).rejects.toThrow("took longer");
        expect(s.calls).not.toContain("capture");
        expect(s.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
    });

    test("only a terminal block is photographed: every other view is refused by name before anything is moved or captured", async () => {
        for (const view of ["web", "preview", "waveai", "sysinfo", "tsunami", "launcher", "", undefined]) {
            const meta: any = view === undefined ? {} : { view };
            const s = setup({ parked: true, info: { blockid: BlockId, tabid: TabId, workspaceid: WorkspaceId, block: { meta } } });
            await expect(blockShot(Data, s.deps)).rejects.toThrow("terminal blocks only");
            expect(s.calls).toEqual(["authorize", "info"]);
        }
    });

    test("a tab that also holds a block that is not a terminal is refused, with the reason, before anything is moved", async () => {
        const web = { blockid: "web-1", meta: { view: "web" } };
        const term = { blockid: BlockId, meta: { view: "term" } };
        for (const blocks of [[term, web], [term, { blockid: "x", meta: {} }], [term, { blockid: "y", meta: { view: "sysinfo" } }]]) {
            const s = setup({ parked: true, blocks });
            await expect(blockShot(Data, s.deps)).rejects.toThrow("also holds");
            expect(s.calls).toEqual(["authorize", "info"]);
        }
        const only = setup({ parked: true, blocks: [term, { blockid: "t2", meta: { view: "term" } }] });
        await expect(blockShot(Data, only.deps)).resolves.toBeTruthy();
    });

    test("a magnified or floating block over the tab is refused with the reason and the view is parked again", async () => {
        const s = setup({ parked: true, overlay: true });
        await expect(blockShot(Data, s.deps)).rejects.toThrow("magnified or floating");
        expect(s.calls).not.toContain("capture");
        expect(s.calls.slice(-2)).toEqual(["off-screen", "throttle:true"]);
        const shown = setup({ overlay: true });
        await expect(blockShot(Data, shown.deps)).rejects.toThrow("magnified or floating");
    });

    test("the rectangle script reports another block that is magnified or ephemeral", () => {
        const script = buildRectScript(BlockId);
        expect(script).toContain("magnified");
        expect(script).toContain("ephemeral");
        expect(script).toContain("overlay");
    });

    test("a tab that became the active tab during the capture is left where the window put it", async () => {
        const s = setup({ parked: true, becomesActive: true });
        await blockShot(Data, s.deps);
        expect(s.calls).not.toContain("off-screen");
        expect(s.calls[s.calls.length - 1]).toBe("throttle:true");
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
        expect(script).toContain(".block[data-blockid=");
        expect(script).toContain("getBoundingClientRect");
        // an id with a quote cannot break out of the string
        expect(buildRectScript('x"); alert(1); ("')).toContain(JSON.stringify('x"); alert(1); ("'));
    });
});
