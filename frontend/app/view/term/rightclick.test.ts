// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { rightClickAction, runRightClick, waitForTerminalFocus } from "./rightclick";

describe("what a right click in a terminal does", () => {
    test("with the setting off or unset it opens the menu, whatever is selected", () => {
        for (const setting of [false, undefined, null]) {
            expect(rightClickAction(setting, false, false)).toBe("menu");
            expect(rightClickAction(setting, true, false)).toBe("menu");
            expect(rightClickAction(setting, true, false, true)).toBe("menu");
        }
    });

    test("with the setting on: copy when something is selected, paste when nothing is", () => {
        expect(rightClickAction(true, true, false)).toBe("copy");
        expect(rightClickAction(true, false, false)).toBe("paste");
    });

    test("Shift or Ctrl held keeps the menu reachable, also in a program that takes the mouse", () => {
        expect(rightClickAction(true, true, true)).toBe("menu");
        expect(rightClickAction(true, false, true)).toBe("menu");
        expect(rightClickAction(true, false, true, true)).toBe("menu");
    });

    test("a program that takes the mouse itself (vim, Claude Code) gets the click: its own outcome, no menu, no copy, no paste", () => {
        expect(rightClickAction(true, false, false, true)).toBe("program");
        expect(rightClickAction(true, true, false, true)).toBe("program");
    });
});

// A simulated clock and two terminals in one window. The native paste goes to the element that has the keyboard focus, and the focus
// takes `focusDelayMs` to settle after a terminal asks for it (as the app's focus handling does: a layout update, then the element).
function twoTerminals(
    opts: { setting?: boolean; focusDelayMs?: number; focusNeverSettles?: boolean; clipboardFails?: boolean } = {}
) {
    const setting = opts.setting ?? true;
    const focusDelayMs = opts.focusDelayMs ?? 0;
    const pasted: Record<string, string[]> = { A: [], B: [] };
    const copied: string[] = [];
    const cleared: string[] = [];
    let now = 0;
    let keyboardFocus = "A";
    const pending: { at: number; to: string }[] = [];
    const settle = () => {
        for (const p of pending.filter((x) => x.at <= now)) {
            keyboardFocus = p.to;
        }
        pending.splice(0, pending.length, ...pending.filter((x) => x.at > now));
    };
    const wait = async (ms: number) => {
        now += ms;
        settle();
    };
    const click = (target: "A" | "B", over: Partial<Parameters<typeof runRightClick>[0]> = {}) =>
        runRightClick({
            rightClickPaste: setting,
            hasSelection: false,
            programTakesMouse: false,
            menuModifier: false,
            focusThisTerminal: () => {
                if (!opts.focusNeverSettles) {
                    pending.push({ at: now + focusDelayMs, to: target });
                    settle();
                }
            },
            isThisTerminalFocused: () => keyboardFocus === target,
            nativePaste: () => {
                pasted[keyboardFocus].push("echo from the clipboard");
            },
            copySelection: async () => {
                if (opts.clipboardFails) {
                    throw new Error("clipboard denied");
                }
                copied.push(target);
                return true;
            },
            clearSelection: () => void cleared.push(target),
            wait,
            ...over,
        });
    const flush = async () => {
        for (let i = 0; i < 40; i++) {
            await Promise.resolve();
        }
    };
    return { pasted, copied, cleared, click, flush, focus: () => keyboardFocus };
}

describe("the right click run against two terminals with different focus", () => {
    test("a right click on B while A holds the keyboard focus pastes into B, not A", async () => {
        const t = twoTerminals();
        expect(t.focus()).toBe("A");
        expect(t.click("B")).toBe(true);
        await t.flush();
        expect(t.pasted.B).toEqual(["echo from the clipboard"]);
        expect(t.pasted.A).toEqual([]);
    });

    test("when the focus takes time to settle the paste waits for it and still lands in B", async () => {
        const t = twoTerminals({ focusDelayMs: 100 });
        t.click("B");
        await t.flush();
        expect(t.pasted.B.length).toBe(1);
        expect(t.pasted.A.length).toBe(0);
    });

    test("when the focus never settles nothing is pasted anywhere", async () => {
        const t = twoTerminals({ focusNeverSettles: true });
        expect(t.click("B")).toBe(true);
        await t.flush();
        expect(t.pasted.A.length + t.pasted.B.length).toBe(0);
        expect(t.focus()).toBe("A");
    });

    test("a right click on the terminal that has the focus pastes there", async () => {
        const t = twoTerminals();
        t.click("A");
        await t.flush();
        expect(t.pasted.A.length).toBe(1);
        expect(t.pasted.B.length).toBe(0);
    });

    test("with the setting off, or with Shift, nothing is pasted or copied and the menu opens", async () => {
        const off = twoTerminals({ setting: false });
        expect(off.click("B")).toBe(false);
        const shift = twoTerminals();
        expect(shift.click("B", { menuModifier: true, hasSelection: true })).toBe(false);
        await off.flush();
        await shift.flush();
        for (const t of [off, shift]) {
            expect(t.pasted.A.length + t.pasted.B.length).toBe(0);
            expect(t.copied).toEqual([]);
        }
        expect(off.focus()).toBe("A");
    });

    test("a program that takes the mouse: handled, so no Wave menu, and nothing is copied, pasted or focused", async () => {
        const t = twoTerminals();
        expect(t.click("B", { programTakesMouse: true })).toBe(true);
        await t.flush();
        expect(t.pasted.A.length + t.pasted.B.length).toBe(0);
        expect(t.copied).toEqual([]);
        expect(t.focus()).toBe("A");
    });

    test("with a selection it copies from the terminal that was clicked, then clears the selection, and pastes nothing", async () => {
        const t = twoTerminals();
        expect(t.click("B", { hasSelection: true })).toBe(true);
        await t.flush();
        expect(t.copied).toEqual(["B"]);
        expect(t.cleared).toEqual(["B"]);
        expect(t.pasted.A.length + t.pasted.B.length).toBe(0);
    });

    test("a clipboard that refuses the text leaves the selection where it is", async () => {
        const t = twoTerminals({ clipboardFails: true });
        expect(t.click("B", { hasSelection: true })).toBe(true);
        await t.flush();
        expect(t.copied).toEqual([]);
        expect(t.cleared).toEqual([]);
    });
});

describe("waiting for the focus", () => {
    test("returns true as soon as the terminal holds the focus, false after the timeout", async () => {
        let polls = 0;
        const slow = await waitForTerminalFocus(
            () => ++polls > 3,
            async () => {},
            300,
            20
        );
        expect(slow).toBe(true);
        expect(
            await waitForTerminalFocus(
                () => false,
                async () => {},
                100,
                20
            )
        ).toBe(false);
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
        expect(fn).toContain("isThisTerminalFocused");
        expect(fn).toContain("mouseTrackingMode");
        expect(fn).toContain("getApi().nativePaste()");
        expect(fn).toContain("trimTerminalSelection");
        expect(fn).toContain("navigator.clipboard.writeText(text)");
    });

    test("the setting exists in the types, the default config and the docs", () => {
        expect(read("pkg/wconfig/settingsconfig.go")).toContain('json:"term:rightclickpaste,omitempty"');
        expect(read("pkg/wconfig/defaultconfig/settings.json")).toContain('"term:rightclickpaste": true');
        expect(read("docs/docs/config.mdx")).toContain("term:rightclickpaste");
    });
});
