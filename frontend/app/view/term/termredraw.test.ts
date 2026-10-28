// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { redrawAfterSettle, redrawTerm, RedrawableTerm } from "./termredraw";

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

describe("redrawAfterSettle", () => {
    test("waits for fonts and the next frame before repainting", async () => {
        const { term, calls } = fakeTerm();
        let releaseFonts: () => void;
        const fontsReady = new Promise<void>((resolve) => (releaseFonts = resolve));
        const frames: (() => void)[] = [];
        const done = redrawAfterSettle(term, fontsReady, (cb) => frames.push(cb));
        expect(calls).toEqual([]);
        releaseFonts();
        await Promise.resolve();
        await Promise.resolve();
        expect(calls).toEqual([]);
        expect(frames.length).toBe(1);
        frames[0]();
        await done;
        expect(calls).toEqual(["clearatlas", "refresh 0-23"]);
    });

    test("a fonts promise that rejects still repaints", async () => {
        const { term, calls } = fakeTerm();
        await redrawAfterSettle(term, Promise.reject(new Error("no fonts")), (cb) => cb());
        expect(calls).toEqual(["clearatlas", "refresh 0-23"]);
    });

    test("a disposed terminal that throws is swallowed", async () => {
        const term: RedrawableTerm = {
            rows: 5,
            clearTextureAtlas: () => {
                throw new Error("disposed");
            },
            refresh: () => {},
        };
        await expect(redrawAfterSettle(term, Promise.resolve(), (cb) => cb())).resolves.toBeUndefined();
    });
});
