// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// A web tile that is hidden behind another tab of its stack stays mounted (so the page and its history survive), and a mounted
// page keeps playing sound. Hidden means muted, shown means the owner's own mute choice again. Media is muted, not paused:
// pausing would need script injected into the page, cannot be undone for streams that have no pause, and changes what the page
// believes happened.
export function effectiveAudioMuted(mutedByOwner: boolean, hiddenInStack: boolean): boolean {
    return mutedByOwner || hiddenInStack;
}

// What to do to the guest page for one state of (owner choice, hidden): nothing when it already is as it should be.
export function audioMuteChange(
    currentlyMuted: boolean,
    mutedByOwner: boolean,
    hiddenInStack: boolean
): boolean | undefined {
    const wanted = effectiveAudioMuted(mutedByOwner, hiddenInStack);
    return wanted === currentlyMuted ? undefined : wanted;
}
