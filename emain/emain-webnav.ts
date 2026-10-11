// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// The navigation commands of a web tile (wsh web reload, back, forward, url). They act on the tile's web contents and never focus anything.

export type WebNavAction = "reload" | "back" | "forward" | "url";

export interface WebNavTarget {
    getURL(): string;
    getTitle(): string;
    reload(): void;
    navigationHistory: {
        canGoBack(): boolean;
        canGoForward(): boolean;
        goBack(): void;
        goForward(): void;
    };
}

export interface WebNavResult {
    url: string;
    title: string;
    cangoback: boolean;
    cangoforward: boolean;
}

// Does the action on the web contents and reports the state before the navigation takes effect (a navigation finishes later, so a script that
// needs the new address asks again with "url"). Back and forward that have nowhere to go are errors, not silent.
export function webNavigate(wc: WebNavTarget, action: string): WebNavResult {
    if (wc == null) {
        throw new Error("no web contents");
    }
    switch (action) {
        case "reload":
            wc.reload();
            break;
        case "back":
            if (!wc.navigationHistory.canGoBack()) {
                throw new Error("nothing to go back to");
            }
            wc.navigationHistory.goBack();
            break;
        case "forward":
            if (!wc.navigationHistory.canGoForward()) {
                throw new Error("nothing to go forward to");
            }
            wc.navigationHistory.goForward();
            break;
        case "url":
            break;
        default:
            throw new Error(`unknown web action ${JSON.stringify(action)} (reload, back, forward or url)`);
    }
    return {
        url: wc.getURL(),
        title: wc.getTitle(),
        cangoback: wc.navigationHistory.canGoBack(),
        cangoforward: wc.navigationHistory.canGoForward(),
    };
}
