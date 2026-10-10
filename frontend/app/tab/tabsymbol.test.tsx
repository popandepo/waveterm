// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { TabBadges } from "./tabbadges";
import { parseTabSymbol } from "./tabsymbol";

const OriginalCss = globalThis.CSS;
const HexColorRegex = /^#([\da-f]{3}|[\da-f]{4}|[\da-f]{6}|[\da-f]{8})$/i;

describe("parseTabSymbol", () => {
    it("treats lowercase names as Font Awesome icons", () => {
        expect(parseTabSymbol("crown")).toEqual({ kind: "icon", value: "crown" });
        expect(parseTabSymbol(" star+fade ")).toEqual({ kind: "icon", value: "star+fade" });
        expect(parseTabSymbol("solid@star")).toEqual({ kind: "icon", value: "solid@star" });
    });

    it("accepts one or two characters of other text, emoji included", () => {
        expect(parseTabSymbol("\u{1F451}")).toEqual({ kind: "text", value: "\u{1F451}" });
        expect(parseTabSymbol("A1")).toEqual({ kind: "text", value: "A1" });
    });

    it("rejects empty, non-string and long values", () => {
        expect(parseTabSymbol("")).toBeNull();
        expect(parseTabSymbol("   ")).toBeNull();
        expect(parseTabSymbol(42)).toBeNull();
        expect(parseTabSymbol(null)).toBeNull();
        expect(parseTabSymbol("<img src=x>")).toBeNull();
        expect(parseTabSymbol("ABC!")).toBeNull();
    });

    it("rejects control, bidirectional and invisible format characters", () => {
        expect(parseTabSymbol("a\u202Eb")).toBeNull();
        expect(parseTabSymbol("\u2066x")).toBeNull();
        expect(parseTabSymbol("\u200Ex")).toBeNull();
        expect(parseTabSymbol("a\u0007")).toBeNull();
        expect(parseTabSymbol("a\u2028")).toBeNull();
    });

    it("keeps emoji sequences that need the zero width joiner", () => {
        const family = "\u{1F468}\u200D\u{1F469}\u200D\u{1F467}";
        expect(parseTabSymbol(family)).toEqual({ kind: "text", value: family });
    });

    it("rejects a joiner that is not inside an emoji sequence", () => {
        expect(parseTabSymbol("\u200D")).toBeNull();
        expect(parseTabSymbol("a\u200D")).toBeNull();
        expect(parseTabSymbol("\u200Da")).toBeNull();
        expect(parseTabSymbol("\u{1F468}\u200D\u200D\u{1F469}")).toBeNull();
    });

    it("bounds a single grapheme stacked with many combining marks, and long icon names", () => {
        expect(parseTabSymbol("e" + "\u0301".repeat(2000))).toBeNull();
        expect(parseTabSymbol("a".repeat(41))).toBeNull();
        expect(parseTabSymbol("a".repeat(40))).toEqual({ kind: "icon", value: "a".repeat(40) });
    });
});

describe("TabBadges with a symbol", () => {
    beforeAll(() => {
        globalThis.CSS = {
            supports: (_property: string, value: string) => HexColorRegex.test(value),
        } as typeof CSS;
    });

    afterAll(() => {
        globalThis.CSS = OriginalCss;
    });

    it("draws the symbol icon in the flag colour instead of the flag", () => {
        const markup = renderToStaticMarkup(<TabBadges flagColor="#ff8800" symbol="crown" />);
        expect(markup).toContain("fa-crown");
        expect(markup).toContain("#ff8800");
        expect(markup).not.toContain("fa-flag");
    });

    it("draws emoji symbols as text", () => {
        const markup = renderToStaticMarkup(<TabBadges flagColor="#ff8800" symbol={"\u{1F451}"} />);
        expect(markup).toContain("\u{1F451}");
        expect(markup).not.toContain("fa-flag");
    });

    it("shows a symbol with no colour", () => {
        const markup = renderToStaticMarkup(<TabBadges symbol="crown" />);
        expect(markup).toContain("fa-crown");
    });

    it("keeps the flag when there is no symbol", () => {
        const markup = renderToStaticMarkup(<TabBadges flagColor="#ff8800" />);
        expect(markup).toContain("fa-flag");
    });

    it("a higher priority badge hides a text symbol without leaving a flag glyph", () => {
        const markup = renderToStaticMarkup(
            <TabBadges
                flagColor="#ff8800"
                symbol={"\u{1F451}"}
                badges={[{ badgeid: "b1", icon: "bell", color: "#4ade80", priority: 5 }]}
            />
        );
        expect(markup).toContain("fa-bell");
        expect(markup).not.toContain("fa-flag");
        expect(markup).not.toContain("\u{1F451}");
        expect(markup).not.toContain("#ff8800");
        expect(markup).not.toContain("rounded-full");
    });

    it("does not show an icon symbol as a dot beside a winning badge", () => {
        const markup = renderToStaticMarkup(
            <TabBadges
                flagColor="#ff8800"
                symbol="crown"
                badges={[{ badgeid: "b1", icon: "bell", color: "#4ade80", priority: 5 }]}
            />
        );
        expect(markup).not.toContain("#ff8800");
        expect(markup).not.toContain("rounded-full");
    });

    it("still shows the flag colour as a dot beside a winning badge when there is no symbol", () => {
        const markup = renderToStaticMarkup(
            <TabBadges flagColor="#ff8800" badges={[{ badgeid: "b1", icon: "bell", color: "#4ade80", priority: 5 }]} />
        );
        expect(markup).toContain("#ff8800");
        expect(markup).toContain("rounded-full");
    });

    it("lets a higher priority badge win over the symbol", () => {
        const markup = renderToStaticMarkup(
            <TabBadges
                flagColor="#ff8800"
                symbol="crown"
                badges={[{ badgeid: "b1", icon: "bell", color: "#4ade80", priority: 5 }]}
            />
        );
        expect(markup).toContain("fa-bell");
        expect(markup).not.toContain("fa-crown");
    });
});
