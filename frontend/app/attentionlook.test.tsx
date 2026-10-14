// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Attack and pin tests for the attention marks, the colours and the stylesheets of the strip tab, the top tab and the window outline (from the run-the-code
// review of PR 52; the tests that say "observation" pin behaviour that is stated in docs/ours/what-wave-can-do.md section 37, on purpose).

import { atom } from "jotai";
import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import * as sass from "sass";
import { afterEach, assert, beforeEach, describe, test, vi } from "vitest";

const meta: Record<string, any> = {};
let badge: any = null;

vi.mock("@/store/global", () => ({
    getBlockMetaKeyAtom: (_blockId: string, key: string) => atom(() => meta[key]),
}));
vi.mock("@/app/store/badge", async (importOriginal) => ({
    ...((await importOriginal()) as object),
    getBlockBadgeAtom: (_blockId: string) => atom(() => badge),
}));

import { ReducedMotionClass } from "@/app/reducedmotion";
import { ClearAttentionMeta } from "@/app/store/attentionclear";
import { AttentionClearEnv, scheduleAttentionClear } from "@/app/store/attentionschedule";
import { StackTabLabel } from "@/app/tab/stacktablabel";
import { badgeColorValue, memberColorValue, stackTabAttention } from "@/app/tab/stacktablook";
import { TabV } from "@/app/tab/tab";
import { ActiveFillPercent, InactiveLinePercent, topTabLook } from "@/app/tab/toptablook";
import { windowOutlineColor } from "@/app/windowoutline";

const cp = (...n: number[]) => String.fromCodePoint(...n);
const root = path.join(__dirname, "..");

function renderLabel(m: Record<string, any>, b: any = null): string {
    for (const k of Object.keys(meta)) {
        delete meta[k];
    }
    Object.assign(meta, m);
    badge = b;
    return renderToStaticMarkup(<StackTabLabel blockId="block-0001-aaaa" />);
}

function renderTab(flagColor: any, active: boolean): string {
    return renderToStaticMarkup(
        <TabV
            tabId="t1"
            tabName="name"
            active={active}
            showDivider={false}
            isDragging={false}
            tabWidth={130}
            isNew={false}
            flagColor={flagColor}
            onClick={() => null}
            onClose={() => null}
            onDragStart={() => null}
            onContextMenu={() => null}
            onRename={() => null}
        />
    );
}

describe("(a) a reason reaching markup", () => {
    test("markup in a reason is rendered as escaped attribute text and creates no element", () => {
        const evil = `"><img src=x onerror=alert(1)><script>x</script>`;
        const html = renderLabel({ view: "term", "attention:level": "need", "attention:reason": evil });
        assert.notInclude(html, "<img");
        assert.notInclude(html, "<script");
        assert.include(html, "&quot;&gt;&lt;img");
        assert.equal(html.split("<span").length - 1, 1, "only the dot span");
    });

    test("the three levels draw what the PR says: info icon only, need dot only, urgent dot and ring", () => {
        const info = renderLabel({ view: "term", "attention:level": "info", "attention:reason": "done" });
        assert.include(info, "stack-tab-attention-info");
        assert.notInclude(info, "stack-tab-attention-dot");
        assert.notInclude(info, "stack-tab-attention-ring");
        assert.include(info, 'title="done"');
        const need = renderLabel({ view: "term", "attention:level": "need", "attention:reason": "come" });
        assert.include(need, "stack-tab-attention-dot need");
        assert.notInclude(need, "stack-tab-attention-ring");
        assert.notInclude(need, "stack-tab-attention-info");
        assert.include(need, 'title="come"');
        const urgent = renderLabel({ view: "term", "attention:level": "urgent", "attention:reason": "now" });
        assert.include(urgent, "stack-tab-attention-dot urgent");
        assert.include(urgent, "stack-tab-attention-ring");
        assert.equal(urgent.split("stack-tab-attention-ring").length - 1, 1);
        assert.equal(urgent.split("stack-tab-attention-dot").length - 1, 1);
        const none = renderLabel({ view: "term" });
        assert.notInclude(none, "attention");
    });

    test("hidden, control, bidirectional and tag characters, and an over-long reason, draw no mark at all", () => {
        const bad = [
            "a" + cp(0x200b) + "b", // zero width space
            "a" + cp(0x202e) + "b", // right-to-left override
            "a" + cp(0x2066) + "b", // isolate
            "a" + cp(0x00ad) + "b", // soft hyphen
            "a" + cp(0x2028) + "b", // line separator
            "a" + cp(0x2029) + "b",
            "a" + cp(0x9) + "b",
            "a" + cp(0xd) + "b",
            "a" + cp(0x0) + "b",
            "a" + cp(0x7f) + "b",
            "a" + cp(0x85) + "b",
            "a" + cp(0xfeff) + "b",
            "a" + cp(0xe0041) + "b", // tag character
            cp(0x200b) + "trimmed away?",
            "x".repeat(121),
            cp(0x1f600).repeat(121),
            "x".repeat(100000),
        ];
        for (const reason of bad) {
            for (const level of ["need", "urgent", "info"]) {
                assert.equal(stackTabAttention(level, reason).kind, "none", JSON.stringify(reason).slice(0, 40));
                const html = renderLabel({ view: "term", "attention:level": level, "attention:reason": reason });
                assert.notInclude(html, "attention", level);
            }
        }
    });

    test("what still passes (observation, not a failure): 120 astral code points, combining marks, a lone surrogate, a no-break space", () => {
        assert.equal(stackTabAttention("need", cp(0x1f600).repeat(120)).kind, "need");
        assert.equal(stackTabAttention("need", "a" + cp(0x301).repeat(119)).kind, "need");
        assert.equal(stackTabAttention("need", "a" + cp(0xa0) + "b").kind, "need");
        // a lone surrogate is category Cs, which the hidden character class does not hold
        assert.equal(stackTabAttention("need", "a" + String.fromCharCode(0xd800) + "b").kind, "need");
    });

    test("a level that is not exactly one of the three, prototype names included, draws nothing", () => {
        for (const level of ["__proto__", "constructor", "toString", "hasOwnProperty", " need", "need ", "Need", ["need"], { toString: () => "need" }, 2, true, null]) {
            assert.equal(stackTabAttention(level, "x").kind, "none", String(level));
        }
    });
});

const BadColours: any[] = [
    "red;background:url(http://x/a.png)",
    "url(http://x/a.png)",
    "url(javascript:alert(1))",
    "expression(alert(1))",
    "rgb(0,0,0);",
    "rgb(0 0 0) url(x)",
    "rgb(var(--x))",
    "rgb(calc(1) 2 3)",
    "rgb(0,0,0)}body{display:none",
    "#fff /* x */",
    "#fff;",
    "#ffff00ff00",
    "var(--accent-color)",
    "env(x)",
    "attr(title)",
    "image-set(url(x) 1x)",
    "color-mix(in srgb, red, blue)",
    "color(display-p3 1 0 0)",
    "linear-gradient(red, blue)",
    "javascript:alert(1)",
    "\\72 ed",
    "re" + cp(0x200b) + "d",
    cp(0x202e) + "red",
    "re" + cp(0xa) + "d",
    "rgb(1," + cp(0x2028) + "2,3)",
    cp(0xff52) + cp(0xff45) + cp(0xff44), // fullwidth letters
    "r".repeat(31),
    "rgb(" + "1".repeat(70) + ")",
    "<b>red</b>",
    '"red"',
    "red blue",
    "-moz-initial",
    "--x",
    "",
    "   ",
    0,
    1,
    true,
    null,
    undefined,
    [],
    ["red"],
    { toString: () => "red" },
];

describe("(a) a colour reaching a style", () => {
    test("no bad value passes any of the four colour entrances", () => {
        for (const bad of BadColours) {
            const name = String(JSON.stringify(bad)).slice(0, 50);
            assert.isNull(memberColorValue(bad), "member " + name);
            assert.isNull(badgeColorValue(bad), "badge " + name);
            assert.deepEqual(topTabLook(bad, true), { fill: null, line: null }, "top active " + name);
            assert.deepEqual(topTabLook(bad, false), { fill: null, line: null }, "top inactive " + name);
            assert.isNull(windowOutlineColor(bad, bad), "outline " + name);
            assert.equal(windowOutlineColor(bad, "#123456"), "#123456", "outline falls through " + name);
        }
    });

    test("the rendered top tab carries no style for a bad colour, and exactly the two elements for a good one", () => {
        for (const bad of BadColours) {
            for (const active of [true, false]) {
                const html = renderTab(bad, active);
                assert.notInclude(html, "tab-colour-fill", String(bad));
                assert.notInclude(html, "tab-colour-line", String(bad));
            }
        }
        const on = renderTab("#ff8800", true);
        assert.equal(on.split("tab-colour-fill").length - 1, 1);
        assert.equal(on.split("tab-colour-line").length - 1, 1);
        assert.include(on, `background:color-mix(in srgb, #ff8800 ${ActiveFillPercent}%, transparent)`);
        assert.include(on, "background:#ff8800");
        const off = renderTab("#ff8800", false);
        assert.notInclude(off, "tab-colour-fill");
        assert.include(off, `background:color-mix(in srgb, #ff8800 ${InactiveLinePercent}%, transparent)`);
    });

    test("the rendered strip tab drops a bad badge colour and a bad member colour", () => {
        for (const bad of BadColours) {
            const html = renderLabel({ view: "term", "icon:color": bad }, { icon: "bell", color: bad, badgeid: "b", priority: 1 });
            assert.include(html, "stack-tab-badge");
            assert.notInclude(html, "style=", String(bad));
            assert.notInclude(html, "stack-tab-fill");
        }
    });

    test("observation: a line break or other white space at the END is trimmed away before the hidden character check; what is drawn is the clean value", () => {
        assert.equal(memberColorValue("red" + cp(0xa)), "red");
        assert.equal(memberColorValue(cp(0x2028) + "#fff" + cp(0xfeff)), "#fff");
        assert.isNull(memberColorValue(cp(0x202e) + "red"));
    });

    test("with a browser-like CSS.supports, the CSS-wide keywords pass (observation): they are valid 'color' values there", () => {
        const original = (globalThis as any).CSS;
        const wide = new Set(["inherit", "initial", "unset", "revert", "currentcolor", "transparent"]);
        (globalThis as any).CSS = { supports: (_p: string, v: string) => wide.has(v.toLowerCase()) || /^#[\da-f]{6}$/i.test(v) };
        try {
            assert.equal(windowOutlineColor("inherit", null), "inherit");
            assert.equal(topTabLook("inherit", true).fill, `color-mix(in srgb, inherit ${ActiveFillPercent}%, transparent)`);
            assert.equal(topTabLook("transparent", false).line, `color-mix(in srgb, transparent ${InactiveLinePercent}%, transparent)`);
            assert.isNull(windowOutlineColor("banana", null));
        } finally {
            (globalThis as any).CSS = original;
        }
    });
});

function fakeEnv(initial: { level?: unknown; reason?: unknown; focused?: string; docFocus?: boolean } = {}) {
    const state = { level: initial.level, reason: initial.reason, focused: initial.focused, docFocus: initial.docFocus ?? true };
    const cleared: string[] = [];
    const env: AttentionClearEnv = {
        documentHasFocus: () => state.docFocus,
        focusedBlockId: () => state.focused,
        read: () => ({ level: state.level, reason: state.reason }),
        clear: (blockId) => void cleared.push(blockId),
    };
    return { env, state, cleared };
}
const req = (o: Partial<Parameters<typeof scheduleAttentionClear>[0]> = {}) => ({
    blockId: "b1",
    level: "need" as unknown,
    reason: "why" as unknown,
    documentHasFocus: true,
    focusSwitched: true,
    ...o,
});

describe("(b) the clear", () => {
    beforeEach(() => void vi.useFakeTimers());
    afterEach(() => void vi.useRealTimers());

    test("after unmount (cancel) nothing is written, even when cancel is called twice or after the timer", () => {
        const f = fakeEnv({ level: "need", reason: "why", focused: "b1" });
        const cancel = scheduleAttentionClear(req(), f.env)!;
        cancel();
        cancel();
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(f.cleared, []);
        const g = fakeEnv({ level: "need", reason: "why", focused: "b1" });
        const cancel2 = scheduleAttentionClear(req(), g.env)!;
        vi.advanceTimersByTime(600);
        cancel2();
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(g.cleared, ["b1"]);
    });

    test("a strict-mode style double run (plan, cancel, plan) writes once", () => {
        const f = fakeEnv({ level: "urgent", reason: "why", focused: "b1" });
        const c1 = scheduleAttentionClear(req({ level: "urgent" }), f.env)!;
        c1();
        scheduleAttentionClear(req({ level: "urgent", focusSwitched: false }), f.env);
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(f.cleared, ["b1"]);
    });

    test("window focus lost at plan time or at fire time: no write, and it is not written later when focus returns", () => {
        const f = fakeEnv({ level: "need", reason: "why", focused: "b1", docFocus: false });
        assert.isUndefined(scheduleAttentionClear(req({ documentHasFocus: false }), f.env));
        // the atom said focus, the document says none when the timer fires
        scheduleAttentionClear(req(), f.env);
        vi.advanceTimersByTime(1000);
        f.state.docFocus = true;
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(f.cleared, []);
    });

    test("info and anything that is not need or urgent is never planned, whatever the timer state says", () => {
        const f = fakeEnv({ level: "need", reason: "why", focused: "b1" });
        for (const level of ["info", "", null, undefined, "NEED", "__proto__", "constructor", 3, { a: 1 }]) {
            assert.isUndefined(scheduleAttentionClear(req({ level }), f.env), String(level));
        }
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(f.cleared, []);
    });

    test("a block that lost the focus, was closed, or whose focus went to nothing is left alone", () => {
        for (const focused of ["b2", undefined, "", "B1"]) {
            const f = fakeEnv({ level: "need", reason: "why", focused: "b1" });
            scheduleAttentionClear(req(), f.env);
            f.state.focused = focused as any;
            vi.advanceTimersByTime(60_000);
            assert.deepEqual(f.cleared, [], String(focused));
        }
        const closed = fakeEnv({ level: "need", reason: "why", focused: "b1" });
        scheduleAttentionClear(req(), closed.env);
        closed.state.level = undefined;
        closed.state.reason = undefined;
        vi.advanceTimersByTime(60_000);
        assert.deepEqual(closed.cleared, []);
    });

    test("a mark replaced before the timer fired is not erased: other reason, other level, downgraded to info, cleared and set anew with other text", () => {
        const cases: Array<[unknown, unknown]> = [
            ["need", "other"],
            ["urgent", "why"],
            ["info", "why"],
            ["need", "why "],
            ["need", undefined],
            [undefined, "why"],
        ];
        for (const [level, reason] of cases) {
            const f = fakeEnv({ level: "need", reason: "why", focused: "b1" });
            scheduleAttentionClear(req(), f.env);
            vi.advanceTimersByTime(499);
            f.state.level = level;
            f.state.reason = reason;
            vi.advanceTimersByTime(60_000);
            assert.deepEqual(f.cleared, [], JSON.stringify([level, reason]));
        }
    });

    test("observation: a mark written anew with the SAME level and reason is indistinguishable and is erased by the old plan", () => {
        const f = fakeEnv({ level: "need", reason: "why", focused: "b1" });
        scheduleAttentionClear(req(), f.env);
        vi.advanceTimersByTime(400);
        f.state.level = undefined; // the session cleared it
        f.state.level = "need"; // and asked again with the same words; the component's effect does not re-run for an equal value
        vi.advanceTimersByTime(101);
        assert.deepEqual(f.cleared, ["b1"]);
    });

    test("an orange level that draws nothing (no reason, an empty one, a hidden character, a non-string) is not planned: nothing is cleared and nothing is sent", () => {
        const bad: unknown[] = ["", undefined, null, 3, "a" + cp(0x200b) + "b", "x".repeat(121)];
        for (const reason of bad) {
            const f = fakeEnv({ level: "need", reason, focused: "b1" });
            assert.equal(stackTabAttention("need", reason).kind, "none");
            assert.isUndefined(scheduleAttentionClear(req({ reason }), f.env), JSON.stringify(reason));
            vi.advanceTimersByTime(60_000);
            assert.deepEqual(f.cleared, [], JSON.stringify(reason));
        }
    });

    test("the write is both keys and null only", () => {
        assert.deepEqual(Object.keys(ClearAttentionMeta).sort(), ["attention:level", "attention:reason"]);
        assert.isTrue(Object.values(ClearAttentionMeta).every((v) => v === null));
    });
});

function compile(file: string): string {
    return sass.compile(path.join(root, file), { loadPaths: [path.join(root, "app"), root], style: "expanded" }).css;
}
function ruleBody(css: string, selector: string): string {
    const start = css.indexOf(selector + " {");
    assert.isAbove(start, -1, selector + " exists in the compiled css");
    return css.slice(start, css.indexOf("}", start));
}
const decls = (body: string) =>
    Object.fromEntries(
        body
            .slice(body.indexOf("{") + 1)
            .split(";")
            .map((d) => d.trim())
            .filter((d) => d.includes(":"))
            .map((d) => [d.slice(0, d.indexOf(":")).trim(), d.slice(d.indexOf(":") + 1).trim()])
    );
const LayoutProps = ["width", "height", "margin", "padding", "min-width", "max-width", "flex", "display", "float"];

describe("(c) mouse events and layout, read from the compiled stylesheets", () => {
    test("the window outline: fixed, inset 0, border-box, no mouse events, no size or background of its own, below every modal layer", () => {
        const css = compile("app/app.scss");
        const d = decls(ruleBody(css, ".window-outline"));
        assert.equal(d["position"], "fixed");
        assert.equal(d["inset"], "0");
        assert.equal(d["box-sizing"], "border-box");
        assert.equal(d["pointer-events"], "none");
        assert.equal(d["border-style"], "solid");
        for (const p of [...LayoutProps, "background", "background-color"]) {
            assert.isUndefined(d[p], p);
        }
        assert.equal(css.split(".window-outline").length - 1, 1, "one rule only");
        const z = Number(d["z-index"]);
        const theme = readFileSync(path.join(root, "app", "theme.scss"), "utf8");
        const layer = (name: string) => Number(new RegExp("--zindex-" + name + ":\\s*(-?\\d+);").exec(theme)![1]);
        assert.isBelow(z, layer("modal-wrapper"));
        assert.isBelow(z, layer("typeahead-modal"));
        assert.isBelow(z, layer("typeahead-modal-backdrop"));
        assert.isBelow(z, layer("elem-modal"));
        assert.isBelow(z, layer("window-drag"));
        assert.isBelow(z, layer("header-hover"));
        assert.isAbove(z, layer("layout-ephemeral-node"));
        assert.isAbove(z, layer("block-mask-inner"));
        assert.isAbove(z, layer("termstickers"));
    });

    test("the top tab fill and line: out of flow inside the tab, no mouse events, under the name layer", () => {
        const css = compile("app/tab/tab.scss");
        const fill = decls(ruleBody(css, ".tab .tab-colour-fill"));
        const line = decls(ruleBody(css, ".tab .tab-colour-line"));
        for (const d of [fill, line]) {
            assert.equal(d["position"], "absolute");
            assert.equal(d["pointer-events"], "none");
            assert.isUndefined(d["z-index"]);
            for (const p of ["margin", "padding", "display", "float", "flex", "width"]) {
                assert.isUndefined(d[p], p);
            }
        }
        assert.equal(fill["inset"], "0");
        assert.isUndefined(fill["height"]);
        assert.equal(line["height"], "2px");
        assert.equal(decls(ruleBody(css, ".tab .tab-inner"))["position"], "relative");
        assert.equal(decls(ruleBody(css, ".tab .name"))["z-index"], "var(--zindex-tab-name)");
        assert.equal(decls(ruleBody(css, ".tab .wave-button"))["z-index"], "var(--zindex-tab-name)");
    });

    test("the attention ring and dot: out of flow against the strip tab; the ring takes no mouse events", () => {
        const css = compile("layout/lib/stackstrip.scss");
        const ring = decls(ruleBody(css, ".tile-layout .stack-strip .stack-tab .stack-tab-attention-ring"));
        const dot = decls(ruleBody(css, ".tile-layout .stack-strip .stack-tab .stack-tab-attention-dot"));
        assert.equal(decls(ruleBody(css, ".tile-layout .stack-strip .stack-tab"))["position"], "relative");
        assert.equal(ring["position"], "absolute");
        assert.equal(ring["inset"], "0");
        assert.equal(ring["pointer-events"], "none");
        assert.isUndefined(ring["z-index"]);
        assert.equal(dot["position"], "absolute");
        assert.equal(dot["pointer-events"], "auto", "the dot does take mouse events (it carries the hover text)");
        assert.isUndefined(dot["animation"]);
        assert.include(css, "@media (prefers-reduced-motion: reduce)");
    });

    test("the app's own reduced-motion class (window:reducedmotion setting) stops the pulse as well as the system media query", () => {
        const css = compile("layout/lib/stackstrip.scss");
        const rule = ruleBody(css, "." + ReducedMotionClass + " .tile-layout .stack-strip .stack-tab .stack-tab-attention-ring");
        assert.equal(decls(rule)["animation"], "none");
    });

    test("the info icon is out of the flow like the dot, so the label's ellipsis cannot clip it", () => {
        const css = compile("layout/lib/stackstrip.scss");
        const info = decls(ruleBody(css, ".tile-layout .stack-strip .stack-tab .stack-tab-attention-info"));
        assert.equal(info["position"], "absolute");
        assert.isUndefined(info["margin-left"]);
        const dot = decls(ruleBody(css, ".tile-layout .stack-strip .stack-tab .stack-tab-attention-dot"));
        assert.equal(info["right"], dot["right"], "the info icon sits where the dot sits, left of the close button");
        const label = decls(ruleBody(css, ".tile-layout .stack-strip .stack-tab .stack-tab-label"));
        assert.equal(label["overflow"], "hidden");
    });

});

describe("merge damage in app.tsx", () => {
    const app = readFileSync(path.join(root, "app", "app.tsx"), "utf8");
    const count = (s: string) => app.split(s).length - 1;
    test("each component is declared once, mounted once, at the top level of AppInner, and the effect returns its cancel", () => {
        for (const name of ["BadgeAutoClearing", "AttentionAutoClearing", "WindowOutline", "AppInner", "AccentController"]) {
            assert.equal(count(`<${name} />`), 1, name + " mounted once");
        }
        for (const name of ["BadgeAutoClearing", "AttentionAutoClearing", "WindowOutline", "AppInner"]) {
            assert.equal(count(`const ${name} = () => {`), 1, name + " declared once");
            assert.match(app, new RegExp(`\\n};\\n\\n(// [^\\n]*\\n)*const ${name} = \\(\\) => \\{`), name + " starts after a closed component");
        }
        assert.equal(count("scheduleAttentionClear("), 1);
        assert.equal(count("return scheduleAttentionClear("), 1, "the effect returns the cancel function");
        assert.equal(count("useEffect("), 8);
        assert.notMatch(app, /^(<<<<<<<|=======|>>>>>>>)/m);
        const inner = app.slice(app.indexOf("const AppInner"));
        assert.isBelow(inner.indexOf("<AttentionAutoClearing />"), inner.indexOf("<DndProvider"));
        assert.isBelow(inner.indexOf("<WindowOutline />"), inner.indexOf("<DndProvider"));
    });
});
