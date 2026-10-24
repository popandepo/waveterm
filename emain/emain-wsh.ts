// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { WindowService } from "@/app/store/services";
import { RpcResponseHelper, WshClient } from "@/app/store/wshclient";
import { RpcApi } from "@/app/store/wshclientapi";
import { Notification, net, safeStorage, session, shell } from "electron";
import { getResolvedUpdateChannel } from "emain/updater";
import { wshCloseRefusal } from "./closetabprompt";
import { unamePlatform } from "./emain-platform";
import { getWebContentsByBlockId, webGetSelector } from "./emain-web";
import { blockShot } from "./emain-blockshot";
import { authorizeWebNav, AuthorizeTimeoutMs, FullBlockIdRe, readWebTile, WebReadDeps, webNavigate, webShot, webText } from "./emain-webnav";
import { createBrowserWindow, getWaveWindowById, getWaveWindowByWorkspaceId } from "./emain-window";

// What readWebTile needs from the window, the store and Electron, for one request.
function webReadDeps(rh: RpcResponseHelper, data: { blockid: string; tabid: string; workspaceid: string }): WebReadDeps {
    return {
        authorize: async () => {
            await RpcApi.WebNavAuthorizeCommand(ElectronWshClient, { callerroute: rh.getSource(), blockid: data.blockid }, { timeout: AuthorizeTimeoutMs });
        },
        getWindow: () => getWaveWindowByWorkspaceId(data.workspaceid),
        blockInfo: () => RpcApi.BlockInfoCommand(ElectronWshClient, data.blockid),
        getContents: (ww) => getWebContentsByBlockId(ww, data.tabid, data.blockid),
        sessionFor: (partition) => session.fromPartition(partition),
    };
}

export class ElectronWshClientType extends WshClient {
    constructor() {
        super("electron");
    }

    async handle_webselector(rh: RpcResponseHelper, data: CommandWebSelectorData): Promise<string[]> {
        // wsh web get reads the page of a tile: the same order and checks as wsh web text and wsh web shot (readWebTile), the live session included
        return await readWebTile(data, webReadDeps(rh, data), (wc) => webGetSelector(wc, data.selector, data.opts));
    }

    async handle_webnav(rh: RpcResponseHelper, data: CommandWebNavData): Promise<CommandWebNavRtnData> {
        if (!data.tabid || !data.blockid || !data.workspaceid) {
            throw new Error("tabid, blockid and workspaceid are required");
        }
        if (!FullBlockIdRe.test(data.blockid)) {
            throw new Error("blockid must be a full block id");
        }
        // the caller is the block behind the source route that the router stamped on this message (a session cannot choose it): it may act only on a
        // web tile in its own workspace. Asked before any window, tab or web contents is looked at.
        await RpcApi.WebNavAuthorizeCommand(ElectronWshClient, { callerroute: rh.getSource(), blockid: data.blockid }, { timeout: AuthorizeTimeoutMs });
        const ww = getWaveWindowByWorkspaceId(data.workspaceid);
        if (ww == null) {
            throw new Error(`no window found with workspace ${data.workspaceid}`);
        }
        if (!ww.allLoadedTabViews?.has(data.tabid)) {
            throw new Error(`the tab ${data.tabid} is not loaded in its window (show it once, then ask again)`);
        }
        // none of the ids is taken on trust: the block must be the one named, in the tab and the workspace named, and a web tile
        authorizeWebNav(data, await RpcApi.BlockInfoCommand(ElectronWshClient, data.blockid));
        const wc = await getWebContentsByBlockId(ww, data.tabid, data.blockid);
        if (wc == null) {
            throw new Error(`no web contents found for block ${data.blockid} (is its page ready?)`);
        }
        // the same question again right before acting (a belt: no code moves a block to another tab or workspace today, but the lookups above take time)
        await RpcApi.WebNavAuthorizeCommand(ElectronWshClient, { callerroute: rh.getSource(), blockid: data.blockid }, { timeout: AuthorizeTimeoutMs });
        return webNavigate(wc, data.action);
    }

    async handle_webtext(rh: RpcResponseHelper, data: CommandWebTextData): Promise<CommandWebTextRtnData> {
        // the order and the checks are in readWebTile: the caller, the record and its partition rule, the page, then the caller and the live session again
        return await readWebTile(data, webReadDeps(rh, data), (wc) => webText(wc, data.selector, data.maxbytes));
    }

    async handle_webshot(rh: RpcResponseHelper, data: CommandWebShotData): Promise<CommandWebShotRtnData> {
        // the same order and checks as handle_webtext (readWebTile): the live session is checked right before capturePage
        return await readWebTile(data, webReadDeps(rh, data), (wc) => webShot(wc));
    }

    async handle_blockshot(rh: RpcResponseHelper, data: CommandBlockShotData): Promise<CommandBlockShotRtnData> {
        // the caller is asked before any window or page is looked at; nothing here shows, moves or focuses a view
        return await blockShot(data, {
            authorize: async () => {
                await RpcApi.WebNavAuthorizeCommand(ElectronWshClient, { callerroute: rh.getSource(), blockid: data.blockid }, { timeout: AuthorizeTimeoutMs });
            },
            blockInfo: () => RpcApi.BlockInfoCommand(ElectronWshClient, data.blockid),
            getWindow: () => getWaveWindowByWorkspaceId(data.workspaceid),
        });
    }

    async handle_notify(rh: RpcResponseHelper, notificationOptions: WaveNotificationOptions) {
        new Notification({
            title: notificationOptions.title,
            body: notificationOptions.body,
            silent: notificationOptions.silent,
        }).show();
    }

    async handle_getupdatechannel(rh: RpcResponseHelper): Promise<string> {
        return getResolvedUpdateChannel();
    }

    async handle_focuswindow(rh: RpcResponseHelper, windowId: string) {
        console.log(`focuswindow ${windowId}`);
        const fullConfig = await RpcApi.GetFullConfigCommand(ElectronWshClient);
        let ww = getWaveWindowById(windowId);
        if (ww == null) {
            const window = await WindowService.GetWindow(windowId);
            if (window == null) {
                throw new Error(`window ${windowId} not found`);
            }
            ww = await createBrowserWindow(window, fullConfig, {
                unamePlatform,
                isPrimaryStartupWindow: false,
            });
        }
        ww.focus();
    }

    async handle_electronencrypt(
        rh: RpcResponseHelper,
        data: CommandElectronEncryptData
    ): Promise<CommandElectronEncryptRtnData> {
        if (!safeStorage.isEncryptionAvailable()) {
            throw new Error("encryption is not available");
        }
        const encrypted = safeStorage.encryptString(data.plaintext);
        const ciphertext = encrypted.toString("base64");

        let storagebackend = "";
        if (process.platform === "linux") {
            storagebackend = safeStorage.getSelectedStorageBackend();
        }

        return {
            ciphertext,
            storagebackend,
        };
    }

    async handle_electrondecrypt(
        rh: RpcResponseHelper,
        data: CommandElectronDecryptData
    ): Promise<CommandElectronDecryptRtnData> {
        if (!safeStorage.isEncryptionAvailable()) {
            throw new Error("encryption is not available");
        }
        const encrypted = Buffer.from(data.ciphertext, "base64");
        const plaintext = safeStorage.decryptString(encrypted);

        let storagebackend = "";
        if (process.platform === "linux") {
            storagebackend = safeStorage.getSelectedStorageBackend();
        }

        return {
            plaintext,
            storagebackend,
        };
    }

    async handle_networkonline(rh: RpcResponseHelper): Promise<boolean> {
        return net.isOnline();
    }

    async handle_electronsystembell(rh: RpcResponseHelper): Promise<void> {
        shell.beep();
    }

    async handle_electronsetactivetab(rh: RpcResponseHelper, data: CommandElectronTabData): Promise<void> {
        const ww = getWaveWindowByWorkspaceId(data.workspaceid);
        if (ww == null) {
            throw new Error(`no window found with workspace ${data.workspaceid}`);
        }
        await ww.setActiveTab(data.tabid, true);
    }

    async handle_electronclosetab(rh: RpcResponseHelper, data: CommandElectronTabData): Promise<void> {
        const ww = getWaveWindowByWorkspaceId(data.workspaceid);
        if (ww == null) {
            throw new Error(`no window found with workspace ${data.workspaceid}`);
        }
        // force is the deliberate bypass for every local RPC caller, not only for `wsh tab close --force`: any local caller
        // that can reach this call can already delete blocks, so the check is a guard against mistakes, not a security
        // boundary. No UI route can pass it (the IPC close route has no such parameter; see closetabhandler.test.ts).
        let approvedCount = -1;
        if (!data.force) {
            let count: number | null = null;
            try {
                const tab = await RpcApi.GetTabCommand(ElectronWshClient, data.tabid, { timeout: 3000 });
                count = tab?.blockids?.length ?? null;
            } catch (e) {
                console.log("electronclosetab: could not read the tab", e);
            }
            const refusal = wshCloseRefusal(count, false);
            if (refusal) {
                throw new Error(refusal);
            }
            approvedCount = count ?? -1;
        }
        const closed = await ww.queueCloseTab(data.tabid, data.force ? -1 : approvedCount);
        if (!closed) {
            throw new Error("the tab was not closed (it gained blocks since it was checked, or the window refused)");
        }
    }

    async handle_electrontabclosed(rh: RpcResponseHelper, data: CommandElectronTabClosedData): Promise<void> {
        const ww = getWaveWindowByWorkspaceId(data.workspaceid);
        if (ww == null) {
            return;
        }
        await ww.tabClosedInBackend(data.tabid, data.newactivetabid, data.closewindow);
    }

    async handle_electronpreloadtab(rh: RpcResponseHelper, data: CommandElectronTabData): Promise<void> {
        const ww = getWaveWindowByWorkspaceId(data.workspaceid);
        if (ww == null) {
            throw new Error(`no window found with workspace ${data.workspaceid}`);
        }
        await ww.preloadTab(data.tabid);
    }

    // async handle_workspaceupdate(rh: RpcResponseHelper) {
    //     console.log("workspaceupdate");
    //     fireAndForget(async () => {
    //         console.log("workspace menu clicked");
    //         const updatedWorkspaceMenu = await getWorkspaceMenu();
    //         const workspaceMenu = Menu.getApplicationMenu().getMenuItemById("workspace-menu");
    //         workspaceMenu.submenu = Menu.buildFromTemplate(updatedWorkspaceMenu);
    //     });
    // }
}

export let ElectronWshClient: ElectronWshClientType;

export function initElectronWshClient() {
    ElectronWshClient = new ElectronWshClientType();
}
