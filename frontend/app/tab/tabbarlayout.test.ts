// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import {
    TabDefaultWidth,
    TabMinWidth,
    WindowButtonsWidth,
    WindowDragRightEdgeWidth,
    WindowDragStripWidth,
    computeTabLayout,
    getWindowDragRightWidth,
    tabsContentWidth,
    tabsEndScrollLeft,
} from "./tabbarlayout";

// What a browser does with scrollLeft on an overflowing box: it is clamped to [0, content - viewport].
function clampedScroll(wanted: number, content: number, viewport: number): number {
    return Math.min(Math.max(wanted, 0), Math.max(content - viewport, 0));
}

describe("the grab strip of the top row", () => {
    test("on Windows the strip comes on top of the room of the window buttons", () => {
        assert.equal(getWindowDragRightWidth(true, 1), WindowButtonsWidth + WindowDragStripWidth);
    });
    test("elsewhere the right end keeps its old fixed 12 px edge at any zoom, and no strip", () => {
        for (const zoom of [0, 0.5, 1, 2]) {
            assert.equal(getWindowDragRightWidth(false, zoom), WindowDragRightEdgeWidth);
        }
    });
    test("the width follows the zoom, and an unknown zoom counts as none", () => {
        assert.equal(getWindowDragRightWidth(true, 2), (WindowButtonsWidth + WindowDragStripWidth) / 2);
        assert.equal(getWindowDragRightWidth(true, 0), getWindowDragRightWidth(true, 1));
        assert.equal(getWindowDragRightWidth(true, -1), getWindowDragRightWidth(true, 1));
    });
});

describe("the tabs never take the right block", () => {
    const wrapper = 1200;
    const others = 200; // left dragger, menu, switcher, add button
    const right = getWindowDragRightWidth(true, 1);

    test("a few tabs get their default width and nothing scrolls", () => {
        const l = computeTabLayout(wrapper, others + right, 3);
        assert.equal(l.tabWidth, TabDefaultWidth);
        assert.isFalse(l.scrollable);
        assert.isAtMost(3 * l.tabWidth, l.spaceForTabs);
    });

    test("more tabs shrink to fit beside the strip", () => {
        const space = wrapper - others - right;
        const n = Math.floor(space / TabMinWidth);
        const l = computeTabLayout(wrapper, others + right, n);
        assert.isBelow(l.tabWidth, TabDefaultWidth);
        assert.isAtLeast(l.tabWidth, TabMinWidth);
        assert.isFalse(l.scrollable);
        assert.isAtMost(n * l.tabWidth, space + 1e-9, "the tabs end before the strip");
    });

    test("a full row scrolls at minimum width instead of covering the strip, and the last tab is reachable", () => {
        const n = 40;
        const l = computeTabLayout(wrapper, others + right, n);
        assert.equal(l.tabWidth, TabMinWidth);
        assert.isTrue(l.scrollable);
        assert.equal(l.spaceForTabs, wrapper - others - right, "the strip is outside the tab viewport");
        // the bar sizes the tabs box with tabsContentWidth and scrolls to the end with tabsEndScrollLeft
        const content = tabsContentWidth(n, l.tabWidth);
        const left = clampedScroll(tabsEndScrollLeft(n, l.tabWidth), content, l.spaceForTabs);
        const lastTabStart = (n - 1) * l.tabWidth;
        const lastTabEnd = n * l.tabWidth;
        assert.isAtMost(left, lastTabStart, "scrolled to the end, the last tab starts inside the viewport");
        assert.isAtLeast(left + l.spaceForTabs, lastTabEnd, "and ends inside it");
    });

    test("the strip is what makes the difference: without it the same row would not scroll", () => {
        const space = wrapper - others - WindowButtonsWidth;
        const n = Math.floor(space / TabMinWidth);
        assert.isFalse(computeTabLayout(wrapper, others + WindowButtonsWidth, n).scrollable);
        assert.isTrue(computeTabLayout(wrapper, others + right, n).scrollable);
    });

    test("no tabs gives no width and no scroll", () => {
        const l = computeTabLayout(wrapper, others + right, 0);
        assert.equal(l.tabWidth, TabDefaultWidth);
        assert.isFalse(l.scrollable);
        const narrow = computeTabLayout(300, others + right, 0);
        assert.equal(narrow.tabWidth, TabDefaultWidth, "controls wider than the row and no tabs");
        assert.isFalse(narrow.scrollable, "nothing to scroll when there are no tabs");
    });
});

describe("the tab bar is wired to the layout module", () => {
    // The behaviour is tested above; this only pins that the bar uses it. Dragging the window by the strip needs a real window.
    const source = readFileSync(path.resolve(__dirname, "tabbar.tsx"), "utf8");
    test("the bar takes the right width and the tab layout from the module", () => {
        assert.include(source, "getWindowDragRightWidth(env.isWindows(), zoomFactor)");
        assert.include(source, "computeTabLayout(");
        assert.include(source, "tabsContentWidth(tabIds.length, tabWidthRef.current)");
        assert.include(source, "tabsEndScrollLeft(tabIds.length, tabWidthRef.current)");
        assert.notInclude(source, "139 / zoomFactor");
    });
});
