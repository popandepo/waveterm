// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { rightClickAction } from "./rightclick";

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
});

describe("the wiring", () => {
    const read = (rel: string) => readFileSync(path.join(__dirname, "..", "..", "..", "..", rel), "utf8");

    test("the context menu handler asks the model first, and the model uses the same paste and copy as the menu items", () => {
        const term = read("frontend/app/view/term/term.tsx");
        const handler = term.slice(
            term.indexOf("const handleContextMenu"),
            term.indexOf("return (", term.indexOf("const handleContextMenu"))
        );
        expect(handler).toContain("model.handleRightClick(");
        const model = read("frontend/app/view/term/term-model.ts");
        const fn = model.slice(model.indexOf("handleRightClick("), model.indexOf("getContextMenuItems()"));
        expect(fn).toContain("rightClickAction(");
        expect(fn).toContain("term:rightclickpaste");
        expect(fn).toContain("getApi().nativePaste()");
        expect(fn).toContain("trimTerminalSelection");
    });

    test("the setting exists in the types, the default config and the docs", () => {
        expect(read("pkg/wconfig/settingsconfig.go")).toContain('json:"term:rightclickpaste,omitempty"');
        expect(read("pkg/wconfig/defaultconfig/settings.json")).toContain('"term:rightclickpaste": true');
        expect(read("docs/docs/config.mdx")).toContain("term:rightclickpaste");
    });
});
