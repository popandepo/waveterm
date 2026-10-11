// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getBlockBadgeAtom } from "@/app/store/badge";
import { getBlockMetaKeyAtom } from "@/store/global";
import { makeIconClass } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useMemo } from "react";
import { badgeColorValue, drawnTitle, lastSegment, stackTabAttention, stackTabLook } from "./stacktablook";

// The label of a member's tab: its symbol (icon or emoji) and name. The member's colour is drawn as a line under the tab and, on the shown
// member's tab, as a tint; both are spans of the label that the strip's stylesheet places against the tab (the tab is position: relative).
export const StackTabLabel = memo(({ blockId }: { blockId: string }) => {
    const title = useAtomValue(getBlockMetaKeyAtom(blockId, "frame:title"));
    const view = useAtomValue(getBlockMetaKeyAtom(blockId, "view"));
    const cwd = useAtomValue(getBlockMetaKeyAtom(blockId, "cmd:cwd"));
    const frameIcon = useAtomValue(getBlockMetaKeyAtom(blockId, "frame:icon"));
    const icon = useAtomValue(getBlockMetaKeyAtom(blockId, "icon"));
    const iconColor = useAtomValue(getBlockMetaKeyAtom(blockId, "icon:color"));
    const attentionLevel = useAtomValue(getBlockMetaKeyAtom(blockId, "attention:level"));
    const attentionReason = useAtomValue(getBlockMetaKeyAtom(blockId, "attention:reason"));
    const badge = useAtomValue(getBlockBadgeAtom(blockId));
    // a badge comes from any program (wsh badge --color): its colour passes the same validation as every other colour that is drawn
    const badgeColor = badgeColorValue(badge?.color);
    const label = drawnTitle(title) || (cwd ? lastSegment(cwd) : null) || view || blockId.slice(0, 8);
    const look = useMemo(() => stackTabLook(frameIcon, icon, iconColor), [frameIcon, icon, iconColor]);
    const attention = useMemo(() => stackTabAttention(attentionLevel, attentionReason), [attentionLevel, attentionReason]);
    return (
        <>
            {look.symbol?.kind === "icon" && (
                <i className={makeIconClass(look.symbol.value, true) + " stack-tab-symbol"} style={{ color: look.color ?? undefined }} />
            )}
            {look.symbol?.kind === "text" && (
                <span className="stack-tab-symbol stack-tab-emoji">{look.symbol.value}</span>
            )}
            {badge?.icon != null && (
                <i
                    className={makeIconClass(badge.icon, true, { defaultIcon: "circle-small" }) + " stack-tab-badge"}
                    style={{ color: badgeColor ?? undefined }}
                />
            )}
            {label}
            {attention.kind === "info" && (
                <i className="fa-sharp fa-solid fa-circle-info stack-tab-attention-info" title={attention.reason || "information"} />
            )}
            {(attention.kind === "need" || attention.kind === "urgent") && (
                <span className={"stack-tab-attention-dot " + attention.kind} title={attention.reason} />
            )}
            {attention.kind === "urgent" && <span className="stack-tab-attention-ring" />}
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
