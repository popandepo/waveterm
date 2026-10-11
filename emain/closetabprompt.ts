// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export interface CloseTabPrompt {
    ask: boolean;
    message: string;
}

// Closing a tab deletes it and stops every program in it, and there is no reopen. In this fork a tab that holds
// blocks therefore always asks, whatever tab:confirmclose says; an empty tab asks only when the setting does.
// When the count could not be read the tab is treated as full: asking too often is better than losing sessions.
export function closeTabPrompt(confirmSetting: boolean, blockCount: number | null | undefined): CloseTabPrompt {
    if (blockCount === 0) {
        return { ask: confirmSetting, message: "Are you sure you want to close this tab?" };
    }
    const what = blockCount == null ? "blocks" : blockCount === 1 ? "1 block" : `${blockCount} blocks`;
    return {
        ask: true,
        message: `This tab holds ${what}. Closing it stops the programs running in them, and it cannot be undone.`,
    };
}

// The command route (`wsh tab close`, a direct RPC call) never opens a dialog: scripts and agents would wait for it forever.
// It refuses a tab that holds blocks unless force is given, and the check happens here, in the main process, right before
// the close, so a direct RPC call is covered too.
export function wshCloseRefusal(blockCount: number | null | undefined, force: boolean): string | null {
    if (force || blockCount === 0) {
        return null;
    }
    const what =
        blockCount == null
            ? "blocks (the count could not be read)"
            : blockCount === 1
              ? "1 block"
              : `${blockCount} blocks`;
    return `the tab holds ${what}; closing it stops the programs in them and cannot be undone: use force to close it anyway`;
}

// The buttons of the box that asks before one block is closed, in the order they are shown. Only "Close Block" (index 1) closes: Cancel,
// Escape, a dismissed box and any other result keep the block.
export const CloseBlockButtons = ["Cancel", "Close Block"];
export function closeBlockAnswer(response: number): boolean {
    return response === CloseBlockButtons.indexOf("Close Block");
}
