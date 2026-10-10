// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export const TabSymbolMetaKey = "tab:symbol";

const MaxTabSymbolGraphemes = 2;
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

// The value comes from tab meta, which any block can write, so it is shown as plain text or a Font Awesome name only.
export function parseTabSymbol(raw: unknown): TabSymbol | null {
    if (typeof raw !== "string") {
        return null;
    }
    const value = raw.trim();
    if (value === "") {
        return null;
    }
    if (IconNameRegex.test(value)) {
        return { kind: "icon", value };
    }
    const graphemes = splitGraphemes(value);
    if (graphemes.length > MaxTabSymbolGraphemes) {
        return null;
    }
    return { kind: "text", value };
}
