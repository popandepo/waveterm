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
        expect(lines[0].startsWith("[error] tab d ")).toBe(true);
        expect(lines[0]).toContain("3 tabs are already pinned");
    });

    test("the pinner the window uses takes 16 tabs and refuses the 17th", () => {
        const lines: string[] = [];
        const pin = makeTabPinner(MaxPinnedTabs, (l) => lines.push(l));
        const results = Array.from({ length: 17 }, (_, i) => pin(view("t" + i)));
        expect(results.filter((r) => r).length).toBe(16);
        expect(results[16]).toBe(false);
        expect(lines).toEqual(["[error] tab t16 is not kept loaded: 16 tabs are already pinned"]);
        const src = readFileSync(path.join(__dirname, "emain-window.ts"), "utf8");
        expect(src).toContain("const pinTabAtBoot = makeTabPinner(MaxPinnedTabs, console.log);");
    });

    test("two views with the same tab id take one slot", () => {
        const pin = makeTabPinner(2, () => {});
        const first = view("a");
        const second = view("a");
        expect(pin(first)).toBe(true);
        expect(pin(second)).toBe(true);
        expect(second.keepLoaded).toBe(true);
        expect(pin(view("b"))).toBe(true);
        expect(pin(view("c"))).toBe(false);
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
        // no other emain file sets a pin: the tab view only declares the field (false) and the cache only reads it
        for (const file of ["emain-tabview.ts", "emain.ts", "emain-wsh.ts", "emain-ipc.ts"]) {
            const other = readFileSync(path.join(__dirname, file), "utf8");
            expect(other.match(/keepLoaded\s*=\s*true/g) ?? []).toEqual([]);
        }
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
