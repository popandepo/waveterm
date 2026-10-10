// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { stackAddEntries } from "./stackaddmenu";

function w(label: string, view: string, order?: number, extra: Partial<WidgetConfigType> = {}): WidgetConfigType {
    return { label, "display:order": order, blockdef: { meta: { view } }, ...extra } as WidgetConfigType;
}

describe("the + menu of a stack", () => {
    test("lists the widget bar entries by display order with terminals first", () => {
        const entries = stackAddEntries(
            {
                a: w("Web", "web", 2),
                b: w("Files", "preview", 3),
                c: w("Terminal", "term", 9),
                d: w("Sysinfo", "sysinfo", 1),
            },
            undefined
        );
        expect(entries.map((e) => e.label)).toEqual(["Terminal", "Sysinfo", "Web", "Files"]);
    });

    test("leaves out hidden widgets and widgets of other workspaces, like the widget bar", () => {
        const entries = stackAddEntries(
            {
                a: w("Term", "term", 1),
                b: w("Secret", "web", 2, { "display:hidden": true }),
                c: w("Other", "web", 3, { workspaces: ["ws-2"] }),
                d: w("Mine", "web", 4, { workspaces: ["ws-1"] }),
            },
            "ws-1"
        );
        expect(entries.map((e) => e.label)).toEqual(["Term", "Mine"]);
    });

    test("a stack member is never created magnified", () => {
        const entries = stackAddEntries({ a: w("Web", "web", 1, { magnified: true }) }, undefined);
        expect(entries[0].magnified).toBe(false);
    });

    test("no widgets gives an empty menu, not an error", () => {
        expect(stackAddEntries(undefined, undefined)).toEqual([]);
        expect(stackAddEntries({}, undefined)).toEqual([]);
    });

    test("a widget without a label falls back to its view", () => {
        const entries = stackAddEntries({ a: { blockdef: { meta: { view: "web" } } } as WidgetConfigType }, undefined);
        expect(entries[0].label).toBe("web");
    });
});
