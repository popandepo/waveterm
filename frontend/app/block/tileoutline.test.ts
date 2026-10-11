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

describe("every candidate is validated", () => {
    test("an override or tab background colour that is not accepted counts as not set, so the next one applies", () => {
        for (const bad of ["url(http://x/a.png)", "var(--x)", "red;background:blue", "rgb(1,\n2,3)", "expression(1)", 42, ""]) {
            assert.equal(tileOutlineColor({ ...base, focused: true, frameActiveBorderColor: bad, blockColor: "#222222" }), "#222222", String(bad));
            assert.equal(tileOutlineColor({ ...base, focused: true, tabActiveBorderColor: bad }), undefined, String(bad));
            assert.equal(tileOutlineColor({ ...base, focused: false, frameBorderColor: bad, tabFlagColor: "#333333" }), "#333333", String(bad));
            assert.equal(tileOutlineColor({ ...base, focused: false, tabBorderColor: bad }), undefined, String(bad));
        }
    });

    test("accepted overrides and tab background colours are used", () => {
        assert.equal(tileOutlineColor({ ...base, focused: true, frameActiveBorderColor: "tomato" }), "tomato");
        assert.equal(tileOutlineColor({ ...base, focused: true, tabActiveBorderColor: "#444444" }), "#444444");
        assert.equal(tileOutlineColor({ ...base, focused: false, tabBorderColor: "#444444" }), "#444444");
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
        // the outline goes to the border of the mask and nowhere else: not into the header, not into a fill
        const uses = [...mask.matchAll(/\boutline\b/g)].length;
        assert.equal(uses, 3, "outline is declared, tested and assigned to style.borderColor, nothing else");
        assert.include(mask, "style.borderColor = outline;");
        for (const file of ["blockframe-header.tsx", "blockutil.tsx"]) {
            const src = read("app", "block", file);
            assert.notInclude(src, "tileoutline");
            assert.notInclude(src, "tab:flagcolor");
        }
    });
});
