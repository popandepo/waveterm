// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
// Stands in for emain/preload.ts: window.api over chrome.webview. The host prepends window.__waveConst.
(function () {
    const C = window.__waveConst || {};
    const listeners = {};
    const pending = new Map();
    let nextId = 1;
    const post = (m) => window.chrome.webview.postMessage(m);
    const send = (ch) => (...args) => post({ t: "send", ch, args });
    const invoke = (ch) => (...args) =>
        new Promise((resolve) => {
            const id = nextId++;
            pending.set(id, resolve);
            post({ t: "invoke", id, ch, args });
        });
    const on = (ch, adapt) => (cb) => {
        (listeners[ch] = listeners[ch] || []).push((args) => cb(...adapt(args)));
    };
    const first = (a) => [a[0]];
    const all = (a) => a;
    window.chrome.webview.addEventListener("message", (e) => {
        const m = e.data;
        if (m.t === "res") {
            const r = pending.get(m.id);
            pending.delete(m.id);
            r && r(m.val);
            return;
        }
        if (m.t === "ev") {
            for (const fn of listeners[m.ch] || []) fn(m.args || []);
        }
    });
    const sync = (k) => () => (k in C ? C[k] : null);
    window.api = {
        getAuthKey: sync("authKey"),
        getIsDev: sync("isDev"),
        getPlatform: sync("platform"),
        getCursorPoint: () => null,
        getUserName: sync("userName"),
        getHostName: sync("hostName"),
        getDataDir: sync("dataDir"),
        getConfigDir: sync("configDir"),
        getHomeDir: sync("homeDir"),
        getAboutModalDetails: sync("about"),
        getWebviewPreload: sync("webviewPreload"),
        getZoomFactor: () => 1,
        getUpdaterStatus: sync("updaterStatus"),
        getUpdaterChannel: sync("updaterChannel"),
        getEnv: (n) => (C.env && n in C.env ? C.env[n] : null),
        getPathForFile: () => "",
        openNewWindow: send("open-new-window"),
        showWorkspaceAppMenu: send("workspace-appmenu-show"),
        showBuilderAppMenu: send("builder-appmenu-show"),
        showContextMenu: send("contextmenu-show"),
        downloadFile: (filePath) => post({ t: "send", ch: "download", args: [{ filePath }] }),
        openExternal: send("open-external"),
        installAppUpdate: send("install-app-update"),
        updateWindowControlsOverlay: send("update-window-controls-overlay"),
        setWebviewFocus: send("webview-focus"),
        registerGlobalWebviewKeys: send("register-global-webview-keys"),
        createWorkspace: send("create-workspace"),
        switchWorkspace: send("switch-workspace"),
        deleteWorkspace: send("delete-workspace"),
        setActiveTab: send("set-active-tab"),
        createTab: send("create-tab"),
        setWindowInitStatus: send("set-window-init-status"),
        sendLog: send("fe-log"),
        onQuicklook: send("quicklook"),
        openNativePath: send("open-native-path"),
        setKeyboardChordMode: send("set-keyboard-chord-mode"),
        setWaveAIOpen: send("set-waveai-open"),
        closeBuilderWindow: send("close-builder-window"),
        incrementTermCommands: send("increment-term-commands"),
        nativePaste: send("native-paste"),
        openBuilder: send("open-builder"),
        setBuilderWindowAppId: send("set-builder-window-appid"),
        doRefresh: send("do-refresh"),
        closeTab: invoke("close-tab"),
        confirmCloseBlock: invoke("confirm-close-block"),
        captureScreenshot: invoke("capture-screenshot"),
        clearWebviewStorage: invoke("clear-webview-storage"),
        saveTextFile: invoke("save-text-file"),
        setIsActive: invoke("set-is-active"),
        onContextMenuClick: on("contextmenu-click", first),
        onFullScreenChange: on("fullscreen-change", first),
        onZoomFactorChange: on("zoom-factor-change", first),
        onUpdaterStatusChange: on("app-update-status", first),
        onMenuItemAbout: on("menu-item-about", all),
        onReinjectKey: on("reinject-key", first),
        onControlShiftStateUpdate: on("control-shift-state-update", first),
        onWaveInit: on("wave-init", first),
        onBuilderInit: on("builder-init", first),
    };
})();
