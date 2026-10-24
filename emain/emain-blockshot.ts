// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { FullBlockIdRe, webShot, WebShotResult } from "./emain-webnav";

// A picture of ONE block (any view) taken from the loaded page of its tab, whether or not the tab is shown. A tab that is not
// shown is parked at (-15000, -15000) and Chromium does not composite it there: capturePage gives an empty picture (seen on a
// real Wave, 25 Oct 2026). So for the capture only, its view is put inside the window at the BOTTOM of the child order, where
// the active tab's opaque view covers it completely, and parked again at once. Nothing here calls show, focus or raises a window.

export interface BlockShotDeps {
    // asks the server whether the caller may act on this block (same workspace as the caller)
    authorize: () => Promise<void>;
    blockInfo: () => Promise<any>;
    getWindow: () => any;
    sleep?: (ms: number) => Promise<void>;
    deadlineMs?: number;
}

export const AllowedViews = ["term"];

// The whole capture, move and cleanup included, ends by then: the caller's own timeout (15 s) does not release the tab lock or park the view.
const DefaultDeadlineMs = 12000;

interface CaptureState {
    aborted: boolean;
    moved: boolean;
    throttled: boolean;
}

// How long a view that was just moved inside the window gets to paint, and how often an empty picture is asked for again.
// 150 ms was enough in a real Wave only some of the time (a capture at once fails with UnknownVizError); 300 ms and retries.
const PaintWaitMs = 300;
const ParkedAttempts = 4;

// A tab that was never shown has its blocks at the layout minimum (29x61, 64x64 seen live) until its page has laid out inside the window
// at the right size, which takes a moment and was slow on some pages. A rectangle narrower or lower than this is "not laid out yet".
const MinLaidOutSide = 100;
const LayoutPolls = 20;
const LayoutPollMs = 250;
// A page that Chromium counts as hidden gets no rendering opportunities, so its ResizeObserver (the layout) never runs. Background
// throttling is switched off for the capture, and a throwaway capture (which makes the page visible for a moment) wakes a page that
// is still at the layout minimum after this many looks.
const WakePolls = [3, 8];

// One capture at a time per tab: two overlapping captures must not park the view under the other.
const TabLocks = new Map<string, Promise<unknown>>();

async function withTabLock<T>(tabId: string, fn: () => Promise<T>): Promise<T> {
    const prev = TabLocks.get(tabId) ?? Promise.resolve();
    const run = prev.catch(() => {}).then(fn);
    const tail = run.catch(() => {});
    TabLocks.set(tabId, tail);
    try {
        return await run;
    } finally {
        if (TabLocks.get(tabId) === tail) {
            TabLocks.delete(tabId);
        }
    }
}

export interface BlockShotData {
    workspaceid?: string;
    tabid?: string;
    blockid?: string;
}

// The block's rectangle in the page of its tab, in CSS pixels. The block id goes in as a JSON string, never joined into code.
export function buildRectScript(blockId: string): string {
    return `(() => {
    try {
        const el = document.querySelector('.block[data-blockid=' + JSON.stringify(${JSON.stringify(blockId)}) + ']');
        if (!el) { return null; }
        const r = el.getBoundingClientRect();
        return { x: r.x, y: r.y, width: r.width, height: r.height };
    } catch (error) {
        return null;
    }
})()`;
}

function checkedRect(raw: any): { x: number; y: number; width: number; height: number } {
    const ok = raw != null && typeof raw === "object" && ["x", "y", "width", "height"].every((k) => Number.isFinite(raw[k]));
    if (!ok) {
        throw new Error("the block is not drawn in its tab (is it behind another member of a stack, or is its page not ready?)");
    }
    const rect = { x: Math.round(raw.x), y: Math.round(raw.y), width: Math.round(raw.width), height: Math.round(raw.height) };
    if (rect.width < 1 || rect.height < 1) {
        throw new Error("the block has no size in its tab");
    }
    return rect;
}

async function readRect(wc: any, blockId: string, polls: number, sleep: (ms: number) => Promise<void>, state: CaptureState) {
    const wake = async () => {
        try {
            await wc.capturePage(undefined, { stayAwake: true });
        } catch {
            // only a nudge: the real capture is made, and judged, later
        }
    };
    let last: unknown = null;
    for (let poll = 1; poll <= polls; poll++) {
        if (state.aborted) {
            throw new Error("the capture was given up");
        }
        try {
            const rect = checkedRect(await wc.executeJavaScript(buildRectScript(blockId)));
            if (polls === 1 || (rect.width >= MinLaidOutSide && rect.height >= MinLaidOutSide)) {
                return rect;
            }
            last = new Error(`the block has not been laid out in its tab yet (${rect.width}x${rect.height}); its page is asleep, ask again in a moment`);
        } catch (err) {
            last = err;
        }
        if (WakePolls.includes(poll) && polls > 1) {
            await wake();
        }
        if (poll < polls) {
            await sleep(LayoutPollMs);
        }
    }
    throw last;
}

export async function blockShot(data: BlockShotData, deps: BlockShotDeps): Promise<WebShotResult> {
    if (!data.tabid || !data.blockid || !data.workspaceid) {
        throw new Error("tabid, blockid and workspaceid are required");
    }
    if (!FullBlockIdRe.test(data.blockid)) {
        throw new Error("blockid must be a full block id");
    }
    await deps.authorize();
    const info = await deps.blockInfo();
    if (info == null || info.block == null) {
        throw new Error("the block could not be read");
    }
    if (info.blockid !== data.blockid) {
        throw new Error("the block is not the one named");
    }
    if (info.tabid !== data.tabid) {
        throw new Error("the block is not in the tab named");
    }
    if (info.workspaceid !== data.workspaceid) {
        throw new Error("the tab is not in the workspace named");
    }
    // An allow-list: a view can embed a page with the owner's sign-ins (web, and others that host one), and the crop would include it.
    // More views are added one at a time, each with its reason.
    const viewType = info.block.meta?.view;
    if (!AllowedViews.includes(viewType)) {
        throw new Error(`wsh blockshot takes terminal blocks only (view "term"); this block is "${viewType ?? ""}" (for a web tile use wsh web shot, which has the sign-in rule)`);
    }
    const ww = deps.getWindow();
    if (ww == null) {
        throw new Error(`no window found with workspace ${data.workspaceid}`);
    }
    const view = ww.allLoadedTabViews?.get(data.tabid);
    if (view == null || view.webContents == null) {
        throw new Error(`the tab ${data.tabid} is not loaded in its window (it loads in the background after a start or when a block is made in it; ask again in a moment)`);
    }
    const wc = view.webContents;
    const sleep = deps.sleep ?? ((ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms)));
    return await withTabLock(data.tabid, async () => {
        const parked = typeof view.isOnScreen === "function" && !view.isOnScreen();
        const state: CaptureState = { aborted: false, moved: false, throttled: false };
        const deadlineMs = deps.deadlineMs ?? DefaultDeadlineMs;
        let timer: any;
        const deadline = new Promise<never>((_, reject) => {
            timer = setTimeout(() => {
                state.aborted = true;
                reject(new Error(`capturing the block took longer than ${deadlineMs} ms and was given up (its page may be hung)`));
            }, deadlineMs);
        });
        const work = (async () => {
            if (parked) {
                state.moved = true;
                ww.contentView.addChildView(view, 0);
                view.positionTabOnScreen(ww.getContentBounds());
                state.throttled = true;
                wc.setBackgroundThrottling(false);
                await sleep(PaintWaitMs);
            }
            const rect = await readRect(wc, data.blockid, parked ? LayoutPolls : 1, sleep, state);
            await deps.authorize();
            const attempts = parked ? ParkedAttempts : 1;
            for (let attempt = 1; ; attempt++) {
                if (state.aborted) {
                    throw new Error("the capture was given up");
                }
                try {
                    // stayAwake: a page that Chromium considers hidden or covered still paints for this one capture
                    return await webShot({
                        capturePage: async () => {
                            const image = await wc.capturePage(rect, { stayAwake: true });
                            // a picture that arrives after the deadline is dropped before it is encoded
                            if (state.aborted) {
                                throw new Error("the capture was given up");
                            }
                            return image;
                        },
                    });
                } catch (err) {
                    if (state.aborted || attempt >= attempts) {
                        throw err;
                    }
                    await sleep(PaintWaitMs);
                }
            }
        })();
        work.catch(() => {});
        try {
            return await Promise.race([work, deadline]);
        } finally {
            clearTimeout(timer);
            state.aborted = true;
            // each step on its own: one that throws must not keep the others from running
            if (state.moved && !view.isActiveTab) {
                try {
                    view.positionTabOffScreen(ww.getContentBounds());
                } catch (err) {
                    console.log("blockshot: parking the tab view again failed:", err);
                }
            }
            if (state.throttled) {
                try {
                    wc.setBackgroundThrottling(true);
                } catch (err) {
                    console.log("blockshot: restoring background throttling failed:", err);
                }
            }
        }
    });
}
