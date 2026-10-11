// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type RightClickAction = "menu" | "copy" | "paste";

// What a right click in a terminal does. With the setting off it opens the context menu, as upstream does. With it on it works like
// Windows Terminal: copy when something is selected, paste when nothing is. The menu stays in three cases: Shift or Ctrl held (so
// the other items stay reachable: open a link, magnify), and a program that takes the mouse itself (vim, Claude Code: the click belongs
// to that program, and the terminal's own mouse handling decides what to do with it).
export function rightClickAction(
    rightClickPaste: boolean | null | undefined,
    hasSelection: boolean,
    menuModifier: boolean,
    programTakesMouse = false
): RightClickAction {
    if (!rightClickPaste || menuModifier || programTakesMouse) {
        return "menu";
    }
    return hasSelection ? "copy" : "paste";
}

export interface RightClickDeps {
    rightClickPaste: boolean | null | undefined;
    hasSelection: boolean;
    programTakesMouse: boolean;
    menuModifier: boolean;
    // make this terminal's block the focused one and give its input element the keyboard focus
    focusThisTerminal: () => void;
    // the native paste: it pastes into whichever element has the keyboard focus
    nativePaste: () => void;
    copySelection: () => void;
    clearSelection: () => void;
}

// Runs one right click. Returns true when it was handled, false when the context menu should open. Paste: this terminal takes the
// focus FIRST and only then is the native paste sent, because that paste goes to the element that has the focus: a terminal
// that did not hold the focus would otherwise leave the text in the one that did.
export function runRightClick(d: RightClickDeps): boolean {
    const action = rightClickAction(d.rightClickPaste, d.hasSelection, d.menuModifier, d.programTakesMouse);
    if (action === "menu") {
        return false;
    }
    if (action === "paste") {
        d.focusThisTerminal();
        d.nativePaste();
        return true;
    }
    d.copySelection();
    d.clearSelection();
    return true;
}
