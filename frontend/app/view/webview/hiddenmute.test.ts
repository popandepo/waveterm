// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { audioMuteChange, effectiveAudioMuted } from "./hiddenmute";

describe("sound of a web tile that is hidden behind another tab of its stack", () => {
    test("hidden means muted, shown means the owner's choice again", () => {
        expect(effectiveAudioMuted(false, true)).toBe(true);
        expect(effectiveAudioMuted(false, false)).toBe(false);
        expect(effectiveAudioMuted(true, true)).toBe(true);
        expect(effectiveAudioMuted(true, false)).toBe(true);
    });

    test("a tile the owner muted stays muted after it is shown", () => {
        expect(audioMuteChange(true, true, false)).toBeUndefined();
    });

    test("hiding a playing tile mutes it, showing it unmutes it, and nothing is touched when nothing changes", () => {
        expect(audioMuteChange(false, false, true)).toBe(true);
        expect(audioMuteChange(true, false, false)).toBe(false);
        expect(audioMuteChange(false, false, false)).toBeUndefined();
        expect(audioMuteChange(true, false, true)).toBeUndefined();
    });

    test("the web view applies it: reads the hidden flag of its node, and the mute button toggles the owner's choice, not the effective one", () => {
        const src = readFileSync(path.join(__dirname, "webview.tsx"), "utf8");
        expect(src).toContain("audioMuteChange(");
        expect(src).toContain("model.nodeModel.isHidden");
        expect(src).toContain("!globalStore.get(this.mediaMuted)");
        expect(src).not.toContain("!this.webviewRef.current?.isAudioMuted()");
    });
});
