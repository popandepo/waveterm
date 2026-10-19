// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { canEvictTabView, makeTabPinner, MaxPinnedTabs, preloadAllTabs } from "./preloadall";

function view(id: string) {
    return { waveTabId: id, keepLoaded: false };
}

describe("pinning the tabs loaded at boot", () => {
    test("the cap is 16", () => {
        expect(MaxPinnedTabs).toBe(16);
    });

    test("tabs are pinned up to the cap, and the first one past it is left unpinned with a log line", () => {
        const lines: string[] = [];
        const pin = makeTabPinner(3, (l) => lines.push(l));
        const views = ["a", "b", "c", "d", "e"].map(view);
        const results = views.map((v) => pin(v));
        expect(results).toEqual([true, true, true, false, false]);
        expect(views.map((v) => v.keepLoaded)).toEqual([true, true, true, false, false]);
        expect(lines.length).toBe(2);
        expect(lines[0]).toContain("tab d");
        expect(lines[0]).toContain("3 tabs are already pinned");
    });

    test("pinning a tab twice counts it once", () => {
        const pin = makeTabPinner(2, () => {});
        const a = view("a");
        expect(pin(a)).toBe(true);
        expect(pin(a)).toBe(true);
        expect(pin(view("b"))).toBe(true);
        expect(pin(view("c"))).toBe(false);
    });

    test("a pin is never released by the pinner", () => {
        const pin = makeTabPinner(1, () => {});
        const a = view("a");
        pin(a);
        pin(view("b"));
        expect(a.keepLoaded).toBe(true);
    });

    test("only the boot pass pins: the window pins the active tab and each tab it preloaded, and nothing else sets keepLoaded", () => {
        const src = readFileSync(path.join(__dirname, "emain-window.ts"), "utf8");
        expect(src.match(/keepLoaded/g) ?? []).toEqual([]);
        const boot = src.slice(src.indexOf("async preloadAllTabs("), src.indexOf("async tabClosedInBackend("));
        expect(boot).toContain("pinTabAtBoot(this.activeTabView)");
        expect(boot).toContain("await this.preloadTab(tabId);");
        expect(boot).toContain("pinTabAtBoot(view)");
        const show = src.slice(src.indexOf("private async setTabViewIntoWindow("), src.indexOf("private finalizePositioning("));
        const preload = src.slice(src.indexOf("async preloadTab("), src.indexOf("async preloadAllTabs("));
        expect(show).not.toContain("pinTabAtBoot");
        expect(preload).not.toContain("pinTabAtBoot");
    });
});

describe("preloading every tab", () => {
    test("loads each tab except the active one, in order and one at a time", async () => {
        const order: string[] = [];
        let running = 0;
        let overlap = false;
        const n = await preloadAllTabs(
            ["a", "b", "c", "d"],
            "b",
            async (id) => {
                running++;
                overlap = overlap || running > 1;
                await new Promise((r) => setTimeout(r, 1));
                order.push(id);
                running--;
            },
            () => {}
        );
        expect(order).toEqual(["a", "c", "d"]);
        expect(n).toBe(3);
        expect(overlap).toBe(false);
    });

    test("a tab that fails is reported and the rest still load", async () => {
        const failed: string[] = [];
        const loaded: string[] = [];
        const n = await preloadAllTabs(
            ["a", "b", "c"],
            "",
            async (id) => {
                if (id === "b") {
                    throw new Error("page failed");
                }
                loaded.push(id);
            },
            (id) => failed.push(id)
        );
        expect(loaded).toEqual(["a", "c"]);
        expect(failed).toEqual(["b"]);
        expect(n).toBe(2);
    });

    test("no tabs is not an error", async () => {
        expect(await preloadAllTabs([], "x", async () => {}, () => {})).toBe(0);
    });
});

describe("which loaded views the cache may drop", () => {
    test("a pinned view is never dropped, however old", () => {
        expect(canEvictTabView({ isActiveTab: false, keepLoaded: true }, 10 * 60 * 1000)).toBe(false);
    });
    test("the active tab is never dropped", () => {
        expect(canEvictTabView({ isActiveTab: true, keepLoaded: false }, 10 * 60 * 1000)).toBe(false);
    });
    test("an ordinary idle view is dropped, a just-used one is not", () => {
        expect(canEvictTabView({ isActiveTab: false, keepLoaded: false }, 5000)).toBe(true);
        expect(canEvictTabView({ isActiveTab: false, keepLoaded: false }, 200)).toBe(false);
    });
});
