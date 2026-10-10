// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { registerStackKeys, StackNextKey, StackPrevKey } from "./stackkeys";

function setup(modelResult: boolean) {
    const map = new Map<string, (e: any) => boolean>();
    const calls: number[] = [];
    registerStackKeys(map, () => ({
        focusAdjacentStackMember: (offset: number) => {
            calls.push(offset);
            return modelResult;
        },
    }));
    return { map, calls };
}

describe("the keys for the tabs inside a tile", () => {
    test("both chords are registered, and no other", () => {
        const { map } = setup(true);
        expect([...map.keys()].sort()).toEqual([StackNextKey, StackPrevKey].sort());
        expect(StackNextKey).toBe("Ctrl:Shift:PageDown");
        expect(StackPrevKey).toBe("Ctrl:Shift:PageUp");
    });

    test("next goes one step forward, previous one step back, and the key is taken", () => {
        const { map, calls } = setup(true);
        expect(map.get(StackNextKey)!({})).toBe(true);
        expect(map.get(StackPrevKey)!({})).toBe(true);
        expect(calls).toEqual([1, -1]);
    });

    test("when the focused tile is not a stack or has one tab the key is not taken, so it passes on", () => {
        const { map, calls } = setup(false);
        expect(map.get(StackNextKey)!({})).toBe(false);
        expect(map.get(StackPrevKey)!({})).toBe(false);
        expect(calls).toEqual([1, -1]);
    });

    test("the key map of the app registers them through this function", () => {
        const src = readFileSync(path.join(__dirname, "keymodel.ts"), "utf8");
        expect(src).toContain("registerStackKeys(globalKeyMap");
        expect(src).not.toContain('globalKeyMap.set("Ctrl:Shift:PageDown"');
    });

    test("none of the chords collides with a chord that the app binds itself", () => {
        const src = readFileSync(path.join(__dirname, "keymodel.ts"), "utf8");
        const bound = [...src.matchAll(/globalKeyMap\.set\("([^"]+)"/g)].map((m) => m[1]);
        expect(bound).not.toContain(StackNextKey);
        expect(bound).not.toContain(StackPrevKey);
    });
});
