// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";

const frontendDir = path.join(__dirname, "..", "..");
const read = (...p: string[]) => readFileSync(path.join(__dirname, ...p), "utf8");

function sourceFiles(dir: string, out: string[] = []): string[] {
    for (const name of readdirSync(dir)) {
        const full = path.join(dir, name);
        if (statSync(full).isDirectory()) {
            if (name !== "node_modules") {
                sourceFiles(full, out);
            }
        } else if (/\.(ts|tsx)$/.test(name) && !/\.test\.(ts|tsx)$/.test(name)) {
            out.push(full);
        }
    }
    return out;
}

describe("nothing in the window can switch usage telemetry on from the AI panel", () => {
    test("no source file calls the enable-telemetry command (only the generated client defines it)", () => {
        const callers = sourceFiles(frontendDir)
            .filter((f) => readFileSync(f, "utf8").includes("WaveAIEnableTelemetryCommand"))
            .map((f) => path.basename(f));
        assert.deepEqual(callers, ["wshclientapi.ts"]);
    });

    test("the AI panel's message has no button and no command", () => {
        const msg = read("telemetryrequired.tsx");
        assert.notInclude(msg, "<button");
        assert.notInclude(msg, "onClick");
        assert.notInclude(msg, "RpcApi");
        assert.include(msg, "not available in this fork");
    });

    test("the mode menu says plainly that the cloud mode needs telemetry and offers no way to switch it on", () => {
        const menu = read("aimode.tsx");
        const section = menu.slice(menu.indexOf("section.noTelemetry"), menu.indexOf("section.configs.map"));
        assert.notInclude(section, "<button");
        assert.notInclude(section, "onClick");
        assert.include(section, "keeps off");
    });
});
