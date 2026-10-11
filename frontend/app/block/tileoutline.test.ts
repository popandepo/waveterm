// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { EphemeralOutline, tileOutlineColor } from "./tileoutline";

const base = { ephemeral: false };

describe("the outline of a focused tile", () => {
    test("block override, then the block's own colour (the agent), then the tab's flag colour, then the tab background, then the stylesheet", () => {
        const all = {
            ...base,
            focused: true,
            frameActiveBorderColor: "#111111",
            blockColor: "#222222",
            tabFlagColor: "#333333",
            tabActiveBorderColor: "#444444",
        };
        assert.equal(tileOutlineColor(all), "#111111");
        assert.equal(tileOutlineColor({ ...all, frameActiveBorderColor: null }), "#222222");
        assert.equal(tileOutlineColor({ ...all, frameActiveBorderColor: null, blockColor: undefined }), "#333333");
        assert.equal(tileOutlineColor({ ...all, frameActiveBorderColor: null, blockColor: "", tabFlagColor: null }), "#444444");
        assert.isUndefined(
            tileOutlineColor({ ...base, focused: true })
        );
    });

    test("a block colour that is not accepted counts as none", () => {
        assert.equal(tileOutlineColor({ ...base, focused: true, blockColor: "url(x)", tabFlagColor: "#333333" }), "#333333");
        assert.isUndefined(tileOutlineColor({ ...base, focused: true, blockColor: "var(--x)" }));
    });
});

describe("the outline of a tile that is not focused", () => {
    test("block override, then the tab's flag colour, then the tab background; the block's own colour is not used", () => {
        const all = {
            ...base,
            focused: false,
            frameBorderColor: "#111111",
            blockColor: "#222222",
            tabFlagColor: "#333333",
            tabBorderColor: "#444444",
        };
        assert.equal(tileOutlineColor(all), "#111111");
        assert.equal(tileOutlineColor({ ...all, frameBorderColor: null }), "#333333");
        assert.equal(tileOutlineColor({ ...all, frameBorderColor: null, tabFlagColor: undefined }), "#444444");
        assert.isUndefined(tileOutlineColor({ ...base, focused: false, blockColor: "#222222" }));
    });

    test("an ephemeral tile with no colour is white, one with a colour keeps the colour", () => {
        assert.equal(tileOutlineColor({ focused: false, ephemeral: true }), EphemeralOutline);
        assert.equal(tileOutlineColor({ focused: false, ephemeral: true, tabFlagColor: "#333333" }), "#333333");
    });
});

describe("the wiring that no test can render", () => {
    const read = (...p: string[]) => readFileSync(path.join(__dirname, "..", "..", ...p), "utf8");

    test("BlockMask asks tileOutlineColor with the block colour and the tab's flag colour, and the header is not filled", () => {
        const mask = read("app", "block", "blockframe.tsx");
        assert.include(mask, "tileOutlineColor({");
        assert.include(mask, 'waveEnv.getTabMetaKeyAtom(tabModel.tabId, "tab:flagcolor")');
        assert.include(mask, '"icon:color"');
        assert.notInclude(read("app", "block", "tileoutline.ts"), "backgroundColor");
    });
});
