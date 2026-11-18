// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, test } from "vitest";
import { makeSwapModifierTracker } from "../lib/swapmodifier";

function drag(type: string, altKey: boolean): Event {
    const e: any = new Event(type);
    e.altKey = altKey;
    return e;
}

describe("the swap modifier tracker", () => {
    test("follows altKey on dragover", () => {
        const target = new EventTarget();
        const tracker = makeSwapModifierTracker(target);
        assert.isFalse(tracker.held());
        target.dispatchEvent(drag("dragover", true));
        assert.isTrue(tracker.held());
        target.dispatchEvent(drag("dragover", false));
        assert.isFalse(tracker.held());
    });

    test("is released when the drag ends or drops, so the next drag starts without it", () => {
        const target = new EventTarget();
        const tracker = makeSwapModifierTracker(target);
        target.dispatchEvent(drag("dragover", true));
        target.dispatchEvent(drag("dragend", true));
        assert.isFalse(tracker.held());
        target.dispatchEvent(drag("dragover", true));
        target.dispatchEvent(drag("drop", true));
        assert.isFalse(tracker.held());
    });

    test("a disposed tracker no longer listens", () => {
        const target = new EventTarget();
        const tracker = makeSwapModifierTracker(target);
        tracker.dispose();
        target.dispatchEvent(drag("dragover", true));
        assert.isFalse(tracker.held());
    });
});
