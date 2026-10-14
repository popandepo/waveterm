// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// ERR_ABORTED: a navigation that was replaced by another one, not a failure.
export const ErrAborted = -3;

export interface LoadFailureEvent {
    errorCode?: number;
    isMainFrame?: boolean;
}

// Whether a did-fail-load event of the webview is a failure of the PAGE that the tile shows. The event also fires for a frame inside the page and
// for a sub-frame's blocked request (a tracking pixel or an embed refused by a content security policy, ERR_BLOCKED_BY_CSP): those leave the page
// as it is, and drawing them as a failure covered a page that had loaded. Only the main frame counts, and an aborted navigation never does. An event
// that does not say which frame it is for is taken as the main frame, so a failure is never hidden by a missing field.
export function isPageLoadFailure(e: LoadFailureEvent): boolean {
    if (e == null) {
        return false;
    }
    if (e.errorCode === ErrAborted) {
        return false;
    }
    return e.isMainFrame !== false;
}
