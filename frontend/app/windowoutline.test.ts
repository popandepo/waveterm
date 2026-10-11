// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { windowOutlineColor, WindowOutlineWidthPx } from "./windowoutline";

describe("the colour of the window outline", () => {
    test("the workspace accent wins, then the colour of the workspace, else no outline", () => {
        assert.equal(windowOutlineColor("#112233", "#445566"), "#112233");
        assert.equal(windowOutlineColor(undefined, "#445566"), "#445566");
        assert.equal(windowOutlineColor("", "tomato"), "tomato");
        assert.isNull(windowOutlineColor(undefined, undefined));
        assert.isNull(windowOutlineColor(null, ""));
    });

    test("a value that is not accepted counts as not set, so the next one applies", () => {
        for (const bad of ["url(http://x/a.png)", "var(--x)", "red; background: blue", "rgb(1,\n2,3)", 7, {}]) {
            assert.equal(windowOutlineColor(bad, "#445566"), "#445566", String(bad));
            assert.isNull(windowOutlineColor(bad, bad), String(bad));
        }
    });

    test("the width is a few pixels", () => {
        assert.isAbove(WindowOutlineWidthPx, 0);
        assert.isBelow(WindowOutlineWidthPx, 8);
    });
});

describe("the wiring that no test can render", () => {
    const read = (...p: string[]) => readFileSync(path.join(__dirname, ...p), "utf8");

    test("the app draws the outline from the workspace's accent and colour, once, as a border", () => {
        const app = read("app.tsx");
        assert.include(app, 'windowOutlineColor(workspace?.meta?.["ws:accent"], workspace?.color)');
        assert.include(app, "<WindowOutline />");
        assert.equal(app.split("<WindowOutline />").length - 1, 1);
        assert.include(app, "borderColor: color, borderWidth: WindowOutlineWidthPx");
        // nothing is filled: no background in the component
        const body = app.slice(app.indexOf("const WindowOutline = "), app.indexOf("const AppInner"));
        assert.notInclude(body, "background");
    });

    test("the stylesheet makes it a border that takes no mouse events, below the modals", () => {
        const css = read("app.scss");
        const rule = css.slice(css.indexOf(".window-outline {"), css.indexOf(".is-transparent {"));
        assert.include(rule, "pointer-events: none;");
        assert.include(rule, "border-style: solid;");
        assert.include(rule, "position: fixed;");
        assert.include(rule, "z-index: 50;");
        assert.notInclude(rule, "background");
    });
});
