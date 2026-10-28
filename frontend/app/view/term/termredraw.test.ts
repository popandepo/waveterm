// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { RedrawableTerm, RedrawScheduler, redrawTerm } from "./termredraw";

function fakeTerm(rows = 24) {
    const calls: string[] = [];
    const term: RedrawableTerm = {
        rows,
        clearTextureAtlas: () => calls.push("clearatlas"),
        refresh: (start, end) => calls.push(`refresh ${start}-${end}`),
    };
    return { term, calls };
}

describe("redrawTerm", () => {
    test("drops the texture atlas, then repaints every row", () => {
        const { term, calls } = fakeTerm(30);
        redrawTerm(term);
        expect(calls).toEqual(["clearatlas", "refresh 0-29"]);
    });

    test("does nothing for a terminal with no rows", () => {
        const { term, calls } = fakeTerm(0);
        redrawTerm(term);
        expect(calls).toEqual([]);
    });
});

function rig(rows = 24) {
    const { term, calls } = fakeTerm(rows);
    const frames = new Map<number, () => void>();
    const cancelled: number[] = [];
    let nextHandle = 1;
    let releaseFonts!: () => void;
    const fontsReady = new Promise<void>((resolve) => (releaseFonts = resolve));
    const scheduler = new RedrawScheduler(
        term,
        fontsReady,
        (cb) => {
            frames.set(nextHandle, cb);
            return nextHandle++;
        },
        (h) => {
            cancelled.push(h);
            frames.delete(h);
        }
    );
    return { scheduler, calls, frames, cancelled, releaseFonts };
}

const tick = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

describe("RedrawScheduler", () => {
    test("waits for fonts and the next frame before repainting", async () => {
        const { scheduler, calls, frames, releaseFonts } = rig();
        const done = scheduler.schedule();
        await tick();
        expect(calls).toEqual([]);
        expect(frames.size).toBe(0);
        releaseFonts();
        await tick();
        expect(calls).toEqual([]);
        expect(frames.size).toBe(1);
        frames.get(1)();
        await done;
        expect(calls).toEqual(["clearatlas", "refresh 0-23"]);
    });

    test("a second schedule while one is pending does not queue another frame", async () => {
        const { scheduler, frames, releaseFonts } = rig();
        const first = scheduler.schedule();
        void scheduler.schedule();
        releaseFonts();
        await tick();
        expect(frames.size).toBe(1);
        frames.get(1)();
        await first;
    });

    test("disposed while waiting for fonts: no frame, no repaint", async () => {
        const { scheduler, calls, frames, releaseFonts } = rig();
        const done = scheduler.schedule();
        scheduler.dispose();
        releaseFonts();
        await done;
        expect(frames.size).toBe(0);
        expect(calls).toEqual([]);
    });

    test("disposed while a frame is pending: the frame is cancelled, schedule() settles and nothing is painted", async () => {
        const { scheduler, calls, frames, cancelled, releaseFonts } = rig();
        let settled = false;
        const done = scheduler.schedule().then(() => (settled = true));
        releaseFonts();
        await tick();
        expect(settled).toBe(false);
        scheduler.dispose();
        expect(cancelled).toEqual([1]);
        await done;
        expect(settled).toBe(true);
        expect(frames.size).toBe(0);
        expect(calls).toEqual([]);
    });

    test("schedule after dispose does nothing", async () => {
        const { scheduler, calls, frames } = rig();
        scheduler.dispose();
        await scheduler.schedule();
        expect(frames.size).toBe(0);
        expect(calls).toEqual([]);
    });

    test("a fonts promise that rejects still repaints", async () => {
        const { term, calls } = fakeTerm();
        const frames: (() => void)[] = [];
        const scheduler = new RedrawScheduler(
            term,
            Promise.reject(new Error("no fonts")),
            (cb) => {
                frames.push(cb);
                return frames.length;
            },
            () => {}
        );
        const done = scheduler.schedule();
        await tick();
        frames[0]();
        await done;
        expect(calls).toEqual(["clearatlas", "refresh 0-23"]);
    });
});
