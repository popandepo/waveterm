// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { authorizeWebNav, MaxTitleLength, MaxUrlLength, schemeOf, webLoadState, webNavigate, WebNavTarget, withoutCredentials } from "./emain-webnav";

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

describe("the caller of a web command (source pin: the handler needs a window)", () => {
    const src = readFileSync(path.join(__dirname, "emain-wsh.ts"), "utf8");
    const body = src.slice(src.indexOf("async handle_webnav"), src.indexOf("async handle_notify"));
    test("emain asks the server whether the caller may act on the block, with the route that the router stamped", () => {
        assert.include(body, "WebNavAuthorizeCommand(ElectronWshClient, { callerroute: rh.getSource(), blockid: data.blockid }");
    });
    test("the question comes before any window, tab or web contents is looked at", () => {
        const ask = body.indexOf("WebNavAuthorizeCommand");
        assert.isAbove(ask, -1);
        for (const later of ["getWaveWindowByWorkspaceId(", "allLoadedTabViews", "BlockInfoCommand", "getWebContentsByBlockId("]) {
            assert.isAbove(body.indexOf(later), ask, later + " must come after the question");
        }
    });
    test("the question is asked again right before the action (a belt against a tile that changed workspace meanwhile)", () => {
        const first = body.indexOf("WebNavAuthorizeCommand");
        const second = body.indexOf("WebNavAuthorizeCommand", first + 1);
        const act = body.indexOf("return webNavigate(wc, data.action)");
        assert.isAbove(second, first);
        assert.isAbove(act, second);
        assert.isAbove(second, body.indexOf("getWebContentsByBlockId("));
    });
    test("the caller is never taken from the request", () => {
        assert.notInclude(body, "data.callerroute");
        assert.notInclude(body, "data.callerblock");
    });
});

describe("round 5: the other web command, the answer's credentials, the time limit, and no focus", () => {
    const wsh = readFileSync(path.join(__dirname, "emain-wsh.ts"), "utf8");
    const nav = readFileSync(path.join(__dirname, "emain-webnav.ts"), "utf8");
    test("wsh web get (webselector) asks the same question before it looks at any window", () => {
        const body = wsh.slice(wsh.indexOf("async handle_webselector"), wsh.indexOf("async handle_webnav"));
        const ask = body.indexOf("WebNavAuthorizeCommand");
        assert.isAbove(ask, -1);
        assert.isAbove(body.indexOf("getWaveWindowByWorkspaceId("), ask);
        assert.isAbove(body.indexOf("getWebContentsByBlockId("), ask);
        assert.include(body, "callerroute: rh.getSource()");
    });
    test("the question has a time limit shorter than wsh's own", () => {
        assert.match(nav, /export const AuthorizeTimeoutMs = 3000;/);
        const asks = wsh.split("WebNavAuthorizeCommand(").length - 1;
        assert.equal(asks, 3);
        assert.equal(wsh.split("{ timeout: AuthorizeTimeoutMs }").length - 1, 3);
    });
    test("no handler focuses a window or switches the shown tab before the action", () => {
        const handlers = wsh.slice(wsh.indexOf("async handle_webselector"), wsh.indexOf("async handle_notify"));
        for (const word of [".focus(", "setActiveTab", ".show(", "moveTop", "activateTab", "switchTab", "setIsActive"]) {
            assert.notInclude(handlers, word, word);
            assert.notInclude(nav, word, word);
        }
    });
    test("an address with credentials is returned without them", () => {
        assert.equal(withoutCredentials("http://user:secret@host.example/path?q=1"), "http://host.example/path?q=1");
        assert.equal(withoutCredentials("https://onlyuser@host.example/"), "https://host.example/");
        assert.equal(withoutCredentials("http://127.0.0.1:8080/x"), "http://127.0.0.1:8080/x");
        assert.equal(withoutCredentials("not a url"), "not a url");
        const res = webNavigate(stub({ url: "http://u:p@h.example/" }).wc, "url");
        assert.equal(res.url, "http://h.example/");
        assert.notInclude(JSON.stringify(res), "p@");
    });
});

describe("wsh web info: the load state of a tile", () => {
    function infoStub(opts: { loading?: boolean; frameUrl?: string; noState?: boolean; url?: string }) {
        const s = stub({ url: opts.url ?? "https://example.org/", back: true });
        const wc: any = s.wc;
        if (!opts.noState) {
            wc.isLoading = () => opts.loading === true;
            wc.mainFrame = { url: opts.frameUrl ?? opts.url ?? "https://example.org/" };
        }
        return { wc: wc as WebNavTarget, calls: s.calls };
    }

    test("a loaded page is ready, and info reports address, title, history and state without navigating", () => {
        const s = infoStub({});
        assert.deepEqual(webNavigate(s.wc, "info"), {
            url: "https://example.org/",
            title: "Page",
            cangoback: true,
            cangoforward: false,
            state: "ready",
        });
        assert.deepEqual(s.calls, []);
    });

    test("a page that is still loading is loading, even when its frame shows the error page of an earlier try", () => {
        assert.equal(webNavigate(infoStub({ loading: true }).wc, "info").state, "loading");
        assert.equal(webNavigate(infoStub({ loading: true, frameUrl: "chrome-error://chromewebdata/" }).wc, "info").state, "loading");
    });

    test("the error page of a failed navigation is failed", () => {
        assert.equal(webNavigate(infoStub({ frameUrl: "chrome-error://chromewebdata/" }).wc, "info").state, "failed");
        assert.equal(webLoadState(infoStub({ frameUrl: "CHROME-ERROR://chromewebdata/" }).wc), "failed");
    });

    test("a page that only looks like an error page is ready", () => {
        assert.equal(webNavigate(infoStub({ frameUrl: "https://example.org/chrome-error:" }).wc, "info").state, "ready");
        assert.equal(webNavigate(infoStub({ frameUrl: "" }).wc, "info").state, "ready");
    });

    test("a web contents that cannot tell is ready, not an error", () => {
        assert.equal(webNavigate(infoStub({ noState: true }).wc, "info").state, "ready");
    });

    test("the other actions do not carry a state, and the address of info loses its credentials", () => {
        assert.notProperty(webNavigate(infoStub({}).wc, "url"), "state");
        assert.equal(webNavigate(infoStub({ url: "https://u:secret@example.org/x" }).wc, "info").url, "https://example.org/x");
    });
});

describe("the lookup of a web tile's page asks the named tab or nothing", () => {
    const web = readFileSync(path.join(__dirname, "emain-web.ts"), "utf8");
    const fn = web.slice(web.indexOf("export function getWebContentsByBlockId("), web.indexOf("function escapeSelector("));
    test("a named tab that is not loaded gives null at once, the shown tab is not asked in its place", () => {
        assert.include(fn, "const tabView = tabId ? ww?.allLoadedTabViews?.get(tabId) : ww?.activeTabView;");
        assert.notMatch(fn, /\?\?\s*ww\?\.activeTabView/);
        const check = fn.indexOf("if (tabView == null)");
        assert.isAbove(check, 0);
        assert.isBelow(check, fn.indexOf("webContents.send("));
        assert.isBelow(check, fn.indexOf("ipcMain.once("));
    });
});
