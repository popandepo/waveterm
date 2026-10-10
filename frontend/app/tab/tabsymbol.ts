// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export const TabSymbolMetaKey = "tab:symbol";

const MaxTabSymbolGraphemes = 2;
const MaxTabSymbolTextBytes = 32;
const MaxTabSymbolIconNameLength = 40;
const ZeroWidthJoiner = 0x200d;
const IconNameRegex = /^(solid@|regular@|brands@|custom@)?[a-z0-9-]+(\+(spin|beat|fade|fw))*$/;

export type TabSymbol = { kind: "icon"; value: string } | { kind: "text"; value: string };

function splitGraphemes(text: string): string[] {
    const SegmenterCtor = typeof Intl !== "undefined" ? (Intl as any).Segmenter : null;
    if (SegmenterCtor != null) {
        const segmenter = new SegmenterCtor(undefined, { granularity: "grapheme" });
        return Array.from(segmenter.segment(text), (s: { segment: string }) => s.segment);
    }
    return [...text];
}

// Control characters and format characters (which include the bidirectional overrides and isolates) can reorder or
// hide what the tab shows. The zero width joiner is the one format character emoji sequences need.
export function hasHiddenOrDirectionalChars(text: string): boolean {
    for (const ch of text) {
        const cp = ch.codePointAt(0);
        if (cp === ZeroWidthJoiner) {
            continue;
        }
        if (/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(ch)) {
            return true;
        }
    }
    return false;
}

function utf8ByteLength(text: string): number {
    return new TextEncoder().encode(text).length;
}

// The value comes from tab meta, which any block can write, so it is shown as plain text or a Font Awesome name only,
// and is bounded in graphemes and in bytes.
export function parseTabSymbol(raw: unknown): TabSymbol | null {
    if (typeof raw !== "string") {
        return null;
    }
    if (hasHiddenOrDirectionalChars(raw)) {
        return null;
    }
    const value = raw.trim();
    if (value === "") {
        return null;
    }
    if (IconNameRegex.test(value)) {
        return value.length <= MaxTabSymbolIconNameLength ? { kind: "icon", value } : null;
    }
    if (utf8ByteLength(value) > MaxTabSymbolTextBytes) {
        return null;
    }
    if (splitGraphemes(value).length > MaxTabSymbolGraphemes) {
        return null;
    }
    return { kind: "text", value };
}