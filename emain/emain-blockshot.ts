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
}

// How long a view that was just moved inside the window gets to paint, and how often an empty picture is asked for again.
// 150 ms was enough in a real Wave only some of the time (a capture at once fails with UnknownVizError); 300 ms and retries.
const PaintWaitMs = 300;
const ParkedAttempts = 4;

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
        const el = document.querySelector('[data-blockid=' + JSON.stringify(${JSON.stringify(blockId)}) + ']');
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
        if (parked) {
            ww.contentView.addChildView(view, 0);
            view.positionTabOnScreen(ww.getContentBounds());
            await sleep(PaintWaitMs);
        }
        try {
            const rect = checkedRect(await wc.executeJavaScript(buildRectScript(data.blockid)));
            await deps.authorize();
            const attempts = parked ? ParkedAttempts : 1;
            for (let attempt = 1; ; attempt++) {
                try {
                    // stayAwake: a page that Chromium considers hidden or covered still paints for this one capture
                    return await webShot({ capturePage: () => wc.capturePage(rect, { stayAwake: true }) });
                } catch (err) {
                    if (attempt >= attempts) {
                        throw err;
                    }
                    await sleep(PaintWaitMs);
                }
            }
        } finally {
            // a tab that became the active one meanwhile is where the window put it: parking it would blank the screen
            if (parked && !view.isActiveTab) {
                view.positionTabOffScreen(ww.getContentBounds());
            }
        }
    });
}
