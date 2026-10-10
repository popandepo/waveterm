// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { closeTabPrompt } from "./closetabprompt";

export const CloseTabResultConfirmed = 1;
// Values of maxBlocks besides a count: no limit (force, scripts) and a close that was approved without a count (both reads failed and
// the dialog said so). The second still goes through the checked call, so the back end reads the tab itself and refuses if it cannot.
export const CloseTabNoLimit = -1;
export const CloseTabCountUnknown = -2;
const MaxRounds = 4;

// C is whatever the caller needs to act on the right window: it is handed back unchanged, so two windows closing tabs at the
// same time cannot get each other's dialog or close.
export interface CloseTabDeps<C> {
    getBlockCount: (tabId: string) => Promise<number | null>;
    // resolves with the index of the pressed button; 1 is "Close Tab", everything else keeps the tab
    showDialog: (message: string, ctx: C) => Promise<number>;
    // maxBlocks is the number of blocks that was approved (CloseTabNoLimit, or CloseTabCountUnknown when approved without a count). The close itself must refuse a tab that holds more by
    // then, and resolves with whether the tab was really closed.
    closeTab: (tabId: string, ctx: C, maxBlocks: number) => Promise<boolean>;
}

// One handler for every UI route that closes a tab (tab bar, vertical tab bar, key). The dialog is asynchronous so the
// main process and the other windows keep running. Only the explicit "Close Tab" answer closes the tab; Escape, a dismissal
// and any other result keep it. A request for a tab that is already being handled is ignored: the guard is set before the
// first await. The count is read again before the close: a tab that gained blocks after it was approved (or after its count
// was read as empty) asks again, and a tab that keeps changing is not closed.
export function makeCloseTabHandler<C>(deps: CloseTabDeps<C>) {
    const handling = new Set<string>();
    return async function handleCloseTab(tabId: string, confirmSetting: boolean, ctx: C): Promise<boolean> {
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
                return await deps.closeTab(tabId, ctx, approved == null ? CloseTabCountUnknown : approved);
            }
            return false;
        } finally {
            handling.delete(tabId);
        }
    };
}

// A window that is destroyed, or a loop that ends early, must not leave a caller waiting for a close that will never run:
// every close that is still queued is told "not closed", and the queue is emptied.
export function settlePendingCloses(queue: { op: string; result?: (closed: boolean) => void }[]) {
    const pending = queue.splice(0, queue.length);
    for (const entry of pending) {
        if (entry.op === "closetab") {
            entry.result?.(false);
        }
    }
}
