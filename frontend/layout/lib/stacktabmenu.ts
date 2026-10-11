// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { CloseBlockAfterConfirm, closeStripTab } from "./stackclose";
import type { StackTabMenuInfo } from "./types";

export interface StackTabMenuDeps {
    closeAfterConfirm: CloseBlockAfterConfirm | undefined;
    closeNode: (nodeId: string) => Promise<void> | void;
    takeNodeOutOfStack: (nodeId: string) => void;
}

// What the right-click menu of a strip tab is given. Close is the same route as the X on the tab (closeStripTab: through the close question,
// nothing is closed when the question is not set up or is answered no).
export function makeStripTabMenuInfo(
    deps: StackTabMenuDeps,
    tab: { blockId: string; nodeId: string },
    stackSize: number
): StackTabMenuInfo {
    return {
        blockId: tab.blockId,
        nodeId: tab.nodeId,
        canTakeOut: stackSize > 1,
        takeOut: () => deps.takeNodeOutOfStack(tab.nodeId),
        close: () => closeStripTab(deps.closeAfterConfirm, tab.blockId, tab.nodeId, deps.closeNode),
    };
}
