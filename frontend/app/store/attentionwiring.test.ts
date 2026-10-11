// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterEach, assert, beforeEach, describe, test, vi } from "vitest";
import { ClearAttentionMeta } from "./attentionclear";
import { focusSwitchedSince, makeAttentionClearEnv, scheduleAttentionClear } from "./attentionschedule";

function sources() {
    const state = {
        docFocus: true,
        focused: "b1" as string | undefined,
        meta: { "attention:level": "need", "attention:reason": "why" } as Record<string, unknown>,
    };
    const written: Array<[string, unknown]> = [];
    const src = {
        documentHasFocus: () => state.docFocus,
        focusedBlockId: () => state.focused,
        readMeta: (_blockId: string, key: string) => state.meta[key],
        writeMeta: (blockId: string, meta: unknown) => void written.push([blockId, meta]),
    };
    return { state, written, src };
}

describe("the attention env reads the present every time it is asked", () => {
    test("focused block, window focus and the mark are read at the call, not when the env was made", () => {
        const s = sources();
        const env = makeAttentionClearEnv(s.src, ClearAttentionMeta);
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

    test("clear writes exactly the two keys as null to the block that was asked for", () => {
        const s = sources();
        makeAttentionClearEnv(s.src, ClearAttentionMeta).clear("b9");
        assert.deepEqual(s.written, [["b9", { "attention:level": null, "attention:reason": null }]]);
    });

    describe("with the timer", () => {
        beforeEach(() => void vi.useFakeTimers());
        afterEach(() => void vi.useRealTimers());

        test("a focus that moved to another block while the timer ran stops the clear (the real env, not a fake)", () => {
            const s = sources();
            scheduleAttentionClear(
                { blockId: "b1", level: "need", reason: "why", documentHasFocus: true, focusSwitched: true },
                makeAttentionClearEnv(s.src, ClearAttentionMeta)
            );
            s.state.focused = "b2";
            vi.advanceTimersByTime(60_000);
            assert.deepEqual(s.written, []);
        });

        test("the same plan with the focus unchanged writes once", () => {
            const s = sources();
            scheduleAttentionClear(
                { blockId: "b1", level: "need", reason: "why", documentHasFocus: true, focusSwitched: true },
                makeAttentionClearEnv(s.src, ClearAttentionMeta)
            );
            vi.advanceTimersByTime(60_000);
            assert.equal(s.written.length, 1);
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
