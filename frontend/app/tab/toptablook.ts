// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { memberColorValue } from "./stacktablook";

export const ActiveFillPercent = 20;
export const InactiveLinePercent = 55;

export interface TopTabLook {
    // the tint behind the name of the ACTIVE tab; null for every other tab and for a tab without a colour
    fill: string | null;
    // the line under the tab: the full colour for the active tab, a dimmer one (55 percent) for the others; null for a tab without a colour
    line: string | null;
}

// How a tab of the top row shows its colour (tab:flagcolor), by the settled scheme: the active tab is filled with its colour and has its line under it;
// the other tabs have only a dimmer line. The top bar itself, the gaps between the tiles and the headers are not coloured by this. A colour that is not
// accepted (memberColorValue) draws nothing.
export function topTabLook(flagColor: unknown, active: boolean): TopTabLook {
    const color = memberColorValue(flagColor);
    if (color == null) {
        return { fill: null, line: null };
    }
    if (active) {
        return { fill: `color-mix(in srgb, ${color} ${ActiveFillPercent}%, transparent)`, line: color };
    }
    return { fill: null, line: `color-mix(in srgb, ${color} ${InactiveLinePercent}%, transparent)` };
}
