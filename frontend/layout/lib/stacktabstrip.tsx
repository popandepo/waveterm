// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { fireAndForget } from "@/util/util";
import clsx from "clsx";
import { useAtomValue } from "jotai";
import { memo, useEffect, useMemo, useRef } from "react";
import type { LayoutModel } from "./layoutModel";
import { buildStripTabs, stripsToShow } from "./stackgeometry";
import type { LayoutNodeAdditionalProps } from "./types";
import { setTransform } from "./utils";

const StripZIndex = 3;

interface StackStripProps {
    layoutModel: LayoutModel;
    stackId: string;
    props: LayoutNodeAdditionalProps;
}

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
                <div
                    key={tab.nodeId}
                    className={clsx("stack-tab", { active: tab.active, focused: tab.focused })}
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
                            fireAndForget(() => layoutModel.closeNode(tab.nodeId));
                        }}
                    >
                        <i className="fa-sharp fa-solid fa-xmark" />
                    </button>
                </div>
            ))}
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
