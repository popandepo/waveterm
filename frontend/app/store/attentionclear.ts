// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { AttentionLevelKey, AttentionReasonKey, attentionRank } from "./attention";

// When a mark that asks for the owner goes away by itself. The two orange levels (need, urgent) clear when the owner really has the tile: the window has
// focus and the tile is the focused one, and stays so for a moment (half a second after the focus moved there, 3 seconds when it was already there,
// the same rule as for badges). Info is not cleared by focus (it stays until the session clears it), and nothing is cleared for a tile that is merely
// shown in a tab nobody has focused. A session can always clear its own mark with "wsh attention clear".

export const AttentionClearAfterSwitchMs = 500;
export const AttentionClearAfterStayMs = 3000;

export const ClearAttentionMeta: { [key: string]: null } = { [AttentionLevelKey]: null, [AttentionReasonKey]: null };

export function clearsOnFocus(level: unknown): boolean {
    return attentionRank(level) >= attentionRank("need");
}

// How long to wait before clearing the mark of the focused block, or null when it must not be cleared now.
export function attentionClearDelayMs(level: unknown, documentHasFocus: boolean, focusSwitched: boolean): number | null {
    if (!documentHasFocus || !clearsOnFocus(level)) {
        return null;
    }
    return focusSwitched ? AttentionClearAfterSwitchMs : AttentionClearAfterStayMs;
}
