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

export interface MutableGuest {
    isAudioMuted(): boolean;
    setAudioMuted(muted: boolean): void;
}

// Brings the guest page in line with (owner choice, hidden). It reads the guest at the moment it runs, so it can be called again
// whenever a guest page becomes ready (first load, reload, navigation, a replaced guest) and for any change of the two inputs;
// a guest that is already right is not touched. Returns whether it changed anything.
export function applyGuestMute(
    guest: MutableGuest | null | undefined,
    mutedByOwner: boolean,
    hiddenInStack: boolean
): boolean {
    if (guest == null) {
        return false;
    }
    const change = audioMuteChange(guest.isAudioMuted(), mutedByOwner, hiddenInStack);
    if (change == null) {
        return false;
    }
    guest.setAudioMuted(change);
    return true;
}
