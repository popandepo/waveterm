// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { accentVars, applyAccent, isValidAccent, pickAccent } from "./accent";

const OriginalCss = globalThis.CSS;
const HexColorRegex = /^#([\da-f]{3}|[\da-f]{4}|[\da-f]{6}|[\da-f]{8})$/i;

describe("accent", () => {
    beforeAll(() => {
        globalThis.CSS = {
            supports: (_property: string, value: string) => HexColorRegex.test(value),
        } as typeof CSS;
    });

    afterAll(() => {
        globalThis.CSS = OriginalCss;
    });

    it("accepts only valid colours", () => {
        expect(isValidAccent("#ff0000")).toBe(true);
        expect(isValidAccent("not a colour")).toBe(false);
        expect(isValidAccent("")).toBe(false);
        expect(isValidAccent(null)).toBe(false);
        expect(isValidAccent("red; background: url(x)")).toBe(false);
    });

    it("prefers connection, then tab, then workspace accent, then workspace colour", () => {
        const all = { connAccent: "#ff0000", tabAccent: "#00ff00", wsAccent: "#0000ff", wsColor: "#111111" };
        expect(pickAccent(all)).toBe("#ff0000");
        expect(pickAccent({ ...all, connAccent: null })).toBe("#00ff00");
        expect(pickAccent({ ...all, connAccent: null, tabAccent: undefined })).toBe("#0000ff");
        expect(pickAccent({ wsColor: "#111111" })).toBe("#111111");
        expect(pickAccent({})).toBeNull();
    });

    it("skips an invalid colour and uses the next one", () => {
        expect(pickAccent({ connAccent: "bogus", tabAccent: "#00ff00" })).toBe("#00ff00");
    });

    it("derives the whole palette from one colour", () => {
        const vars = accentVars("#ff0000");
        expect(vars["--accent-color"]).toBe("#ff0000");
        expect(vars["--color-accent"]).toBe("#ff0000");
        expect(vars["--color-accent-400"]).toBe("#ff0000");
        expect(vars["--color-accent-100"]).toContain("#ff0000 25%");
        expect(Object.keys(vars).length).toBeGreaterThanOrEqual(14);
    });

    it("sets and removes the variables on the root element", () => {
        const set = new Map<string, string>();
        const root = {
            style: {
                setProperty: (k: string, v: string) => set.set(k, v),
                removeProperty: (k: string) => set.delete(k),
            },
        } as unknown as HTMLElement;
        const applied = applyAccent(root, "#ff0000", []);
        expect(set.get("--accent-color")).toBe("#ff0000");
        expect(applied.length).toBe(set.size);
        expect(applyAccent(root, null, applied)).toEqual([]);
        expect(set.size).toBe(0);
    });
});
