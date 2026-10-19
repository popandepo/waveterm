// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// A block has a screen the local API can read only while its tab's page is loaded, so after boot every tab of the window's
// workspace is loaded in the background (parked off screen by preloadTab), one at a time so a slow tab cannot stall the rest.
// One tab failing is reported and does not stop the others.
export async function preloadAllTabs(
    tabIds: string[],
    activeTabId: string,
    preload: (tabId: string) => Promise<void>,
    report: (tabId: string, err: unknown) => void
): Promise<number> {
    let loaded = 0;
    for (const tabId of tabIds) {
        if (tabId === activeTabId) {
            continue;
        }
        try {
            await preload(tabId);
            loaded++;
        } catch (err) {
            report(tabId, err);
        }
    }
    return loaded;
}

export const MaxPinnedTabs = 16;

// The tabs loaded at boot are pinned so the cache never unloads them (their blocks' screens exist only while the page is
// loaded). A pin is never released, and only the boot pass pins, so the count cannot grow after boot. Past the cap a tab
// is left unpinned and a line says so.
export function makeTabPinner(cap: number, log: (line: string) => void): (view: { waveTabId: string; keepLoaded: boolean }) => boolean {
    const pinned = new Set<string>();
    return (view) => {
        if (pinned.has(view.waveTabId)) {
            view.keepLoaded = true;
            return true;
        }
        if (pinned.size >= cap) {
            log(`[error] tab ${view.waveTabId} is not kept loaded: ${cap} tabs are already pinned`);
            return false;
        }
        pinned.add(view.waveTabId);
        view.keepLoaded = true;
        return true;
    };
}

// A loaded view is dropped to make room only when it is not the active tab, was not pinned at boot,
// and was not used a moment ago.
export function canEvictTabView(view: { isActiveTab: boolean; keepLoaded: boolean }, msSinceUsed: number): boolean {
    if (view.isActiveTab || view.keepLoaded) {
        return false;
    }
    return msSinceUsed >= 1000;
}
