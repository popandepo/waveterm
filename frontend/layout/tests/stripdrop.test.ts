// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { assert, describe, test } from "vitest";
import { newLayoutNode } from "../lib/layoutNode";
import { stackNodes } from "../lib/layoutTree";
import { StripTabRect } from "../lib/stackgeometry";
import { applyStripHover, StripDropTarget, stripHover } from "../lib/stripdrop";
import { FlexDirection, LayoutNode, LayoutTreeAction, LayoutTreeActionType } from "../lib/types";
import { newLayoutTreeState } from "./model";

function leaf(blockId: string): LayoutNode {
    return newLayoutNode(FlexDirection.Column, undefined, undefined, { blockId });
}

// a stack of three (p q r) and a tile o beside it, with the rectangles of the three tabs on the strip
function setup() {
    const [p, q, r, o] = [leaf("p"), leaf("q"), leaf("r"), leaf("o")];
    const state = newLayoutTreeState(newLayoutNode(FlexDirection.Row, undefined, [p, q, r, o]));
    stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: p.id, nodeId: q.id });
    stackNodes(state, { type: LayoutTreeActionType.Stack, targetNodeId: q.id, nodeId: r.id });
    const tabs: StripTabRect[] = [p, q, r].map((n, i) => ({ nodeId: n.id, left: i * 100, right: (i + 1) * 100 }));
    return { root: state.rootNode, p, q, r, o, tabs };
}

function sink() {
    const calls: string[] = [];
    let pending: LayoutTreeAction | null = null;
    let target: StripDropTarget | null = null;
    return {
        calls,
        get pending() {
            return pending;
        },
        get target() {
            return target;
        },
        sink: {
            setPending: (a: LayoutTreeAction) => {
                pending = a;
                calls.push("set");
            },
            clearPending: () => {
                pending = null;
                target = null;
                calls.push("clear");
            },
            setTarget: (t: StripDropTarget | null) => {
                target = t;
                calls.push("target");
            },
        },
    };
}

describe("a drag over a strip", () => {
    test("a tab of the same stack dragged to the right lands after the target, to the left before it", () => {
        const s = setup();
        const right = stripHover(s.root, s.p.id, s.tabs, 250)!;
        assert.equal(right.action.type, LayoutTreeActionType.StackReorder);
        assert.deepEqual(right.target, { nodeId: s.r.id, side: "after" });
        const left = stripHover(s.root, s.r.id, s.tabs, 10)!;
        assert.equal(left.action.type, LayoutTreeActionType.StackReorder);
        assert.deepEqual(left.target, { nodeId: s.p.id, side: "before" });
    });

    test("over the empty end of the strip the target is the last tab, and the tab lands after it, also from outside", () => {
        const s = setup();
        const end = stripHover(s.root, s.p.id, s.tabs, 5000)!;
        assert.deepEqual(end.target, { nodeId: s.r.id, side: "after" });
        const outside = stripHover(s.root, s.o.id, s.tabs, 5000)!;
        assert.equal(outside.action.type, LayoutTreeActionType.Stack);
        assert.deepEqual(outside.target, { nodeId: s.r.id, side: "after" });
    });

    test("a tab over its own slot, no tabs, and no pointer give nothing", () => {
        const s = setup();
        assert.isNull(stripHover(s.root, s.q.id, s.tabs, 150));
        assert.isNull(stripHover(s.root, s.q.id, [], 150));
        assert.isNull(stripHover(s.root, s.q.id, s.tabs, undefined));
        assert.isNull(stripHover(s.root, undefined, s.tabs, 150));
    });
});

describe("what the strip does with a hover (the old action of a tile body must not stay)", () => {
    test("a hover with a possible drop sets the pending action and the outline, and clears nothing", () => {
        const s = setup();
        const k = sink();
        applyStripHover(k.sink, s.root, s.p.id, s.tabs, 250);
        assert.deepEqual(k.calls, ["set", "target"]);
        assert.equal(k.pending?.type, LayoutTreeActionType.StackReorder);
        assert.deepEqual(k.target, { nodeId: s.r.id, side: "after" });
    });

    test("a hover where nothing can be dropped CLEARS the pending action: an earlier insert-above from the body does not stay", () => {
        const s = setup();
        const k = sink();
        k.sink.setPending({ type: LayoutTreeActionType.Move } as LayoutTreeAction);
        k.calls.length = 0;
        applyStripHover(k.sink, s.root, s.q.id, s.tabs, 150);
        assert.deepEqual(k.calls, ["clear"]);
        assert.isNull(k.pending);
        assert.isNull(k.target);
    });
});

describe("the wiring that no test can render", () => {
    const read = (rel: string) => readFileSync(path.join(__dirname, "..", "lib", rel), "utf8");

    test("the overlay renders a strip drop target for every stack, it asks applyStripHover, and a drop on it commits the pending action", () => {
        const tile = read("TileLayout.tsx");
        assert.include(tile, "{overlayNodes}");
        assert.include(tile, "{overlayStrips}");
        assert.include(tile, "const overlayStrips = stripsToShow(additionalProps).map(");
        const strip = tile.slice(tile.indexOf("const OverlayStrip = memo("));
        assert.include(strip.slice(0, 2500), "applyStripHover(");
        assert.include(strip.slice(0, 2500), "layoutModel.onDrop();");
        assert.include(strip.slice(0, 2500), "!monitor.didDrop()");
    });

    test("the tab shows the outline from the model's target, on the side where the tab lands", () => {
        const strip = read("stacktabstrip.tsx");
        assert.include(strip, "const dropTarget = useAtomValue(layoutModel.stripDropTarget);");
        assert.include(strip, '"drop-over": isOver');
        assert.include(strip, '"drop-before": dropSide === "before"');
        assert.include(strip, '"drop-after": dropSide === "after"');
        const css = read("stackstrip.scss");
        assert.include(css, "&.drop-over.drop-before");
        assert.include(css, "&.drop-over.drop-after");
    });
});
