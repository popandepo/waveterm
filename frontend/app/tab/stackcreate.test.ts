// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, test } from "vitest";
import { createIntoStack, entryStillAllowed } from "./stackcreate";

function fakeEnv(opts: { targetNodeBefore?: string | null; targetNodeAfter?: string | null; createFails?: boolean }) {
    const calls: string[] = [];
    const lookups = [opts.targetNodeBefore ?? null, opts.targetNodeAfter ?? null];
    let lookup = 0;
    return {
        calls,
        env: {
            findTargetNodeId: (_blockId: string) => {
                calls.push("find");
                return lookups[Math.min(lookup++, 1)];
            },
            createBlock: async () => {
                calls.push("create");
                if (opts.createFails) {
                    throw new Error("create failed");
                }
                return "new-block";
            },
            deleteBlock: async (blockId: string) => {
                calls.push("delete:" + blockId);
            },
            insert: (newBlockId: string, targetNodeId: string) => {
                calls.push(`insert:${newBlockId}:${targetNodeId}`);
            },
        },
    };
}

describe("creating a block into a stack", () => {
    test("a missing target stops before anything is created", async () => {
        const { calls, env } = fakeEnv({ targetNodeBefore: null });
        await expect(createIntoStack("tgt", env)).rejects.toThrow();
        expect(calls).toEqual(["find"]);
    });

    test("a target that is gone after the block was created removes the new block again", async () => {
        const { calls, env } = fakeEnv({ targetNodeBefore: "n1", targetNodeAfter: null });
        await expect(createIntoStack("tgt", env)).rejects.toThrow();
        expect(calls).toEqual(["find", "create", "find", "delete:new-block"]);
    });

    test("the normal path creates once and inserts next to the target", async () => {
        const { calls, env } = fakeEnv({ targetNodeBefore: "n1", targetNodeAfter: "n1" });
        expect(await createIntoStack("tgt", env)).toBe("new-block");
        expect(calls).toEqual(["find", "create", "find", "insert:new-block:n1"]);
    });

    test("a failed create leaves nothing to clean up", async () => {
        const { calls, env } = fakeEnv({ targetNodeBefore: "n1", createFails: true });
        await expect(createIntoStack("tgt", env)).rejects.toThrow("create failed");
        expect(calls).toEqual(["find", "create"]);
    });
});

describe("the + menu entry is re-checked at selection", () => {
    test("the same workspace is allowed, another one is not", () => {
        expect(entryStillAllowed("ws-1", "ws-1")).toBe(true);
        expect(entryStillAllowed("ws-1", "ws-2")).toBe(false);
        expect(entryStillAllowed(undefined, undefined)).toBe(true);
    });
});
