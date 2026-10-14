// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, test } from "vitest";

// Fork rule: nothing in the app looks for an update at the vendor on its own or on a click ("Check for Updates" would ask dl.waveterm.dev even with
// automatic updates off, and would offer upstream's build over the fork).
test("the app menu has no Check for Updates item and does not call the updater", () => {
    const text = readFileSync(path.join(__dirname, "emain-menu.ts"), "utf8");
    assert.notInclude(text, "Check for Updates");
    assert.notMatch(text, /checkForUpdates\s*\(/);
    assert.notMatch(text, /from "\.\/updater"/);
});

test("the only caller of an update check with userInput true is gone", () => {
    for (const name of ["emain.ts", "emain-window.ts", "emain-wsh.ts", "emain-wavesrv.ts", "emain-menu.ts"]) {
        const text = readFileSync(path.join(__dirname, name), "utf8");
        assert.notMatch(text, /checkForUpdates\(\s*true\s*\)/, name);
    }
});
