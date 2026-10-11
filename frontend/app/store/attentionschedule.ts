// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { attentionClearDelayMs } from "./attentionclear";

export interface AttentionClearEnv {
    // whether the window has the keyboard focus right now
    documentHasFocus: () => boolean;
    // the block that is focused right now
    focusedBlockId: () => string | undefined;
    // the attention keys of a block right now
    read: (blockId: string) => { level: unknown; reason: unknown };
    // writes null to both keys of the block
    clear: (blockId: string) => void;
}

export interface AttentionClearRequest {
    blockId: string;
    level: unknown;
    reason: unknown;
    documentHasFocus: boolean;
    focusSwitched: boolean;
}

// Plans the automatic clearing of a mark (see attentionclear.ts for when) and returns the function that cancels the plan (undefined when nothing is
// planned). The caller plans again, after cancelling, whenever the focused block, the window focus, the level OR the reason changes. When the timer fires, the
// plan is checked against the present once more, synchronously and right before the write: the window still has focus, the block is still the focused one, and
// the mark is still exactly the one that was planned for (level and reason): a newer mark that a program wrote meanwhile is never erased.
// What this cannot close: the write itself takes a round trip to the server, and a mark written during that round trip is erased (there is no
// compare-and-set on block meta).
export function scheduleAttentionClear(req: AttentionClearRequest, env: AttentionClearEnv): (() => void) | undefined {
    if (!req.blockId) {
        return undefined;
    }
    const delay = attentionClearDelayMs(req.level, req.documentHasFocus, req.focusSwitched);
    if (delay == null) {
        return undefined;
    }
    const timeoutId = setTimeout(() => {
        if (!env.documentHasFocus()) {
            return;
        }
        if (env.focusedBlockId() !== req.blockId) {
            return;
        }
        const now = env.read(req.blockId);
        if (now.level !== req.level || now.reason !== req.reason) {
            return;
        }
        env.clear(req.blockId);
    }, delay);
    return () => clearTimeout(timeoutId);
}
