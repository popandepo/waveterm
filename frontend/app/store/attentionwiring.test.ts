// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterEach, assert, beforeEach, describe, test, vi } from "vitest";
import { focusSwitchedSince, makeAttentionClearEnv, scheduleAttentionClear } from "./attentionschedule";

function sources() {
    const state = {
        docFocus: true,
        focused: "b1" as string | undefined,
        meta: { "attention:level": "need", "attention:reason": "why" } as Record<string, unknown>,
    };
    const written: Array<[string, string, string]> = [];
    const src = {
        documentHasFocus: () => state.docFocus,
        focusedBlockId: () => state.focused,
        readMeta: (_blockId: string, key: string) => state.meta[key],
        clearIf: (blockId: string, level: string, reason: string) => void written.push([blockId, level, reason]),
    };
    return { state, written, src };
}

describe("the attention env reads the present every time it is asked", () => {
    test("focused block, window focus and the mark are read at the call, not when the env was made", () => {
        const s = sources();
        const env = makeAttentionClearEnv(s.src);
        assert.equal(env.focusedBlockId(), "b1");
        s.state.focused = "b2";
        assert.equal(env.focusedBlockId(), "b2");
        assert.isTrue(env.documentHasFocus());
        s.state.docFocus = false;
        assert.isFalse(env.documentHasFocus());
        assert.deepEqual(env.read("b1"), { level: "need", reason: "why" });
        s.state.meta["attention:level"] = "urgent";
        assert.deepEqual(env.read("b1"), { level: "urgent", reason: "why" });
    });

    test("clear hands the block and the planned level and reason to the server's compare-and-clear", () => {
        const s = sources();
        makeAttentionClearEnv(s.src).clear("b9", "need", "why");
        assert.deepEqual(s.written, [["b9", "need", "why"]]);
    });

    describe("with the timer", () => {
        beforeEach(() => void vi.useFakeTimers());
        afterEach(() => void vi.useRealTimers());

        test("a focus that moved to another block while the timer ran stops the clear (the real env, not a fake)", () => {
            const s = sources();
            scheduleAttentionClear(
                { blockId: "b1", level: "need", reason: "why", documentHasFocus: true, focusSwitched: true },
                makeAttentionClearEnv(s.src)
            );
            s.state.focused = "b2";
            vi.advanceTimersByTime(60_000);
            assert.deepEqual(s.written, []);
        });

        test("the same plan with the focus unchanged writes once", () => {
            const s = sources();
            scheduleAttentionClear(
                { blockId: "b1", level: "need", reason: "why", documentHasFocus: true, focusSwitched: true },
                makeAttentionClearEnv(s.src)
            );
            vi.advanceTimersByTime(60_000);
            assert.deepEqual(s.written, [["b1", "need", "why"]]);
        });
    });
});

describe("focusSwitchedSince", () => {
    const a = { blockId: "b1", documentHasFocus: true };
    test("equal is not a switch", () => assert.isFalse(focusSwitchedSince(a, { ...a })));
    test("another block is a switch", () => assert.isTrue(focusSwitchedSince(a, { ...a, blockId: "b2" })));
    test("the window focus coming back is a switch", () =>
        assert.isTrue(focusSwitchedSince({ ...a, documentHasFocus: false }, a)));
    test("the first plan (nothing before) is a switch", () =>
        assert.isTrue(focusSwitchedSince({ blockId: undefined, documentHasFocus: false }, a)));
});
