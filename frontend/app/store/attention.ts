// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// The rules for the "this tile needs the owner" marks: the same as pkg/attention (Go), both tested against pkg/attention/testdata/vectors.json.

export type AttentionLevel = "" | "info" | "need" | "urgent";

export const AttentionLevelKey = "attention:level";
export const AttentionReasonKey = "attention:reason";
export const MaxAttentionReasonLength = 120;

const Ranks: Record<string, number> = { info: 1, need: 2, urgent: 3 };
const HiddenChars = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u;

export function attentionRank(level: unknown): number {
    return typeof level === "string" && Object.prototype.hasOwnProperty.call(Ranks, level) ? Ranks[level] : 0;
}

// The highest level among several (a container shows the highest level of what it holds); unknown levels count as none.
export function highestAttention(levels: unknown[]): AttentionLevel {
    let best: AttentionLevel = "";
    for (const level of levels) {
        if (attentionRank(level) > attentionRank(best)) {
            best = level as AttentionLevel;
        }
    }
    return best;
}

export function attentionNeedsReason(level: unknown): boolean {
    return level === "need" || level === "urgent";
}

// Validates a level and a reason as they would be written on a block. Returns the trimmed reason, or null when refused.
export function checkAttention(level: unknown, reason: unknown): string | null {
    if (attentionRank(level) === 0) {
        return null;
    }
    const text = typeof reason === "string" ? reason.trim() : "";
    if (text === "") {
        return attentionNeedsReason(level) ? null : "";
    }
    if (HiddenChars.test(text) || [...text].length > MaxAttentionReasonLength) {
        return null;
    }
    return text;
}

export interface Attention {
    level: AttentionLevel;
    reason: string;
}

// The attention of a block, read from its meta values (any program can write them): anything that does not pass counts as no attention.
export function attentionOf(level: unknown, reason: unknown): Attention {
    const text = checkAttention(level, reason);
    if (text == null) {
        return { level: "", reason: "" };
    }
    return { level: level as AttentionLevel, reason: text };
}
