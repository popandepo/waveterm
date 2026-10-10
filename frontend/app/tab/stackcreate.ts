// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export interface StackCreateEnv {
    findTargetNodeId: (blockId: string) => string | null;
    createBlock: () => Promise<string>;
    deleteBlock: (blockId: string) => Promise<void>;
    insert: (newBlockId: string, targetNodeId: string) => void;
}

// The tab can change between the click and the end of the create call. The target is checked before anything
// is created and again before the insert; a block that cannot be placed is deleted so none is left over.
export async function createIntoStack(targetBlockId: string, env: StackCreateEnv): Promise<string> {
    if (env.findTargetNodeId(targetBlockId) == null) {
        throw new Error(`the stack of block ${targetBlockId} is not in this tab any more`);
    }
    const newBlockId = await env.createBlock();
    const targetNodeId = env.findTargetNodeId(targetBlockId);
    if (targetNodeId == null) {
        await env.deleteBlock(newBlockId);
        throw new Error(`the stack of block ${targetBlockId} went away; the new block was removed again`);
    }
    env.insert(newBlockId, targetNodeId);
    return newBlockId;
}

export function entryStillAllowed(workspaceIdAtOpen: string | undefined, workspaceIdNow: string | undefined): boolean {
    return workspaceIdAtOpen === workspaceIdNow;
}
