// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import {
    authorizeWebNav,
    buildTextScript,
    capUtf8,
    checkSelector,
    effectiveMaxBytes,
    MaxSelectorLength,
    MaxTitleLength,
    MaxShotBytes,
    MaxUrlLength,
    MaxWebTextBytes,
    schemeOf,
    ShotImage,
    ShrunkShotWidth,
    webLoadState,
    webNavigate,
    WebNavTarget,
    webText,
    webReadRefusal,
    webShot,
    withoutCredentials,
} from "./emain-webnav";

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
        // webselector 1, webnav 2, webtext 2, webshot 2
        const asks = wsh.split("WebNavAuthorizeCommand(").length - 1;
        assert.equal(asks, 7);
        assert.equal(wsh.split("{ timeout: AuthorizeTimeoutMs }").length - 1, 7);
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

describe("wsh web text: what is read, and from which tiles", () => {
    test("a tile with no partition of its own is refused, whatever the record holds in its place", () => {
        for (const meta of [undefined, null, {}, { "web:partition": "" }, { "web:partition": "   " }, { "web:partition": 3 }, { "web:partition": null }, { "web:partition": ["persist:x"] }, { web: { partition: "x" } }]) {
            assert.include(webReadRefusal(meta as any), "shared default sign-in store", JSON.stringify(meta));
        }
    });

    test("a tile with a partition of its own is allowed", () => {
        assert.equal(webReadRefusal({ "web:partition": "persist:agent-1" }), "");
        assert.equal(webReadRefusal({ "web:partition": "agent-1", view: "web" }), "");
    });

    test("the selector goes into the script as a JSON string, never as code", () => {
        const evil = `"); require("child_process"); ("`;
        const script = buildTextScript(evil, 100);
        assert.include(script, JSON.stringify(evil));
        assert.notInclude(script.replace(JSON.stringify(evil), ""), "child_process");
        assert.include(buildTextScript("", 100), 'const sel = "";');
    });

    test("the selector is limited and has no control characters", () => {
        assert.equal(checkSelector(undefined), "");
        assert.equal(checkSelector(""), "");
        assert.equal(checkSelector("main > h1"), "main > h1");
        assert.throws(() => checkSelector("a".repeat(MaxSelectorLength + 1)), /selector/);
        assert.throws(() => checkSelector("a" + String.fromCharCode(10) + "b"), /selector/);
        assert.throws(() => checkSelector("a" + String.fromCharCode(0) + "b"), /selector/);
        assert.throws(() => checkSelector(12 as any), /selector/);
    });

    test("the cap is in bytes of UTF-8 and never splits a character", () => {
        assert.deepEqual(capUtf8("abc", 3), { text: "abc", truncated: false });
        assert.deepEqual(capUtf8("abcd", 3), { text: "abc", truncated: true });
        const rocket = String.fromCodePoint(0x1f680);
        assert.deepEqual(capUtf8(rocket + rocket, 7), { text: rocket, truncated: true });
        assert.deepEqual(capUtf8(rocket, 3), { text: "", truncated: true });
    });

    test("the caller can lower the cap and never raise it", () => {
        assert.equal(effectiveMaxBytes(undefined), MaxWebTextBytes);
        assert.equal(effectiveMaxBytes(0), MaxWebTextBytes);
        assert.equal(effectiveMaxBytes(-5), MaxWebTextBytes);
        assert.equal(effectiveMaxBytes(NaN), MaxWebTextBytes);
        assert.equal(effectiveMaxBytes("9" as any), MaxWebTextBytes);
        assert.equal(effectiveMaxBytes(100), 100);
        assert.equal(effectiveMaxBytes(MaxWebTextBytes * 10), MaxWebTextBytes);
    });

    test("the text of the page comes back cut, and a page that says more is marked", async () => {
        const wc = { executeJavaScript: async (_code: string) => ({ text: "hello world", more: false }) };
        assert.deepEqual(await webText(wc, "", 5), { text: "hello", truncated: true });
        assert.deepEqual(await webText(wc, "", undefined), { text: "hello world", truncated: false });
        const more = { executeJavaScript: async (_code: string) => ({ text: "x", more: true }) };
        assert.deepEqual(await webText(more, "", undefined), { text: "x", truncated: true });
    });

    test("a selector that matches nothing, a selector the page refuses, and no answer are errors", async () => {
        await webText({ executeJavaScript: async () => ({ missing: true }) }, "h9", undefined).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /nothing on the page matches/)
        );
        await webText({ executeJavaScript: async () => ({ error: "bad selector" }) }, "[[", undefined).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /refused the selector: bad selector/)
        );
        await webText({ executeJavaScript: async () => null }, "", undefined).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /no answer/)
        );
        await webText(null, "", undefined).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /no web contents/)
        );
    });

    test("an invalid selector never reaches the page", async () => {
        let ran = false;
        const wc = { executeJavaScript: async () => ((ran = true), { text: "" }) };
        await webText(wc, "a" + String.fromCharCode(10) + "b", undefined).then(
            () => assert.fail("must throw"),
            () => undefined
        );
        assert.isFalse(ran);
    });
});

describe("wsh web text: the wiring (source pins)", () => {
    const wsh = readFileSync(path.resolve(__dirname, "emain-wsh.ts"), "utf8");
    const body = wsh.slice(wsh.indexOf("async handle_webtext"), wsh.indexOf("async handle_notify"));
    test("the caller is asked first, then the record of the block, then the partition rule, and only then the page", () => {
        const ask = body.indexOf("WebNavAuthorizeCommand");
        const info = body.indexOf("BlockInfoCommand");
        const rule = body.indexOf("webReadRefusal(");
        const page = body.indexOf("getWebContentsByBlockId");
        const run = body.indexOf("await webText(");
        assert.isAbove(ask, -1);
        assert.isTrue(ask < info && info < rule && rule < page && page < run, `${ask} ${info} ${rule} ${page} ${run}`);
    });
    test("the caller is asked again right before the page is read, and the rule uses the record, not the request", () => {
        const first = body.indexOf("WebNavAuthorizeCommand");
        const second = body.indexOf("WebNavAuthorizeCommand", first + 1);
        assert.isAbove(second, body.indexOf("getWebContentsByBlockId"));
        assert.include(body, "webReadRefusal(info.block?.meta)");
        assert.notInclude(body, "data.partition");
    });
});

describe("wsh web shot: the picture of one tile", () => {
    function image(opts: { empty?: boolean; bytes?: number; width?: number; height?: number; shrunkBytes?: number } = {}): ShotImage {
        const make = (bytes: number, width: number): ShotImage => ({
            isEmpty: () => opts.empty === true,
            getSize: () => ({ width, height: opts.height ?? 600 }),
            toPNG: () => new Uint8Array(bytes).fill(7),
            resize: (o: { width: number }) => make(opts.shrunkBytes ?? 100, o.width),
        });
        return make(opts.bytes ?? 100, opts.width ?? 800);
    }

    test("the picture of the tile comes back as base64 with its size", async () => {
        const res = await webShot({ capturePage: async () => image({ bytes: 3 }) });
        assert.deepEqual(res, { png: Buffer.from([7, 7, 7]).toString("base64"), width: 800, height: 600 });
    });

    test("an empty picture is an error, not a blank file", async () => {
        await webShot({ capturePage: async () => image({ empty: true }) }).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /empty picture/)
        );
        await webShot({ capturePage: async () => null }).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /empty picture/)
        );
        await webShot(null).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /no web contents/)
        );
    });

    test("a picture above the cap is scaled down once, and refused if it is still too large", async () => {
        const res = await webShot({ capturePage: async () => image({ bytes: MaxShotBytes + 1, shrunkBytes: 50 }) });
        assert.equal(res.width, ShrunkShotWidth);
        assert.equal(Buffer.from(res.png, "base64").length, 50);
        await webShot({ capturePage: async () => image({ bytes: MaxShotBytes + 1, shrunkBytes: MaxShotBytes + 1 }) }).then(
            () => assert.fail("must throw"),
            (e) => assert.match(String(e), /larger than/)
        );
    });

    test("a picture exactly at the cap is not scaled", async () => {
        const res = await webShot({ capturePage: async () => image({ bytes: MaxShotBytes }) });
        assert.equal(res.width, 800);
    });
});

describe("wsh web shot: the wiring (source pins)", () => {
    const wsh = readFileSync(path.resolve(__dirname, "emain-wsh.ts"), "utf8");
    const body = wsh.slice(wsh.indexOf("async handle_webshot"), wsh.indexOf("async handle_notify"));
    test("the caller is asked first, then the record, then the sign-in rule, and only then the page is taken", () => {
        const ask = body.indexOf("WebNavAuthorizeCommand");
        const info = body.indexOf("BlockInfoCommand");
        const rule = body.indexOf("webReadRefusal(info.block?.meta)");
        const page = body.indexOf("getWebContentsByBlockId");
        const run = body.indexOf("await webShot(");
        assert.isAbove(ask, -1);
        assert.isTrue(ask < info && info < rule && rule < page && page < run, `${ask} ${info} ${rule} ${page} ${run}`);
        const second = body.indexOf("WebNavAuthorizeCommand", ask + 1);
        assert.isTrue(page < second && second < run);
    });
    test("the picture is taken from the tile's web contents, never from the window", () => {
        const nav = readFileSync(path.resolve(__dirname, "emain-webnav.ts"), "utf8");
        const shot = nav.slice(nav.indexOf("export async function webShot"));
        assert.include(shot, "wc.capturePage()");
        assert.notInclude(shot, "BrowserWindow");
        assert.notInclude(shot, "desktopCapturer");
    });
});
