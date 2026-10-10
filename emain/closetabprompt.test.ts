// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { closeTabPrompt } from "./closetabprompt";

describe("asking before a tab is closed", () => {
    test("a tab that holds blocks always asks, even when the setting is off", () => {
        const p = closeTabPrompt(false, 3);
        expect(p.ask).toBe(true);
        expect(p.message).toContain("3 blocks");
        expect(p.message.toLowerCase()).toContain("stops");
    });

    test("one block is singular", () => {
        expect(closeTabPrompt(false, 1).message).toContain("1 block");
        expect(closeTabPrompt(false, 1).message).not.toContain("1 blocks");
    });

    test("an empty tab closes without asking unless the setting asks for it", () => {
        expect(closeTabPrompt(false, 0).ask).toBe(false);
        const p = closeTabPrompt(true, 0);
        expect(p.ask).toBe(true);
        expect(p.message).toBe("Are you sure you want to close this tab?");
    });

    test("an unknown block count is treated as holding blocks, never as empty", () => {
        expect(closeTabPrompt(false, null).ask).toBe(true);
        expect(closeTabPrompt(false, undefined).ask).toBe(true);
        expect(closeTabPrompt(false, null).message).toContain("blocks");
    });
});
