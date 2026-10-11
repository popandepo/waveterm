// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { closeTabPrompt } from "./closetabprompt";

describe("asking before a tab is closed", () => {
    test("a tab that holds blocks always asks, even when the setting is off", () => {
        const p = closeTabPrompt(false, 3);
        expect(p.ask).toBe(true);
        expect(p.message).toContain("3 blocks");
        expect(p.message.toLowerCase()).toContain("stops");
    });

    test("one block is singular", () => {
        expect(closeTabPrompt(false, 1).message).toContain("1 block");
        expect(closeTabPrompt(false, 1).message).not.toContain("1 blocks");
    });

    test("an empty tab closes without asking unless the setting asks for it", () => {
        expect(closeTabPrompt(false, 0).ask).toBe(false);
        const p = closeTabPrompt(true, 0);
        expect(p.ask).toBe(true);
        expect(p.message).toBe("Are you sure you want to close this tab?");
    });

    test("an unknown block count is treated as holding blocks, never as empty", () => {
        expect(closeTabPrompt(false, null).ask).toBe(true);
        expect(closeTabPrompt(false, undefined).ask).toBe(true);
        expect(closeTabPrompt(false, null).message).toContain("blocks");
    });
});

describe("the answer of the box that asks before one block is closed", () => {
    test("only Close Block closes; Cancel, Escape and anything else keep the block", async () => {
        const { closeBlockAnswer, CloseBlockButtons } = await import("./closetabprompt");
        expect(CloseBlockButtons).toEqual(["Cancel", "Close Block"]);
        expect(closeBlockAnswer(1)).toBe(true);
        expect(closeBlockAnswer(0)).toBe(false);
        for (const other of [-1, 2, 5, NaN, undefined as unknown as number]) {
            expect(closeBlockAnswer(other)).toBe(false);
        }
    });

    test("the handler uses them: Cancel is the Esc button and Close Block the default", () => {
        const win = readFileSync(path.join(__dirname, "emain-window.ts"), "utf8");
        const handler = win.slice(win.indexOf('ipcMain.handle("confirm-close-block"'));
        const body = handler.slice(0, handler.indexOf("\n});"));
        expect(body).toContain("buttons: CloseBlockButtons");
        expect(body).toContain("defaultId: 1");
        expect(body).toContain("cancelId: 0");
        expect(body).toContain("closeBlockAnswer(result.response)");
    });
});
