// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { attentionClearDelayMs, AttentionClearAfterStayMs, AttentionClearAfterSwitchMs, clearsOnFocus, ClearAttentionMeta } from "../store/attentionclear";
import { stackTabAttention } from "./stacktablook";

describe("the mark on a member's strip tab", () => {
    test("info, need and urgent give their kind and the reason; nothing set gives no mark", () => {
        assert.deepEqual(stackTabAttention(undefined, undefined), { kind: "none", reason: "" });
        assert.deepEqual(stackTabAttention("info", undefined), { kind: "info", reason: "" });
        assert.deepEqual(stackTabAttention("info", "build done"), { kind: "info", reason: "build done" });
        assert.deepEqual(stackTabAttention("need", " waiting for you "), { kind: "need", reason: "waiting for you" });
        assert.deepEqual(stackTabAttention("urgent", "cannot go on without you"), { kind: "urgent", reason: "cannot go on without you" });
    });

    test("anything a program could write that does not pass is no mark: an orange level without a reason, a bad level, hidden characters, too long", () => {
        assert.equal(stackTabAttention("need", "").kind, "none");
        assert.equal(stackTabAttention("urgent", undefined).kind, "none");
        assert.equal(stackTabAttention("loud", "x").kind, "none");
        assert.equal(stackTabAttention("URGENT", "x").kind, "none");
        assert.equal(stackTabAttention("need", "a\nb").kind, "none");
        assert.equal(stackTabAttention("need", "x".repeat(121)).kind, "none");
        assert.equal(stackTabAttention(5, "x").kind, "none");
        assert.equal(stackTabAttention({}, {}).kind, "none");
    });
});

describe("when a mark clears by itself", () => {
    test("only the orange levels clear on focus", () => {
        assert.isTrue(clearsOnFocus("need"));
        assert.isTrue(clearsOnFocus("urgent"));
        assert.isFalse(clearsOnFocus("info"));
        assert.isFalse(clearsOnFocus(""));
        assert.isFalse(clearsOnFocus(undefined));
        assert.isFalse(clearsOnFocus("bogus"));
    });

    test("not while the window has no focus; half a second after the focus moved there, 3 seconds when it was already there", () => {
        assert.isNull(attentionClearDelayMs("urgent", false, true));
        assert.isNull(attentionClearDelayMs("need", false, false));
        assert.equal(attentionClearDelayMs("urgent", true, true), AttentionClearAfterSwitchMs);
        assert.equal(attentionClearDelayMs("need", true, false), AttentionClearAfterStayMs);
        assert.isNull(attentionClearDelayMs("info", true, true));
        assert.isNull(attentionClearDelayMs(undefined, true, true));
    });

    test("clearing writes null to both keys and nothing else", () => {
        assert.deepEqual(ClearAttentionMeta, { "attention:level": null, "attention:reason": null });
    });
});

describe("the wiring that no test can render", () => {
    const read = (...p: string[]) => readFileSync(path.join(__dirname, "..", "..", ...p), "utf8");

    test("the label reads the two keys and the badge, draws info as an icon, need as a dot, urgent as dot and ring, with the reason on hover", () => {
        const label = read("app", "tab", "stacktablabel.tsx");
        assert.include(label, 'getBlockMetaKeyAtom(blockId, "attention:level")');
        assert.include(label, 'getBlockMetaKeyAtom(blockId, "attention:reason")');
        assert.include(label, "getBlockBadgeAtom(blockId)");
        assert.include(label, "stackTabAttention(attentionLevel, attentionReason)");
        assert.include(label, "stack-tab-attention-info");
        assert.include(label, "stack-tab-attention-dot");
        assert.include(label, "stack-tab-attention-ring");
        assert.include(label, "title={attention.reason}");
        // the reason is shown as text (a React attribute), never as markup
        assert.notInclude(label, "dangerouslySetInnerHTML");
        // the ring only for urgent, never for need
        assert.include(label, 'attention.kind === "urgent" && <span className="stack-tab-attention-ring"');
    });

    test("the stylesheet pulses only the ring, stops the pulse for reduced motion, and keeps its own colour separate from the member's line", () => {
        const css = read("layout", "lib", "stackstrip.scss");
        assert.include(css, "--attention-color: #ff9500;");
        assert.include(css, "animation: stack-tab-attention-pulse 1.6s ease-in-out infinite;");
        assert.include(css, "@media (prefers-reduced-motion: reduce)");
        const reduced = css.slice(css.indexOf("@media (prefers-reduced-motion: reduce)"));
        assert.include(reduced.slice(0, 200), "animation: none;");
        // the dot has no animation of its own: need is a steady dot
        const dot = css.slice(css.indexOf(".stack-tab-attention-dot {"), css.indexOf(".stack-tab-attention-ring {"));
        assert.notInclude(dot, "animation");
        assert.notInclude(css.slice(css.indexOf(".stack-tab-line {"), css.indexOf(".stack-tab-fill {")), "attention");
    });

    test("the app clears the focused block's orange mark through the clear rule, with both keys null", () => {
        const app = read("app", "app.tsx");
        assert.include(app, "<AttentionAutoClearing />");
        assert.include(app, "scheduleAttentionClear(");
        assert.include(app, "{ blockId: focusedBlockId, level, reason, documentHasFocus, focusSwitched }");
        // the plan is made again when ANY of these changes, the reason included
        assert.include(app, "}, [focusedBlockId, level, reason, documentHasFocus]);");
        assert.include(app, "meta: ClearAttentionMeta");
        assert.include(app, 'WOS.makeORef("block", blockId)');
        assert.include(app, 'getBlockMetaKeyAtom(blockId, "attention:reason")');
        // it checks the window still has focus and the tile is still the focused one when the timer fires
        const body = app.slice(app.indexOf("const AttentionAutoClearing"), app.indexOf("const AppInner"));
        assert.include(body, "documentHasFocus: () => document.hasFocus()");
        assert.include(body, "layoutModel.focusedNode");
    });
});
