// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { canEvictTabView, preloadAllTabs } from "./preloadall";

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
