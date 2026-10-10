// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { shouldIncludeWidgetForWorkspace } from "@/app/workspace/widgetfilter";

export interface StackAddEntry {
    label: string;
    icon?: string;
    blockdef: BlockDef;
    magnified: boolean;
}

// The + on a stack offers what the widget bar offers, so the menu follows widgets.json. Terminals come first,
// the rest keep the widget bar order. A new member is never created magnified: it joins a tile.
export function stackAddEntries(
    widgets: { [key: string]: WidgetConfigType } | undefined,
    workspaceId: string | undefined
): StackAddEntry[] {
    const list = Object.values(widgets ?? {}).filter(
        (w) => !w["display:hidden"] && shouldIncludeWidgetForWorkspace(w, workspaceId)
    );
    const isTerm = (w: WidgetConfigType) => w.blockdef?.meta?.view === "term";
    list.sort((a, b) => {
        if (isTerm(a) !== isTerm(b)) {
            return isTerm(a) ? -1 : 1;
        }
        return (a["display:order"] ?? 0) - (b["display:order"] ?? 0);
    });
    return list.map((w) => ({
        label: w.label ?? w.blockdef?.meta?.view ?? "block",
        icon: w.icon,
        blockdef: w.blockdef,
        magnified: false,
    }));
}
