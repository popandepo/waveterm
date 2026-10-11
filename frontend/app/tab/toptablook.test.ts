// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { ActiveFillPercent, InactiveLinePercent, topTabLook } from "./toptablook";

describe("how a tab of the top row shows its colour", () => {
    test("the active tab is filled with its colour and has its full line", () => {
        const look = topTabLook("#ff8800", true);
        assert.equal(look.line, "#ff8800");
        assert.equal(look.fill, `color-mix(in srgb, #ff8800 ${ActiveFillPercent}%, transparent)`);
    });

    test("the other tabs have a dimmer line and no fill", () => {
        const look = topTabLook("#ff8800", false);
        assert.isNull(look.fill);
        assert.equal(look.line, `color-mix(in srgb, #ff8800 ${InactiveLinePercent}%, transparent)`);
    });

    test("a tab without a colour, or with one that is not accepted, shows nothing", () => {
        for (const bad of [undefined, null, "", "  ", "url(http://x/a.png)", "var(--x)", "red; background: blue", 5, "rgb(1,\n2,3)"]) {
            assert.deepEqual(topTabLook(bad, true), { fill: null, line: null }, String(bad));
            assert.deepEqual(topTabLook(bad, false), { fill: null, line: null }, String(bad));
        }
    });

    test("named colours and functions are accepted like everywhere else", () => {
        assert.equal(topTabLook("tomato", true).line, "tomato");
        assert.equal(topTabLook("rgb(255, 136, 0)", true).line, "rgb(255, 136, 0)");
    });
});

describe("the wiring that no test can render", () => {
    const read = (...p: string[]) => readFileSync(path.join(__dirname, ...p), "utf8");

    test("the tab draws the fill and the line from topTabLook, inside the tab, behind the name", () => {
        const tab = read("tab.tsx");
        assert.include(tab, "const look = topTabLook(flagColor, active);");
        assert.include(tab, 'look.fill != null && <span className="tab-colour-fill"');
        assert.include(tab, 'look.line != null && <span className="tab-colour-line"');
        const inner = tab.slice(tab.indexOf('<div className="tab-inner">'));
        assert.isBelow(inner.indexOf("tab-colour-fill"), inner.indexOf('className={clsx("name"'));
    });

    test("only the tab is coloured: the stylesheet adds two small elements and changes no rule of the bar, the gaps or the headers", () => {
        const css = read("tab.scss");
        const rule = (selector: string) => {
            const start = css.indexOf(selector + " {");
            assert.isAbove(start, -1, selector + " exists");
            return css.slice(start, css.indexOf("}", start));
        };
        // the geometry: the fill covers the tab, the line sits on its lower edge; without these the colour vanishes or shifts the layout
        const fill = rule(".tab-colour-fill");
        assert.include(fill, "position: absolute;");
        assert.include(fill, "inset: 0;");
        assert.include(fill, "pointer-events: none;");
        const line = rule(".tab-colour-line");
        assert.include(line, "position: absolute;");
        assert.include(line, "left: 6px;");
        assert.include(line, "right: 6px;");
        assert.include(line, "bottom: 0;");
        assert.include(line, "height: 2px;");
        assert.include(line, "pointer-events: none;");
        // the tab itself is the positioning parent of its inner part
        assert.include(css.slice(css.indexOf(".tab-inner {"), css.indexOf(".tab-inner {") + 120), "position: relative;");
        assert.notInclude(css, "tabbar-colour");
    });
});
