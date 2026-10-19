// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import {
    DefaultStartCols,
    DefaultStartRows,
    isUsableTermSize,
    MinTrustedCols,
    MinTrustedRows,
    TermDims,
    TermSizer,
} from "./termsize";

// A stand-in terminal: the fit addon is replaced by whatever size the test says the container measures at that moment.
function rig(start: TermDims = { cols: 80, rows: 24 }) {
    const term = { ...start };
    let measured: TermDims = { ...start };
    const sent: TermDims[] = [];
    const resyncedAt: TermDims[] = [];
    const sizer = new TermSizer({
        get: () => ({ ...term }),
        fit: () => {
            term.cols = measured.cols;
            term.rows = measured.rows;
        },
        set: (d) => {
            term.cols = d.cols;
            term.rows = d.rows;
        },
        sendSize: (d) => sent.push({ ...d }),
        resync: () => resyncedAt.push({ ...term }),
    });
    return {
        term,
        sent,
        resyncedAt,
        sizer,
        measure(cols: number, rows: number) {
            measured = { cols, rows };
            sizer.handle();
        },
    };
}

const usual: TermDims = { cols: DefaultStartCols, rows: DefaultStartRows };

describe("the size a block's program is started with", () => {
    test("a first measurement of a few columns starts the program at the usual size, not at the few", () => {
        const r = rig();
        r.measure(2, 1);
        expect(r.term).toEqual(usual);
        expect(r.sent).toEqual([usual]);
        expect(r.resyncedAt).toEqual([usual]);
    });

    test("a second and a third tiny measurement while the tab is still hidden do not undo it", () => {
        const r = rig();
        r.measure(2, 1);
        r.measure(6, 3);
        r.measure(2, 1);
        expect(r.term).toEqual(usual);
        expect(r.sent).toEqual([usual]);
        expect(r.resyncedAt.length).toBe(1);
    });

    test("when the tab is shown and the block has room, the real size replaces the usual one and is sent", () => {
        const r = rig();
        r.measure(2, 1);
        r.measure(6, 3);
        r.measure(123, 31);
        expect(r.term).toEqual({ cols: 123, rows: 31 });
        expect(r.sent).toEqual([usual, { cols: 123, rows: 31 }]);
        expect(r.resyncedAt.length).toBe(1);
    });

    test("once a usable size was seen, a narrow tile is believed (the user made it narrow)", () => {
        const r = rig();
        r.measure(2, 1);
        r.measure(123, 31);
        r.measure(10, 5);
        expect(r.term).toEqual({ cols: 10, rows: 5 });
        expect(r.sent[r.sent.length - 1]).toEqual({ cols: 10, rows: 5 });
    });

    test("a first measurement with room is kept as measured and starts the program once", () => {
        const r = rig();
        r.measure(100, 40);
        expect(r.term).toEqual({ cols: 100, rows: 40 });
        expect(r.sent).toEqual([{ cols: 100, rows: 40 }]);
        expect(r.resyncedAt).toEqual([{ cols: 100, rows: 40 }]);
        r.measure(90, 40);
        expect(r.resyncedAt.length).toBe(1);
    });

    test("a measurement equal to the size the terminal already has sends nothing", () => {
        const r = rig({ cols: 80, rows: 24 });
        r.measure(80, 24);
        expect(r.sent).toEqual([]);
        expect(r.resyncedAt).toEqual([{ cols: 80, rows: 24 }]);
    });

    test("the limits sit above what was measured live (2 and 6 columns) and below a usual terminal", () => {
        expect(isUsableTermSize({ cols: 6, rows: 40 })).toBe(false);
        expect(isUsableTermSize({ cols: MinTrustedCols - 1, rows: 30 })).toBe(false);
        expect(isUsableTermSize({ cols: 80, rows: MinTrustedRows - 1 })).toBe(false);
        expect(isUsableTermSize({ cols: MinTrustedCols, rows: MinTrustedRows })).toBe(true);
        expect(isUsableTermSize({ cols: 80, rows: 24 })).toBe(true);
        expect(isUsableTermSize(usual)).toBe(true);
    });
});
