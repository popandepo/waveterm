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
