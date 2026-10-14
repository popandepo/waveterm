// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { ErrAborted, isPageLoadFailure } from "./loadfailure";

describe("which load failures are the page's own", () => {
    test("a failure of the main frame is shown", () => {
        assert.isTrue(isPageLoadFailure({ errorCode: -105, isMainFrame: true }));
        assert.isTrue(isPageLoadFailure({ errorCode: -27, isMainFrame: true }));
    });

    test("a failure of a sub-frame or a blocked sub-resource is not shown, whatever its code", () => {
        assert.isFalse(isPageLoadFailure({ errorCode: -27, isMainFrame: false }));
        assert.isFalse(isPageLoadFailure({ errorCode: -105, isMainFrame: false }));
        assert.isFalse(isPageLoadFailure({ errorCode: -2, isMainFrame: false }));
    });

    test("an aborted navigation is never a failure, in either frame", () => {
        assert.equal(ErrAborted, -3);
        assert.isFalse(isPageLoadFailure({ errorCode: ErrAborted, isMainFrame: true }));
        assert.isFalse(isPageLoadFailure({ errorCode: ErrAborted, isMainFrame: false }));
    });

    test("an event that does not say which frame is taken as the main frame, and nothing at all is not a failure", () => {
        assert.isTrue(isPageLoadFailure({ errorCode: -105 }));
        assert.isFalse(isPageLoadFailure(null as any));
        assert.isFalse(isPageLoadFailure(undefined as any));
    });
});

describe("the tile uses it (source pin)", () => {
    const source = readFileSync(path.resolve(__dirname, "webview.tsx"), "utf8");
    const handler = source.slice(source.indexOf("const failLoadHandler"), source.indexOf("const webviewFocus"));
    test("the handler asks isPageLoadFailure before it shows anything or tells the caller", () => {
        const ask = handler.indexOf("isPageLoadFailure(e)");
        assert.isAbove(ask, -1);
        assert.isAbove(handler.indexOf("setErrorText("), ask);
        assert.isAbove(handler.indexOf("onFailLoad("), ask);
    });
    test("the old test on the code alone is gone", () => {
        assert.notInclude(handler, "e.errorCode === -3");
    });
});
