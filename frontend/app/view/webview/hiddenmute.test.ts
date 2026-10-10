// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { applyGuestMute, audioMuteChange, effectiveAudioMuted } from "./hiddenmute";

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

    function guest(muted = false) {
        const calls: boolean[] = [];
        return {
            calls,
            isAudioMuted: () => muted,
            setAudioMuted: (m: boolean) => {
                muted = m;
                calls.push(m);
            },
        };
    }

    test("applying to a guest: hide mutes, show unmutes, repeating changes nothing", () => {
        const g = guest();
        expect(applyGuestMute(g, false, true)).toBe(true);
        expect(applyGuestMute(g, false, true)).toBe(false);
        expect(applyGuestMute(g, false, false)).toBe(true);
        expect(applyGuestMute(g, false, false)).toBe(false);
        expect(g.calls).toEqual([true, false]);
    });

    test("the owner's mute is kept through hide and show", () => {
        const g = guest(true);
        expect(applyGuestMute(g, true, true)).toBe(false);
        expect(applyGuestMute(g, true, false)).toBe(false);
        expect(g.calls).toEqual([]);
    });

    test("a guest that replaces the old one (reload, navigation) is corrected the next time it is applied, with the same inputs", () => {
        const first = guest();
        applyGuestMute(first, false, true);
        expect(first.isAudioMuted()).toBe(true);
        const replacement = guest(false);
        expect(applyGuestMute(replacement, false, true)).toBe(true);
        expect(replacement.isAudioMuted()).toBe(true);
    });

    test("no guest yet, or none any more, is a no-op", () => {
        expect(applyGuestMute(null, false, true)).toBe(false);
        expect(applyGuestMute(undefined, false, true)).toBe(false);
    });

    // The component cannot be mounted here (no DOM test environment is installed, as with react-dnd), so the places that call the
    // applier are pinned in the source and are on the window check list.
    test("the web view applies it when a guest page is ready and stops loading, when the inputs change, and the mute button toggles the owner's choice", () => {
        const src = readFileSync(path.join(__dirname, "webview.tsx"), "utf8");
        const domReadyHandler = src.slice(src.indexOf("const handleDomReady = () => {"));
        expect(domReadyHandler.slice(0, domReadyHandler.indexOf("};"))).toContain("model.applyAudioMute();");
        const stopHandler = src.slice(src.indexOf("const stopLoadingHandler = () => {"));
        expect(stopHandler.slice(0, stopHandler.indexOf("};"))).toContain("model.applyAudioMute();");
        expect(src).toContain("}, [domReady, hiddenInStack, mutedByOwner]);");
        expect(src).toContain("model.nodeModel.isHidden");
        expect(src).toContain("!globalStore.get(this.mediaMuted)");
        expect(src).not.toContain("!this.webviewRef.current?.isAudioMuted()");
        expect(src.match(/setAudioMuted\(/g)).toBeNull();
    });
});
