// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getApi } from "@/app/store/global";
import { BlockService } from "@/app/store/services";

export const RunningBlockQuestion = "This closes the program running in it. It cannot be undone.";
export const UnknownBlockQuestion =
    "Wave could not tell whether a program is running in this block. Closing it stops any program in it and cannot be undone.";

// Closing one block (a stack member's X, the header menu, the close key) stops its program, and until now nothing asked. It asks
// only when a program is running: "running" asks, a block that has no program (a web tile, a file view; the status is empty) or whose
// program has ended or not started does not. When the state could not be read it asks, because asking too often is better than
// losing a session.
export function closeBlockQuestion(shellProcStatus: string | null | undefined, readFailed: boolean): string | null {
    if (readFailed) {
        return UnknownBlockQuestion;
    }
    if (shellProcStatus === "running") {
        return RunningBlockQuestion;
    }
    return null;
}

export interface ConfirmCloseDeps {
    readStatus: (blockId: string) => Promise<{ shellprocstatus?: string } | null | undefined>;
    ask: (message: string) => Promise<boolean>;
}

const DefaultDeps: ConfirmCloseDeps = {
    readStatus: (blockId) => BlockService.GetControllerStatus(blockId),
    ask: (message) => getApi().confirmCloseBlock(message),
};

// Resolves with whether the block may be closed now.
export async function confirmCloseBlock(blockId: string, deps: ConfirmCloseDeps = DefaultDeps): Promise<boolean> {
    let status: string | null | undefined = null;
    let readFailed = false;
    try {
        status = (await deps.readStatus(blockId))?.shellprocstatus;
    } catch (e) {
        console.log("confirm close block: could not read the state of the program", blockId, e);
        readFailed = true;
    }
    const question = closeBlockQuestion(status, readFailed);
    if (question == null) {
        return true;
    }
    try {
        return await deps.ask(question);
    } catch (e) {
        console.log("confirm close block: the question could not be shown, keeping the block", e);
        return false;
    }
}
