// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Block } from "@/app/block/block";
import { ContextMenuModel } from "@/app/store/contextmenu";
import { globalStore } from "@/app/store/jotaiStore";
import { stackAddEntries } from "@/app/tab/stackaddmenu";
import { entryStillAllowed } from "@/app/tab/stackcreate";
import { buildStripTabMenu } from "@/app/block/memberlook";
import { MemberLookWindowDeps } from "@/app/block/memberlook-ui";
import { StackTabLabel } from "@/app/tab/stacktablabel";
import { CenteredDiv } from "@/element/quickelems";
import { ContentRenderer, NodeModel, PreviewRenderer, TileLayout } from "@/layout/index";
import type { LayoutModel } from "@/layout/lib/layoutModel";
import { StackTabMenuInfo, TileLayoutContents } from "@/layout/lib/types";
import { closeBlockAfterConfirm } from "@/app/store/closeblock";
import { atoms, createBlockInStack, getApi, getBlockMetaKeyAtom } from "@/store/global";
import * as services from "@/store/services";
import * as WOS from "@/store/wos";
import { fireAndForget } from "@/util/util";
import { atom, useAtomValue } from "jotai";
import * as React from "react";
import { useMemo } from "react";

const tileGapSizeAtom = atom((get) => {
    const settings = get(atoms.settingsAtom);
    return settings["window:tilegapsize"];
});

const TabContent = React.memo(({ tabId, noTopPadding }: { tabId: string; noTopPadding?: boolean }) => {
    const oref = useMemo(() => WOS.makeORef("tab", tabId), [tabId]);
    const loadingAtom = useMemo(() => WOS.getWaveObjectLoadingAtom(oref), [oref]);
    const tabLoading = useAtomValue(loadingAtom);
    const tabAtom = useMemo(() => WOS.getWaveObjectAtom<Tab>(oref), [oref]);
    const tabData = useAtomValue(tabAtom);
    const tileGapSize = useAtomValue(tileGapSizeAtom);

    const tileLayoutContents = useMemo(() => {
        const renderContent: ContentRenderer = (nodeModel: NodeModel) => {
            return <Block key={nodeModel.blockId} nodeModel={nodeModel} preview={false} />;
        };

        const renderPreview: PreviewRenderer = (nodeModel: NodeModel) => {
            return <Block key={nodeModel.blockId} nodeModel={nodeModel} preview={true} />;
        };

        const renderStackTabLabel = (blockId: string) => <StackTabLabel key={blockId} blockId={blockId} />;

        const onStackAdd = (targetBlockId: string, ev: React.MouseEvent<any>, layoutModel: LayoutModel) => {
            const widgets = globalStore.get(atoms.fullConfigAtom)?.widgets;
            const workspaceIdAtOpen = globalStore.get(atoms.workspaceId);
            const entries = stackAddEntries(widgets, workspaceIdAtOpen);
            const menu: ContextMenuItem[] = entries.map((entry) => ({
                label: entry.label,
                click: () => {
                    if (!entryStillAllowed(workspaceIdAtOpen, globalStore.get(atoms.workspaceId))) {
                        return;
                    }
                    fireAndForget(() => createBlockInStack(entry.blockdef, targetBlockId, layoutModel));
                },
            }));
            ContextMenuModel.getInstance().showContextMenu(menu, ev);
        };

        const onStackTabMenu = (info: StackTabMenuInfo, ev: React.MouseEvent<any>) => {
            if (!MemberLookWindowDeps.exists(info.blockId)) {
                return;
            }
            const meta = (key: string) => globalStore.get(getBlockMetaKeyAtom(info.blockId, key as any)) as string | null;
            const menu = buildStripTabMenu(
                info.blockId,
                { color: meta("icon:color"), icon: meta("frame:icon"), title: meta("frame:title") },
                info,
                MemberLookWindowDeps
            );
            ContextMenuModel.getInstance().showContextMenu(menu, ev);
        };

        function onNodeDelete(data: TabLayoutData) {
            return services.ObjectService.DeleteBlock(data.blockId);
        }

        return {
            renderContent,
            renderPreview,
            renderStackTabLabel,
            onStackAdd,
            onStackTabMenu,
            closeBlockAfterConfirm,
            tabId,
            onNodeDelete,
            gapSizePx: tileGapSize,
        } as TileLayoutContents;
    }, [tabId, tileGapSize]);

    let innerContent;

    if (tabLoading) {
        innerContent = <CenteredDiv>Tab Loading</CenteredDiv>;
    } else if (!tabData) {
        innerContent = <CenteredDiv>Tab Not Found</CenteredDiv>;
    } else if (tabData?.blockids?.length == 0) {
        innerContent = null;
    } else {
        innerContent = (
            <TileLayout
                key={tabId}
                contents={tileLayoutContents}
                tabAtom={tabAtom}
                getCursorPoint={getApi().getCursorPoint}
            />
        );
    }

    return (
        <div
            className={`flex flex-row flex-grow min-h-0 w-full items-center justify-center overflow-hidden relative ${noTopPadding ? "" : "pt-[3px]"} pr-[3px]`}
        >
            {innerContent}
        </div>
    );
});

export { TabContent };
