// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export const StackNextKey = "Ctrl:Shift:PageDown";
export const StackPrevKey = "Ctrl:Shift:PageUp";

interface StackKeyModel {
    // true when the focused tile is a stack with at least two tabs and the next or previous one was shown
    focusAdjacentStackMember(offset: number): boolean;
}

// Not taken (false) when there is nothing to switch, so that the key goes on to whatever else wants it.
export function registerStackKeys(
    keyMap: Map<string, (event: WaveKeyboardEvent) => boolean>,
    getModel: () => StackKeyModel
) {
    keyMap.set(StackNextKey, () => getModel().focusAdjacentStackMember(1));
    keyMap.set(StackPrevKey, () => getModel().focusAdjacentStackMember(-1));
}
