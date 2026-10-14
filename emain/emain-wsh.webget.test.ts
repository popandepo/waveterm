// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, test, vi } from "vitest";

// The handler itself is run; only what it reaches outside this file is stood in for.
const h = vi.hoisted(() => {
    const sessions = new Map<string, object>();
    return {
        sessionFor: (p: string) => {
            if (!sessions.has(p)) {
                sessions.set(p, { p });
            }
            return sessions.get(p);
        },
        record: { meta: {} as { [key: string]: any } },
        livePartition: "",
        selected: 0,
        authorized: 0,
    };
});

vi.mock("electron", () => ({
    Notification: class {},
    net: {},
    safeStorage: {},
    shell: {},
    session: { fromPartition: (p: string) => h.sessionFor(p) },
}));
vi.mock("@/app/store/services", () => ({ WindowService: {} }));
vi.mock("@/app/store/wshclient", () => ({ WshClient: class {}, RpcResponseHelper: class {} }));
vi.mock("@/app/store/wshclientapi", () => ({
    RpcApi: {
        WebNavAuthorizeCommand: async () => {
            h.authorized++;
        },
        BlockInfoCommand: async (_client: any, blockid: string) => ({
            blockid,
            tabid: "tab-1",
            workspaceid: "ws-1",
            block: { meta: h.record.meta },
        }),
    },
}));
vi.mock("emain/updater", () => ({ getResolvedUpdateChannel: () => "latest" }));
vi.mock("./closetabprompt", () => ({ wshCloseRefusal: () => "" }));
vi.mock("./emain-platform", () => ({ unamePlatform: "win32" }));
vi.mock("./emain-web", () => ({
    getWebContentsByBlockId: async () => ({ session: h.sessionFor(h.livePartition) }),
    webGetSelector: async () => {
        h.selected++;
        return ["<p>secret</p>"];
    },
}));
vi.mock("./emain-window", () => ({
    createBrowserWindow: () => null,
    getWaveWindowById: () => null,
    getWaveWindowByWorkspaceId: () => ({ allLoadedTabViews: new Map([["tab-1", {}]]) }),
}));

import { ElectronWshClientType } from "./emain-wsh";

const blockId = "0b6f3c1e-4f1a-4d7e-9a43-2f1d5c7b8e90";
const rh: any = { getSource: () => "proc:caller" };
const data: any = { workspaceid: "ws-1", tabid: "tab-1", blockid: blockId, selector: "body", opts: {} };

describe("wsh web get, the handler run", () => {
    test("a tile whose live session is not the one its record names: the selector never runs", async () => {
        h.record.meta = { view: "web", "web:partition": "persist:" + blockId };
        h.livePartition = "persist:gmail";
        h.selected = 0;
        const client = new ElectronWshClientType();
        await client.handle_webselector(rh, data).then(
            () => assert.fail("must refuse"),
            (e) => assert.match(String(e), /not on its own partition/)
        );
        assert.equal(h.selected, 0, "webGetSelector must not run");
    });

    test("a tile on the store its record names is read, after the caller is asked twice", async () => {
        h.record.meta = { view: "web", "web:partition": "persist:" + blockId };
        h.livePartition = "persist:" + blockId;
        h.selected = 0;
        h.authorized = 0;
        const client = new ElectronWshClientType();
        assert.deepEqual(await client.handle_webselector(rh, data), ["<p>secret</p>"]);
        assert.equal(h.selected, 1);
        assert.equal(h.authorized, 2);
    });

    test("a tile with no partition of its own is refused before the page is fetched", async () => {
        h.record.meta = { view: "web" };
        h.livePartition = "";
        h.selected = 0;
        const client = new ElectronWshClientType();
        await client.handle_webselector(rh, data).then(
            () => assert.fail("must refuse"),
            (e) => assert.match(String(e), /shared default sign-in store/)
        );
        assert.equal(h.selected, 0);
    });
});
