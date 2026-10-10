// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { CloseTabCountUnknown, CloseTabNoLimit, makeCloseTabHandler, settlePendingCloses } from "./closetabhandler";
import { wshCloseRefusal } from "./closetabprompt";

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
            return true;
        },
    });
    return { handler, closed, dialogs, release: (n: number) => release(n) };
}

describe("the close-tab handler", () => {
    test("a populated tab stays after Cancel", async () => {
        const t = setup({ blockCount: 3, dialogResponse: 0 });
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(t.closed).toEqual([]);
        expect(t.dialogs[0]).toContain("3 blocks");
    });

    test("only the explicit Close Tab answer closes the tab", async () => {
        const yes = setup({ blockCount: 3, dialogResponse: 1 });
        expect(await yes.handler("tab1", false, undefined)).toBe(true);
        expect(yes.closed).toEqual(["tab1"]);
        for (const odd of [-1, 2, 7, NaN, undefined as unknown as number]) {
            const t = setup({ blockCount: 3, dialogResponse: odd });
            expect(await t.handler("tab1", false, undefined)).toBe(false);
            expect(t.closed).toEqual([]);
        }
    });

    test("an empty tab closes without a dialog", async () => {
        const t = setup({ blockCount: 0 });
        expect(await t.handler("tab1", false, undefined)).toBe(true);
        expect(t.dialogs).toEqual([]);
        expect(t.closed).toEqual(["tab1"]);
    });

    test("an unreadable tab asks instead of closing", async () => {
        const t = setup({ blockCount: null, dialogResponse: 0 });
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(t.dialogs.length).toBe(1);
    });

    test("a second close request for the same tab while the dialog is open is ignored", async () => {
        const t = setup({ blockCount: 2, dialogResponse: "pending" });
        const first = t.handler("tab1", false, undefined);
        await Promise.resolve();
        await Promise.resolve();
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(t.dialogs.length).toBe(1);
        t.release(1);
        expect(await first).toBe(true);
        expect(t.closed).toEqual(["tab1"]);
    });

    test("another tab can be asked about while one dialog is open, and the guard is released after the answer", async () => {
        const t = setup({ blockCount: 2, dialogResponse: 0 });
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(t.dialogs.length).toBe(2);
    });
});

describe("races around the close request (review of PR 21)", () => {
    function deferredSetup(counts: (number | null)[], answers: number[]) {
        const closed: string[] = [];
        const dialogs: string[] = [];
        const reads: Array<(n: number | null) => void> = [];
        let readIndex = 0;
        const handler = makeCloseTabHandler<void>({
            getBlockCount: () => {
                const i = readIndex++;
                if (counts[i] === undefined) {
                    return new Promise<number | null>((resolve) => reads.push(resolve));
                }
                return Promise.resolve(counts[i] as number | null);
            },
            showDialog: (message) => {
                dialogs.push(message);
                return Promise.resolve(answers.shift() ?? 0);
            },
            closeTab: async (tabId) => {
                closed.push(tabId);
                return true;
            },
        });
        return { handler, closed, dialogs, reads };
    }

    test("two requests whose count reads overlap open ONE dialog: the guard is set before the first await", async () => {
        const t = deferredSetup([undefined as unknown as number, 2], [1]);
        const first = t.handler("tab1", false, undefined);
        const second = await t.handler("tab1", false, undefined);
        expect(second).toBe(false);
        expect(t.reads.length).toBe(1);
        t.reads[0](2);
        await Promise.resolve();
        const closed = await first;
        expect(t.dialogs.length).toBe(1);
        expect(closed).toBe(true);
        expect(t.closed).toEqual(["tab1"]);
    });

    test("an empty tab that gains a block after its count was read does not close without asking", async () => {
        const t = deferredSetup([0, 2], [0]);
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(t.dialogs.length).toBe(1);
        expect(t.dialogs[0]).toContain("2 blocks");
        expect(t.closed).toEqual([]);
    });

    test("a tab that grows after the user confirmed asks again", async () => {
        const t = deferredSetup([3, 5], [1, 0]);
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(t.dialogs.length).toBe(2);
        expect(t.dialogs[1]).toContain("5 blocks");
        expect(t.closed).toEqual([]);
    });

    test("a confirmed tab whose count stays or shrinks closes with one dialog", async () => {
        const t = deferredSetup([3, 3], [1]);
        expect(await t.handler("tab1", false, undefined)).toBe(true);
        expect(t.dialogs.length).toBe(1);
        const shrink = deferredSetup([3, 2], [1]);
        expect(await shrink.handler("tab1", false, undefined)).toBe(true);
        expect(shrink.dialogs.length).toBe(1);
    });

    test("an empty tab that stays empty closes without a dialog", async () => {
        const t = deferredSetup([0, 0], []);
        expect(await t.handler("tab1", false, undefined)).toBe(true);
        expect(t.dialogs).toEqual([]);
    });

    test("a tab that keeps changing is not closed", async () => {
        const t = deferredSetup([1, 2, 3, 4, 5, 6], [1, 1, 1, 1, 1]);
        expect(await t.handler("tab1", false, undefined)).toBe(false);
        expect(t.closed).toEqual([]);
    });
});

describe("the wsh route is checked in the main process", () => {
    test("a populated tab is refused without force, with the count, and closes with force", () => {
        expect(wshCloseRefusal(3, false)).toContain("3 blocks");
        expect(wshCloseRefusal(1, false)).toContain("1 block");
        expect(wshCloseRefusal(3, true)).toBeNull();
        expect(wshCloseRefusal(0, false)).toBeNull();
        expect(wshCloseRefusal(null, false)).not.toBeNull();
        expect(wshCloseRefusal(null, true)).toBeNull();
    });
});

describe("route level: every UI way to close a tab goes through the one handler", () => {
    const read = (p: string) => readFileSync(path.join(__dirname, "..", p), "utf8");
    test("the IPC route calls the handler and nothing else closes tabs from the UI", () => {
        const win = read("emain/emain-window.ts");
        expect(win).toContain('ipcMain.handle("close-tab"');
        expect(win).toContain("closeTabHandler(tabId, confirmClose, ww)");
        const preload = read("emain/preload.ts");
        expect(preload).toContain('ipcRenderer.invoke("close-tab"');
        const calls = ["emain/emain-window.ts", "emain/emain-wsh.ts", "emain/emain-ipc.ts", "emain/emain-menu.ts"]
            .map((f) => ({ f, n: (read(f).match(/queueCloseTab\(/g) ?? []).length }))
            .filter((x) => x.n > 0);
        expect(calls.map((c) => c.f).sort()).toEqual(["emain/emain-window.ts", "emain/emain-wsh.ts"]);
    });
    test("the front end closes tabs only through the electron close-tab route", () => {
        for (const f of [
            "frontend/app/tab/tabbar.tsx",
            "frontend/app/tab/vtabbar.tsx",
            "frontend/app/store/keymodel.ts",
        ]) {
            const src = read(f);
            expect(src).not.toMatch(/WorkspaceService\.CloseTab\(/);
            expect(src).toMatch(/closeTab\(/);
        }
    });
});

describe("force is the deliberate bypass of local callers and no UI route can pass it (review of PR 21)", () => {
    const read = (p: string) => readFileSync(path.join(__dirname, "..", p), "utf8");
    test("the UI close route has no force parameter anywhere", () => {
        expect(read("emain/preload.ts")).toMatch(
            /closeTab: \(workspaceId, tabId, confirmClose\) =>\s*ipcRenderer\.invoke\("close-tab", workspaceId, tabId, confirmClose\)/
        );
        expect(read("frontend/types/custom.d.ts")).toContain(
            "closeTab: (workspaceId: string, tabId: string, confirmClose: boolean) => Promise<boolean>"
        );
        expect(read("emain/emain-window.ts")).toContain("closeTabHandler(tabId, confirmClose, ww)");
    });
    test("no front end file calls the electron close command, so none can set force", () => {
        for (const f of [
            "frontend/app/tab/tabbar.tsx",
            "frontend/app/tab/vtabbar.tsx",
            "frontend/app/store/keymodel.ts",
            "frontend/app/tab/tabbarenv.ts",
        ]) {
            expect(read(f)).not.toMatch(/ElectronCloseTabCommand|force\s*:/);
        }
    });
});

describe("the close itself carries the approved count (second review of PR 21)", () => {
    function closeSetup(counts: (number | null)[], answers: number[], closeResult: boolean) {
        const closes: { tabId: string; maxBlocks: number }[] = [];
        let i = 0;
        const handler = makeCloseTabHandler<void>({
            getBlockCount: async () => counts[Math.min(i++, counts.length - 1)],
            showDialog: async () => answers.shift() ?? 0,
            closeTab: async (tabId, _ctx, maxBlocks) => {
                closes.push({ tabId, maxBlocks });
                return closeResult;
            },
        });
        return { handler, closes };
    }

    test("the close is limited to the count that was approved, so a block added later stops it", async () => {
        const t = closeSetup([3, 3], [1], true);
        expect(await t.handler("tab1", false, undefined)).toBe(true);
        expect(t.closes).toEqual([{ tabId: "tab1", maxBlocks: 3 }]);
    });

    test("an empty tab is closed with a limit of zero", async () => {
        const t = closeSetup([0, 0], [], true);
        expect(await t.handler("tab1", false, undefined)).toBe(true);
        expect(t.closes).toEqual([{ tabId: "tab1", maxBlocks: 0 }]);
    });

    test("a close that the back end refused (the tab gained blocks) is not reported as closed", async () => {
        const t = closeSetup([0, 0], [], false);
        expect(await t.handler("tab1", false, undefined)).toBe(false);
    });

    test("an unreadable count that was confirmed still goes through the checked close, as 'count unknown' and not as 'no limit'", async () => {
        const t = closeSetup([null, null], [1], true);
        expect(await t.handler("tab1", false, undefined)).toBe(true);
        expect(t.closes[0].maxBlocks).toBe(CloseTabCountUnknown);
        expect(t.closes[0].maxBlocks).not.toBe(CloseTabNoLimit);
    });

    test("the window sends both a count and 'count unknown' to the checked back end call, and only no limit to the plain one", () => {
        const src = readFileSync(path.join(__dirname, "emain-window.ts"), "utf8");
        expect(src).toContain("maxBlocks >= 0 || maxBlocks === CloseTabCountUnknown");
        expect(src).toContain("WorkspaceService.CloseTabChecked(this.workspaceId, tabId, true, maxBlocks)");
    });
});

describe("close requests that are left in a queue are settled (third review of PR 21)", () => {
    test("every pending close resolves false and the queue is emptied", () => {
        const results: boolean[] = [];
        const queue = [
            { op: "closetab", result: (b: boolean) => results.push(b) },
            { op: "switchtab" },
            { op: "closetab", result: (b: boolean) => results.push(b) },
            { op: "closetab" },
        ];
        settlePendingCloses(queue);
        expect(results).toEqual([false, false]);
        expect(queue.length).toBe(0);
    });

    test("an empty queue is fine and settling twice does not resolve twice", () => {
        const results: boolean[] = [];
        const queue = [{ op: "closetab", result: (b: boolean) => results.push(b) }];
        settlePendingCloses(queue);
        settlePendingCloses(queue);
        settlePendingCloses([]);
        expect(results).toEqual([false]);
    });

    test("the window calls it on every way out of the queue loop", () => {
        const src = readFileSync(path.join(__dirname, "emain-window.ts"), "utf8");
        expect(src).toContain("settlePendingCloses(this.actionQueue)");
        expect(src).toMatch(
            /try \{\s*await this\.processActionQueueInner\(\);\s*\} finally \{\s*settlePendingCloses\(this\.actionQueue\);/
        );
    });

    test("the unchecked CloseTab is only reached when no limit applies", () => {
        const src = readFileSync(path.join(__dirname, "emain-window.ts"), "utf8");
        expect(src).toContain("maxBlocks >= 0");
        expect((src.match(/WorkspaceService\.CloseTab\(/g) ?? []).length).toBe(1);
        expect((src.match(/WorkspaceService\.CloseTabChecked\(/g) ?? []).length).toBe(1);
    });
});
