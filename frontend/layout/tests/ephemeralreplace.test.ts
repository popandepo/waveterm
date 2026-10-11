// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { DiscardLedger, discardEphemeralBlock, replaceEphemeral } from "../lib/ephemeralreplace";

function setup(answer: boolean | undefined) {
    const log: string[] = [];
    const deps = {
        closeAfterConfirm:
            answer === undefined
                ? undefined
                : async (blockId: string, close: () => Promise<void> | void) => {
                      log.push("ask:" + blockId);
                      if (!answer) {
                          return false;
                      }
                      await close();
                      return true;
                  },
        closePrevious: () => void log.push("close-previous"),
        showNew: () => void log.push("show-new"),
        discardNew: () => (log.push("discard-new"), true),
    };
    return { log, deps };
}

describe("a new ephemeral block replacing the one on screen", () => {
    test("asks about the old block first, then closes it, then shows the new one", async () => {
        const k = setup(true);
        assert.deepEqual(await replaceEphemeral("old", k.deps), { shown: true, discarded: null });
        assert.deepEqual(k.log, ["ask:old", "close-previous", "show-new"]);
    });

    test("answered no: the old block stays and the new block is discarded, nothing is shown", async () => {
        const k = setup(false);
        assert.deepEqual(await replaceEphemeral("old", k.deps), { shown: false, discarded: true });
        assert.deepEqual(k.log, ["ask:old", "discard-new"]);
    });

    test("no question set up: the old block stays (a missing callback is not permission) and the new block is discarded", async () => {
        const k = setup(undefined);
        assert.deepEqual(await replaceEphemeral("old", k.deps), { shown: false, discarded: true });
        assert.deepEqual(k.log, ["discard-new"]);
    });
});

describe("a failing question or close", () => {
    test("keeps the old block and discards the new one", async () => {
        const k = setup(true);
        k.deps.closeAfterConfirm = async () => {
            throw new Error("dialog failed");
        };
        assert.deepEqual(await replaceEphemeral("old", k.deps), { shown: false, discarded: true });
        assert.deepEqual(k.log, ["discard-new"]);
    });
});

describe("a discard that fails", () => {
    test("is reported, not hidden: the caller sees discarded false", async () => {
        const k = setup(false);
        k.deps.discardNew = () => false;
        assert.deepEqual(await replaceEphemeral("old", k.deps), { shown: false, discarded: false });
    });

    test("the ledger keeps the id and tries again until the delete works", async () => {
        const ledger = new DiscardLedger();
        ledger.add("b1");
        ledger.add("b2");
        const deleted: string[] = [];
        let down = true;
        const del = async (d: { blockId: string }) => {
            if (down) throw new Error("down");
            deleted.push(d.blockId);
        };
        await ledger.retryAll(del);
        assert.deepEqual([...ledger.ids].sort(), ["b1", "b2"]);
        down = false;
        await ledger.retryAll(del);
        assert.deepEqual(deleted.sort(), ["b1", "b2"]);
        assert.equal(ledger.ids.size, 0);
    });
});

describe("deleting a block that is not shown", () => {
    test("deletes it", async () => {
        const deleted: string[] = [];
        assert.isTrue(await discardEphemeralBlock(async (d) => void deleted.push(d.blockId), "b9"));
        assert.deepEqual(deleted, ["b9"]);
    });

    test("a failure is retried once, a second failure reports false (and is logged)", async () => {
        let calls = 0;
        assert.isTrue(
            await discardEphemeralBlock(async () => {
                if (++calls === 1) throw new Error("db busy");
            }, "b9")
        );
        assert.equal(calls, 2);
        calls = 0;
        assert.isFalse(
            await discardEphemeralBlock(async () => {
                calls++;
                throw new Error("down");
            }, "b9")
        );
        assert.equal(calls, 2);
    });

    test("with no way to delete set up it reports false instead of pretending", async () => {
        assert.isFalse(await discardEphemeralBlock(undefined, "b9"));
    });
});

describe("the wiring that no test can render", () => {
    test("newEphemeralNode no longer closes the previous block directly", () => {
        const src = readFileSync(path.join(__dirname, "..", "lib", "layoutModel.ts"), "utf8");
        const body = src.slice(src.indexOf("    newEphemeralNode(blockId: string) {"), src.indexOf("    private showEphemeralNode("));
        assert.include(body, "replaceEphemeral(previous.data.blockId");
        assert.include(body, "closeAfterConfirm: this.closeBlockAfterConfirm");
        assert.equal(body.split("this.discardOrRemember(blockId)").length - 1, 2);
        assert.equal(body.split("discardEphemeralBlock(this.onNodeDelete, blockId)").length - 1, 1);
        const tabContent = readFileSync(path.join(__dirname, "..", "..", "app", "tab", "tabcontent.tsx"), "utf8");
        assert.include(tabContent, "services.ObjectService.DeleteBlock(data.blockId)");
        assert.include(tabContent, "onNodeDelete,");
        assert.include(body, "this.undiscardedEphemeralBlocks.add(blockId)");
        assert.include(body, "this.undiscardedEphemeralBlocks.retryAll(this.onNodeDelete)");
        const guard = body.indexOf("if (this.ephemeralReplaceOpen) {");
        const empty = body.indexOf("if (previous == null) {");
        assert.isAbove(guard, -1);
        assert.isBelow(guard, empty, "the replacement guard must come before the empty-slot path");
        assert.notInclude(body.replace(/closePrevious: \(\) => this\.closeNode\(previous\.id\)/, ""), "this.closeNode(");
    });
});
