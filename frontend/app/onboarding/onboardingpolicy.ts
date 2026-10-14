// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type OnboardingChoice = "none" | "new" | "upgrade";

// Which first-run screen opens by itself. In this fork none does, on a fresh profile or after a version change: no screen is needed to reach a working
// window, and the screens that carry the "usage data" tick box (the welcome screen and the upgrade screens) never appear on their own. The arguments are
// kept so that the call site reads like upstream's two conditions (no terms agreed yet; last version older than the current one on the first tab of a start).
export function onboardingToOpen(
    tosAgreed: number | undefined,
    lastVersion: string | undefined,
    currentVersion: string,
    primaryTabStartup: boolean
): OnboardingChoice {
    return "none";
}
