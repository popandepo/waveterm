// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { closeTabPrompt } from "./closetabprompt";

export const CloseTabResultConfirmed = 1;
const MaxRounds = 4;

// C is whatever the caller needs to act on the right window: it is handed back unchanged, so two windows closing tabs at the
// same time cannot get each other's dialog or close.
export interface CloseTabDeps<C> {
    getBlockCount: (tabId: string) => Promise<number | null>;
    // resolves with the index of the pressed button; 1 is "Close Tab", everything else keeps the tab
    showDialog: (message: string, ctx: C) => Promise<number>;
    closeTab: (tabId: string, ctx: C) => Promise<void>;
}

// One handler for every UI route that closes a tab (tab bar, vertical tab bar, key). The dialog is asynchronous so the
// main process and the other windows keep running. Only the explicit "Close Tab" answer closes the tab; Escape, a dismissal
// and any other result keep it. A request for a tab that is already being handled is ignored: the guard is set before the
// first await. The count is read again before the close: a tab that gained blocks after it was approved (or after its count
// was read as empty) asks again, and a tab that keeps changing is not closed.
export function makeCloseTabHandler<C>(deps: CloseTabDeps<C>) {
    const handling = new Set<string>();
    return async function handleCloseTab(tabId: string, confirmSetting: boolean, ctx?: C): Promise<boolean> {
        if (handling.has(tabId)) {
            return false;
        }
        handling.add(tabId);
        try {
            let approved: number | null | undefined = undefined;
            for (let round = 0; round < MaxRounds; round++) {
                let count: number | null = null;
                try {
                    count = await deps.getBlockCount(tabId);
                } catch (e) {
                    console.log("close-tab: could not read the tab, asking anyway", e);
                }
                const covered =
                    approved !== undefined &&
                    (count == null ? approved == null : approved != null && count <= approved);
                if (!covered) {
                    const prompt = closeTabPrompt(confirmSetting, count);
                    if (prompt.ask) {
                        const answer = await deps.showDialog(prompt.message, ctx);
                        if (answer !== CloseTabResultConfirmed) {
                            return false;
                        }
                    }
                    approved = count;
                    continue;
                }
                await deps.closeTab(tabId, ctx);
                return true;
            }
            return false;
        } finally {
            handling.delete(tabId);
        }
    };
}
