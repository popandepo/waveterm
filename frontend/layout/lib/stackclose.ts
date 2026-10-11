// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type CloseBlockAfterConfirm = (blockId: string, close: () => Promise<void> | void) => Promise<boolean>;

// The X on a stack member. It closes the node only through the question (which asks when a program is running and holds one question per
// block through the close). A strip that was rendered without the question closes nothing: a missing callback is not permission.
// Resolves with whether the block was closed.
export async function closeStripTab(
    closeAfterConfirm: CloseBlockAfterConfirm | undefined,
    blockId: string,
    nodeId: string,
    closeNode: (nodeId: string) => Promise<void> | void
): Promise<boolean> {
    if (closeAfterConfirm == null) {
        console.log("stack strip: no close question is set up, nothing was closed", blockId);
        return false;
    }
    return await closeAfterConfirm(blockId, () => closeNode(nodeId));
}
