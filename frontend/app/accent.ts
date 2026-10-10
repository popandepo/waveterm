// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atoms, getConnConfigKeyAtom, getOrefMetaKeyAtom } from "@/app/store/global";
import { makeORef } from "@/app/store/wos";
import { getLayoutModelForStaticTab } from "@/layout/index";
import { validateCssColor } from "@/util/color-validator";
import { Atom, atom, useAtomValue } from "jotai";
import { useEffect, useMemo } from "react";

export type AccentSources = {
    connAccent?: string | null;
    tabAccent?: string | null;
    wsAccent?: string | null;
    wsColor?: string | null;
};

export function isValidAccent(color: unknown): color is string {
    if (typeof color != "string" || color.trim() == "") {
        return false;
    }
    try {
        validateCssColor(color);
        return true;
    } catch {
        return false;
    }
}

// Order matters. A connection colour comes from configuration and wins, so a session on a host that has one cannot
// look like anything else; then the tab, then the workspace accent, then the workspace icon colour.
export function pickAccent(sources: AccentSources): string | null {
    const ordered = [sources.connAccent, sources.tabAccent, sources.wsAccent, sources.wsColor];
    for (const c of ordered) {
        if (isValidAccent(c)) {
            return c.trim();
        }
    }
    return null;
}

const MixSteps: [string, string, number][] = [
    ["--color-accent-50", "white", 12],
    ["--color-accent-100", "white", 25],
    ["--color-accent-200", "white", 45],
    ["--color-accent-300", "white", 70],
    ["--color-accent-500", "black", 82],
    ["--color-accent-600", "black", 66],
    ["--color-accent-700", "black", 50],
    ["--color-accent-800", "black", 36],
    ["--color-accent-900", "black", 26],
];

// The whole palette is derived from one colour with color-mix, so no colour maths runs in script.
export function accentVars(color: string): Record<string, string> {
    const vars: Record<string, string> = {
        "--accent-color": color,
        "--color-accent": color,
        "--color-accent-400": color,
        "--color-accenthover": `color-mix(in srgb, ${color} 80%, white)`,
        "--color-accentbg": `color-mix(in srgb, ${color} 50%, transparent)`,
    };
    for (const [name, other, pct] of MixSteps) {
        vars[name] = `color-mix(in srgb, ${color} ${pct}%, ${other})`;
    }
    return vars;
}

export function applyAccent(root: HTMLElement, color: string | null, previous: string[]): string[] {
    for (const name of previous) {
        root.style.removeProperty(name);
    }
    if (color == null) {
        return [];
    }
    const vars = accentVars(color);
    for (const [name, value] of Object.entries(vars)) {
        root.style.setProperty(name, value);
    }
    return Object.keys(vars);
}

function makeAccentAtom(tabId: string): Atom<string | null> {
    return atom((get) => {
        const layoutModel = getLayoutModelForStaticTab();
        const blockId = layoutModel ? get(layoutModel.focusedNode)?.data?.blockId : null;
        const connName = blockId ? get(getOrefMetaKeyAtom(makeORef("block", blockId), "connection")) : null;
        const connAccent = connName ? get(getConnConfigKeyAtom(connName, "display:accent")) : null;
        const tabAccent = tabId ? get(getOrefMetaKeyAtom(makeORef("tab", tabId), "tab:accent")) : null;
        const workspace = get(atoms.workspace);
        return pickAccent({
            connAccent,
            tabAccent,
            wsAccent: workspace?.meta?.["ws:accent"],
            wsColor: workspace?.color,
        });
    });
}

// Recolours the whole frame of this tab's page: the accent follows the focused block's connection, then the tab, then
// the workspace. Each tab is its own page, so switching tabs switches the colour.
export function AccentController(): null {
    const tabId = useAtomValue(atoms.staticTabId);
    const accentAtom = useMemo(() => makeAccentAtom(tabId), [tabId]);
    const accent = useAtomValue(accentAtom);
    useEffect(() => {
        const root = document.documentElement;
        let applied = applyAccent(root, accent, []);
        return () => {
            applied = applyAccent(root, null, applied);
        };
    }, [accent]);
    return null;
}
