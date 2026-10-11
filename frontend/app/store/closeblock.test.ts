// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { beforeAll, describe, expect, test, vi } from "vitest";

vi.mock("@/app/store/global", () => ({ getApi: () => ({}) }));
vi.mock("@/app/store/services", () => ({ BlockService: {} }));

let closeBlockQuestion: any;
let confirmCloseBlock: any;
let RunningBlockQuestion: string;
let UnknownBlockQuestion: string;

beforeAll(async () => {
    ({ closeBlockQuestion, confirmCloseBlock, RunningBlockQuestion, UnknownBlockQuestion } =
        await import("./closeblock"));
});

function deps(status: any, answer = true, readFails = false) {
    const asked: string[] = [];
    return {
        asked,
        deps: {
            readStatus: async () => {
                if (readFails) throw new Error("no backend");
                return status;
            },
            ask: async (m: string) => {
                asked.push(m);
                return answer;
            },
        },
    };
}

describe("closing one block that has a running program asks first", () => {
    test("which states ask", () => {
        expect(closeBlockQuestion("running", false)).toBe(RunningBlockQuestion);
        expect(closeBlockQuestion("done", false)).toBeNull();
        expect(closeBlockQuestion("init", false)).toBeNull();
        expect(closeBlockQuestion("", false)).toBeNull();
        expect(closeBlockQuestion(undefined, false)).toBeNull();
        expect(closeBlockQuestion(null, false)).toBeNull();
        expect(closeBlockQuestion("surprise", false)).toBeNull();
        expect(closeBlockQuestion(undefined, true)).toBe(UnknownBlockQuestion);
    });

    test("a running program asks, and the answer decides", async () => {
        const yes = deps({ shellprocstatus: "running" }, true);
        expect(await confirmCloseBlock("b", yes.deps)).toBe(true);
        expect(yes.asked).toEqual([RunningBlockQuestion]);
        const no = deps({ shellprocstatus: "running" }, false);
        expect(await confirmCloseBlock("b", no.deps)).toBe(false);
    });

    test("no program, an ended program and a block without a controller do not ask", async () => {
        for (const status of [{ shellprocstatus: "done" }, { shellprocstatus: "init" }, null, undefined, {}]) {
            const t = deps(status);
            expect(await confirmCloseBlock("b", t.deps)).toBe(true);
            expect(t.asked).toEqual([]);
        }
    });

    test("a state that cannot be read asks, and a question that cannot be shown keeps the block", async () => {
        const t = deps(null, true, true);
        expect(await confirmCloseBlock("b", t.deps)).toBe(true);
        expect(t.asked).toEqual([UnknownBlockQuestion]);
        const broken = {
            readStatus: async () => ({ shellprocstatus: "running" }),
            ask: async () => {
                throw new Error("dialog failed");
            },
        };
        expect(await confirmCloseBlock("b", broken)).toBe(false);
    });
});

describe("every UI route that closes one block goes through the question", () => {
    const read = (rel: string) => readFileSync(path.join(__dirname, "..", "..", "..", rel), "utf8");

    test("the stack member X, the header menu and key, and the focused-block close", () => {
        const strip = read("frontend/layout/lib/stacktabstrip.tsx");
        expect(strip).toContain("layoutModel.confirmCloseBlock");
        const keymodel = read("frontend/app/store/keymodel.ts");
        const ux = keymodel.slice(keymodel.indexOf("function uxCloseBlock"), keymodel.indexOf("function genericClose"));
        expect(ux).toContain("confirmCloseBlock(blockId)");
        const generic = keymodel.slice(keymodel.indexOf("function genericClose"));
        expect(generic.slice(0, generic.indexOf("closeFocusedNode") + 20)).toContain("confirmCloseBlock(");
        expect(read("frontend/app/tab/tabcontent.tsx")).toContain("confirmCloseBlock,");
    });

    test("a block closed by a command (wsh deleteblock, the backend delete action) is not asked", () => {
        const model = read("frontend/layout/lib/layoutModel.ts");
        const deleteCase = model.slice(
            model.indexOf("case LayoutTreeActionType.DeleteNode: {"),
            model.indexOf("Cannot apply eventbus layout action DeleteNode")
        );
        expect(deleteCase).not.toContain("confirmCloseBlock(");
        const closeNode = model.slice(model.indexOf("async closeNode("), model.indexOf("async closeFocusedNode"));
        expect(closeNode).not.toContain("confirmCloseBlock");
    });

    test("the electron side: type, preload, handler and preview stub exist, and the handler caps the message", () => {
        expect(read("frontend/types/custom.d.ts")).toContain(
            "confirmCloseBlock: (message: string) => Promise<boolean>; // confirm-close-block"
        );
        expect(read("emain/preload.ts")).toContain('ipcRenderer.invoke("confirm-close-block", message)');
        const win = read("emain/emain-window.ts");
        const handler = win.slice(win.indexOf('ipcMain.handle("confirm-close-block"'));
        expect(handler.slice(0, 900)).toContain("slice(0, 500)");
        expect(handler.slice(0, 900)).toContain("cancelId: 0");
        expect(read("frontend/preview/mock/preview-electron-api.ts")).toContain("confirmCloseBlock:");
    });
});
