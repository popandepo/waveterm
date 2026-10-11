// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { memberColorValue } from "@/app/tab/stacktablook";

export const WindowOutlineWidthPx = 3;

// The colour of the outline round the whole window, by the settled scheme: the workspace's colour. Its accent (ws:accent, set by command) wins, then the
// colour of the workspace itself. Both go through memberColorValue (a value that is not accepted counts as not set); null draws no outline. The outline is
// only a border: the top bar, the tabs and the tiles are not filled by it.
export function windowOutlineColor(wsAccent: unknown, wsColor: unknown): string | null {
    return memberColorValue(wsAccent) ?? memberColorValue(wsColor);
}
