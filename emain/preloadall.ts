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

const SessionIdMetaKey = "session:id";
const SessionIdPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

// A block is a session block when its meta names a Claude session the way the local API reads it (a lowercase uuid).
export function isSessionBlock(block: { meta?: Record<string, any> } | null | undefined): boolean {
    const id = block?.meta?.[SessionIdMetaKey];
    return typeof id === "string" && SessionIdPattern.test(id);
}

// Only a tab that holds a session block is kept loaded: the phone needs the screens of sessions, and pinning every tab
// that was ever shown would keep all their memory. Throws when an object cannot be read; the caller decides what that means.
export async function tabHoldsSessionBlock(getObject: (oref: string) => Promise<any>, tabId: string): Promise<boolean> {
    const tab = await getObject("tab:" + tabId);
    for (const blockId of tab?.blockids ?? []) {
        if (isSessionBlock(await getObject("block:" + blockId))) {
            return true;
        }
    }
    return false;
}

// A loaded view is dropped to make room only when it is not the active tab, was not pinned (it holds a session block),
// and was not used a moment ago.
export function canEvictTabView(view: { isActiveTab: boolean; keepLoaded: boolean }, msSinceUsed: number): boolean {
    if (view.isActiveTab || view.keepLoaded) {
        return false;
    }
    return msSinceUsed >= 1000;
}
