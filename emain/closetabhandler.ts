// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { closeTabPrompt } from "./closetabprompt";

export const CloseTabResultConfirmed = 1;

// C is whatever the caller needs to act on the right window: it is handed back unchanged, so two windows closing tabs at the
// same time cannot get each other's dialog or close.
export interface CloseTabDeps<C> {
    getBlockCount: (tabId: string) => Promise<number | null>;
    // resolves with the index of the pressed button; 1 is "Close Tab", everything else keeps the tab
    showDialog: (message: string, ctx: C) => Promise<number>;
    closeTab: (tabId: string, ctx: C) => Promise<void>;
}

// One handler for every UI route that closes a tab (tab bar, vertical tab bar, key). The dialog is asynchronous so the
// main process and the other windows keep running while it is open. Only the explicit "Close Tab" answer closes the tab;
// Escape, a dismissal and any other result keep it. A second request for a tab whose dialog is open is ignored.
export function makeCloseTabHandler<C>(deps: CloseTabDeps<C>) {
    const asking = new Set<string>();
    return async function handleCloseTab(tabId: string, confirmSetting: boolean, ctx?: C): Promise<boolean> {
        if (asking.has(tabId)) {
            return false;
        }
        let blockCount: number | null = null;
        try {
            blockCount = await deps.getBlockCount(tabId);
        } catch (e) {
            console.log("close-tab: could not read the tab, asking anyway", e);
        }
        const prompt = closeTabPrompt(confirmSetting, blockCount);
        if (prompt.ask) {
            asking.add(tabId);
            try {
                const answer = await deps.showDialog(prompt.message, ctx);
                if (answer !== CloseTabResultConfirmed) {
                    return false;
                }
            } finally {
                asking.delete(tabId);
            }
        }
        await deps.closeTab(tabId, ctx);
        return true;
    };
}
