// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Taken from the run-the-code reviewer's tests of PR 56 (what holds, wider than the first scan): the policy for every pair, the two 'open' atoms start
// closed, any store or setter, pushModal with a variable, and the release notes page that writes nothing on close.

import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { createStore } from "jotai";
import { assert, describe, test } from "vitest";
import { CurrentOnboardingVersion } from "./onboarding-common";
import { onboardingToOpen } from "./onboardingpolicy";

const frontend = path.join(__dirname, "..", "..");
const emain = path.join(frontend, "..", "emain");

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

describe("the policy for every pair", () => {
    test("no combination of terms time, last version, current version and first-tab flag opens a screen", () => {
        const patch = readFileSync(path.join(__dirname, "onboarding-upgrade-patch.tsx"), "utf8");
        const versions = [...patch.matchAll(/version: "(v[\d.]+)"/g)].map((m) => m[1]);
        assert.isAbove(versions.length, 5, "the release list was read");
        const tos: any[] = [0, undefined, null, NaN, -1, 1, 1700000000000, "1700000000000", true];
        const lasts: any[] = [undefined, null, "", "v0.0.0", "0.0.0", "v99.0.0", "garbage", CurrentOnboardingVersion, ...versions];
        const currents: any[] = [CurrentOnboardingVersion, "v0.0.1", "v99.0.0", "", undefined, ...versions];
        let n = 0;
        for (const t of tos) {
            for (const l of lasts) {
                for (const c of currents) {
                    for (const p of [true, false, undefined as any]) {
                        assert.equal(onboardingToOpen(t, l, c, p), "none", JSON.stringify([t, l, c, p]));
                        n++;
                    }
                }
            }
        }
        assert.isAbove(n, 3000);
    });
});

describe("the renderer's decision", () => {
    test("the two 'open' atoms start closed", async () => {
        const { modalsModel } = await import("@/app/store/modalmodel");
        const store = createStore();
        assert.isFalse(store.get(modalsModel.newInstallOnboardingOpen));
        assert.isFalse(store.get(modalsModel.upgradeOnboardingOpen));
        assert.deepEqual(store.get(modalsModel.modalsAtom), []);
    });

    test("the renderer has exactly one effect that can open a screen, and it only acts on the policy's answer", () => {
        const r = readFileSync(path.join(frontend, "app", "modals", "modalsrenderer.tsx"), "utf8");
        const effect = r.slice(r.indexOf("const choice = onboardingToOpen("), r.indexOf("}, [clientData]);"));
        assert.match(effect, /onboardingToOpen\(\s*clientData\.tosagreed,\s*clientData\.meta\?\.\["onboarding:lastversion"\],\s*CurrentOnboardingVersion,\s*globalPrimaryTabStartup\s*\)/);
        assert.equal(r.split("OnboardingOpen(true)").length - 1, 2);
        assert.equal(effect.split("OnboardingOpen(true)").length - 1, 2, "both opens are inside the policy effect");
        assert.equal(r.split("useEffect(").length - 1, 2);
    });
});

describe("every way to open a screen, wider than the PR's own scan", () => {
    const files = [...sourceFiles(frontend), ...sourceFiles(emain)].filter((f) => !f.includes(path.sep + "preview" + path.sep));
    const text = (f: string) => readFileSync(f, "utf8");
    const rel = (f: string) => path.relative(path.join(frontend, ".."), f).split(path.sep).join("/");

    test("nothing sets either 'open' atom to true through any store or setter, outside the renderer", () => {
        const hits = files
            .filter((f) => /(newInstallOnboardingOpen|upgradeOnboardingOpen)\s*,\s*true|set(NewInstall|Upgrade)OnboardingOpen\(\s*(true|!)/.test(text(f)))
            .map(rel);
        assert.deepEqual(hits, ["frontend/app/modals/modalsrenderer.tsx"]);
    });

    test("every pushModal call names its modal with a literal, and only the widget menu names an onboarding one", () => {
        const nonLiteral: string[] = [];
        const onboarding: string[] = [];
        for (const f of files) {
            for (const m of text(f).matchAll(/pushModal\(\s*([^,)]+)/g)) {
                const arg = m[1].trim();
                if (!/^"[A-Za-z]+"$/.test(arg)) {
                    nonLiteral.push(rel(f) + ": " + arg);
                } else if (/Onboarding/.test(arg)) {
                    onboarding.push(rel(f) + ": " + arg);
                }
            }
        }
        // the definition itself: pushModal(displayName: string, props?: any)
        assert.deepEqual(nonLiteral, []);
        assert.deepEqual(onboarding, ['frontend/app/workspace/widgets.tsx: "UpgradeOnboardingPatch"']);
    });

    test("the welcome and upgrade components are rendered only by the renderer, the registry and each other", () => {
        const users = files
            .filter((f) => /<(NewInstallOnboardingModal|UpgradeOnboardingModal|UpgradeOnboardingMinor|UpgradeOnboardingPatch)[\s/>]/.test(text(f)))
            .map(rel)
            .sort();
        assert.deepEqual(users, ["frontend/app/modals/modalsrenderer.tsx", "frontend/app/onboarding/onboarding-upgrade.tsx"]);
    });

    test("the electron side knows nothing of onboarding, terms or a custom URL scheme", () => {
        const em = sourceFiles(emain);
        for (const f of em) {
            const t = text(f);
            assert.notMatch(t, /[Oo]nboarding|tosagreed|AgreeTos|setAsDefaultProtocolClient|open-url|registerSchemesAsPrivileged/, rel(f));
        }
    });

    test("callers that can write telemetry:enabled from the window: no new one beyond the known ones", () => {
        const callers = files.filter((f) => /TelemetryUpdate\(|"telemetry:enabled"\s*:|WaveAIEnableTelemetryCommand\(TabRpcClient/.test(text(f))).map(rel).sort();
        // PR 58 removes the AI panel's two callers: the list may shrink, it may not grow
        const allowed = [
            "frontend/app/aipanel/aimode.tsx",
            "frontend/app/aipanel/telemetryrequired.tsx",
            "frontend/app/onboarding/onboarding.tsx",
            "frontend/app/store/services.ts",
        ];
        for (const c of callers) {
            assert.include(allowed, c, "a new caller that can write telemetry:enabled");
        }
    });

    test("the release notes page: closing it writes nothing (the docs say so)", () => {
        const patch = text(path.join(__dirname, "onboarding-upgrade-patch.tsx"));
        const handleClose = patch.slice(patch.indexOf("const handleClose = () => {"), patch.indexOf("if (alreadyStarred)"));
        assert.match(handleClose, /if \(isReleaseNotes\) \{\s*doClose\(\);\s*return;\s*\}\s*const clientId/);
    });
});
