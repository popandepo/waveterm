// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeAll, describe, expect, test, vi } from "vitest";

const Addons: FakeWebglAddon[] = [];

class FakeWebglAddon {
    disposed = false;
    lossCallback: () => void = null;
    onContextLoss(cb: () => void) {
        this.lossCallback = cb;
        return { dispose: () => (this.lossCallback = null) };
    }
    activate() {}
    dispose() {
        this.disposed = true;
    }
    constructor() {
        Addons.push(this);
    }
}

vi.mock("@xterm/addon-webgl", () => ({ WebglAddon: FakeWebglAddon }));
vi.mock("@/app/store/jotaiStore", () => ({ globalStore: { get: () => null, set: () => {} } }));
vi.mock("@/app/store/badge", () => ({ setBadge: () => {} }));
vi.mock("@/app/store/wps", () => ({ getFileSubject: () => ({}) }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/store/global", () => ({
    fetchWaveFile: async () => ({}),
    getApi: () => ({}),
    getOverrideConfigAtom: () => ({}),
    getSettingsKeyAtom: () => ({}),
    globalStore: { get: () => null, set: () => {} },
    openLink: () => {},
    recordTEvent: () => {},
    WOS: {},
}));
vi.mock("@/store/services", () => ({}));
vi.mock("@/store/wos", () => ({ makeORef: () => "" }));

let TermWrapClass: any;

beforeAll(async () => {
    (globalThis as any).document = {
        createElement: () => ({ getContext: () => ({}) }),
    };
    TermWrapClass = (await import("./termwrap")).TermWrap;
});

function bareTermWrap() {
    const tw = Object.create(TermWrapClass.prototype);
    const scheduled: string[] = [];
    tw.webglAddon = null;
    tw.webglContextLossDisposable = null;
    tw.webglEnabledAtom = {};
    tw.terminal = { loadAddon: () => {} };
    tw.redrawScheduler = { schedule: async () => void scheduled.push("schedule") };
    return { tw, scheduled };
}

describe("TermWrap WebGL context loss", () => {
    test("switches to the DOM renderer and schedules a repaint", () => {
        const { tw, scheduled } = bareTermWrap();
        tw.setTermRenderer("webgl");
        expect(tw.getTermRenderer()).toBe("webgl");
        const addon = Addons[Addons.length - 1];
        expect(scheduled).toEqual([]);

        addon.lossCallback();

        expect(tw.getTermRenderer()).toBe("dom");
        expect(addon.disposed).toBe(true);
        expect(scheduled).toEqual(["schedule"]);
    });
});
