// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, test, vi } from "vitest";

const { once } = vi.hoisted(() => ({ once: vi.fn() }));
vi.mock("electron", () => ({ ipcMain: { once }, webContents: { fromId: vi.fn() } }));

import { getWebContentsByBlockId } from "./emain-web";

describe("the lookup of a web tile's page, run", () => {
    test("a named tab that is not loaded answers null at once and asks no page, not even the shown one", async () => {
        const shownSend = vi.fn();
        const ww: any = { allLoadedTabViews: new Map(), activeTabView: { webContents: { send: shownSend } } };
        const answer = await Promise.race([
            getWebContentsByBlockId(ww, "tab-unloaded", "block-1"),
            new Promise((resolve) => setTimeout(() => resolve("no answer in time"), 200)),
        ]);
        assert.isNull(answer);
        assert.equal(shownSend.mock.calls.length, 0, "the shown tab is never asked in place of the named one");
        assert.equal(once.mock.calls.length, 0, "nothing waits for a reply");
    });

    test("a loaded named tab is the one asked", () => {
        const namedSend = vi.fn();
        const shownSend = vi.fn();
        const ww: any = {
            allLoadedTabViews: new Map([["tab-a", { webContents: { send: namedSend } }]]),
            activeTabView: { webContents: { send: shownSend } },
        };
        getWebContentsByBlockId(ww, "tab-a", "block-1").catch(() => {});
        assert.equal(namedSend.mock.calls.length, 1);
        assert.equal(namedSend.mock.calls[0][1], "block-1");
        assert.equal(shownSend.mock.calls.length, 0);
    });
});
