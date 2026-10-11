// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { FlagColors } from "@/app/tab/flagcolors";
import { memberColorValue, memberSymbolValue, memberTitleValue } from "@/app/tab/stacktablook";

export const ColorMetaKey = "icon:color";
export const IconMetaKey = "frame:icon";
export const TitleMetaKey = "frame:title";

const MemberIcons: { label: string; value: string }[] = [
    { label: "Terminal", value: "terminal" },
    { label: "Robot", value: "robot" },
    { label: "Code", value: "code" },
    { label: "Bug", value: "bug" },
    { label: "Rocket", value: "rocket" },
    { label: "Star", value: "star" },
    { label: "Flask", value: "flask" },
    { label: "Wrench", value: "wrench" },
];

export interface MemberLookDeps {
    // whether the block is still there (a strip can still show a tab of a block that was deleted a moment ago)
    exists: (blockId: string) => boolean;
    setMeta: (blockId: string, meta: { [key: string]: string | null }) => Promise<void>;
    // asks for one line of text; resolves null when the user cancels
    ask: (opts: { title: string; label: string; initial: string }) => Promise<string | null>;
}

export type MemberLookKind = "color" | "icon" | "title";

// The meta a menu choice writes. The value goes through the same checks as the strip applies when it draws the tab (memberColorValue,
// memberSymbolValue), so a value that would not be shown is not written either. An empty value clears the key. null: refused.
export function memberLookMeta(kind: MemberLookKind, raw: string | null | undefined): { [key: string]: string | null } | null {
    const text = (raw ?? "").trim();
    if (kind === "title") {
        if (text === "") {
            return { [TitleMetaKey]: null };
        }
        const title = memberTitleValue(text);
        return title == null ? null : { [TitleMetaKey]: title };
    }
    if (text === "") {
        return { [kind === "color" ? ColorMetaKey : IconMetaKey]: null };
    }
    if (kind === "color") {
        const color = memberColorValue(text);
        return color == null ? null : { [ColorMetaKey]: color };
    }
    const symbol = memberSymbolValue(text);
    return symbol == null ? null : { [IconMetaKey]: symbol.value };
}

async function applyChoice(deps: MemberLookDeps, blockId: string, kind: MemberLookKind, raw: string | null) {
    const meta = memberLookMeta(kind, raw);
    if (meta == null || !deps.exists(blockId)) {
        return false;
    }
    await deps.setMeta(blockId, meta);
    return true;
}

// The free-text choice: asks, and asks again with the refused text while it is refused (a cancel ends it).
async function askUntilAccepted(deps: MemberLookDeps, blockId: string, kind: MemberLookKind, title: string, label: string, initial: string) {
    let current = initial;
    for (let tries = 0; tries < 5; tries++) {
        const answer = await deps.ask({ title, label: tries === 0 ? label : label + " (not accepted, try another)", initial: current });
        if (answer == null) {
            return false;
        }
        if (await applyChoice(deps, blockId, kind, answer)) {
            return true;
        }
        current = answer;
    }
    return false;
}

export interface MemberLookCurrent {
    color?: string | null;
    icon?: string | null;
    title?: string | null;
}

// "Set colour" and "Set icon" for one block: the flag colours and a short icon list, and a free choice (a CSS colour, an icon name, an
// emoji or a word typed or pasted).
export function buildMemberLookItems(blockId: string, current: MemberLookCurrent, deps: MemberLookDeps): ContextMenuItem[] {
    const colorSubmenu: ContextMenuItem[] = [
        { label: "None", type: "checkbox", checked: current.color == null, click: () => void applyChoice(deps, blockId, "color", "") },
        ...FlagColors.map((fc) => ({
            label: fc.label,
            type: "checkbox" as const,
            checked: current.color === fc.value,
            click: () => void applyChoice(deps, blockId, "color", fc.value),
        })),
        { type: "separator" },
        {
            label: "Other...",
            click: () =>
                void askUntilAccepted(deps, blockId, "color", "Set colour", "A CSS colour, such as #ff8800 or tomato", current.color ?? ""),
        },
    ];
    const iconSubmenu: ContextMenuItem[] = [
        { label: "None", type: "checkbox", checked: current.icon == null, click: () => void applyChoice(deps, blockId, "icon", "") },
        ...MemberIcons.map((ic) => ({
            label: ic.label,
            type: "checkbox" as const,
            checked: current.icon === ic.value,
            click: () => void applyChoice(deps, blockId, "icon", ic.value),
        })),
        { type: "separator" },
        {
            label: "Other (type or paste)...",
            click: () =>
                void askUntilAccepted(
                    deps,
                    blockId,
                    "icon",
                    "Set icon",
                    "An emoji, up to two characters, or a Font Awesome icon name",
                    current.icon ?? ""
                ),
        },
    ];
    return [
        { label: "Set colour", type: "submenu", submenu: colorSubmenu },
        { label: "Set icon", type: "submenu", submenu: iconSubmenu },
    ];
}

export interface StripTabMenuParts {
    canTakeOut: boolean;
    takeOut: () => void;
    close: () => unknown;
}

// The right-click menu of a tab in a strip: Take out of stack, Close (through the close question), Rename (frame:title), and the look items.
export function buildStripTabMenu(blockId: string, current: MemberLookCurrent, parts: StripTabMenuParts, deps: MemberLookDeps): ContextMenuItem[] {
    return [
        ...(parts.canTakeOut ? [{ label: "Take out of stack", click: () => parts.takeOut() }] : []),
        { label: "Close", click: () => void parts.close() },
        { type: "separator" },
        {
            label: "Rename",
            click: () => void askUntilAccepted(deps, blockId, "title", "Rename", "A name for this tab", current.title ?? ""),
        },
        ...buildMemberLookItems(blockId, current, deps),
    ];
}
