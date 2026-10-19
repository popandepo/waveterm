// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { canEvictTabView, isSessionBlock, preloadAllTabs, tabHoldsSessionBlock } from "./preloadall";

const SessionA = "3c641601-9a7a-44be-a93c-e5a89419429b";

function objects(tab: string[], blocks: Record<string, any>) {
    return async (oref: string) => {
        if (oref === "tab:t1") {
            return { blockids: tab };
        }
        if (oref.startsWith("block:") && oref.slice(6) in blocks) {
            return blocks[oref.slice(6)];
        }
        throw new Error("no such object " + oref);
    };
}

describe("which tabs are kept loaded", () => {
    test("a block is a session block only with a lowercase uuid in session:id", () => {
        expect(isSessionBlock({ meta: { "session:id": SessionA } })).toBe(true);
        expect(isSessionBlock({ meta: { "session:id": SessionA.toUpperCase() } })).toBe(false);
        expect(isSessionBlock({ meta: { "session:id": "not-a-uuid" } })).toBe(false);
        expect(isSessionBlock({ meta: { "session:id": 7 } })).toBe(false);
        expect(isSessionBlock({ meta: {} })).toBe(false);
        expect(isSessionBlock({})).toBe(false);
        expect(isSessionBlock(null)).toBe(false);
    });

    test("a tab holds a session block when any of its blocks is one", async () => {
        const get = objects(["a", "b", "c"], {
            a: { meta: { view: "term" } },
            b: { meta: { view: "web" } },
            c: { meta: { view: "term", "session:id": SessionA } },
        });
        expect(await tabHoldsSessionBlock(get, "t1")).toBe(true);
    });

    test("a tab of plain shells, or an empty tab, is not pinned", async () => {
        expect(await tabHoldsSessionBlock(objects(["a"], { a: { meta: { view: "term" } } }), "t1")).toBe(false);
        expect(await tabHoldsSessionBlock(objects([], {}), "t1")).toBe(false);
    });

    test("an unreadable object is an error for the caller to decide on, not a quiet false", async () => {
        await expect(tabHoldsSessionBlock(objects(["missing"], {}), "t1")).rejects.toThrow();
    });

    test("the window decides the pin from the tab's blocks at every load and every show, and never pins unconditionally", () => {
        const src = readFileSync(path.join(__dirname, "emain-window.ts"), "utf8");
        const pins = src.match(/keepLoaded = (.*);/g) ?? [];
        expect(pins).toEqual(["keepLoaded = await tabHoldsSessionBlock((oref) => ObjectService.GetObject(oref), tabView.waveTabId);", "keepLoaded = true;"]);
        const preload = src.slice(src.indexOf("async preloadTab("), src.indexOf("async preloadAllTabs("));
        expect(preload).toContain("await this.pinIfSessionTab(tabView);");
        const show = src.slice(src.indexOf("private async setTabViewIntoWindow("), src.indexOf("private finalizePositioning("));
        expect(show).toContain("this.pinIfSessionTab(tabView)");
        const catchBlock = src.slice(src.indexOf("private async pinIfSessionTab("), src.indexOf("async preloadAllTabs("));
        expect(catchBlock.indexOf("keepLoaded = true;")).toBeGreaterThan(catchBlock.indexOf("catch (err)"));
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
    test("a background-loaded view is never dropped, however old", () => {
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
