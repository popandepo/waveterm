// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { rightClickAction, runRightClick } from "./rightclick";

describe("what a right click in a terminal does", () => {
    test("with the setting off or unset it opens the menu, whatever is selected", () => {
        for (const setting of [false, undefined, null]) {
            expect(rightClickAction(setting, false, false)).toBe("menu");
            expect(rightClickAction(setting, true, false)).toBe("menu");
        }
    });

    test("with the setting on: copy when something is selected, paste when nothing is", () => {
        expect(rightClickAction(true, true, false)).toBe("copy");
        expect(rightClickAction(true, false, false)).toBe("paste");
    });

    test("Shift or Ctrl held keeps the menu reachable", () => {
        expect(rightClickAction(true, true, true)).toBe("menu");
        expect(rightClickAction(true, false, true)).toBe("menu");
    });

    test("a program that takes the mouse itself (vim, Claude Code) gets the click: no copy, no paste", () => {
        expect(rightClickAction(true, false, false, true)).toBe("menu");
        expect(rightClickAction(true, true, false, true)).toBe("menu");
    });
});

// Two terminals in one window: the native paste goes to the element that has the keyboard focus, so a stand-in for that rule runs
// the whole click against two terminals.
function twoTerminals(setting = true) {
    const pasted: Record<string, string[]> = { A: [], B: [] };
    const copied: string[] = [];
    let keyboardFocus = "A";
    const clipboard = "echo from the clipboard";
    const click = (target: "A" | "B", over: Partial<Parameters<typeof runRightClick>[0]> = {}) =>
        runRightClick({
            rightClickPaste: setting,
            hasSelection: false,
            programTakesMouse: false,
            menuModifier: false,
            focusThisTerminal: () => {
                keyboardFocus = target;
            },
            nativePaste: () => {
                pasted[keyboardFocus].push(clipboard);
            },
            copySelection: () => void copied.push(target),
            clearSelection: () => {},
            ...over,
        });
    return { pasted, copied, click, focus: () => keyboardFocus };
}

describe("the right click run against two terminals with different focus", () => {
    test("a right click on B while A holds the keyboard focus pastes into B, not A", () => {
        const t = twoTerminals();
        expect(t.focus()).toBe("A");
        expect(t.click("B")).toBe(true);
        expect(t.pasted.B).toEqual(["echo from the clipboard"]);
        expect(t.pasted.A).toEqual([]);
        expect(t.focus()).toBe("B");
    });

    test("a right click on the terminal that has the focus pastes there", () => {
        const t = twoTerminals();
        t.click("A");
        expect(t.pasted.A.length).toBe(1);
        expect(t.pasted.B.length).toBe(0);
    });

    test("with the setting off, in a mouse-taking program, or with Shift, nothing is pasted or copied and the menu opens", () => {
        const off = twoTerminals(false);
        expect(off.click("B")).toBe(false);
        const mouse = twoTerminals();
        expect(mouse.click("B", { programTakesMouse: true })).toBe(false);
        const shift = twoTerminals();
        expect(shift.click("B", { menuModifier: true, hasSelection: true })).toBe(false);
        for (const t of [off, mouse, shift]) {
            expect(t.pasted.A.length + t.pasted.B.length).toBe(0);
            expect(t.copied).toEqual([]);
        }
        expect(off.focus()).toBe("A");
    });

    test("with a selection it copies from the terminal that was clicked and pastes nothing", () => {
        const t = twoTerminals();
        expect(t.click("B", { hasSelection: true })).toBe(true);
        expect(t.copied).toEqual(["B"]);
        expect(t.pasted.A.length + t.pasted.B.length).toBe(0);
    });
});

describe("the wiring", () => {
    const read = (rel: string) => readFileSync(path.join(__dirname, "..", "..", "..", "..", rel), "utf8");

    test("the context menu handler asks the model first, and the model builds the deps from its own terminal", () => {
        const term = read("frontend/app/view/term/term.tsx");
        const handler = term.slice(
            term.indexOf("const handleContextMenu"),
            term.indexOf("return (", term.indexOf("const handleContextMenu"))
        );
        expect(handler).toContain("model.handleRightClick(");
        const model = read("frontend/app/view/term/term-model.ts");
        const fn = model.slice(model.indexOf("handleRightClick("), model.indexOf("getContextMenuItems()"));
        expect(fn).toContain("runRightClick(");
        expect(fn).toContain("term:rightclickpaste");
        expect(fn).toContain("this.nodeModel.focusNode()");
        expect(fn).toContain("terminal.focus()");
        expect(fn).toContain("mouseTrackingMode");
        expect(fn).toContain("getApi().nativePaste()");
        expect(fn).toContain("trimTerminalSelection");
        expect(fn.indexOf("terminal.focus()")).toBeLessThan(fn.indexOf("getApi().nativePaste()"));
    });

    test("the setting exists in the types, the default config and the docs", () => {
        expect(read("pkg/wconfig/settingsconfig.go")).toContain('json:"term:rightclickpaste,omitempty"');
        expect(read("pkg/wconfig/defaultconfig/settings.json")).toContain('"term:rightclickpaste": true');
        expect(read("docs/docs/config.mdx")).toContain("term:rightclickpaste");
    });
});
