// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// The navigation commands of a web tile (wsh web reload, back, forward, url). They act on the tile's web contents and never focus anything.

export type WebNavAction = "reload" | "back" | "forward" | "url" | "info";

// The schemes that these commands navigate to (reload, back, forward). The tile itself also opens file: addresses and turns text that is not an address
// into a search (frontend ensureUrlScheme); these commands only step through, and reload, http and https pages. A page of another scheme in the history
// (file:, data:, javascript:, devtools:, chrome:, view-source:, about:) is refused, and the owner says if file: pages are to be added.
export const AllowedNavSchemes = ["http:", "https:"];

// The window's question to the server must be answered before wsh gives up (5000 ms), or a late yes would act after the caller has been told no.
export const AuthorizeTimeoutMs = 3000;

export const MaxTitleLength = 200;
export const MaxUrlLength = 2048;

export interface WebNavTarget {
    getURL(): string;
    getTitle(): string;
    reload(): void;
    isLoading?(): boolean;
    mainFrame?: { url: string };
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
    state?: WebLoadState;
}

export type WebLoadState = "loading" | "ready" | "failed";

// Chromium shows a failed navigation as an error page at this address (the frame, not getURL, tells it).
const ErrorPageScheme = "chrome-error:";

// loading while the page is still loading; failed when the frame shows the error page of a navigation that failed; else ready. Read only.
export function webLoadState(wc: WebNavTarget): WebLoadState {
    if (wc.isLoading?.() === true) {
        return "loading";
    }
    if (schemeOf(wc.mainFrame?.url) === ErrorPageScheme) {
        return "failed";
    }
    return "ready";
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

// An address with a user name or a password in it (http://user:secret@host/) is returned without them: the answer goes to whoever asked and may be logged.
export function withoutCredentials(url: string): string {
    try {
        const u = new URL(url);
        if (u.username === "" && u.password === "") {
            return url;
        }
        u.username = "";
        u.password = "";
        return u.toString();
    } catch {
        return url;
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
        case "info":
            break;
        default:
            throw new Error(`unknown web action ${JSON.stringify(action)} (reload, back, forward, url or info)`);
    }
    const rtn: WebNavResult = {
        url: capText(withoutCredentials(wc.getURL()), MaxUrlLength),
        title: capText(wc.getTitle(), MaxTitleLength),
        cangoback: history.canGoBack(),
        cangoforward: history.canGoForward(),
    };
    if (action === "info") {
        rtn.state = webLoadState(wc);
    }
    return rtn;
}

// ---- wsh web text ----

// The most text one call returns, in bytes of UTF-8. The caller may ask for less.
export const MaxWebTextBytes = 262144;
export const MaxSelectorLength = 512;

export interface WebTextTarget {
    executeJavaScript(code: string): Promise<any>;
}

export interface WebTextResult {
    text: string;
    truncated: boolean;
}

// The tile must have a partition of its own: block meta web:partition exactly <blockId> or persist:<blockId> of that same tile. A tile without one
// shows its pages in the default store of Wave, which every web tile shares and which keeps logins across restarts. Any other name is refused too:
// a session can set web:partition, and a name the owner has signed in to elsewhere would hand that login to the tile. A name made of the tile's own
// id cannot be one the owner uses for another tile. The check is on the record of the block, read by the window itself, never on a value from the
// caller; the owner's allow-list (not built yet) is the planned second way past it.
export function webTextRefusal(blockId: string, meta: { [key: string]: any } | null | undefined): string {
    const partition = meta?.["web:partition"];
    if (typeof partition !== "string" || partition.trim() === "") {
        return "this web tile uses the shared default sign-in store of Wave, so its text is not given out: give the tile its own partition (web:partition) or wait for the allow-list";
    }
    const id = typeof blockId === "string" ? blockId.trim() : "";
    if (id !== "" && (partition === id || partition === "persist:" + id)) {
        return "";
    }
    return `this web tile's partition is not its own, so its text is not given out: set web:partition to ${id} or persist:${id} (a name another tile or a sign-in may use is refused)`;
}

// The same rule on the web contents that is about to be read: its live session must be the session of the tile's own partition name. The record
// can change while the page is fetched, and a tile keeps the session it was loaded with, so the record alone is not enough. Session objects are
// compared (Electron gives one object per partition name), never names.
export function liveSessionRefusal(blockId: string, wc: { session?: unknown } | null | undefined, sessionFor: (partition: string) => unknown): string {
    const id = typeof blockId === "string" ? blockId.trim() : "";
    const live = wc?.session;
    if (id !== "" && live != null && (live === sessionFor(id) || live === sessionFor("persist:" + id))) {
        return "";
    }
    return `this web tile's page is not on its own partition, so its text is not given out (the tile may have been loaded before its web:partition was set; reload it)`;
}

export interface WebReadDeps {
    authorize(): Promise<void>;
    getWindow(): { allLoadedTabViews?: { has(tabId: string): boolean } } | null | undefined;
    blockInfo(): Promise<any>;
    getContents(ww: any): Promise<any>;
    sessionFor(partition: string): unknown;
}

// A block id as Wave writes it: a lower-case canonical uuid.
export const FullBlockIdRe = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

// The order every read of a tile's page follows: the caller first; the record of the block (the one named, a web tile) and its partition rule; the
// window and its loaded tab; only then the page; then the caller again and the live session of the contents, right before the read.
export async function readWebTile<T>(
    data: { blockid?: string; tabid?: string; workspaceid?: string },
    deps: WebReadDeps,
    read: (wc: any) => Promise<T>
): Promise<T> {
    if (!data.tabid || !data.blockid || !data.workspaceid) {
        throw new Error("tabid, blockid and workspaceid are required");
    }
    if (!FullBlockIdRe.test(data.blockid)) {
        throw new Error("blockid must be a full block id");
    }
    await deps.authorize();
    const info = await deps.blockInfo();
    authorizeWebNav(data, info);
    const refusal = webTextRefusal(data.blockid, info.block?.meta);
    if (refusal) {
        throw new Error(refusal);
    }
    const ww = deps.getWindow();
    if (ww == null) {
        throw new Error(`no window found with workspace ${data.workspaceid}`);
    }
    if (!ww.allLoadedTabViews?.has(data.tabid)) {
        throw new Error(`the tab ${data.tabid} is not loaded in its window (show it once, then ask again)`);
    }
    const wc = await deps.getContents(ww);
    if (wc == null) {
        throw new Error(`no web contents found for block ${data.blockid} (is its page ready?)`);
    }
    await deps.authorize();
    const live = liveSessionRefusal(data.blockid, wc, deps.sessionFor);
    if (live) {
        throw new Error(live);
    }
    return await read(wc);
}

export function checkSelector(selector: unknown): string {
    if (selector == null || selector === "") {
        return "";
    }
    // characters, as the CLI counts them, not UTF-16 units; every C0 and C1 control is refused
    if (typeof selector !== "string" || [...selector].length > MaxSelectorLength || /\p{Cc}/u.test(selector)) {
        throw new Error(`the selector must be text of at most ${MaxSelectorLength} characters without control characters`);
    }
    return selector;
}

// The script that runs in the page: innerText of the body or of the first match, cut in the page so that a huge page is not copied out whole.
// The selector goes in as a JSON string, never joined into code.
export function buildTextScript(selector: string, maxChars: number): string {
    return `(() => {
    try {
        const sel = ${JSON.stringify(selector)};
        const el = sel ? document.querySelector(sel) : document.body;
        if (!el) { return { missing: true }; }
        const t = typeof el.innerText === "string" ? el.innerText : "";
        let s = t.slice(0, ${Math.floor(maxChars)});
        const last = s.charCodeAt(s.length - 1);
        if (last >= 0xd800 && last <= 0xdbff) { s = s.slice(0, -1); }
        return { text: s, more: t.length > ${Math.floor(maxChars)} };
    } catch (error) {
        return { error: String(error && error.message ? error.message : error).slice(0, 200) };
    }
})()`;
}

// The characters that never reach a terminal from a page: controls (Cc), every format character (Cf: bidi, zero-width, soft hyphen, tags ...), line
// and paragraph separators, lone surrogates (Cs), and the default-ignorable code points (variation selectors, Hangul fillers ...), which draw
// nothing. The same classes as safeText in the CLI.
const unprintableRe = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}\p{Cs}\p{Default_Ignorable_Code_Point}]/u;

// An error message the page wrote (a getter can throw anything) goes to a terminal: every unprintable character becomes "?", and it is cut at
// maxChars characters.
export function printableErrorText(text: string, maxChars: number): string {
    let out = "";
    let n = 0;
    for (const ch of text) {
        if (n >= maxChars) {
            break;
        }
        out += unprintableRe.test(ch) ? "?" : ch;
        n++;
    }
    return out;
}

// Cuts text to at most maxBytes of UTF-8 without splitting a character.
export function capUtf8(text: string, maxBytes: number): WebTextResult {
    const enc = new TextEncoder();
    if (enc.encode(text).length <= maxBytes) {
        return { text, truncated: false };
    }
    let out = "";
    let bytes = 0;
    for (const ch of text) {
        const n = enc.encode(ch).length;
        if (bytes + n > maxBytes) {
            break;
        }
        out += ch;
        bytes += n;
    }
    return { text: out, truncated: true };
}

export function effectiveMaxBytes(asked: unknown): number {
    if (typeof asked !== "number" || !Number.isFinite(asked) || asked <= 0) {
        return MaxWebTextBytes;
    }
    return Math.min(Math.floor(asked), MaxWebTextBytes);
}

export async function webText(wc: WebTextTarget, selector: unknown, askedMaxBytes: unknown): Promise<WebTextResult> {
    if (wc == null) {
        throw new Error("no web contents");
    }
    const sel = checkSelector(selector);
    const max = effectiveMaxBytes(askedMaxBytes);
    const res = await wc.executeJavaScript(buildTextScript(sel, max));
    if (res == null || typeof res !== "object") {
        throw new Error("the page gave no answer");
    }
    if (res.error) {
        throw new Error(`the page refused the selector: ${printableErrorText(String(res.error), 200)}`);
    }
    if (res.missing) {
        throw new Error("nothing on the page matches the selector");
    }
    const capped = capUtf8(typeof res.text === "string" ? res.text : "", max);
    return { text: capped.text, truncated: capped.truncated || res.more === true };
}
