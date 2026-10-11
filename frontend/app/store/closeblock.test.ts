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
let RunningBlockQuestion: string;
let UnknownBlockQuestion: string;

beforeAll(async () => {
    ({ closeBlockQuestion, confirmCloseBlock, closeBlockAfterConfirm, RunningBlockQuestion, UnknownBlockQuestion } =
        await import("./closeblock"));
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
        for (const view of ["web", "preview", "sysinfo", "help", "launcher"]) {
            expect(closeBlockQuestion(view, undefined, false)).toBeNull();
        }
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

describe("the routes", () => {
    const read = (rel: string) => readFileSync(path.join(__dirname, "..", "..", "..", rel), "utf8");

    test("the stack member X, the header menu and key, and the focused-block close go through the question", () => {
        expect(read("frontend/layout/lib/stacktabstrip.tsx")).toContain("layoutModel.confirmCloseBlock");
        const keymodel = read("frontend/app/store/keymodel.ts");
        const ux = keymodel.slice(keymodel.indexOf("function uxCloseBlock"), keymodel.indexOf("function genericClose"));
        expect(ux).toContain("closeBlockAfterConfirm(blockId, () => layoutModel.closeNode(node.id))");
        const generic = keymodel.slice(keymodel.indexOf("function genericClose"));
        expect(generic).toContain("closeBlockAfterConfirm(");
        expect(generic).toContain("const nodeId = focusedNode?.id;");
        expect(read("frontend/app/tab/tabcontent.tsx")).toContain("confirmCloseBlock,");
    });

    test("the last block of a tab goes through the tab question only, before the block question can be asked", () => {
        const keymodel = read("frontend/app/store/keymodel.ts");
        const ux = keymodel.slice(keymodel.indexOf("function uxCloseBlock"), keymodel.indexOf("function genericClose"));
        expect(ux.indexOf("getStaticTabBlockCount() === 1")).toBeGreaterThan(-1);
        expect(ux.lastIndexOf("simpleCloseStaticTab()")).toBeLessThan(ux.indexOf("closeBlockAfterConfirm("));
        const generic = keymodel.slice(keymodel.indexOf("function genericClose"));
        expect(generic.indexOf("blockCount === 1")).toBeLessThan(generic.indexOf("closeBlockAfterConfirm("));
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
