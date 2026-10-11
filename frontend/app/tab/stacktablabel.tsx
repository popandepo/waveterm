// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getBlockMetaKeyAtom } from "@/store/global";
import { makeIconClass } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useMemo } from "react";
import { lastSegment, stackTabLook } from "./stacktablook";

// The label of a member's tab: its symbol (icon or emoji) and name. The member's colour is drawn as a line under the tab and, on the shown
// member's tab, as a tint; both are spans of the label that the strip's stylesheet places against the tab (the tab is position: relative).
export const StackTabLabel = memo(({ blockId }: { blockId: string }) => {
    const title = useAtomValue(getBlockMetaKeyAtom(blockId, "frame:title"));
    const view = useAtomValue(getBlockMetaKeyAtom(blockId, "view"));
    const cwd = useAtomValue(getBlockMetaKeyAtom(blockId, "cmd:cwd"));
    const frameIcon = useAtomValue(getBlockMetaKeyAtom(blockId, "frame:icon"));
    const icon = useAtomValue(getBlockMetaKeyAtom(blockId, "icon"));
    const iconColor = useAtomValue(getBlockMetaKeyAtom(blockId, "icon:color"));
    const label = title || (cwd ? lastSegment(cwd) : null) || view || blockId.slice(0, 8);
    const look = useMemo(() => stackTabLook(frameIcon, icon, iconColor), [frameIcon, icon, iconColor]);
    return (
        <>
            {look.symbol?.kind === "icon" && (
                <i className={makeIconClass(look.symbol.value, true) + " stack-tab-symbol"} style={{ color: look.color ?? undefined }} />
            )}
            {look.symbol?.kind === "text" && (
                <span className="stack-tab-symbol stack-tab-emoji">{look.symbol.value}</span>
            )}
            {label}
            {look.color != null && (
                <>
                    <span className="stack-tab-fill" style={{ backgroundColor: look.color }} />
                    <span className="stack-tab-line" style={{ backgroundColor: look.color }} />
                </>
            )}
        </>
    );
});
StackTabLabel.displayName = "StackTabLabel";
