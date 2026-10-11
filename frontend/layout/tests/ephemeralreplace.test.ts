// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { replaceEphemeral } from "../lib/ephemeralreplace";

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
        discardNew: () => void log.push("discard-new"),
    };
    return { log, deps };
}

describe("a new ephemeral block replacing the one on screen", () => {
    test("asks about the old block first, then closes it, then shows the new one", async () => {
        const k = setup(true);
        assert.isTrue(await replaceEphemeral("old", k.deps));
        assert.deepEqual(k.log, ["ask:old", "close-previous", "show-new"]);
    });

    test("answered no: the old block stays and the new block is discarded, nothing is shown", async () => {
        const k = setup(false);
        assert.isFalse(await replaceEphemeral("old", k.deps));
        assert.deepEqual(k.log, ["ask:old", "discard-new"]);
    });

    test("no question set up: the old block stays (a missing callback is not permission) and the new block is discarded", async () => {
        const k = setup(undefined);
        assert.isFalse(await replaceEphemeral("old", k.deps));
        assert.deepEqual(k.log, ["discard-new"]);
    });
});

describe("the wiring that no test can render", () => {
    test("newEphemeralNode no longer closes the previous block directly", () => {
        const src = readFileSync(path.join(__dirname, "..", "lib", "layoutModel.ts"), "utf8");
        const body = src.slice(src.indexOf("    newEphemeralNode(blockId: string) {"), src.indexOf("    private showEphemeralNode("));
        assert.include(body, "replaceEphemeral(previous.data.blockId");
        assert.include(body, "closeAfterConfirm: this.closeBlockAfterConfirm");
        assert.notInclude(body.replace(/closePrevious: \(\) => this\.closeNode\(previous\.id\)/, ""), "this.closeNode(");
    });
});
