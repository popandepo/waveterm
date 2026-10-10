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
