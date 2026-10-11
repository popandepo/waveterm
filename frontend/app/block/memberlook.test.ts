// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { buildMemberLookItems, buildStripTabMenu, memberLookMeta, MemberLookDeps } from "./memberlook";

function fakeDeps(answers: (string | null)[] = [], gone = new Set<string>()) {
    const writes: { blockId: string; meta: { [key: string]: string | null } }[] = [];
    const asked: { title: string; label: string; initial: string }[] = [];
    const deps: MemberLookDeps = {
        exists: (blockId) => !gone.has(blockId),
        setMeta: async (blockId, meta) => {
            writes.push({ blockId, meta });
        },
        ask: async (opts) => {
            asked.push(opts);
            return answers.length > 0 ? answers.shift() : null;
        },
    };
    return { deps, writes, asked };
}

const flush = () => new Promise((r) => setTimeout(r, 0));
const find = (items: ContextMenuItem[], label: string) => items.find((i) => i.label === label);
const sub = (items: ContextMenuItem[], label: string) => find(items, label).submenu as ContextMenuItem[];

describe("what a menu choice writes", () => {
    test("a colour goes to icon:color, a symbol to frame:icon, a name to frame:title; empty clears", () => {
        assert.deepEqual(memberLookMeta("color", " #ff8800 "), { "icon:color": "#ff8800" });
        assert.deepEqual(memberLookMeta("icon", "rocket"), { "frame:icon": "rocket" });
        assert.deepEqual(memberLookMeta("icon", "\u{1F680}"), { "frame:icon": "\u{1F680}" });
        assert.deepEqual(memberLookMeta("title", "build"), { "frame:title": "build" });
        assert.deepEqual(memberLookMeta("color", ""), { "icon:color": null });
        assert.deepEqual(memberLookMeta("icon", "  "), { "frame:icon": null });
        assert.deepEqual(memberLookMeta("title", ""), { "frame:title": null });
    });

    test("what the strip would not draw is not written either", () => {
        assert.isNull(memberLookMeta("color", "url(http://x/a.png)"));
        assert.isNull(memberLookMeta("color", "var(--x)"));
        assert.isNull(memberLookMeta("icon", "\u{1F680}\u{1F680}\u{1F680}"));
        assert.isNull(memberLookMeta("icon", "a".repeat(41)));
        assert.isNull(memberLookMeta("icon", "<b>"));
    });
});

describe("Set colour and Set icon", () => {
    test("the colour list is None, the flag colours and Other; a flag colour is written to the block", async () => {
        const k = fakeDeps();
        const items = buildMemberLookItems("b1", { color: "#58C142" }, k.deps);
        const colors = sub(items, "Set colour");
        assert.equal(colors[0].label, "None");
        assert.isFalse(colors[0].checked);
        assert.isTrue(find(colors, "Green").checked);
        find(colors, "Red").click();
        await flush();
        assert.deepEqual(k.writes, [{ blockId: "b1", meta: { "icon:color": "#FF453A" } }]);
    });

    test("None clears the key", async () => {
        const k = fakeDeps();
        sub(buildMemberLookItems("b1", { color: "red" }, k.deps), "Set colour")[0].click();
        sub(buildMemberLookItems("b1", { icon: "star" }, k.deps), "Set icon")[0].click();
        await flush();
        assert.deepEqual(k.writes.map((w) => w.meta), [{ "icon:color": null }, { "frame:icon": null }]);
    });

    test("Other asks, writes an emoji that was typed, and a cancel writes nothing", async () => {
        const k = fakeDeps(["\u{1F680}"]);
        const other = find(sub(buildMemberLookItems("b1", {}, k.deps), "Set icon"), "Other (type or paste)...");
        other.click();
        await flush();
        assert.deepEqual(k.writes, [{ blockId: "b1", meta: { "frame:icon": "\u{1F680}" } }]);
        const c = fakeDeps([null]);
        find(sub(buildMemberLookItems("b1", {}, c.deps), "Set colour"), "Other...").click();
        await flush();
        assert.deepEqual(c.writes, []);
    });

    test("a refused text asks again with the text kept, and nothing is written until one is accepted", async () => {
        const k = fakeDeps(["url(x)", "tomato"]);
        find(sub(buildMemberLookItems("b1", {}, k.deps), "Set colour"), "Other...").click();
        await flush();
        assert.equal(k.asked.length, 2);
        assert.equal(k.asked[1].initial, "url(x)");
        assert.include(k.asked[1].label, "not accepted");
        assert.deepEqual(k.writes, [{ blockId: "b1", meta: { "icon:color": "tomato" } }]);
    });

    test("it stops asking after five refusals", async () => {
        const k = fakeDeps(Array(10).fill("url(x)"));
        find(sub(buildMemberLookItems("b1", {}, k.deps), "Set colour"), "Other...").click();
        await flush();
        assert.equal(k.asked.length, 5);
        assert.deepEqual(k.writes, []);
    });
});

describe("names and blocks that are gone", () => {
    test("a name with control characters or over 60 characters is refused, 60 passes", () => {
        assert.isNull(memberLookMeta("title", "a\nb"));
        assert.isNull(memberLookMeta("title", "a\u0000b"));
        assert.isNull(memberLookMeta("title", "a\u202eb"));
        assert.isNull(memberLookMeta("title", "x".repeat(61)));
        assert.deepEqual(memberLookMeta("title", "x".repeat(60)), { "frame:title": "x".repeat(60) });
    });

    test("nothing is written for a block that was deleted, before or after the question", async () => {
        const gone = new Set(["b1"]);
        const k = fakeDeps(["tomato"], gone);
        sub(buildMemberLookItems("b1", {}, k.deps), "Set colour")[1].click();
        find(sub(buildMemberLookItems("b1", {}, k.deps), "Set colour"), "Other...").click();
        await flush();
        assert.deepEqual(k.writes, []);
        const gone2 = new Set<string>();
        const k2 = fakeDeps(["tomato"], gone2);
        const other = find(sub(buildMemberLookItems("b2", {}, k2.deps), "Set colour"), "Other...");
        k2.deps.ask = async () => {
            gone2.add("b2");
            return "tomato";
        };
        other.click();
        await flush();
        assert.deepEqual(k2.writes, []);
    });
});

describe("the right-click menu of a strip tab", () => {
    const parts = (canTakeOut: boolean) => {
        const calls: string[] = [];
        return { calls, parts: { canTakeOut, takeOut: () => calls.push("takeout"), close: () => calls.push("close") } };
    };

    test("Take out of stack is there only when the stack has more than one member; Close goes through the given close", () => {
        const k = fakeDeps();
        const many = parts(true);
        const menu = buildStripTabMenu("b1", {}, many.parts, k.deps);
        find(menu, "Take out of stack").click();
        find(menu, "Close").click();
        assert.deepEqual(many.calls, ["takeout", "close"]);
        const one = parts(false);
        assert.isUndefined(find(buildStripTabMenu("b1", {}, one.parts, k.deps), "Take out of stack"));
    });

    test("Rename asks with the current name and writes frame:title; Set colour and Set icon are in the menu", async () => {
        const k = fakeDeps(["build"]);
        const menu = buildStripTabMenu("b1", { title: "old" }, parts(true).parts, k.deps);
        find(menu, "Rename").click();
        await flush();
        assert.equal(k.asked[0].initial, "old");
        assert.deepEqual(k.writes, [{ blockId: "b1", meta: { "frame:title": "build" } }]);
        assert.isDefined(find(menu, "Set colour"));
        assert.isDefined(find(menu, "Set icon"));
    });
});

describe("the wiring that no test can render", () => {
    const read = (...p: string[]) => readFileSync(path.join(__dirname, "..", "..", ...p), "utf8");

    test("the block gear menu, the strip tab and the window deps use these functions", () => {
        assert.include(read("app", "block", "blockframe-header.tsx"), "...buildMemberLookItems(");
        const strip = read("layout", "lib", "stacktabstrip.tsx");
        assert.include(strip, "onContextMenu={(e) => {");
        assert.include(strip, "layoutModel.onStackTabMenu(");
        assert.include(strip, "makeStripTabMenuInfo(");
        assert.include(read("app", "tab", "tabcontent.tsx"), "buildStripTabMenu(");
        const ui = read("app", "block", "memberlook-ui.ts");
        assert.include(ui, "RpcApi.SetMetaCommand(TabRpcClient");
        assert.include(ui, 'pushModal("TextPromptModal"');
        assert.include(read("app", "modals", "modalregistry.tsx"), "TextPromptModal");
    });
});
