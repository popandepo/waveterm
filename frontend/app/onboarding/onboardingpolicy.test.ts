// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { onboardingToOpen } from "./onboardingpolicy";

const read = (...p: string[]) => readFileSync(path.join(__dirname, "..", ...p), "utf8");

describe("no first-run screen opens by itself", () => {
    test("a fresh profile (no terms agreed) opens no welcome screen, on the first tab of a start or not", () => {
        for (const primary of [true, false]) {
            assert.equal(onboardingToOpen(0, undefined, "v0.14.0", primary), "none");
            assert.equal(onboardingToOpen(undefined, undefined, "v0.14.0", primary), "none");
        }
    });

    test("a version change opens no upgrade screen, from any older version, on the first tab of a start", () => {
        for (const last of ["v0.0.0", "v0.13.0", "v0.13.9", "v0.14.0-beta", undefined, "not-a-version", ""]) {
            assert.equal(onboardingToOpen(1700000000000, last, "v0.14.0", true), "none", String(last));
        }
    });

    test("the same version opens nothing either", () => {
        assert.equal(onboardingToOpen(1700000000000, "v0.14.0", "v0.14.0", true), "none");
    });
});

describe("the wiring that no test can render", () => {
    const renderer = read("modals", "modalsrenderer.tsx");
    test("the modals renderer opens the screens only through the decision", () => {
        assert.include(renderer, "onboardingToOpen(");
        assert.equal(renderer.split("setNewInstallOnboardingOpen(true)").length - 1, 1);
        assert.equal(renderer.split("setUpgradeOnboardingOpen(true)").length - 1, 1);
        assert.match(renderer, /if \(choice === "new"\) \{\s*setNewInstallOnboardingOpen\(true\);/);
        assert.match(renderer, /if \(choice === "upgrade"\) \{\s*setUpgradeOnboardingOpen\(true\);/);
        assert.notInclude(renderer, "if (!clientData.tosagreed)");
        assert.notInclude(renderer, "semver");
    });

    test("the decision has no path that returns a screen", () => {
        const policy = read("onboarding", "onboardingpolicy.ts");
        const body = policy.slice(policy.indexOf("export function onboardingToOpen"));
        assert.notInclude(body, '"new"');
        assert.notInclude(body, '"upgrade"');
        assert.include(body, 'return "none";');
    });
});
