// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { CloseBlockAfterConfirm } from "./stackclose";

export interface EphemeralReplaceDeps {
    closeAfterConfirm: CloseBlockAfterConfirm | undefined;
    closePrevious: () => Promise<void> | void;
    showNew: () => void;
    // the new block is not shown and its block is deleted (the old one stays); resolves with whether it was deleted
    discardNew: () => Promise<boolean> | boolean;
}

// Deletes a block that is not going to be shown. The block exists in the back end already, so a failure is retried once and then logged
// loudly: it must not be left behind silently. Resolves with whether the block was deleted.
export async function discardEphemeralBlock(
    deleteBlock: ((data: { blockId: string }) => Promise<void>) | undefined,
    blockId: string
): Promise<boolean> {
    if (deleteBlock == null) {
        console.error("ephemeral block: no way to delete a block is set up, the block stays in the back end", blockId);
        return false;
    }
    for (let attempt = 1; attempt <= 2; attempt++) {
        try {
            await deleteBlock({ blockId });
            return true;
        } catch (e) {
            console.error("ephemeral block: could not delete the discarded block, attempt " + attempt, blockId, e);
        }
    }
    return false;
}

// The ids of blocks that were to be discarded and could not be deleted. They are tried again later (and the orphan clean-up of the tab
// that runs when a layout loads removes any that are still not in the layout); until then the failure stays in the log.
export class DiscardLedger {
    ids = new Set<string>();

    add(blockId: string) {
        this.ids.add(blockId);
    }

    async retryAll(deleteBlock: ((data: { blockId: string }) => Promise<void>) | undefined): Promise<void> {
        for (const id of [...this.ids]) {
            if (await discardEphemeralBlock(deleteBlock, id)) {
                this.ids.delete(id);
            }
        }
    }
}

// A new ephemeral block replaces the one on screen, and closing that one stops its program, so it goes through the close question like every
// other user route. Answered "no", no question set up (a missing callback is not permission), or the question or the close failing: the old
// block stays and the new block is discarded, because there is only one ephemeral place. Resolves with whether the new block is shown and, if not, whether it was really deleted.
export interface EphemeralReplaceResult {
    shown: boolean;
    // null when the new block was shown; otherwise whether the discarded block was really deleted (false: it is still in the back end)
    discarded: boolean | null;
}

export async function replaceEphemeral(previousBlockId: string, deps: EphemeralReplaceDeps): Promise<EphemeralReplaceResult> {
    let closed = false;
    try {
        closed = deps.closeAfterConfirm == null ? false : await deps.closeAfterConfirm(previousBlockId, deps.closePrevious);
    } catch (e) {
        console.error("ephemeral block: the question or the close of the old block failed, keeping it", previousBlockId, e);
    }
    if (!closed) {
        const discarded = (await deps.discardNew()) === true;
        return { shown: false, discarded };
    }
    deps.showNew();
    return { shown: true, discarded: null };
}
