// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { validateCssColor } from "@/util/color-validator";
import { parseTabSymbol, TabSymbol } from "./tabsymbol";

export interface StackTabLook {
    symbol: TabSymbol | null;
    color: string | null;
}

const MaxColorLength = 64;
const HexColorShape = /^#([\da-f]{3,4}|[\da-f]{6}|[\da-f]{8})$/i;
// A colour function with plain arguments: no nested parentheses, so no var(), url(), calc() or the like.
const FunctionColorShape = /^(rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch)\([\d .,%/a-z+-]*\)$/i;
// Control, format and line characters (a newline inside an otherwise valid function included) are refused before any pattern is tried.
const HiddenChars = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u;
const KeywordColorShape = /^[a-z]{3,30}$/i;

// The one place where a colour from block meta (which any program can write) is checked before it is used as a style. The shape check works
// everywhere; where a window exists, CSS.supports must also accept the value (so a keyword that is not a colour is refused there).
export function memberColorValue(raw: unknown, validate: (c: string) => string = validateCssColor): string | null {
    if (typeof raw !== "string") {
        return null;
    }
    const value = raw.trim();
    if (value === "" || value.length > MaxColorLength || HiddenChars.test(value)) {
        return null;
    }
    if (!HexColorShape.test(value) && !FunctionColorShape.test(value) && !KeywordColorShape.test(value)) {
        return null;
    }
    if (typeof CSS != "undefined" && typeof CSS.supports == "function") {
        try {
            validate(value);
        } catch {
            return null;
        }
    }
    return value;
}

// The one place where a symbol from block meta is checked: parseTabSymbol (a Font Awesome name of at most 40 characters, or text of at most
// two graphemes and 32 bytes; control, format and bidirectional characters refused).
export function memberSymbolValue(raw: unknown): TabSymbol | null {
    return parseTabSymbol(raw);
}

// What a member's tab in a stack strip shows besides its label. The symbol is frame:icon when that is usable, else icon. The colour is
// icon:color; a value that is not an accepted colour counts as no colour, and a member without one looks as it did before.
export function stackTabLook(
    frameIcon: unknown,
    icon: unknown,
    color: unknown,
    validate: (c: string) => string = validateCssColor
): StackTabLook {
    return {
        symbol: memberSymbolValue(frameIcon) ?? memberSymbolValue(icon),
        color: memberColorValue(color, validate),
    };
}

// The last folder name of a path with / or \ as separator (a Windows cwd such as C:\Users\x\repo gives repo).
export function lastSegment(path: string): string {
    const parts = path.split(/[\\/]/).filter((p) => p.length > 0);
    return parts.length > 0 ? parts[parts.length - 1] : path;
}

const MaxTitleLength = 60;

// A name typed in the Rename box (and any other caller that writes frame:title): no control, format or line characters, at most 60 characters.
// null: refused.
export function memberTitleValue(raw: unknown): string | null {
    if (typeof raw !== "string") {
        return null;
    }
    const value = raw.trim();
    if (value === "" || value.length > MaxTitleLength || HiddenChars.test(value)) {
        return null;
    }
    return value;
}

// A title as the strip draws it, whoever wrote it (wsh setmeta included): one with hidden characters is not used (the label falls back to the
// folder or the view), a long one is cut to 60 characters.
export function drawnTitle(raw: unknown): string | null {
    if (typeof raw !== "string") {
        return null;
    }
    const value = raw.trim();
    if (value === "" || HiddenChars.test(value)) {
        return null;
    }
    return value.length > MaxTitleLength ? value.slice(0, MaxTitleLength - 3) + "..." : value;
}
