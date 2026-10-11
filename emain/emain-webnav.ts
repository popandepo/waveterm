// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// The navigation commands of a web tile (wsh web reload, back, forward, url). They act on the tile's web contents and never focus anything.

export type WebNavAction = "reload" | "back" | "forward" | "url";

// The schemes that these commands navigate to (reload, back, forward). The tile itself also opens file: addresses and turns text that is not an address
// into a search (frontend ensureUrlScheme); these commands only step through, and reload, http and https pages. A page of another scheme in the history
// (file:, data:, javascript:, devtools:, chrome:, view-source:, about:) is refused, and the owner says if file: pages are to be added.
export const AllowedNavSchemes = ["http:", "https:"];

export const MaxTitleLength = 200;
export const MaxUrlLength = 2048;

export interface WebNavTarget {
    getURL(): string;
    getTitle(): string;
    reload(): void;
    navigationHistory: {
        canGoBack(): boolean;
        canGoForward(): boolean;
        goBack(): void;
        goForward(): void;
        getActiveIndex(): number;
        getEntryAtIndex(index: number): { url: string; title: string } | null | undefined;
    };
}

export interface WebNavResult {
    url: string;
    title: string;
    cangoback: boolean;
    cangoforward: boolean;
}

export function schemeOf(url: unknown): string {
    if (typeof url !== "string") {
        return "";
    }
    try {
        return new URL(url).protocol.toLowerCase();
    } catch {
        return "";
    }
}

function requireAllowed(url: unknown, what: string) {
    if (!AllowedNavSchemes.includes(schemeOf(url))) {
        const shown = schemeOf(url) || "no scheme";
        throw new Error(`${what} is not an http or https page (${shown}); wsh web only reloads and steps through http and https pages`);
    }
}

function capText(text: unknown, max: number): string {
    const s = typeof text === "string" ? text : "";
    const chars = [...s];
    return chars.length > max ? chars.slice(0, max).join("") : s;
}

// What the window may say about a block before a web command acts on it: the block must be the one named, in the tab and the workspace named, and a web
// tile. The ids come from the caller of the command, so none of them is taken on trust.
export function authorizeWebNav(
    data: { blockid?: string; tabid?: string; workspaceid?: string },
    info: { blockid?: string; tabid?: string; workspaceid?: string; block?: { meta?: { [key: string]: any } } } | null | undefined
) {
    if (info == null || info.block == null) {
        throw new Error("the block could not be read");
    }
    if (info.blockid !== data.blockid) {
        throw new Error("the block is not the one named");
    }
    if (info.tabid !== data.tabid) {
        throw new Error("the block is not in the tab named");
    }
    if (info.workspaceid !== data.workspaceid) {
        throw new Error("the tab is not in the workspace named");
    }
    if (info.block.meta?.view !== "web") {
        throw new Error("the block is not a web tile");
    }
}

// Does the action on the web contents and reports the state before the navigation takes effect (a navigation finishes later, so a script that
// needs the new address asks again with "url"). Back and forward that have nowhere to go are errors, not silent, and so are pages of a scheme that is
// not allowed (see AllowedNavSchemes): checked on the entry that the step would land on. Title and address are cut to a sensible length.
export function webNavigate(wc: WebNavTarget, action: string): WebNavResult {
    if (wc == null) {
        throw new Error("no web contents");
    }
    const history = wc.navigationHistory;
    switch (action) {
        case "reload":
            requireAllowed(wc.getURL(), "the page shown");
            wc.reload();
            break;
        case "back": {
            if (!history.canGoBack()) {
                throw new Error("nothing to go back to");
            }
            requireAllowed(history.getEntryAtIndex(history.getActiveIndex() - 1)?.url, "the previous page");
            history.goBack();
            break;
        }
        case "forward": {
            if (!history.canGoForward()) {
                throw new Error("nothing to go forward to");
            }
            requireAllowed(history.getEntryAtIndex(history.getActiveIndex() + 1)?.url, "the next page");
            history.goForward();
            break;
        }
        case "url":
            break;
        default:
            throw new Error(`unknown web action ${JSON.stringify(action)} (reload, back, forward or url)`);
    }
    return {
        url: capText(wc.getURL(), MaxUrlLength),
        title: capText(wc.getTitle(), MaxTitleLength),
        cangoback: history.canGoBack(),
        cangoforward: history.canGoForward(),
    };
}
