// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { attentionOf } from "./attention";
import { attentionClearDelayMs } from "./attentionclear";

export interface AttentionClearEnv {
    // whether the window has the keyboard focus right now
    documentHasFocus: () => boolean;
    // the block that is focused right now
    focusedBlockId: () => string | undefined;
    // the attention keys of a block right now
    read: (blockId: string) => { level: unknown; reason: unknown };
    // asks the server to clear the mark of the block if it is STILL exactly (level, reason): the compare and the write are one step there
    clear: (blockId: string, level: string, reason: string) => void;
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
    // Nothing is planned for a mark that does not draw (a missing, empty or invalid reason, a level that is not one of the three): there is nothing to
    // clear, and the server compares exact strings, so only a mark whose level and reason are both strings that passed the validation is planned.
    if (typeof req.level !== "string" || typeof req.reason !== "string" || attentionOf(req.level, req.reason).level === "") {
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
        env.clear(req.blockId, req.level as string, req.reason as string);
    }, delay);
    return () => clearTimeout(timeoutId);
}

// What the window supplies to the plan: each function reads the present when it is called (never a value captured when the plan was made), which is
// what lets the plan notice that the focus or the mark changed while the timer ran.
export interface AttentionClearSources {
    documentHasFocus: () => boolean;
    focusedBlockId: () => string | undefined;
    readMeta: (blockId: string, key: string) => unknown;
    clearIf: (blockId: string, level: string, reason: string) => void;
}

export function makeAttentionClearEnv(src: AttentionClearSources): AttentionClearEnv {
    return {
        documentHasFocus: () => src.documentHasFocus(),
        focusedBlockId: () => src.focusedBlockId(),
        read: (blockId) => ({ level: src.readMeta(blockId, "attention:level"), reason: src.readMeta(blockId, "attention:reason") }),
        clear: (blockId, level, reason) => src.clearIf(blockId, level, reason),
    };
}

// The focus counts as switched when the focused block or the window focus differs from the last plan (the first plan counts as a switch).
export function focusSwitchedSince(
    prev: { blockId: string | undefined; documentHasFocus: boolean },
    now: { blockId: string | undefined; documentHasFocus: boolean }
): boolean {
    return prev.blockId !== now.blockId || prev.documentHasFocus !== now.documentHasFocus;
}
