// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { DefaultStartCols, DefaultStartRows, firstTermSize, MinTrustedCols, MinTrustedRows } from "./termsize";

describe("the first size of a terminal", () => {
    test("a size measured while the block had no real room is replaced by a usual one", () => {
        expect(firstTermSize({ cols: 2, rows: 1 })).toEqual({ cols: DefaultStartCols, rows: DefaultStartRows });
        expect(firstTermSize({ cols: 6, rows: 40 })).toEqual({ cols: DefaultStartCols, rows: DefaultStartRows });
        expect(firstTermSize({ cols: 120, rows: 2 })).toEqual({ cols: DefaultStartCols, rows: DefaultStartRows });
        expect(firstTermSize({ cols: MinTrustedCols - 1, rows: 30 })).toEqual({ cols: DefaultStartCols, rows: DefaultStartRows });
        expect(firstTermSize({ cols: 80, rows: MinTrustedRows - 1 })).toEqual({ cols: DefaultStartCols, rows: DefaultStartRows });
    });

    test("a size with room is kept as measured", () => {
        expect(firstTermSize({ cols: 80, rows: 24 })).toEqual({ cols: 80, rows: 24 });
        expect(firstTermSize({ cols: 123, rows: 31 })).toEqual({ cols: 123, rows: 31 });
        expect(firstTermSize({ cols: MinTrustedCols, rows: MinTrustedRows })).toEqual({ cols: MinTrustedCols, rows: MinTrustedRows });
    });

    test("the limits are what the live measurement showed: the fit floor is 2 columns, the bad blocks had 2 and 6", () => {
        expect(MinTrustedCols).toBeGreaterThan(6);
        expect(DefaultStartCols).toBeGreaterThanOrEqual(MinTrustedCols);
        expect(DefaultStartRows).toBeGreaterThanOrEqual(MinTrustedRows);
    });

    test("only the first resize is corrected, before the size is sent and before the controller is started", () => {
        const src = readFileSync(path.join(__dirname, "termwrap.ts"), "utf8");
        const body = src.slice(src.indexOf("    handleResize() {"), src.indexOf("    processAndCacheData()"));
        const guard = body.indexOf("if (!this.hasResized) {\n            const first = firstTermSize(");
        const send = body.indexOf("ControllerInputCommand");
        const resync = body.indexOf('this.resyncController("initial resize")');
        expect(guard).toBeGreaterThan(0);
        expect(guard).toBeLessThan(send);
        expect(send).toBeLessThan(resync);
        expect(src.match(/firstTermSize\(/g)?.length).toBe(1);
    });
});
