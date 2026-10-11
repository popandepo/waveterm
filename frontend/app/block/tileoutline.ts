// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { memberColorValue } from "@/app/tab/stacktablook";

export interface TileOutlineInput {
    focused: boolean;
    ephemeral: boolean;
    // the colour of the block itself (icon:color: the agent's colour)
    blockColor?: unknown;
    // the colour flag of the tab the block is in (tab:flagcolor)
    tabFlagColor?: unknown;
    // overrides written on the block (frame:activebordercolor, frame:bordercolor)
    frameActiveBorderColor?: string | null;
    frameBorderColor?: string | null;
    // the colours of the tab's background (bg:activebordercolor, bg:bordercolor)
    tabActiveBorderColor?: string | null;
    tabBorderColor?: string | null;
}

export const EphemeralOutline = "rgba(255, 255, 255, 0.7)";

// The border colour of a tile; undefined leaves the stylesheet's colour (focused: the accent of the window, which is the workspace colour; not focused: none).
//   focused:     the block's override, then the colour of the block (the agent), then the tab's flag colour, then the tab background's colour.
//   not focused: the block's override, then the tab's flag colour, then the tab background's colour; an ephemeral block with none is white.
// Only the outline is coloured: nothing here fills a header or a tile.
export function tileOutlineColor(i: TileOutlineInput): string | undefined {
    const block = memberColorValue(i.blockColor);
    const tab = memberColorValue(i.tabFlagColor);
    if (i.focused) {
        return i.frameActiveBorderColor || block || tab || i.tabActiveBorderColor || undefined;
    }
    const color = i.frameBorderColor || tab || i.tabBorderColor;
    if (color) {
        return color;
    }
    return i.ephemeral ? EphemeralOutline : undefined;
}
