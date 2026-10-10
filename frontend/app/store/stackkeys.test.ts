// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { newLayoutNode } from "../../layout/lib/layoutNode";
import { adjacentStackMember, stackNodes, turnIntoStack } from "../../layout/lib/layoutTree";
import { FlexDirection, LayoutTreeActionType } from "../../layout/lib/types";
import { newLayoutTreeState } from "../../layout/tests/model";
import * as keyutil from "../../util/keyutil";
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

    // Same step as appHandleKeyDown takes for the global key map: find the chord for the event, run its handler, and the key
    // is consumed only when the handler says true.
    function dispatch(map: Map<string, (e: any) => boolean>, event: any): boolean {
        for (const key of map.keys()) {
            if (keyutil.checkKeyPressed(event, key)) {
                return map.get(key)(event);
            }
        }
        return false;
    }

    function chord(key: string) {
        return {
            control: true,
            shift: true,
            cmd: false,
            option: false,
            alt: false,
            meta: false,
            key,
            code: key,
            type: "keydown",
        };
    }

    function leaf(blockId: string) {
        return newLayoutNode(FlexDirection.Column, undefined, undefined, { blockId });
    }

    // The model is the real tree rule (adjacentStackMember) on a real tree, with the focused node in it.
    function dispatchOn(rootBuilder: () => { root: any; focused: string }, key: string) {
        const { root, focused } = rootBuilder();
        const map = new Map<string, (e: any) => boolean>();
        const shown: string[] = [];
        registerStackKeys(map, () => ({
            focusAdjacentStackMember: (offset: number) => {
                const target = adjacentStackMember(root, focused, offset);
                if (!target) return false;
                shown.push(target);
                return true;
            },
        }));
        return { consumed: dispatch(map, chord(key)), shown };
    }

    function stackOf(n: number) {
        const leaves = Array.from({ length: n }, (_, i) => leaf("b" + i));
        const root = newLayoutNode(FlexDirection.Row, undefined, [...leaves, leaf("other")]);
        const state = newLayoutTreeState(root);
        for (let i = 1; i < n; i++) {
            stackNodes(state, {
                type: LayoutTreeActionType.Stack,
                targetNodeId: leaves[i - 1].id,
                nodeId: leaves[i].id,
            });
        }
        return { root: state.rootNode, first: leaves[0].id, ids: leaves.map((l) => l.id) };
    }

    test("through the dispatch with an event: a stack of three consumes the chord and shows the next or previous tab", () => {
        const s = stackOf(3);
        const next = dispatchOn(() => ({ root: s.root, focused: s.ids[0] }), "PageDown");
        expect(next.consumed).toBe(true);
        expect(next.shown).toEqual([s.ids[1]]);
        const prev = dispatchOn(() => ({ root: s.root, focused: s.ids[0] }), "PageUp");
        expect(prev.consumed).toBe(true);
        expect(prev.shown).toEqual([s.ids[2]]);
    });

    test("through the dispatch with an event: a tile that is not a stack does not consume the chord", () => {
        const s = stackOf(3);
        const outside = s.root.children.find((c: any) => c.data?.blockId === "other");
        for (const key of ["PageDown", "PageUp"]) {
            const r = dispatchOn(() => ({ root: s.root, focused: outside.id }), key);
            expect(r.consumed).toBe(false);
            expect(r.shown).toEqual([]);
        }
    });

    test("through the dispatch with an event: a stack with one tab does not consume the chord", () => {
        const only = leaf("only");
        const state = newLayoutTreeState(newLayoutNode(FlexDirection.Row, undefined, [only, leaf("other")]));
        expect(turnIntoStack(state, only.id)).toBe(true);
        for (const key of ["PageDown", "PageUp"]) {
            const r = dispatchOn(() => ({ root: state.rootNode, focused: only.id }), key);
            expect(r.consumed).toBe(false);
            expect(r.shown).toEqual([]);
        }
    });

    test("an event that is not one of the two chords is not consumed", () => {
        const s = stackOf(3);
        const r = dispatchOn(() => ({ root: s.root, focused: s.ids[0] }), "PageHome");
        expect(r.consumed).toBe(false);
        expect(r.shown).toEqual([]);
    });
});
