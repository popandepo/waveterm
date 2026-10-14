// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readdirSync, readFileSync, statSync } from "node:fs";
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

function sourceFiles(dir: string, out: string[] = []): string[] {
    for (const name of readdirSync(dir)) {
        const full = path.join(dir, name);
        if (statSync(full).isDirectory()) {
            if (name !== "node_modules" && name !== "preview") {
                sourceFiles(full, out);
            }
        } else if (/\.(ts|tsx)$/.test(name) && !/\.test\.(ts|tsx)$/.test(name)) {
            out.push(full);
        }
    }
    return out;
}

// Every way into a first-run or upgrade screen that the code has, found by reading it (11-Oct-2026): the modals renderer (above), the "Release Notes" entry of
// the widget menu, the modal registry (which only makes the names known). No menu item, shortcut, command, deep link or AI panel path opens the welcome
// screen or the upgrade screens; these tests fail if one is added.
describe("every entry point into a first-run or upgrade screen", () => {
    const frontend = path.join(__dirname, "..", "..");
    const emain = path.join(frontend, "..", "emain");
    const files = [...sourceFiles(frontend), ...sourceFiles(emain)];
    const onlyOnboardingFolder = (f: string) => f.includes(path.join("frontend", "app", "onboarding") + path.sep);

    test("no code outside the onboarding screens opens them but the policy-guarded renderer and the Release Notes menu entry", () => {
        const openers = files
            .filter((f) => !onlyOnboardingFolder(f))
            .filter((f) => /pushModal\(\s*"(NewInstallOnboardingModal|UpgradeOnboardingModal|UpgradeOnboardingPatch)"|set(NewInstall|Upgrade)OnboardingOpen\(true\)|globalStore\.set\(modalsModel\.(newInstallOnboardingOpen|upgradeOnboardingOpen), true\)/.test(readFileSync(f, "utf8")))
            .map((f) => path.basename(f))
            .sort();
        assert.deepEqual(openers, ["modalsrenderer.tsx", "widgets.tsx"]);
    });

    test("the widget menu opens only the release notes page, on a click, and never the welcome or the other upgrade screens", () => {
        const widgets = readFileSync(path.join(frontend, "app", "workspace", "widgets.tsx"), "utf8");
        assert.equal(widgets.split("pushModal(").filter((x) => /^"(NewInstall|UpgradeOnboardingModal)/.test(x)).length, 0);
        assert.match(widgets, /label: "Release Notes",\s*onClick: \(\) => \{\s*modalsModel\.pushModal\("UpgradeOnboardingPatch", \{ isReleaseNotes: true \}\);/);
    });

    test("the release notes page has no tick box, no terms and no telemetry call", () => {
        const patch = readFileSync(path.join(__dirname, "onboarding-upgrade-patch.tsx"), "utf8");
        for (const word of ["telemetry", "TelemetryUpdate", "AgreeTos", "checkbox", "tosagreed"]) {
            assert.notInclude(patch.toLowerCase(), word.toLowerCase(), word);
        }
    });

    test("nothing but the welcome screen itself asks the server to agree to the terms or to change telemetry", () => {
        const callers = files
            .filter((f) => /ClientService\.(AgreeTos|TelemetryUpdate)\(/.test(readFileSync(f, "utf8")))
            .map((f) => path.basename(f))
            .sort();
        assert.deepEqual(callers, ["onboarding.tsx"]);
    });
});
