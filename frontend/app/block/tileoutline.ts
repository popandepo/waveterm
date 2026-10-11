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
    frameActiveBorderColor?: unknown;
    frameBorderColor?: unknown;
    // the colours of the tab's background (bg:activebordercolor, bg:bordercolor)
    tabActiveBorderColor?: unknown;
    tabBorderColor?: unknown;
}

export const EphemeralOutline = "rgba(255, 255, 255, 0.7)";

// The border colour of a tile; undefined leaves the stylesheet's colour (focused: the accent of the window, which is the workspace colour;
// not focused: none). Every candidate, the explicit overrides and the tab background's colours included, goes through memberColorValue: they
// are set by command by any session, and a value that is not accepted counts as not set.
//   focused:     the block's override, then the colour of the block (the agent), then the tab's colour (its flag colour, then its background's
//                border colour), then the stylesheet (the workspace colour).
//   not focused: the block's override, then the tab's colour (its flag colour, then its background's border colour); an ephemeral tile
//                with none is white.
// Only the outline is coloured: nothing here fills a header or a tile.
export function tileOutlineColor(i: TileOutlineInput): string | undefined {
    const frameActive = memberColorValue(i.frameActiveBorderColor);
    const frame = memberColorValue(i.frameBorderColor);
    const block = memberColorValue(i.blockColor);
    const tabFlag = memberColorValue(i.tabFlagColor);
    const tabActive = memberColorValue(i.tabActiveBorderColor);
    const tabBorder = memberColorValue(i.tabBorderColor);
    if (i.focused) {
        return frameActive ?? block ?? tabFlag ?? tabActive ?? undefined;
    }
    const color = frame ?? tabFlag ?? tabBorder;
    if (color != null) {
        return color;
    }
    return i.ephemeral ? EphemeralOutline : undefined;
}
