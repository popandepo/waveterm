// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type RightClickAction = "menu" | "copy" | "paste" | "program";

// What a right click in a terminal does. With the setting off it opens the context menu, as upstream does. With it on it works like
// Windows Terminal: copy when something is selected, paste when nothing is. Two cases stay out of it:
//   - Shift or Ctrl held: the context menu opens, so the other items (open a link, magnify) stay reachable.
//   - a program that takes the mouse itself (vim, Claude Code): the click belongs to that program. Wave opens no menu and copies and
//     pastes nothing ("program"); the terminal's own mouse handling passes the click on. Shift + right click still gets the menu.
export function rightClickAction(
    rightClickPaste: boolean | null | undefined,
    hasSelection: boolean,
    menuModifier: boolean,
    programTakesMouse = false
): RightClickAction {
    if (!rightClickPaste || menuModifier) {
        return "menu";
    }
    if (programTakesMouse) {
        return "program";
    }
    return hasSelection ? "copy" : "paste";
}

export interface RightClickDeps {
    rightClickPaste: boolean | null | undefined;
    hasSelection: boolean;
    programTakesMouse: boolean;
    menuModifier: boolean;
    // make this terminal's block the focused one and give its input element the keyboard focus (it may take a moment to settle)
    focusThisTerminal: () => void;
    // whether this terminal's input element now really holds the keyboard focus
    isThisTerminalFocused: () => boolean;
    // the native paste: it pastes into whichever element has the keyboard focus
    nativePaste: () => void;
    // copies the selection; resolves true when the clipboard took it
    copySelection: () => Promise<boolean> | boolean;
    clearSelection: () => void;
    // waits for the given time (a stand-in in tests)
    wait?: (ms: number) => Promise<void>;
}

export const FocusPollMs = 20;
export const FocusTimeoutMs = 300;

// Waits until this terminal holds the keyboard focus, for at most FocusTimeoutMs. Resolves with whether it does.
export async function waitForTerminalFocus(
    isFocused: () => boolean,
    wait: (ms: number) => Promise<void> = (ms) => new Promise((r) => setTimeout(r, ms)),
    timeoutMs = FocusTimeoutMs,
    pollMs = FocusPollMs
): Promise<boolean> {
    for (let waited = 0; waited <= timeoutMs; waited += pollMs) {
        if (isFocused()) {
            return true;
        }
        await wait(pollMs);
    }
    return isFocused();
}

// Runs one right click. Returns true when it was handled (the context menu must not open), false when the context menu should open.
// Paste: this terminal takes the focus FIRST, and the native paste is sent only when it really holds the keyboard focus (it goes to the
// element that has the focus: a terminal that did not hold it would otherwise leave the text in the one that did); if the focus does
// not settle, nothing is pasted. Copy: the selection is cleared only after the clipboard took the text.
export function runRightClick(d: RightClickDeps): boolean {
    const action = rightClickAction(d.rightClickPaste, d.hasSelection, d.menuModifier, d.programTakesMouse);
    if (action === "menu") {
        return false;
    }
    if (action === "program") {
        return true;
    }
    if (action === "paste") {
        d.focusThisTerminal();
        void waitForTerminalFocus(d.isThisTerminalFocused, d.wait).then((focused) => {
            if (focused) {
                d.nativePaste();
            } else {
                console.log("right click paste: the terminal did not take the keyboard focus, nothing was pasted");
            }
        });
        return true;
    }
    void Promise.resolve(d.copySelection())
        .then((copied) => {
            if (copied) {
                d.clearSelection();
            }
        })
        .catch((e) => console.log("right click copy: the clipboard did not take the text, the selection stays", e));
    return true;
}
