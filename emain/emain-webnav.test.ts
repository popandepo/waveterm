// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, test } from "vitest";
import { authorizeWebNav, MaxTitleLength, MaxUrlLength, schemeOf, webNavigate, WebNavTarget } from "./emain-webnav";

function stub(opts: { back?: boolean; forward?: boolean; url?: string; title?: string; history?: string[]; index?: number } = {}) {
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
            getActiveIndex: () => opts.index ?? 1,
            getEntryAtIndex: (i: number) => {
                const h = opts.history ?? ["http://a.example/", "http://b.example/", "http://c.example/"];
                return h[i] == null ? null : { url: h[i], title: "t" };
            },
        },
    };
    return { wc, calls };
}

describe("the navigation commands of a web tile", () => {
    test("reload reloads an http or https page and reports the address", () => {
        const s = stub({ url: "https://example.org/x" });
        const res = webNavigate(s.wc, "reload");
        assert.deepEqual(s.calls, ["reload"]);
        assert.equal(res.url, "https://example.org/x");
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

describe("which pages these commands step to", () => {
    test("only http and https, checked on the entry that the step lands on", () => {
        for (const bad of ["file:///C:/x/y.html", "data:text/html,<b>x</b>", "javascript:alert(1)", "chrome://settings", "devtools://devtools/x", "view-source:http://a/", "about:blank", "", "not a url"]) {
            const back = stub({ back: true, history: ["http://a.example/", bad, "http://c.example/"], index: 2 });
            assert.throws(() => webNavigate(back.wc, "back"), /not an http or https page/, "back to " + bad);
            assert.deepEqual(back.calls, [], "back to " + bad);
            const fwd = stub({ forward: true, history: ["http://a.example/", "http://b.example/", bad], index: 1 });
            assert.throws(() => webNavigate(fwd.wc, "forward"), /not an http or https page/, "forward to " + bad);
            assert.deepEqual(fwd.calls, [], "forward to " + bad);
            const rel = stub({ url: bad });
            assert.throws(() => webNavigate(rel.wc, "reload"), /not an http or https page/, "reload of " + bad);
            assert.deepEqual(rel.calls, [], "reload of " + bad);
        }
    });

    test("an http or https entry is stepped to, in either case", () => {
        const s = stub({ back: true, forward: true, history: ["HTTP://A.example/", "https://b.example/", "http://c.example/"], index: 1 });
        webNavigate(s.wc, "back");
        webNavigate(s.wc, "forward");
        assert.deepEqual(s.calls, ["back", "forward"]);
        assert.equal(schemeOf("HTTPS://x/"), "https:");
        assert.equal(schemeOf(5), "");
    });

    test("url still tells about a page of another scheme (it only reads), with the address and title cut to a sensible length", () => {
        const s = stub({ url: "file:///C:/x/y.html?" + "q".repeat(5000), title: "T".repeat(1000) });
        const res = webNavigate(s.wc, "url");
        assert.equal([...res.url].length, MaxUrlLength);
        assert.equal([...res.title].length, MaxTitleLength);
        assert.deepEqual(s.calls, []);
    });
});

describe("who may be navigated", () => {
    const data = { blockid: "b1", tabid: "t1", workspaceid: "w1" };
    const info = (over: any = {}) => ({ blockid: "b1", tabid: "t1", workspaceid: "w1", block: { meta: { view: "web" } }, ...over });

    test("the block named, in the tab and the workspace named, and a web tile, passes", () => {
        authorizeWebNav(data, info());
    });

    test("every other combination is refused", () => {
        assert.throws(() => authorizeWebNav(data, null), /could not be read/);
        assert.throws(() => authorizeWebNav(data, info({ block: undefined })), /could not be read/);
        assert.throws(() => authorizeWebNav(data, info({ blockid: "other" })), /not the one named/);
        assert.throws(() => authorizeWebNav(data, info({ tabid: "other" })), /not in the tab named/);
        assert.throws(() => authorizeWebNav(data, info({ workspaceid: "other" })), /not in the workspace named/);
        assert.throws(() => authorizeWebNav(data, info({ block: { meta: { view: "term" } } })), /not a web tile/);
        assert.throws(() => authorizeWebNav(data, info({ block: { meta: {} } })), /not a web tile/);
        assert.throws(() => authorizeWebNav(data, info({ block: { meta: { view: "webx" } } })), /not a web tile/);
    });

    test("the handler asks the block's own record before it touches any web contents, and the window of the workspace must hold the tab", async () => {
        const { readFileSync } = await import("node:fs");
        const path = await import("node:path");
        const wsh = readFileSync(path.join(__dirname, "emain-wsh.ts"), "utf8");
        const body = wsh.slice(wsh.indexOf("async handle_webnav("), wsh.indexOf("async handle_notify("));
        const auth = body.indexOf("authorizeWebNav(data, await RpcApi.BlockInfoCommand(ElectronWshClient, data.blockid))");
        const contents = body.indexOf("getWebContentsByBlockId(");
        assert.isAbove(auth, -1);
        assert.isBelow(auth, contents, "authorization comes before the web contents are looked up");
        assert.include(body, "allLoadedTabViews?.has(data.tabid)");
    });
});
