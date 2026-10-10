// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { sortBadgesForTab } from "@/app/store/badge";
import { cn, makeIconClass } from "@/util/util";
import { useMemo } from "react";
import { v7 as uuidv7 } from "uuid";
import { parseTabSymbol } from "./tabsymbol";

export interface TabBadgesProps {
    badges?: Badge[] | null;
    flagColor?: string | null;
    symbol?: string | null;
    className?: string;
}

const DefaultClassName =
    "pointer-events-none absolute left-[4px] top-1/2 z-[3] flex h-[20px] w-[20px] -translate-y-1/2 items-center justify-center px-[2px] py-[1px]";

export function TabBadges({ badges, flagColor, symbol, className }: TabBadgesProps) {
    const flagBadgeId = useMemo(() => uuidv7(), []);
    const parsedSymbol = useMemo(() => parseTabSymbol(symbol), [symbol]);
    const allBadges = useMemo(() => {
        const base = badges ?? [];
        if (!flagColor && !parsedSymbol) {
            return base;
        }
        const flagBadge: Badge = {
            icon: parsedSymbol?.kind === "icon" ? parsedSymbol.value : "flag",
            color: flagColor ?? "var(--main-text-color)",
            priority: 0,
            badgeid: flagBadgeId,
        };
        return sortBadgesForTab([...base, flagBadge]);
    }, [badges, flagColor, parsedSymbol, flagBadgeId]);
    const firstIsTextSymbol = parsedSymbol?.kind === "text" && allBadges[0]?.badgeid === flagBadgeId;
    if (!allBadges[0]) {
        return null;
    }
    const firstBadge = allBadges[0];
    const extraBadges = allBadges.slice(1, 3).filter((b) => !(parsedSymbol && b.badgeid === flagBadgeId));
    return (
        <div className={cn(DefaultClassName, className)}>
            {firstIsTextSymbol ? (
                <span className="text-[12px] leading-none" style={{ color: firstBadge.color || "#fbbf24" }}>
                    {parsedSymbol.value}
                </span>
            ) : (
                <i
                    className={makeIconClass(firstBadge.icon, true, { defaultIcon: "circle-small" }) + " text-[12px]"}
                    style={{ color: firstBadge.color || "#fbbf24" }}
                />
            )}
            {extraBadges.length > 0 && (
                <div className="ml-[2px] flex flex-col items-center justify-center gap-[2px]">
                    {extraBadges.map((badge, idx) => (
                        <div
                            key={idx}
                            className="h-[4px] w-[4px] rounded-full"
                            style={{ backgroundColor: badge.color || "#fbbf24" }}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}
