// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { validateCssColor } from "@/util/color-validator";
import { parseTabSymbol, TabSymbol } from "./tabsymbol";

export interface StackTabLook {
    symbol: TabSymbol | null;
    color: string | null;
}

function colorOrNull(raw: unknown, validate: (c: string) => string): string | null {
    if (typeof raw !== "string" || raw.trim() === "") {
        return null;
    }
    try {
        validate(raw);
        return raw.trim();
    } catch {
        return null;
    }
}

// What a member's tab in a stack strip shows besides its label. The icon is frame:icon when that is a usable symbol, else icon (both come
// from block meta that any program can write, so both go through parseTabSymbol). The colour is icon:color; a value that is not a CSS colour
// counts as no colour, and a member without one looks as it did before.
export function stackTabLook(
    frameIcon: unknown,
    icon: unknown,
    color: unknown,
    validate: (c: string) => string = validateCssColor
): StackTabLook {
    return {
        symbol: parseTabSymbol(frameIcon) ?? parseTabSymbol(icon),
        color: colorOrNull(color, validate),
    };
}
