// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { WindowButtonsWidth, WindowDragStripWidth, getWindowDragRightWidth } from "./tabbarlayout";

describe("the grab strip of the top row", () => {
    test("on Windows the strip comes on top of the room of the window buttons", () => {
        assert.equal(getWindowDragRightWidth(true, 1), WindowButtonsWidth + WindowDragStripWidth);
        assert.isAbove(getWindowDragRightWidth(true, 1), WindowButtonsWidth);
    });
    test("elsewhere the strip is still there", () => {
        assert.isAtLeast(getWindowDragRightWidth(false, 1), WindowDragStripWidth);
    });
    test("the width follows the zoom, and an unknown zoom counts as none", () => {
        assert.equal(getWindowDragRightWidth(true, 2), (WindowButtonsWidth + WindowDragStripWidth) / 2);
        assert.equal(getWindowDragRightWidth(true, 0), getWindowDragRightWidth(true, 1));
        assert.equal(getWindowDragRightWidth(true, -1), getWindowDragRightWidth(true, 1));
    });
    test("the strip is wide enough to grab, and not wider than a quarter of a small window", () => {
        assert.isAtLeast(WindowDragStripWidth, 64);
        assert.isAtMost(WindowDragStripWidth, 160);
    });
});

describe("the tab bar uses the strip (source pins)", () => {
    const source = readFileSync(path.resolve(__dirname, "tabbar.tsx"), "utf8");
    test("the right drag width comes from the layout module, not from a number in the bar", () => {
        assert.include(source, "getWindowDragRightWidth(env.isWindows(), zoomFactor)");
        assert.notInclude(source, "139 / zoomFactor");
    });
    test("the right block is counted in the room that the tabs do not get, and is a drag region", () => {
        assert.match(source, /rightContainerWidth\s*=\s*rightContainerRef\.current\?\.getBoundingClientRect\(\)\.width/);
        assert.match(source, /style=\{\{ width: windowDragRightWidth, WebkitAppRegion: "drag" \}/);
    });
});
