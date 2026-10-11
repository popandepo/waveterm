// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { globalStore } from "@/app/store/jotaiStore";
import * as WOS from "@/app/store/wos";
import { makeORef } from "@/app/store/wos";
import { modalsModel } from "@/store/modalmodel";
import type { MemberLookDeps } from "./memberlook";

// The deps of the member look menus in a real window: meta goes to the block through the RPC, and a question is the text prompt modal.
export const MemberLookWindowDeps: MemberLookDeps = {
    exists: (blockId) => globalStore.get(WOS.getWaveObjectAtom<Block>(makeORef("block", blockId))) != null,
    setMeta: (blockId, meta) => RpcApi.SetMetaCommand(TabRpcClient, { oref: makeORef("block", blockId), meta }),
    ask: (opts) =>
        new Promise<string | null>((resolve) => {
            modalsModel.pushModal("TextPromptModal", { ...opts, resolve });
        }),
};
