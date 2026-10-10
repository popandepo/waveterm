// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { makeCloseTabHandler } from "./closetabhandler";

function setup(opts: { blockCount?: number | null; dialogResponse?: number | "pending"; confirmSetting?: boolean }) {
    const closed: string[] = [];
    const dialogs: string[] = [];
    let release: (n: number) => void = () => {};
    const handler = makeCloseTabHandler({
        getBlockCount: async () => (opts.blockCount === undefined ? 2 : opts.blockCount),
        showDialog: (message) => {
            dialogs.push(message);
            if (opts.dialogResponse === "pending") {
                return new Promise<number>((resolve) => (release = resolve));
            }
            return Promise.resolve(opts.dialogResponse ?? 0);
        },
        closeTab: async (tabId) => {
            closed.push(tabId);
        },
    });
    return { handler, closed, dialogs, release: (n: number) => release(n) };
}

describe("the close-tab handler", () => {
    test("a populated tab stays after Cancel", async () => {
        const t = setup({ blockCount: 3, dialogResponse: 0 });
        expect(await t.handler("tab1", false)).toBe(false);
        expect(t.closed).toEqual([]);
        expect(t.dialogs[0]).toContain("3 blocks");
    });

    test("only the explicit Close Tab answer closes the tab", async () => {
        const yes = setup({ blockCount: 3, dialogResponse: 1 });
        expect(await yes.handler("tab1", false)).toBe(true);
        expect(yes.closed).toEqual(["tab1"]);
        for (const odd of [-1, 2, 7, NaN, undefined as unknown as number]) {
            const t = setup({ blockCount: 3, dialogResponse: odd });
            expect(await t.handler("tab1", false)).toBe(false);
            expect(t.closed).toEqual([]);
        }
    });

    test("an empty tab closes without a dialog", async () => {
        const t = setup({ blockCount: 0 });
        expect(await t.handler("tab1", false)).toBe(true);
        expect(t.dialogs).toEqual([]);
        expect(t.closed).toEqual(["tab1"]);
    });

    test("an unreadable tab asks instead of closing", async () => {
        const t = setup({ blockCount: null, dialogResponse: 0 });
        expect(await t.handler("tab1", false)).toBe(false);
        expect(t.dialogs.length).toBe(1);
    });

    test("a second close request for the same tab while the dialog is open is ignored", async () => {
        const t = setup({ blockCount: 2, dialogResponse: "pending" });
        const first = t.handler("tab1", false);
        await Promise.resolve();
        await Promise.resolve();
        expect(await t.handler("tab1", false)).toBe(false);
        expect(t.dialogs.length).toBe(1);
        t.release(1);
        expect(await first).toBe(true);
        expect(t.closed).toEqual(["tab1"]);
    });

    test("another tab can be asked about while one dialog is open, and the guard is released after the answer", async () => {
        const t = setup({ blockCount: 2, dialogResponse: 0 });
        expect(await t.handler("tab1", false)).toBe(false);
        expect(await t.handler("tab1", false)).toBe(false);
        expect(t.dialogs.length).toBe(2);
    });
});
