// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { stackTabLook } from "./stacktablook";

// CSS.supports does not exist outside a window; this stands in for it with the colours the tests use
const validate = (c: string) => {
    if (/^(#[0-9a-f]{3,8}|red|rgb\([^)]*\))$/i.test(c.trim())) {
        return "x";
    }
    throw new Error("Invalid CSS color: " + c);
};

describe("what a member's strip tab shows", () => {
    test("frame:icon wins over icon, a bad frame:icon falls back to icon", () => {
        assert.deepEqual(stackTabLook("rocket", "star", "#f00", validate), {
            symbol: { kind: "icon", value: "rocket" },
            color: "#f00",
        });
        const fallback = stackTabLook("a‮b", "star", undefined, validate);
        assert.deepEqual(fallback.symbol, { kind: "icon", value: "star" });
    });

    test("an emoji is a text symbol, and one of three graphemes is refused", () => {
        assert.deepEqual(stackTabLook("\u{1F680}", undefined, undefined, validate).symbol, {
            kind: "text",
            value: "\u{1F680}",
        });
        assert.isNull(stackTabLook("\u{1F680}\u{1F680}\u{1F680}", undefined, undefined, validate).symbol);
    });

    test("a member without icon and colour looks as before", () => {
        assert.deepEqual(stackTabLook(undefined, undefined, undefined, validate), { symbol: null, color: null });
        assert.deepEqual(stackTabLook("", "", "", validate), { symbol: null, color: null });
    });

    test("a colour that is not a CSS colour counts as no colour", () => {
        assert.isNull(stackTabLook(undefined, undefined, "red; background:url(x)", validate).color);
        assert.isNull(stackTabLook(undefined, undefined, 42, validate).color);
        assert.equal(stackTabLook(undefined, undefined, " red ", validate).color, "red");
    });
});

describe("the wiring that no test can render", () => {
    const root = path.join(__dirname, "..", "..");
    test("the label draws symbol, line and fill, and the stylesheet places them against the tab", () => {
        const label = readFileSync(path.join(__dirname, "stacktablabel.tsx"), "utf8");
        assert.include(label, "stackTabLook(frameIcon, icon, iconColor)");
        assert.include(label, "stack-tab-line");
        assert.include(label, "stack-tab-fill");
        const css = readFileSync(path.join(root, "layout", "lib", "stackstrip.scss"), "utf8");
        assert.include(css, "position: relative;");
        assert.include(css, "&.active .stack-tab-fill");
    });
});
