// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { attentionOf, attentionRank, checkAttention, highestAttention } from "./attention";

const vectors = JSON.parse(readFileSync(path.join(__dirname, "..", "..", "..", "pkg", "attention", "testdata", "vectors.json"), "utf8"));

describe("the shared vectors (the same file the Go tests read)", () => {
    test("the file loaded", () => {
        assert.isAbove(vectors.check.length, 10);
        assert.isAbove(vectors.highest.length, 5);
    });

    test("checkAttention agrees with Go on every case", () => {
        for (const c of vectors.check) {
            const reason = c.repeat_to ? "x".repeat(c.repeat_to) : c.reason;
            const got = checkAttention(c.level, reason);
            assert.equal(got != null, c.ok, JSON.stringify(c));
            if (c.ok && !c.repeat_to) {
                assert.equal(got, c.reason_out, JSON.stringify(c));
            }
        }
    });

    test("highestAttention agrees with Go on every case", () => {
        for (const c of vectors.highest) {
            assert.equal(highestAttention(c.levels), c.want, JSON.stringify(c));
        }
    });
});

describe("reading a block's meta", () => {
    test("a valid mark is read, a program that writes an orange level without a reason gets no mark", () => {
        assert.deepEqual(attentionOf("urgent", " why "), { level: "urgent", reason: "why" });
        assert.deepEqual(attentionOf("urgent", ""), { level: "", reason: "" });
        assert.deepEqual(attentionOf("need", undefined), { level: "", reason: "" });
        assert.deepEqual(attentionOf("info", undefined), { level: "info", reason: "" });
    });

    test("values that are not strings, and names of object properties, are no level", () => {
        assert.equal(attentionRank(5), 0);
        assert.equal(attentionRank(null), 0);
        assert.equal(attentionRank("toString"), 0);
        assert.equal(attentionRank("__proto__"), 0);
        assert.deepEqual(attentionOf({}, "x"), { level: "", reason: "" });
        assert.equal(highestAttention(["constructor", "info"]), "info");
    });

    test("the order is none < info < need < urgent", () => {
        assert.isBelow(attentionRank("info"), attentionRank("need"));
        assert.isBelow(attentionRank("need"), attentionRank("urgent"));
        assert.isBelow(attentionRank(""), attentionRank("info"));
    });
});
