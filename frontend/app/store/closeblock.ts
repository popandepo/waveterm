// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getApi, globalStore, WOS } from "@/app/store/global";
import { BlockService } from "@/app/store/services";

export const RunningBlockQuestion = "This closes the program running in it. It cannot be undone.";
export const UnknownBlockQuestion =
    "Wave could not tell whether a program is running in this block. Closing it stops any program in it and cannot be undone.";

// The views that run a program under a block controller. Every other view (web, preview, files, ...) has no program to stop.
const ProgramViews = new Set(["term", "tsunami", "vdom"]);

// The views that are known to have no program (the registered views of this build: see blockregistry.ts). A view name that is in
// neither list, such as one added by a later upstream version, asks until it is put here on purpose.
const NoProgramViews = new Set([
    "preview",
    "web",
    "waveai",
    "cpuplot",
    "sysinfo",
    "tips",
    "help",
    "launcher",
    "aifilediff",
    "waveconfig",
    "processviewer",
]);

// Closing one block (a stack member's X, the header menu, the close key) stops its program, and until now nothing asked.
// It is silent only when the answer is known: the block's view has no program, or the program is known to have ended (done) or not
// started (init). "running" asks, and so does everything else: a state that could not be read, a controller that is missing for a
// view that has programs, a state string this code does not know, a view that could not be read. Asking too often is better than
// losing a session.
export function closeBlockQuestion(
    view: string | null | undefined,
    shellProcStatus: string | null | undefined,
    readFailed: boolean
): string | null {
    if (readFailed || view == null || view === "") {
        return UnknownBlockQuestion;
    }
    if (NoProgramViews.has(view)) {
        return null;
    }
    if (!ProgramViews.has(view)) {
        return UnknownBlockQuestion;
    }
    if (shellProcStatus === "running") {
        return RunningBlockQuestion;
    }
    if (shellProcStatus === "done" || shellProcStatus === "init") {
        return null;
    }
    return UnknownBlockQuestion;
}

export interface ConfirmCloseDeps {
    readView: (blockId: string) => string | null | undefined;
    readStatus: (blockId: string) => Promise<{ shellprocstatus?: string } | null | undefined>;
    ask: (message: string) => Promise<boolean>;
}

const DefaultDeps: ConfirmCloseDeps = {
    readView: (blockId) =>
        globalStore.get(WOS.getWaveObjectAtom<Block>(WOS.makeORef("block", blockId)))?.meta?.view as string,
    readStatus: (blockId) => BlockService.GetControllerStatus(blockId),
    ask: (message) => getApi().confirmCloseBlock(message),
};

// One question per block at a time: a second click on the same X while the first question is open is ignored (it resolves false, so
// the caller does nothing), not another box.
const pendingQuestions = new Set<string>();

// Resolves with whether the block may be closed now.
export async function confirmCloseBlock(blockId: string, deps: ConfirmCloseDeps = DefaultDeps): Promise<boolean> {
    if (pendingQuestions.has(blockId)) {
        return false;
    }
    pendingQuestions.add(blockId);
    try {
        let view: string | null | undefined = null;
        let status: string | null | undefined = null;
        let readFailed = false;
        try {
            view = deps.readView(blockId);
            // only a view with programs has a controller to ask
            if (view != null && ProgramViews.has(view)) {
                status = (await deps.readStatus(blockId))?.shellprocstatus;
            }
        } catch (e) {
            console.log("confirm close block: could not read the state of the program", blockId, e);
            readFailed = true;
        }
        const question = closeBlockQuestion(view, status, readFailed);
        if (question == null) {
            return true;
        }
        try {
            return await deps.ask(question);
        } catch (e) {
            console.log("confirm close block: the question could not be shown, keeping the block", e);
            return false;
        }
    } finally {
        pendingQuestions.delete(blockId);
    }
}

// A close for which the block could not be identified at all (no focused node, no block id): nothing is known about what it would stop,
// so the question is always asked.
export async function closeUnidentifiedAfterConfirm(
    close: () => Promise<void> | void,
    ask: (message: string) => Promise<boolean> = DefaultDeps.ask
): Promise<boolean> {
    let yes = false;
    try {
        yes = await ask(UnknownBlockQuestion);
    } catch (e) {
        console.log("confirm close block: the question could not be shown, keeping the block", e);
    }
    if (!yes) {
        return false;
    }
    await close();
    return true;
}

// Closes a block after the question, if the answer is yes. `close` closes exactly the node that was chosen before the question (not
// "whatever is focused after it"), so that a focus change while the box is open cannot close another block unasked. Resolves with
// whether the block was closed; on Cancel nothing is touched (focus and the active member of a stack stay).
export async function closeBlockAfterConfirm(
    blockId: string,
    close: () => Promise<void> | void,
    deps: ConfirmCloseDeps = DefaultDeps
): Promise<boolean> {
    if (!(await confirmCloseBlock(blockId, deps))) {
        return false;
    }
    await close();
    return true;
}
