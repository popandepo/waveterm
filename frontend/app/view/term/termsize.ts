// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type TermDims = { cols: number; rows: number };

// A block made in a tab nobody has shown is laid out at a fraction of its size (or at none), so the first fit of its terminal measures a
// few columns (2 is the floor of the fit addon). That size would be sent to the program when its controller starts, and a program such as
// Claude Code would draw one or two characters per line until the tab is shown. Below these limits the first measurement is not believed.
export const MinTrustedCols = 20;
export const MinTrustedRows = 4;
export const DefaultStartCols = 120;
export const DefaultStartRows = 30;

export function firstTermSize(measured: TermDims): TermDims {
    if (measured.cols < MinTrustedCols || measured.rows < MinTrustedRows) {
        return { cols: DefaultStartCols, rows: DefaultStartRows };
    }
    return measured;
}
