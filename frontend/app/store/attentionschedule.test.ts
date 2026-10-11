// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterEach, assert, beforeEach, describe, test, vi } from "vitest";
import { AttentionClearAfterStayMs, AttentionClearAfterSwitchMs } from "./attentionclear";
import { AttentionClearEnv, scheduleAttentionClear } from "./attentionschedule";

// There is no DOM test environment in this repository (no jsdom), so the effect of the app component is a thin call of this scheduler; what the
// component does (focus change, a changed mark, unmount) is tested here with fake timers, and the call is pinned in stacktabattention.test.ts.

function fakeEnv(initial: { level?: unknown; reason?: unknown; focused?: string; docFocus?: boolean } = {}) {
    const state = { level: initial.level, reason: initial.reason, focused: initial.focused, docFocus: initial.docFocus ?? true };
    const cleared: string[] = [];
    const env: AttentionClearEnv = {
        documentHasFocus: () => state.docFocus,
        focusedBlockId: () => state.focused,
        read: () => ({ level: state.level, reason: state.reason }),
        clear: (blockId) => void cleared.push(blockId),
    };
    return { env, state, cleared };
}

beforeEach(() => {
    vi.useFakeTimers();
});
afterEach(() => {
    vi.useRealTimers();
});

describe("the planned clearing of a mark", () => {
    test("an urgent mark on the focused block is cleared after the delay, once, for that block", () => {
        const f = fakeEnv({ level: "urgent", reason: "why", focused: "b1" });
        const cancel = scheduleAttentionClear({ blockId: "b1", level: "urgent", reason: "why", documentHasFocus: true, focusSwitched: true }, f.env);
        assert.isFunction(cancel);
        vi.advanceTimersByTime(AttentionClearAfterSwitchMs - 1);
        assert.deepEqual(f.cleared, []);
        vi.advanceTimersByTime(2);
        assert.deepEqual(f.cleared, ["b1"]);
        vi.advanceTimersByTime(10_000);
        assert.deepEqual(f.cleared, ["b1"]);
    });

    test("a tile that already had the focus waits longer", () => {
        const f = fakeEnv({ level: "need", reason: "why", focused: "b1" });
        scheduleAttentionClear({ blockId: "b1", level: "need", reason: "why", documentHasFocus: true, focusSwitched: false }, f.env);
        vi.advanceTimersByTime(AttentionClearAfterSwitchMs + 100);
        assert.deepEqual(f.cleared, []);
        vi.advanceTimersByTime(AttentionClearAfterStayMs);
        assert.deepEqual(f.cleared, ["b1"]);
    });

    test("nothing is planned for info, for no mark, for a window without focus or without a block", () => {
        const f = fakeEnv({ focused: "b1" });
        assert.isUndefined(scheduleAttentionClear({ blockId: "b1", level: "info", reason: "", documentHasFocus: true, focusSwitched: true }, f.env));
        assert.isUndefined(scheduleAttentionClear({ blockId: "b1", level: undefined, reason: undefined, documentHasFocus: true, focusSwitched: true }, f.env));
        assert.isUndefined(scheduleAttentionClear({ blockId: "b1", level: "urgent", reason: "x", documentHasFocus: false, focusSwitched: true }, f.env));
        assert.isUndefined(scheduleAttentionClear({ blockId: undefined, level: "urgent", reason: "x", documentHasFocus: true, focusSwitched: true }, f.env));
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(f.cleared, []);
    });

    test("cancelling (the component's cleanup: focus moved, unmount, a new plan) means no write", () => {
        const f = fakeEnv({ level: "urgent", reason: "why", focused: "b1" });
        const cancel = scheduleAttentionClear({ blockId: "b1", level: "urgent", reason: "why", documentHasFocus: true, focusSwitched: true }, f.env)!;
        vi.advanceTimersByTime(AttentionClearAfterSwitchMs - 100);
        cancel();
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(f.cleared, []);
    });

    test("the focus left the block, or the window lost its focus, before the timer fired: no write", () => {
        const a = fakeEnv({ level: "urgent", reason: "why", focused: "b1" });
        scheduleAttentionClear({ blockId: "b1", level: "urgent", reason: "why", documentHasFocus: true, focusSwitched: true }, a.env);
        a.state.focused = "b2";
        vi.advanceTimersByTime(5000);
        assert.deepEqual(a.cleared, []);
        const b = fakeEnv({ level: "urgent", reason: "why", focused: "b1" });
        scheduleAttentionClear({ blockId: "b1", level: "urgent", reason: "why", documentHasFocus: true, focusSwitched: true }, b.env);
        b.state.docFocus = false;
        vi.advanceTimersByTime(5000);
        assert.deepEqual(b.cleared, []);
    });
});

describe("a newer mark is never erased by the plan of an older one", () => {
    test("the reason was replaced while the timer ran and the component did not re-plan yet: the check at the moment of the write refuses", () => {
        const f = fakeEnv({ level: "need", reason: "first", focused: "b1" });
        scheduleAttentionClear({ blockId: "b1", level: "need", reason: "first", documentHasFocus: true, focusSwitched: true }, f.env);
        f.state.reason = "second"; // a program wrote a new reason
        vi.advanceTimersByTime(5000);
        assert.deepEqual(f.cleared, [], "the new mark must survive");
    });

    test("the level was replaced meanwhile: also refused", () => {
        const f = fakeEnv({ level: "need", reason: "x", focused: "b1" });
        scheduleAttentionClear({ blockId: "b1", level: "need", reason: "x", documentHasFocus: true, focusSwitched: true }, f.env);
        f.state.level = "urgent";
        vi.advanceTimersByTime(5000);
        assert.deepEqual(f.cleared, []);
    });

    test("the component's re-plan on a changed reason (cancel, then plan for the new mark) gives the new mark its own full wait", () => {
        const f = fakeEnv({ level: "need", reason: "first", focused: "b1" });
        const cancel = scheduleAttentionClear({ blockId: "b1", level: "need", reason: "first", documentHasFocus: true, focusSwitched: true }, f.env)!;
        vi.advanceTimersByTime(300);
        f.state.reason = "second";
        cancel();
        scheduleAttentionClear({ blockId: "b1", level: "need", reason: "second", documentHasFocus: true, focusSwitched: false }, f.env);
        vi.advanceTimersByTime(AttentionClearAfterStayMs - 1);
        assert.deepEqual(f.cleared, [], "not erased early");
        vi.advanceTimersByTime(2);
        assert.deepEqual(f.cleared, ["b1"]);
    });
});
