// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type TermDims = { cols: number; rows: number };

// A block made in a tab nobody has shown is laid out at a fraction of its size (or at none), so a fit of its terminal measures a few
// columns (2 is the floor of the fit addon). That size would be sent to the program, and a program such as Claude Code would draw one or
// two characters per line until the tab is shown. Below these limits a measurement is not believed while the block has not yet had a usable one.
export const MinTrustedCols = 20;
export const MinTrustedRows = 4;
export const DefaultStartCols = 120;
export const DefaultStartRows = 30;

export function isUsableTermSize(d: TermDims): boolean {
    return d.cols >= MinTrustedCols && d.rows >= MinTrustedRows;
}

export type TermSizerOps = {
    get: () => TermDims;
    fit: () => void;
    set: (d: TermDims) => void;
    sendSize: (d: TermDims) => void;
    resync: () => void;
};

// The decisions of a terminal's resize, apart from xterm and the RPC so they can be driven in a test. Until a measurement has been usable, an
// unusable one is replaced by the usual size (also a second and a third one measured while the tab is still hidden); from the first usable one
// on, every measurement is believed, a narrow tile included. The program is started once, at the size the terminal has at that moment.
export class TermSizer {
    private usableSeen = false;
    private resynced = false;

    constructor(private ops: TermSizerOps) {}

    get hasResynced(): boolean {
        return this.resynced;
    }

    handle(): void {
        const old = this.ops.get();
        this.ops.fit();
        let now = this.ops.get();
        if (!this.usableSeen) {
            if (isUsableTermSize(now)) {
                this.usableSeen = true;
            } else {
                now = { cols: DefaultStartCols, rows: DefaultStartRows };
                this.ops.set(now);
            }
        }
        if (old.rows !== now.rows || old.cols !== now.cols) {
            this.ops.sendSize(now);
        }
        if (!this.resynced) {
            this.resynced = true;
            this.ops.resync();
        }
    }
}
