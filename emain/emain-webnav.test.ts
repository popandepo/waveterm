// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, test } from "vitest";
import { webNavigate, WebNavTarget } from "./emain-webnav";

function stub(opts: { back?: boolean; forward?: boolean; url?: string; title?: string } = {}) {
    const calls: string[] = [];
    const wc: WebNavTarget = {
        getURL: () => opts.url ?? "http://127.0.0.1:8080/",
        getTitle: () => opts.title ?? "Page",
        reload: () => void calls.push("reload"),
        navigationHistory: {
            canGoBack: () => opts.back ?? false,
            canGoForward: () => opts.forward ?? false,
            goBack: () => void calls.push("back"),
            goForward: () => void calls.push("forward"),
        },
    };
    return { wc, calls };
}

describe("the navigation commands of a web tile", () => {
    test("reload reloads and reports the address", () => {
        const s = stub({ url: "file:///C:/x/y.html" });
        const res = webNavigate(s.wc, "reload");
        assert.deepEqual(s.calls, ["reload"]);
        assert.equal(res.url, "file:///C:/x/y.html");
    });

    test("back and forward go when there is somewhere to go", () => {
        const s = stub({ back: true, forward: true });
        webNavigate(s.wc, "back");
        webNavigate(s.wc, "forward");
        assert.deepEqual(s.calls, ["back", "forward"]);
    });

    test("back and forward with nowhere to go are errors and navigate nothing", () => {
        const s = stub();
        assert.throws(() => webNavigate(s.wc, "back"), /nothing to go back to/);
        assert.throws(() => webNavigate(s.wc, "forward"), /nothing to go forward to/);
        assert.deepEqual(s.calls, []);
    });

    test("url only reads: address, title and the history state, nothing is navigated", () => {
        const s = stub({ back: true, title: "Status" });
        const res = webNavigate(s.wc, "url");
        assert.deepEqual(res, { url: "http://127.0.0.1:8080/", title: "Status", cangoback: true, cangoforward: false });
        assert.deepEqual(s.calls, []);
    });

    test("an unknown action and a missing web contents are errors", () => {
        const s = stub();
        assert.throws(() => webNavigate(s.wc, "stop"), /unknown web action/);
        assert.throws(() => webNavigate(s.wc, ""), /unknown web action/);
        assert.throws(() => webNavigate(null, "url"), /no web contents/);
        assert.deepEqual(s.calls, []);
    });
});

describe("the wiring that no test can run without a window", () => {
    test("the handler looks in the tab of the request, checks it is loaded, and the block id is validated", async () => {
        const { readFileSync } = await import("node:fs");
        const path = await import("node:path");
        const wsh = readFileSync(path.join(__dirname, "emain-wsh.ts"), "utf8");
        const body = wsh.slice(wsh.indexOf("async handle_webnav("), wsh.indexOf("async handle_notify("));
        assert.include(body, "allLoadedTabViews?.has(data.tabid)");
        assert.include(body, "getWebContentsByBlockId(ww, data.tabid, data.blockid)");
        assert.include(body, "webNavigate(wc, data.action)");
        assert.include(body, "[0-9a-f-]{36}");
        const web = readFileSync(path.join(__dirname, "emain-web.ts"), "utf8");
        assert.include(web, "ww?.allLoadedTabViews?.get(tabId)");
    });
});
