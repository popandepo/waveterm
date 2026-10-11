// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { CloseBlockAfterConfirm } from "./stackclose";

export interface EphemeralReplaceDeps {
    closeAfterConfirm: CloseBlockAfterConfirm | undefined;
    closePrevious: () => Promise<void> | void;
    showNew: () => void;
    // the new block is not shown and its block is deleted (the old one stays)
    discardNew: () => Promise<void> | void;
}

// A new ephemeral block replaces the one on screen, and closing that one stops its program, so it goes through the close question like every
// other user route. Answered "no", or no question set up (a missing callback is not permission): the old block stays and the new block is
// discarded, because there is only one ephemeral place. Resolves with whether the new block is shown.
export async function replaceEphemeral(previousBlockId: string, deps: EphemeralReplaceDeps): Promise<boolean> {
    const closed =
        deps.closeAfterConfirm == null ? false : await deps.closeAfterConfirm(previousBlockId, deps.closePrevious);
    if (!closed) {
        await deps.discardNew();
        return false;
    }
    deps.showNew();
    return true;
}
