// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { lastSegment, memberColorValue, memberSymbolValue, stackTabLook } from "./stacktablook";

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
        const fallback = stackTabLook("a\u202eb", "star", undefined, validate);
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

describe("the real validation path (no stub)", () => {
    test("colours: hex, functions with plain arguments and keywords pass the shape check; everything else is refused", () => {
        for (const ok of ["#f00", "#ff0000", "#ff000080", "rgb(255, 0, 0)", "hsl(120deg 50% 50% / 0.5)", "oklch(60% 0.2 30)", "tomato"]) {
            assert.equal(memberColorValue(ok), ok, ok);
        }
        for (const bad of [
            "url(http://x/a.png)",
            "var(--accent-color)",
            "expression(alert(1))",
            "rgb(var(--x))",
            "calc(1px + 2px)",
            "red; background: url(x)",
            "red\nblue",
            "#ggg",
            "#ff",
            "rgb(" + "1,".repeat(40) + "1)",
            "a".repeat(65),
            "x",
            "",
            "   ",
        ]) {
            assert.isNull(memberColorValue(bad), bad);
        }
    });

    test("symbols: the longest icon name is 40 characters, text is at most two graphemes and 32 bytes", () => {
        assert.deepEqual(memberSymbolValue("a".repeat(40)), { kind: "icon", value: "a".repeat(40) });
        assert.isNull(memberSymbolValue("a".repeat(41)));
        assert.isNull(memberSymbolValue("ab cd"));
        assert.deepEqual(memberSymbolValue("\u{1F680}\u{1F680}"), { kind: "text", value: "\u{1F680}\u{1F680}" });
        assert.isNull(memberSymbolValue("\u{1F680}\u{1F680}\u{1F680}"));
        assert.isNull(memberSymbolValue("<b>x</b>"));
    });

    test("combining characters: one base with up to 15 marks is one grapheme and fits 32 bytes; one more mark is refused", () => {
        const base = "e";
        assert.equal(memberSymbolValue(base + "\u0301".repeat(15))?.kind, "text");
        assert.isNull(memberSymbolValue(base + "\u0301".repeat(16)));
    });
});

describe("the label of a member without a title", () => {
    test("a Windows cwd and a Unix cwd give the last folder, a trailing separator is ignored", () => {
        assert.equal(lastSegment("C:\\Users\\x\\repo"), "repo");
        assert.equal(lastSegment("C:\\Users\\x\\repo\\"), "repo");
        assert.equal(lastSegment("/home/x/repo"), "repo");
        assert.equal(lastSegment("C:/Users/x/repo"), "repo");
        assert.equal(lastSegment("/"), "/");
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
        assert.include(css, "isolation: isolate;");
        assert.include(css, "z-index: -1;");
        assert.include(css, "max-width: 3em;");
    });
});
