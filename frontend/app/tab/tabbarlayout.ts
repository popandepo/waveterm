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

// The width of the draggable block at the right end of the row, in CSS pixels at the given zoom (0 or less means no zoom is known).
export function getWindowDragRightWidth(isWindows: boolean, zoomFactor: number): number {
    const zoom = zoomFactor > 0 ? zoomFactor : 1;
    const width = isWindows ? WindowButtonsWidth + WindowDragStripWidth : WindowDragRightEdgeWidth;
    return width / zoom;
}

export type TabLayout = { tabWidth: number; scrollable: boolean; spaceForTabs: number };

// The width of each tab and whether the row scrolls, given the whole row and everything in it that is not a tab (the right block included).
// The tabs get only what is left, so they shrink to the minimum and then scroll; they never cover the right block.
export function computeTabLayout(rowWidth: number, nonTabWidth: number, numberOfTabs: number): TabLayout {
    const spaceForTabs = rowWidth - nonTabWidth;
    const ideal = spaceForTabs / numberOfTabs;
    const tabWidth = Math.max(TabMinWidth, Math.min(ideal, TabDefaultWidth));
    return { tabWidth, scrollable: tabWidth * numberOfTabs > spaceForTabs, spaceForTabs };
}
