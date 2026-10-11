// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { beforeAll, describe, expect, test, vi } from "vitest";

vi.mock("@/app/store/global", () => ({ getApi: () => ({}), globalStore: {}, WOS: {} }));
vi.mock("@/app/store/services", () => ({ BlockService: {} }));

let closeBlockQuestion: any;
let confirmCloseBlock: any;
let closeBlockAfterConfirm: any;
let closeUnidentifiedAfterConfirm: any;
let closeFocusedBlockByUser: any;
let closeStripTab: any;
let RunningBlockQuestion: string;
let UnknownBlockQuestion: string;

beforeAll(async () => {
    ({ closeStripTab } = await import("../../layout/lib/stackclose"));
    ({
        closeBlockQuestion,
        confirmCloseBlock,
        closeBlockAfterConfirm,
        closeUnidentifiedAfterConfirm,
        closeFocusedBlockByUser,
        RunningBlockQuestion,
        UnknownBlockQuestion,
    } = await import("./closeblock"));
});

function deps(view: any, status: any, answer: boolean | Promise<boolean> = true, readFails = false) {
    const asked: string[] = [];
    const statusReads: string[] = [];
    return {
        asked,
        statusReads,
        deps: {
            readView: () => view,
            readStatus: async (id: string) => {
                statusReads.push(id);
                if (readFails) throw new Error("no backend");
                return status;
            },
            ask: async (m: string) => {
                asked.push(m);
                return await answer;
            },
        },
    };
}

describe("which blocks ask before they close", () => {
    test("a program that is running asks; ended, not started and views without a program do not", () => {
        for (const view of ["term", "tsunami"]) {
            expect(closeBlockQuestion(view, "running", false)).toBe(RunningBlockQuestion);
            expect(closeBlockQuestion(view, "done", false)).toBeNull();
            expect(closeBlockQuestion(view, "init", false)).toBeNull();
        }
        for (const view of [
            "web",
            "preview",
            "sysinfo",
            "help",
            "launcher",
            "waveai",
            "cpuplot",
            "tips",
            "aifilediff",
            "waveconfig",
            "processviewer",
        ]) {
            expect(closeBlockQuestion(view, undefined, false), view).toBeNull();
        }
    });

    test("a view name that is not in the list of known views asks, so a view added later is never silent by accident", () => {
        for (const view of ["somethingnew", "Web", "terminal", "vdom-x"]) {
            expect(closeBlockQuestion(view, undefined, false), view).toBe(UnknownBlockQuestion);
            expect(closeBlockQuestion(view, "done", false), view).toBe(UnknownBlockQuestion);
        }
        expect(closeBlockQuestion("vdom", "running", false)).toBe(RunningBlockQuestion);
        expect(closeBlockQuestion("vdom", "done", false)).toBeNull();
    });

    test("everything that is not known to mean no running program asks", () => {
        for (const status of [undefined, null, "", "surprise", "Running", "DONE"]) {
            expect(closeBlockQuestion("term", status, false), String(status)).toBe(UnknownBlockQuestion);
        }
        expect(closeBlockQuestion("term", "done", true)).toBe(UnknownBlockQuestion);
        expect(closeBlockQuestion(undefined, "done", false)).toBe(UnknownBlockQuestion);
        expect(closeBlockQuestion(null, "running", false)).toBe(UnknownBlockQuestion);
        expect(closeBlockQuestion("", "done", false)).toBe(UnknownBlockQuestion);
    });
});

describe("confirmCloseBlock", () => {
    test("a running program asks, and the answer decides", async () => {
        const yes = deps("term", { shellprocstatus: "running" }, true);
        expect(await confirmCloseBlock("b", yes.deps)).toBe(true);
        expect(yes.asked).toEqual([RunningBlockQuestion]);
        const no = deps("term", { shellprocstatus: "running" }, false);
        expect(await confirmCloseBlock("b", no.deps)).toBe(false);
    });

    test("an ended or not-started program and a view without a program close silently; a view without a program is not even asked about", async () => {
        for (const t of [deps("term", { shellprocstatus: "done" }), deps("term", { shellprocstatus: "init" })]) {
            expect(await confirmCloseBlock("b", t.deps)).toBe(true);
            expect(t.asked).toEqual([]);
        }
        const web = deps("web", undefined);
        expect(await confirmCloseBlock("b", web.deps)).toBe(true);
        expect(web.asked).toEqual([]);
        expect(web.statusReads).toEqual([]);
    });

    test("a terminal with no controller, an empty or unknown state, or a state that cannot be read asks", async () => {
        for (const t of [
            deps("term", null),
            deps("term", undefined),
            deps("term", {}),
            deps("term", { shellprocstatus: "weird" }),
            deps("term", null, true, true),
        ]) {
            expect(await confirmCloseBlock("b", t.deps)).toBe(true);
            expect(t.asked).toEqual([UnknownBlockQuestion]);
        }
    });

    test("a question that cannot be shown keeps the block", async () => {
        const broken = {
            readView: () => "term",
            readStatus: async () => ({ shellprocstatus: "running" }),
            ask: async () => {
                throw new Error("dialog failed");
            },
        };
        expect(await confirmCloseBlock("b", broken)).toBe(false);
    });

    test("one question per block: clicking the X again while the box is open starts no second box and closes nothing", async () => {
        let release: (v: boolean) => void = () => {};
        const open = new Promise<boolean>((r) => (release = r));
        const t = deps("term", { shellprocstatus: "running" }, open);
        const first = confirmCloseBlock("same", t.deps);
        await Promise.resolve();
        await Promise.resolve();
        const second = await confirmCloseBlock("same", t.deps);
        expect(second).toBe(false);
        const other = deps("term", { shellprocstatus: "running" }, true);
        expect(await confirmCloseBlock("another", other.deps)).toBe(true);
        release(true);
        expect(await first).toBe(true);
        expect(t.asked.length).toBe(1);
        const again = deps("term", { shellprocstatus: "running" }, true);
        expect(await confirmCloseBlock("same", again.deps)).toBe(true);
    });
});

describe("closing after the question", () => {
    test("Cancel closes nothing and touches nothing", async () => {
        const touched: string[] = [];
        const t = deps("term", { shellprocstatus: "running" }, false);
        expect(await closeBlockAfterConfirm("b", () => void touched.push("closed"), t.deps)).toBe(false);
        expect(touched).toEqual([]);
    });

    test("yes closes once; a program that has ended closes without a question", async () => {
        const closed: string[] = [];
        const t = deps("term", { shellprocstatus: "running" }, true);
        expect(await closeBlockAfterConfirm("b", () => void closed.push("b"), t.deps)).toBe(true);
        const done = deps("term", { shellprocstatus: "done" });
        expect(await closeBlockAfterConfirm("c", () => void closed.push("c"), done.deps)).toBe(true);
        expect(closed).toEqual(["b", "c"]);
        expect(done.asked).toEqual([]);
    });

    test("the node chosen before the question is the one that closes, even when focus moves while the box is open", async () => {
        const layout = { focused: "node-A" };
        const closed: string[] = [];
        let release: (v: boolean) => void = () => {};
        const open = new Promise<boolean>((r) => (release = r));
        const t = deps("term", { shellprocstatus: "running" }, open);
        const chosen = layout.focused;
        const pending = closeBlockAfterConfirm("block-A", () => void closed.push(chosen), t.deps);
        await Promise.resolve();
        await Promise.resolve();
        layout.focused = "node-B";
        release(true);
        expect(await pending).toBe(true);
        expect(closed).toEqual(["node-A"]);
    });

    test("a double click closes the block once", async () => {
        const closed: string[] = [];
        let release: (v: boolean) => void = () => {};
        const open = new Promise<boolean>((r) => (release = r));
        const t = deps("term", { shellprocstatus: "running" }, open);
        const first = closeBlockAfterConfirm("dbl", () => void closed.push("dbl"), t.deps);
        await Promise.resolve();
        await Promise.resolve();
        const second = await closeBlockAfterConfirm("dbl", () => void closed.push("dbl"), t.deps);
        release(true);
        await first;
        expect(second).toBe(false);
        expect(closed).toEqual(["dbl"]);
    });
});

describe("a close that cannot name the block", () => {
    test("always asks, and closes only on yes", async () => {
        const asked: string[] = [];
        const closed: string[] = [];
        const no = async (m: string) => (asked.push(m), false);
        const yes = async (m: string) => (asked.push(m), true);
        expect(await closeUnidentifiedAfterConfirm(() => void closed.push("x"), no)).toBe(false);
        expect(closed).toEqual([]);
        expect(await closeUnidentifiedAfterConfirm(() => void closed.push("x"), yes)).toBe(true);
        expect(closed).toEqual(["x"]);
        expect(asked).toEqual([UnknownBlockQuestion, UnknownBlockQuestion]);
    });

    test("a box that cannot be shown keeps the block", async () => {
        const closed: string[] = [];
        const broken = async () => {
            throw new Error("no dialog");
        };
        expect(await closeUnidentifiedAfterConfirm(() => void closed.push("x"), broken)).toBe(false);
        expect(closed).toEqual([]);
    });
});

// The user routes run with a block whose program is running: nothing is closed before the answer, and one answer closes once.
describe("each user route with a running block", () => {
    function route() {
        const closedNodes: string[] = [];
        let answer: (v: boolean) => void = () => {};
        const open = new Promise<boolean>((r) => (answer = r));
        const t = deps("term", { shellprocstatus: "running" }, open);
        const closeNode = async (id: string) => void closedNodes.push(id);
        const settle = async () => {
            for (let i = 0; i < 20; i++) await Promise.resolve();
        };
        return { t, closedNodes, closeNode, answer: (v: boolean) => answer(v), settle };
    }

    test("the X of a stack member: nothing is closed before the answer, a yes closes once, Cancel closes nothing", async () => {
        const yes = route();
        const hook = (blockId: string, close: () => Promise<void> | void) =>
            closeBlockAfterConfirm(blockId, close, yes.t.deps);
        const pending = closeStripTab(hook, "blk", "node-1", yes.closeNode);
        await yes.settle();
        expect(yes.closedNodes).toEqual([]);
        expect(yes.t.asked).toEqual([RunningBlockQuestion]);
        yes.answer(true);
        expect(await pending).toBe(true);
        expect(yes.closedNodes).toEqual(["node-1"]);

        const no = route();
        const hookNo = (blockId: string, close: () => Promise<void> | void) =>
            closeBlockAfterConfirm(blockId, close, no.t.deps);
        const cancelled = closeStripTab(hookNo, "blk2", "node-2", no.closeNode);
        await no.settle();
        no.answer(false);
        expect(await cancelled).toBe(false);
        expect(no.closedNodes).toEqual([]);
    });

    test("a strip without the question closes nothing", async () => {
        const closed: string[] = [];
        expect(await closeStripTab(undefined, "blk", "node-1", (id: string) => void closed.push(id))).toBe(false);
        expect(closed).toEqual([]);
    });

    test("a second X on the strip, while the box is open or while the close is running, does not ask again and does not close twice", async () => {
        const r = route();
        const hook = (blockId: string, close: () => Promise<void> | void) =>
            closeBlockAfterConfirm(blockId, close, r.t.deps);
        let finishClose: () => void = () => {};
        const closing = new Promise<void>((res) => (finishClose = res));
        const slowClose = async (id: string) => {
            r.closedNodes.push(id);
            await closing;
        };
        const first = closeStripTab(hook, "same", "node-1", slowClose);
        await r.settle();
        expect(await closeStripTab(hook, "same", "node-1", slowClose)).toBe(false);
        r.answer(true);
        await r.settle();
        expect(r.closedNodes).toEqual(["node-1"]);
        // the box is answered and the close has started but not finished: another X still asks nothing and closes nothing
        expect(await closeStripTab(hook, "same", "node-1", slowClose)).toBe(false);
        expect(r.t.asked.length).toBe(1);
        finishClose();
        expect(await first).toBe(true);
        expect(r.closedNodes).toEqual(["node-1"]);
    });

    test("the close key: the node chosen before the question closes after a yes, nothing before it, also when focus moved", async () => {
        const r = route();
        const layout = { focused: "node-A" };
        const pending = closeFocusedBlockByUser(
            {
                blockId: "block-A",
                nodeId: layout.focused,
                closeNode: r.closeNode,
                closeFocused: async () => void r.closedNodes.push("FOCUSED"),
            },
            r.t.deps
        );
        await r.settle();
        layout.focused = "node-B";
        expect(r.closedNodes).toEqual([]);
        r.answer(true);
        expect(await pending).toBe(true);
        expect(r.closedNodes).toEqual(["node-A"]);
    });

    test("the close key with no identifiable block always asks, and closes the focused one only after a yes", async () => {
        for (const target of [
            { blockId: null, nodeId: "n" },
            { blockId: "b", nodeId: null },
            { blockId: undefined, nodeId: undefined },
        ]) {
            const r = route();
            const pending = closeFocusedBlockByUser(
                { ...target, closeNode: r.closeNode, closeFocused: async () => void r.closedNodes.push("FOCUSED") },
                r.t.deps
            );
            await r.settle();
            expect(r.closedNodes).toEqual([]);
            expect(r.t.asked).toEqual([UnknownBlockQuestion]);
            r.answer(true);
            expect(await pending).toBe(true);
            expect(r.closedNodes).toEqual(["FOCUSED"]);
        }
    });

    test("the header menu route (one block, closed by id after a yes) and Cancel", async () => {
        const r = route();
        const pending = closeBlockAfterConfirm("blk", () => r.closeNode("node-9"), r.t.deps);
        await r.settle();
        expect(r.closedNodes).toEqual([]);
        r.answer(false);
        expect(await pending).toBe(false);
        expect(r.closedNodes).toEqual([]);
    });
});

describe("the routes", () => {
    const read = (rel: string) => readFileSync(path.join(__dirname, "..", "..", "..", rel), "utf8");

    test("the stack member X, the header menu and key, and the focused-block close go through the question", () => {
        expect(read("frontend/layout/lib/stacktabstrip.tsx")).toContain(
            "closeStripTab(layoutModel.closeBlockAfterConfirm"
        );
        const keymodel = read("frontend/app/store/keymodel.ts");
        const ux = keymodel.slice(keymodel.indexOf("function uxCloseBlock"), keymodel.indexOf("function genericClose"));
        expect(ux).toContain("closeBlockAfterConfirm(blockId, () => layoutModel.closeNode(node.id))");
        const generic = keymodel.slice(keymodel.indexOf("function genericClose"));
        expect(generic).toContain("closeFocusedBlockByUser(");
        expect(generic).toContain("const nodeId = focusedNode?.id;");
        expect(generic).toContain("closeFocusedBlockByUser({");
        expect(read("frontend/app/tab/tabcontent.tsx")).toContain("closeBlockAfterConfirm,");
    });

    test("the last block of a tab goes through the tab question only, before the block question can be asked", () => {
        const keymodel = read("frontend/app/store/keymodel.ts");
        const ux = keymodel.slice(keymodel.indexOf("function uxCloseBlock"), keymodel.indexOf("function genericClose"));
        expect(ux.indexOf("getStaticTabBlockCount() === 1")).toBeGreaterThan(-1);
        expect(ux.lastIndexOf("simpleCloseStaticTab()")).toBeLessThan(ux.lastIndexOf("closeBlockAfterConfirm("));
        const generic = keymodel.slice(keymodel.indexOf("function genericClose"));
        expect(generic.indexOf("blockCount === 1")).toBeLessThan(generic.indexOf("closeFocusedBlockByUser("));
    });

    test("the AI input is refocused only when the block was closed, not on Cancel", () => {
        const keymodel = read("frontend/app/store/keymodel.ts");
        const ux = keymodel.slice(keymodel.indexOf("function uxCloseBlock"), keymodel.indexOf("function genericClose"));
        expect(ux).toContain("if (closed && isAIFileDiff && isAIPanelOpen)");
    });

    test("closing by a command or the backend is a named exception: wsh deleteblock and the backend delete action never ask", () => {
        const model = read("frontend/layout/lib/layoutModel.ts");
        const deleteCase = model.slice(
            model.indexOf("case LayoutTreeActionType.DeleteNode: {"),
            model.indexOf("Cannot apply eventbus layout action DeleteNode")
        );
        expect(deleteCase).not.toContain("confirmCloseBlock(");
        const closeNode = model.slice(model.indexOf("async closeNode("), model.indexOf("async closeFocusedNode"));
        expect(closeNode).not.toContain("confirmCloseBlock");
    });

    test("the electron side: type, preload, a handler that answers only a Wave tab view and caps the message, preview stub", () => {
        expect(read("frontend/types/custom.d.ts")).toContain(
            "confirmCloseBlock: (message: string) => Promise<boolean>; // confirm-close-block"
        );
        expect(read("emain/preload.ts")).toContain('ipcRenderer.invoke("confirm-close-block", message)');
        const win = read("emain/emain-window.ts");
        const handler = win.slice(win.indexOf('ipcMain.handle("confirm-close-block"'));
        const body = handler.slice(0, handler.indexOf("\n});"));
        expect(body.indexOf("if (ww == null)")).toBeGreaterThan(-1);
        expect(body.indexOf("if (ww == null)")).toBeLessThan(body.indexOf("showMessageBox"));
        expect(body).toContain("return false;");
        expect(body).toContain("slice(0, 500)");
        expect(body).toContain("cancelId: 0");
        expect(body).not.toContain("showMessageBox(options)");
        expect(read("frontend/preview/mock/preview-electron-api.ts")).toContain("confirmCloseBlock:");
    });
});

describe("more routes (review of the run-the-code reviewer)", () => {
    const read = (rel: string) => readFileSync(path.join(__dirname, "..", "..", "..", rel), "utf8");

    test("the layout model keeps the callback that the tab content gives it, and the ephemeral backdrop goes through the strip close", () => {
        const model = read("frontend/layout/lib/layoutModel.ts");
        expect(model).toContain("this.closeBlockAfterConfirm = contents.closeBlockAfterConfirm;");
        const tile = read("frontend/layout/lib/TileLayout.tsx");
        const backdrop = tile.slice(tile.indexOf('className="ephemeral-node-backdrop"'));
        expect(backdrop.slice(0, 700)).toContain("closeStripTab(layoutModel.closeBlockAfterConfirm");
        expect(backdrop.slice(0, 700)).not.toMatch(/layoutModel\.closeNode\(ephemeralNode/);
    });

    test("the routes that replace a block by a launcher (and delete it 300 ms later) ask first: header X and menu, close key, Ctrl+Shift+X", () => {
        const keymodel = read("frontend/app/store/keymodel.ts");
        for (const name of ["function uxCloseBlock", "function genericClose"]) {
            const fn = keymodel.slice(keymodel.indexOf(name));
            const body = fn.slice(0, fn.indexOf("\n}\n"));
            const replaceAt = body.indexOf("replaceBlock(");
            expect(replaceAt, name).toBeGreaterThan(-1);
            expect(body.lastIndexOf("closeBlockAfterConfirm(", replaceAt), name).toBeGreaterThan(-1);
        }
        const x = keymodel.slice(keymodel.indexOf('globalKeyMap.set("Ctrl:Shift:x"'));
        const xBody = x.slice(0, x.indexOf("\n    });"));
        expect(xBody.indexOf("closeBlockAfterConfirm(")).toBeGreaterThan(-1);
        expect(xBody.indexOf("closeBlockAfterConfirm(")).toBeLessThan(xBody.indexOf("replaceBlock("));
    });

    test("no key named Ctrl+Shift+W is claimed anywhere: the close key is Cmd:w (Alt+W on Windows)", () => {
        const keymodel = read("frontend/app/store/keymodel.ts");
        expect(keymodel).toContain('globalKeyMap.set("Cmd:w", () => {');
        expect(keymodel).not.toContain("Ctrl:Shift:w");
        expect(read("frontend/app/store/closeblock.ts")).not.toContain("Ctrl+Shift+W");
        const docs = read("docs/ours/what-wave-can-do.md");
        for (const m of docs.matchAll(/Ctrl+Shift+W[^;]{0,30}/g)) {
            expect(m[0]).toContain("is not bound");
        }
    });
});

describe("the close of a block that cannot be identified holds one question at a time", () => {
    test("a second one while the first box is open does not ask again", async () => {
        let release: (v: boolean) => void = () => {};
        const open = new Promise<boolean>((r) => (release = r));
        const asked: string[] = [];
        const ask = async (m: string) => (asked.push(m), await open);
        const closed: string[] = [];
        const first = closeUnidentifiedAfterConfirm(() => void closed.push("a"), ask);
        await Promise.resolve();
        await Promise.resolve();
        expect(await closeUnidentifiedAfterConfirm(() => void closed.push("b"), ask)).toBe(false);
        release(true);
        expect(await first).toBe(true);
        expect(asked.length).toBe(1);
        expect(closed).toEqual(["a"]);
    });
});
