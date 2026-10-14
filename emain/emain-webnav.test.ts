// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import {
    authorizeWebNav,
    buildTextScript,
    capUtf8,
    printableErrorText,
    readWebTile,
    WebReadDeps,
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
        assert.include(body, "FullBlockIdRe.test(data.blockid)");
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
    test("wsh web get (webselector) goes through readWebTile, so the caller is asked before any window is looked at", () => {
        const body = wsh.slice(wsh.indexOf("async handle_webselector"), wsh.indexOf("async handle_webnav"));
        assert.include(body, "readWebTile(data, webReadDeps(rh, data), (wc) => webGetSelector(wc, data.selector, data.opts))");
        assert.include(wsh, "callerroute: rh.getSource()");
    });
    test("the question has a time limit shorter than wsh's own", () => {
        assert.match(nav, /export const AuthorizeTimeoutMs = 3000;/);
        // webnav 2, and the one in webReadDeps that readWebTile calls twice (text, shot and get)
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

describe("wsh web text: what is read, and from which tiles", () => {
    const tileId = "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90";
    const otherTile = "11111111-2222-4333-8444-555555555555";

    test("a tile with no partition is refused, whatever the record holds in its place", () => {
        for (const meta of [undefined, null, {}, { "web:partition": "" }, { "web:partition": "   " }, { "web:partition": 3 }, { "web:partition": null }, { "web:partition": ["persist:x"] }, { web: { partition: "x" } }]) {
            assert.include(webReadRefusal(tileId, meta as any), "shared default sign-in store", JSON.stringify(meta));
        }
    });

    test("a partition named exactly after the tile's own block id is the tile's own and allowed", () => {
        assert.equal(webReadRefusal(tileId, { "web:partition": "persist:" + tileId }), "");
        assert.equal(webReadRefusal(tileId, { "web:partition": tileId, view: "web" }), "");
    });

    test("any other name is refused: a session can point its tile at a store the owner is signed in to", () => {
        for (const p of [
            "persist:agent-1",
            "agent-1",
            "persist:gmail",
            otherTile,
            "persist:" + otherTile,
            "persist:" + tileId + "-x",
            "x-" + tileId,
            "persist:" + tileId.toUpperCase(),
            " persist:" + tileId,
            "persist:" + tileId + " ",
            "persist:persist:" + tileId,
            "persist:",
        ]) {
            const why = webReadRefusal(tileId, { "web:partition": p });
            assert.notEqual(why, "", p);
            assert.include(why, tileId, "the refusal names the partition the tile needs: " + p);
        }
    });

    test("a tile named like another tile's persist: form is refused for itself, and the other tile's own name stays its own", () => {
        assert.notEqual(webReadRefusal(tileId, { "web:partition": "persist:" + otherTile }), "");
        assert.equal(webReadRefusal(otherTile, { "web:partition": "persist:" + otherTile }), "");
    });

    test("no block id means nothing is the tile's own", () => {
        for (const id of ["", undefined, null, "   "]) {
            assert.notEqual(webReadRefusal(id as any, { "web:partition": "persist:" + id }), "", String(id));
            assert.notEqual(webReadRefusal(id as any, { "web:partition": String(id) }), "", String(id));
        }
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

    test("the selector limit counts characters, not UTF-16 units, and every C0 or C1 control is refused", () => {
        const accented = String.fromCodePoint(0xe9);
        const rocket = String.fromCodePoint(0x1f680);
        assert.equal(checkSelector(accented.repeat(300)), accented.repeat(300));
        assert.equal(checkSelector(rocket.repeat(MaxSelectorLength)), rocket.repeat(MaxSelectorLength), "512 emoji are 512 characters");
        assert.throws(() => checkSelector(rocket.repeat(MaxSelectorLength + 1)), /selector/);
        for (const c of [0x85, 0x9b, 0x80, 0x9f, 0x7f]) {
            assert.throws(() => checkSelector("a" + String.fromCharCode(c) + "b"), /selector/, c.toString(16));
        }
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

    // Runs the page script against a stand-in document, the way executeJavaScript would.
    const runTextScript = (selector: string, maxChars: number, doc: any) =>
        new Function("document", "return " + buildTextScript(selector, maxChars))(doc);

    test("the page script cuts at a character, never inside a surrogate pair", () => {
        const rocket = String.fromCodePoint(0x1f680);
        const res = runTextScript("", 3, { body: { innerText: "ab" + rocket + "c" } });
        assert.equal(res.text, "ab", "a cut between the two halves of the rocket drops the half");
        assert.isTrue(res.more);
        assert.equal(runTextScript("", 4, { body: { innerText: "ab" + rocket + "c" } }).text, "ab" + rocket);
        assert.equal(runTextScript("", 10, { body: { innerText: "ab" + rocket } }).text, "ab" + rocket);
    });

    test("the page script reads innerText once: a getter that changes or throws on a second read gets no second read", () => {
        let reads = 0;
        const doc = {
            body: {
                get innerText(): any {
                    reads++;
                    if (reads > 1) {
                        throw new Error("read twice");
                    }
                    return "once";
                },
            },
        };
        const res = runTextScript("", 100, doc);
        assert.equal(reads, 1);
        assert.equal(res.text, "once");
    });

    test("an error the page writes reaches the caller with no control or format characters", async () => {
        const nasty = ["\u001b]0;owned", "\u0007\u001b[2J", String.fromCharCode(0x202e), "gnp.exe\r\nfake$ ", String.fromCharCode(0x9b), "31m"].join("");
        const doc = {
            body: {
                get innerText(): string {
                    throw new Error(nasty);
                },
            },
        };
        const answer = runTextScript("", 100, doc);
        assert.property(answer, "error");
        const wc = { executeJavaScript: async (_code: string) => answer };
        await webText(wc, "", undefined).then(
            () => assert.fail("must throw"),
            (e) => {
                const msg = String(e.message);
                const unsafe = [...msg].filter((ch) => {
                    const c = ch.codePointAt(0);
                    return c < 0x20 || (c >= 0x7f && c <= 0x9f) || (c >= 0x200b && c <= 0x200f) || (c >= 0x2028 && c <= 0x202e) || (c >= 0x2060 && c <= 0x206f) || c === 0xfeff;
                });
                assert.deepEqual(unsafe, [], "no control or format character in " + JSON.stringify(msg));
                assert.include(msg, "owned");
            }
        );
    });

    test("every control, format, line or paragraph separator character and every lone surrogate becomes ?", () => {
        for (const c of [0x061c, 0x00ad, 0x180e, 0xfff9, 0xfffb, 0x0600, 0x110bd, 0x1d173, 0xe0001, 0xe007f, 0x2066, 0x2069, 0x85, 0x2028, 0x2029]) {
            assert.equal(printableErrorText("a" + String.fromCodePoint(c) + "b", 200), "a?b", c.toString(16));
        }
        assert.equal(printableErrorText("a" + String.fromCharCode(0xd800) + "b" + String.fromCharCode(0xdc00), 200), "a?b?");
        for (const c of [0x034f, 0x115f, 0x1160, 0x17b4, 0x17b5, 0x180b, 0x180f, 0x3164, 0xfe00, 0xfe0f, 0xffa0, 0xe0100, 0xe01ef]) {
            assert.equal(printableErrorText("a" + String.fromCodePoint(c) + "b", 200), "a?b", "default-ignorable " + c.toString(16));
        }
        const unsafe = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}\p{Default_Ignorable_Code_Point}]/u;
        let all = "";
        for (let c = 0; c <= 0x10ffff; c++) {
            if (c >= 0xd800 && c <= 0xdfff) {
                continue;
            }
            const ch = String.fromCodePoint(c);
            if (unsafe.test(ch)) {
                all += ch;
            }
        }
        const out = printableErrorText(all, all.length);
        assert.isFalse(unsafe.test(out), "a Cc, Cf, Zl or Zp character got through");
        assert.equal(printableErrorText("plain text, accents: " + String.fromCodePoint(0xe9) + " and emoji " + String.fromCodePoint(0x1f680), 200), "plain text, accents: " + String.fromCodePoint(0xe9) + " and emoji " + String.fromCodePoint(0x1f680));
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

describe("reading a web tile, run with stand-ins for the window, the store and Electron's sessions", () => {
    const blockId = "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90";
    const data = { workspaceid: "ws-1", tabid: "tab-1", blockid: blockId };

    // Electron gives one session object per partition name; the stand-in does the same.
    function sessions() {
        const made = new Map<string, object>();
        return (partition: string) => {
            if (!made.has(partition)) {
                made.set(partition, { partition });
            }
            return made.get(partition);
        };
    }

    function setup(opts: { partition?: any; livePartition?: string; changeMetaTo?: any; authorizeFails?: number } = {}) {
        const steps: string[] = [];
        const sessionFor = sessions();
        const meta: { [key: string]: any } = { view: "web" };
        if (opts.partition !== undefined) {
            meta["web:partition"] = opts.partition;
        }
        let asks = 0;
        const deps: WebReadDeps = {
            authorize: async () => {
                asks++;
                steps.push("authorize");
                if (opts.authorizeFails === asks) {
                    throw new Error("denied");
                }
            },
            getWindow: () => (steps.push("window"), { allLoadedTabViews: new Map([["tab-1", {}]]) }),
            blockInfo: async () => (steps.push("info"), { blockid: blockId, tabid: "tab-1", workspaceid: "ws-1", block: { meta } }),
            getContents: async () => {
                steps.push("contents");
                if (opts.changeMetaTo !== undefined) {
                    meta["web:partition"] = opts.changeMetaTo;
                }
                return { session: sessionFor(opts.livePartition ?? meta["web:partition"]) };
            },
            sessionFor,
        };
        const read = async (_wc: any) => (steps.push("read"), "page text");
        return { deps, steps, read };
    }

    test("a tile on its own partition, live and recorded, is read after the caller is asked twice", async () => {
        const { deps, steps, read } = setup({ partition: "persist:" + blockId });
        assert.equal(await readWebTile(data, deps, read), "page text");
        assert.deepEqual(steps, ["authorize", "info", "window", "contents", "authorize", "read"]);
    });

    test("the record and its partition rule come before the window: an unloaded tab still gets the partition answer", async () => {
        const { deps, steps, read } = setup({ partition: "persist:gmail" });
        deps.getWindow = () => (steps.push("window"), { allLoadedTabViews: new Map() });
        await readWebTile(data, deps, read).then(
            () => assert.fail("must refuse"),
            (e) => assert.match(String(e), /partition is not its own/)
        );
        assert.notInclude(steps, "window", "the window is not looked at for a tile the rule refuses");
    });

    test("only a canonical block id is accepted, before anything is asked", async () => {
        for (const bad of ["-".repeat(36), "0b6f3c1e4f1a4d7e9a432f1d5c7b8e90abcd", "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e9", "0B6F3C1E-4F1A-4D7E-9A43-2F1D5C7B8E90", "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e9g"]) {
            const { deps, steps, read } = setup({ partition: "persist:" + bad });
            await readWebTile({ ...data, blockid: bad }, deps, read).then(
                () => assert.fail("must refuse " + bad),
                (e) => assert.match(String(e), /full block id/)
            );
            assert.deepEqual(steps, [], bad);
        }
    });

    test("a tile with no partition, or another tile's or a sign-in's, is refused before any page is touched", async () => {
        for (const p of [undefined, "", "persist:gmail", "persist:11111111-2222-4333-8444-555555555555", "agent-1"]) {
            const { deps, steps, read } = setup({ partition: p });
            await readWebTile(data, deps, read).then(
                () => assert.fail("must refuse " + p),
                () => undefined
            );
            assert.notInclude(steps, "contents", String(p));
            assert.notInclude(steps, "read", String(p));
        }
    });

    test("a record changed to the tile's own name while the page is fetched does not let the old session be read", async () => {
        // loaded on persist:gmail, the record says the tile's own name by the time it is read: the live session decides
        const { deps, steps, read } = setup({ partition: "persist:" + blockId, livePartition: "persist:gmail" });
        await readWebTile(data, deps, read).then(
            () => assert.fail("the live session is not the tile's own"),
            (e) => assert.match(String(e), /not on its own partition/)
        );
        assert.notInclude(steps, "read");
    });

    test("a record that loses its partition during the fetch is refused on the live session", async () => {
        const { deps, steps, read } = setup({ partition: "persist:" + blockId, changeMetaTo: "", livePartition: "" });
        await readWebTile(data, deps, read).then(
            () => assert.fail("the default store must be refused"),
            () => undefined
        );
        assert.notInclude(steps, "read");
    });

    test("the live session must be the one the record names exactly: persist and in-memory forms are not interchangeable", async () => {
        // the record says the in-memory name, the contents are still on the persistent store of the same id (a signed-in page could stay there)
        let s = setup({ partition: blockId, livePartition: "persist:" + blockId });
        await readWebTile(data, s.deps, s.read).then(
            () => assert.fail("record <id>, live persist:<id> must be refused"),
            (e) => assert.match(String(e), /not on its own partition/)
        );
        assert.notInclude(s.steps, "read");
        s = setup({ partition: "persist:" + blockId, livePartition: blockId });
        await readWebTile(data, s.deps, s.read).then(
            () => assert.fail("record persist:<id>, live <id> must be refused"),
            (e) => assert.match(String(e), /not on its own partition/)
        );
        assert.notInclude(s.steps, "read");
    });

    test("the non-persistent own name is accepted live too", async () => {
        const { deps, read } = setup({ partition: blockId });
        assert.equal(await readWebTile(data, deps, read), "page text");
    });

    test("a caller refused the second time reads nothing", async () => {
        const { deps, steps, read } = setup({ partition: "persist:" + blockId, authorizeFails: 2 });
        await readWebTile(data, deps, read).then(
            () => assert.fail("must refuse"),
            () => undefined
        );
        assert.notInclude(steps, "read");
    });
});

describe("wsh web text: the handler uses readWebTile with Electron's sessions", () => {
    const wsh = readFileSync(path.resolve(__dirname, "emain-wsh.ts"), "utf8");
    const body = wsh.slice(wsh.indexOf("async handle_webtext"), wsh.indexOf("async handle_notify"));
    test("the handler goes through readWebTile and gives it session.fromPartition", () => {
        assert.include(body, "readWebTile(data, webReadDeps(rh, data)");
        assert.include(wsh, "sessionFor: (partition) => session.fromPartition(partition)");
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
        const res = await webShot({ capturePage: async () => image({ bytes: MaxShotBytes + 1, shrunkBytes: 50, width: 3000 }) });
        assert.equal(res.width, ShrunkShotWidth);
        assert.equal(Buffer.from(res.png, "base64").length, 50);
        // a picture already narrower than ShrunkShotWidth is made smaller, never scaled up
        for (const width of [800, ShrunkShotWidth, 2]) {
            const narrow = await webShot({ capturePage: async () => image({ bytes: MaxShotBytes + 1, shrunkBytes: 50, width }) });
            assert.isBelow(narrow.width, width, "from " + width);
            assert.isAtLeast(narrow.width, 1, "from " + width);
        }
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
    test("the shot handler goes through readWebTile, as the text handler does", () => {
        assert.include(body, "readWebTile(data, webReadDeps(rh, data), (wc) => webShot(wc))");
    });

    test("a tile whose live session is not its own when the page is fetched is not pictured", async () => {
        const blockId = "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90";
        const made = new Map<string, object>();
        const sessionFor = (p: string) => {
            if (!made.has(p)) {
                made.set(p, { p });
            }
            return made.get(p);
        };
        const meta: { [key: string]: any } = { view: "web", "web:partition": "persist:" + blockId };
        let captured = false;
        const deps = {
            authorize: async () => {},
            getWindow: () => ({ allLoadedTabViews: new Map([["tab-1", {}]]) }),
            blockInfo: async () => ({ blockid: blockId, tabid: "tab-1", workspaceid: "ws-1", block: { meta } }),
            // the record still says the tile's own name, but the contents were loaded on another store
            getContents: async () => ({
                session: sessionFor("persist:gmail"),
                capturePage: async () => ((captured = true), null),
            }),
            sessionFor,
        };
        await readWebTile({ workspaceid: "ws-1", tabid: "tab-1", blockid: blockId }, deps, (wc) => webShot(wc)).then(
            () => assert.fail("must refuse"),
            (e) => assert.match(String(e), /not on its own partition/)
        );
        assert.isFalse(captured, "capturePage must not run");
    });
    test("the picture is taken from the tile's web contents, never from the window", () => {
        const nav = readFileSync(path.resolve(__dirname, "emain-webnav.ts"), "utf8");
        const shot = nav.slice(nav.indexOf("export async function webShot"));
        assert.include(shot, "wc.capturePage()");
        assert.notInclude(shot, "BrowserWindow");
        assert.notInclude(shot, "desktopCapturer");
    });
});

describe("wsh web get: the sign-in rule", () => {
    const wsh = readFileSync(path.resolve(__dirname, "emain-wsh.ts"), "utf8");
    const body = wsh.slice(wsh.indexOf("async handle_webselector"), wsh.indexOf("async handle_webnav"));
    test("the handler goes through readWebTile, the same path as text and shot, and the request carries no partition", () => {
        assert.include(body, "readWebTile(data, webReadDeps(rh, data)");
        assert.notInclude(body, "data.partition");
    });

    test("a tile whose live session is not its own when the page is fetched is not read by web get", async () => {
        const blockId = "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90";
        const made = new Map<string, object>();
        const sessionFor = (p: string) => {
            if (!made.has(p)) {
                made.set(p, { p });
            }
            return made.get(p);
        };
        const meta: { [key: string]: any } = { view: "web", "web:partition": "persist:" + blockId };
        let selected = false;
        const deps = {
            authorize: async () => {},
            getWindow: () => ({ allLoadedTabViews: new Map([["tab-1", {}]]) }),
            blockInfo: async () => ({ blockid: blockId, tabid: "tab-1", workspaceid: "ws-1", block: { meta } }),
            getContents: async () => ({ session: sessionFor("persist:gmail") }),
            sessionFor,
        };
        const getSelector = async (_wc: any) => ((selected = true), ["<html>"]);
        await readWebTile({ workspaceid: "ws-1", tabid: "tab-1", blockid: blockId }, deps, getSelector).then(
            () => assert.fail("must refuse"),
            (e) => assert.match(String(e), /not on its own partition/)
        );
        assert.isFalse(selected, "the selector must not run");
    });
});
