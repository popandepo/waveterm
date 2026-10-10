// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test, vi } from "vitest";

vi.mock("electron", () => {
    class BaseWindow {
        destroyed = false;
        destroy() {
            this.destroyed = true;
        }
    }
    return {
        BaseWindow,
        dialog: {},
        globalShortcut: {},
        ipcMain: { on: () => {}, handle: () => {}, once: () => {} },
        screen: {},
        webContents: {},
        app: { getPath: () => "", getAppPath: () => "", isPackaged: false },
    };
});
vi.mock("@/app/store/services", () => ({
    ClientService: {},
    ObjectService: {},
    WindowService: {},
    WorkspaceService: {},
}));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: () => {} }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("emain/emain-events", () => ({ globalEvents: { emit: () => {}, on: () => {} } }));
vi.mock("./emain-activity", () => ({
    getGlobalIsQuitting: () => false,
    getGlobalIsRelaunching: () => false,
    setGlobalIsRelaunching: () => {},
    setWasActive: () => {},
    setWasInFg: () => {},
}));
vi.mock("./emain-log", () => ({ log: () => {} }));
vi.mock("./emain-platform", () => ({
    getElectronAppBasePath: () => "",
    isDev: false,
    unamePlatform: "win32",
}));
vi.mock("./emain-tabview", () => ({ getOrCreateWebViewForTab: () => {}, getWaveTabViewByWebContentsId: () => {} }));
vi.mock("./emain-util", () => ({ delay: () => {}, ensureBoundsAreVisible: () => {}, waveKeyToElectronKey: () => "" }));
vi.mock("./emain-wsh", () => ({ ElectronWshClient: {} }));
vi.mock("./updater", () => ({ updater: null }));

const { WaveBrowserWindow } = await import("./emain-window");

// A window whose action in front of the queue is stuck in a service call that never returns. Only the members that the close
// path uses exist on it; the methods are the real ones from the class.
function stuckWindow() {
    const win: any = {
        actionQueue: [{ op: "switchtab", tabId: "t0", setInBackend: true }],
        waveWindowId: "w1",
        deleteAllowed: false,
    };
    const proto: any = WaveBrowserWindow.prototype;
    win.settleQueuedCloses = proto.settleQueuedCloses;
    win._queueActionInternal = proto._queueActionInternal;
    win.queueCloseTab = proto.queueCloseTab;
    win.destroy = proto.destroy;
    return win;
}

describe("a close queued behind an action that never finishes (final review of PR 21)", () => {
    test("it stays open while the window is alive, and resolves false when the window is destroyed", async () => {
        const win = stuckWindow();
        let result: boolean | undefined;
        const pending = win.queueCloseTab("t1", 2).then((r: boolean) => (result = r));
        await Promise.resolve();
        await Promise.resolve();
        expect(win.actionQueue.length).toBe(2);
        expect(result).toBeUndefined();
        win.destroy();
        expect(await pending).toBe(false);
        expect(win.actionQueue.length).toBe(0);
    });

    test("settling again, and a destroy with nothing queued, are harmless", async () => {
        const win = stuckWindow();
        const pending = win.queueCloseTab("t1", 2);
        await Promise.resolve();
        win.settleQueuedCloses();
        win.settleQueuedCloses();
        win.destroy();
        expect(await pending).toBe(false);
        expect(win.actionQueue.length).toBe(0);
    });
});
