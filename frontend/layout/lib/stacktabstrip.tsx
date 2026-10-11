// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { fireAndForget } from "@/util/util";
import clsx from "clsx";
import { useAtomValue } from "jotai";
import { memo, useEffect, useMemo, useRef } from "react";
import { useDrag, useDrop } from "react-dnd";
import { tileItemType } from "./dndtypes";
import type { LayoutModel } from "./layoutModel";
import { tabDropAction } from "./layoutTree";
import { buildStripTabs, stripsToShow } from "./stackgeometry";
import type { LayoutNode, LayoutNodeAdditionalProps } from "./types";
import { setTransform } from "./utils";

const StripZIndex = 3;

interface StackStripProps {
    layoutModel: LayoutModel;
    stackId: string;
    props: LayoutNodeAdditionalProps;
}

// One tab of a strip. It is a drag source of the same kind as a tile header: dropped on the edge of a tile it becomes a
// tile of its own, dropped on the middle of a tile it joins that tile's stack (the overlay rules decide). Dropped on another
// tab it takes that place (same stack) or joins the stack (from outside).
const StripTab = memo(
    ({
        layoutModel,
        tab,
        node,
    }: {
        layoutModel: LayoutModel;
        tab: ReturnType<typeof buildStripTabs>[number];
        node: LayoutNode;
    }) => {
        const tabRef = useRef<HTMLDivElement>(null);
        const [{ isDragging }, drag] = useDrag(
            () => ({
                type: tileItemType,
                item: () => node,
                collect: (monitor) => ({ isDragging: monitor.isDragging() }),
            }),
            [node]
        );
        const [{ isOver }, drop] = useDrop(
            () => ({
                accept: tileItemType,
                canDrop: (item: LayoutNode) =>
                    tabDropAction(layoutModel.treeState.rootNode, item.id, tab.nodeId) != null,
                drop: (item: LayoutNode) => {
                    const action = tabDropAction(layoutModel.treeState.rootNode, item.id, tab.nodeId);
                    if (action) {
                        layoutModel.treeReducer(action);
                    }
                },
                collect: (monitor) => ({ isOver: monitor.isOver({ shallow: true }) && monitor.canDrop() }),
            }),
            [layoutModel, tab.nodeId]
        );
        useEffect(() => {
            drag(drop(tabRef));
        }, [drag, drop]);
        return (
            <div
                ref={tabRef}
                className={clsx("stack-tab", {
                    active: tab.active,
                    focused: tab.focused,
                    dragging: isDragging,
                    "drop-over": isOver,
                })}
                data-blockid={tab.blockId}
                onClick={() => layoutModel.focusNode(tab.nodeId)}
            >
                <span className="stack-tab-label">
                    {layoutModel.renderStackTabLabel?.(tab.blockId) ?? tab.blockId.slice(0, 8)}
                </span>
                <button
                    className="stack-tab-close cursor-pointer"
                    title="Close this tab and its terminal"
                    onClick={(e) => {
                        e.stopPropagation();
                        fireAndForget(async () => {
                            if ((await layoutModel.confirmCloseBlock?.(tab.blockId)) === false) {
                                return;
                            }
                            await layoutModel.closeNode(tab.nodeId);
                        });
                    }}
                >
                    <i className="fa-sharp fa-solid fa-xmark" />
                </button>
            </div>
        );
    }
);
StripTab.displayName = "StripTab";

const StackStrip = memo(({ layoutModel, stackId, props }: StackStripProps) => {
    const leafs = useAtomValue(layoutModel.leafs);
    const focusedNode = useAtomValue(layoutModel.focusedNode);
    const tabs = useMemo(
        () => buildStripTabs(props.stack, leafs, focusedNode?.id),
        [props.stack, leafs, focusedNode?.id]
    );
    const stripRef = useRef<HTMLDivElement>(null);
    useEffect(() => {
        const active = stripRef.current?.querySelector<HTMLElement>(".stack-tab.active");
        active?.scrollIntoView?.({ inline: "nearest", block: "nearest" });
    }, [props.stack.activeId, tabs.length]);
    return (
        <div
            ref={stripRef}
            className="stack-strip"
            data-stackid={stackId}
            onWheel={(e) => {
                if (e.deltaY !== 0 && e.deltaX === 0 && stripRef.current) {
                    stripRef.current.scrollLeft += e.deltaY;
                }
            }}
            style={setTransform(props.stack.strip, true, true, StripZIndex)}
        >
            {tabs.map((tab) => (
                <StripTab
                    key={tab.nodeId}
                    layoutModel={layoutModel}
                    tab={tab}
                    node={leafs.find((l) => l.id === tab.nodeId)}
                />
            ))}
            <button
                className="stack-tab-add cursor-pointer"
                title="Add a block to this stack"
                onClick={(e) => {
                    const active = tabs.find((t) => t.active) ?? tabs[tabs.length - 1];
                    layoutModel.onStackAdd?.(active.blockId, e, layoutModel);
                }}
            >
                <i className="fa-sharp fa-solid fa-plus" />
            </button>
        </div>
    );
});
StackStrip.displayName = "StackStrip";

export const StackStrips = memo(({ layoutModel }: { layoutModel: LayoutModel }) => {
    const additionalProps = useAtomValue(layoutModel.additionalProps);
    const stacks = stripsToShow(additionalProps);
    return (
        <>
            {stacks.map(([stackId, props]) => (
                <StackStrip key={stackId} layoutModel={layoutModel} stackId={stackId} props={props} />
            ))}
        </>
    );
});
StackStrips.displayName = "StackStrips";
