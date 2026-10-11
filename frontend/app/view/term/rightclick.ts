// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type RightClickAction = "menu" | "copy" | "paste";

// What a right click in a terminal does. With the setting off it opens the context menu, as upstream does. With it on it works like
// Windows Terminal: copy when something is selected, paste when nothing is. Shift or Ctrl held still opens the menu, so the menu stays
// reachable (open a link, magnify, and the other items).
export function rightClickAction(
    rightClickPaste: boolean | null | undefined,
    hasSelection: boolean,
    menuModifier: boolean
): RightClickAction {
    if (!rightClickPaste || menuModifier) {
        return "menu";
    }
    return hasSelection ? "copy" : "paste";
}
