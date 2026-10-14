// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export const TabDefaultWidth = 130;
export const TabMinWidth = 100;

// The part of the top row that is always a grab handle for the window. On Windows the 139 px at the right end are taken by the window's own buttons
// (minimise, maximise, close), so with a full row of tabs nothing was left to grab: the tab bar is "no-drag" over its whole box. This strip sits left
// of those buttons and is counted before the tabs get their width, so the tabs shrink (down to their minimum) and then scroll instead of taking it.
// Other platforms keep their 12 px edge: the obstruction is the Windows button area.
export const WindowDragStripWidth = 96;
export const WindowButtonsWidth = 139;
export const WindowDragRightEdgeWidth = 12;

// The width of the draggable block at the right end of the row, in CSS pixels at the given zoom (0 or less means no zoom is known). Only the
// Windows block follows the zoom, because it must cover the window buttons in screen pixels; the 12 px edge elsewhere is fixed.
export function getWindowDragRightWidth(isWindows: boolean, zoomFactor: number): number {
    const zoom = zoomFactor > 0 ? zoomFactor : 1;
    if (!isWindows) {
        return WindowDragRightEdgeWidth;
    }
    return (WindowButtonsWidth + WindowDragStripWidth) / zoom;
}

export type TabLayout = { tabWidth: number; scrollable: boolean; spaceForTabs: number };

// The width of each tab and whether the row scrolls, given the whole row and everything in it that is not a tab (the right block included).
// The tabs get only what is left, so they shrink to the minimum and then scroll; they never cover the right block.
export function computeTabLayout(rowWidth: number, nonTabWidth: number, numberOfTabs: number): TabLayout {
    const spaceForTabs = rowWidth - nonTabWidth;
    if (numberOfTabs <= 0) {
        return { tabWidth: TabDefaultWidth, scrollable: false, spaceForTabs };
    }
    const ideal = spaceForTabs / numberOfTabs;
    const tabWidth = Math.max(TabMinWidth, Math.min(ideal, TabDefaultWidth));
    return { tabWidth, scrollable: tabWidth * numberOfTabs > spaceForTabs, spaceForTabs };
}

// The width of the box that holds the tabs; the tab viewport scrolls over it.
export function tabsContentWidth(numberOfTabs: number, tabWidth: number): number {
    return numberOfTabs * tabWidth;
}

// The scrollLeft the bar asks for to show the end of the row. It may overshoot: the browser clamps it to content minus viewport.
export function tabsEndScrollLeft(numberOfTabs: number, tabWidth: number): number {
    return numberOfTabs * tabWidth;
}
