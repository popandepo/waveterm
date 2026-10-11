// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, describe, test } from "vitest";
import { makeStripTabMenuInfo } from "../lib/stacktabmenu";

function setup(answer: boolean | undefined) {
    const calls: string[] = [];
    const asked: string[] = [];
    const deps = {
        closeAfterConfirm:
            answer === undefined
                ? undefined
                : async (blockId: string, close: () => Promise<void> | void) => {
                      asked.push(blockId);
                      if (!answer) {
                          return false;
                      }
                      await close();
                      return true;
                  },
        closeNode: (id: string) => void calls.push("close:" + id),
        takeNodeOutOfStack: (id: string) => void calls.push("takeout:" + id),
    };
    return { calls, asked, deps };
}

describe("the Close of the right-click menu of a strip tab", () => {
    test("goes through the close question: refused closes nothing, accepted closes that node", async () => {
        const no = setup(false);
        const refused = await makeStripTabMenuInfo(no.deps, { blockId: "b1", nodeId: "n1" }, 2).close();
        assert.isFalse(refused);
        assert.deepEqual(no.asked, ["b1"]);
        assert.deepEqual(no.calls, []);
        const yes = setup(true);
        assert.isTrue(await makeStripTabMenuInfo(yes.deps, { blockId: "b1", nodeId: "n1" }, 2).close());
        assert.deepEqual(yes.calls, ["close:n1"]);
    });

    test("without a question set up nothing is closed", async () => {
        const k = setup(undefined);
        assert.isFalse(await makeStripTabMenuInfo(k.deps, { blockId: "b1", nodeId: "n1" }, 2).close());
        assert.deepEqual(k.calls, []);
    });

    test("take out is offered with more than one member and takes out that node", () => {
        const k = setup(true);
        const one = makeStripTabMenuInfo(k.deps, { blockId: "b1", nodeId: "n1" }, 1);
        assert.isFalse(one.canTakeOut);
        const two = makeStripTabMenuInfo(k.deps, { blockId: "b1", nodeId: "n1" }, 2);
        assert.isTrue(two.canTakeOut);
        two.takeOut();
        assert.deepEqual(k.calls, ["takeout:n1"]);
    });
});
