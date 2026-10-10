// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getBlockMetaKeyAtom } from "@/store/global";
import { useAtomValue } from "jotai";
import { memo } from "react";

function lastSegment(path: string): string {
    const parts = path.split(/[\\/]/).filter((p) => p.length > 0);
    return parts.length > 0 ? parts[parts.length - 1] : path;
}

export const StackTabLabel = memo(({ blockId }: { blockId: string }) => {
    const title = useAtomValue(getBlockMetaKeyAtom(blockId, "frame:title"));
    const view = useAtomValue(getBlockMetaKeyAtom(blockId, "view"));
    const cwd = useAtomValue(getBlockMetaKeyAtom(blockId, "cmd:cwd"));
    const label = title || (cwd ? lastSegment(cwd) : null) || view || blockId.slice(0, 8);
    return <>{label}</>;
});
StackTabLabel.displayName = "StackTabLabel";
