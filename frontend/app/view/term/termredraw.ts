// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export interface RedrawableTerm {
    rows: number;
    clearTextureAtlas(): void;
    refresh(start: number, end: number): void;
}

// A terminal built while its tab is still laying out, or whose renderer was swapped after a lost WebGL context, keeps a
// stale glyph atlas or an unpainted canvas until new output arrives. Dropping the atlas and repainting every row fixes both.
export function redrawTerm(term: RedrawableTerm): void {
    if (term.rows <= 0) {
        return;
    }
    term.clearTextureAtlas();
    term.refresh(0, term.rows - 1);
}

// Tabs are torn down on every switch, so the wait for fonts and a frame can outlive the terminal: dispose() both stops the
// continuation and cancels the frame.
export class RedrawScheduler {
    private term: RedrawableTerm;
    private fontsReady: Promise<unknown>;
    private requestFrame: (cb: () => void) => number;
    private cancelFrame: (handle: number) => void;
    private disposed = false;
    private pending = false;
    private frameHandle: number | null = null;

    constructor(
        term: RedrawableTerm,
        fontsReady: Promise<unknown>,
        requestFrame: (cb: () => void) => number,
        cancelFrame: (handle: number) => void
    ) {
        this.term = term;
        this.fontsReady = fontsReady;
        this.requestFrame = requestFrame;
        this.cancelFrame = cancelFrame;
    }

    async schedule(): Promise<void> {
        if (this.disposed || this.pending) {
            return;
        }
        this.pending = true;
        try {
            try {
                await this.fontsReady;
            } catch (_) {
                /* repaint anyway */
            }
            if (this.disposed) {
                return;
            }
            await new Promise<void>((resolve) => {
                this.frameHandle = this.requestFrame(() => {
                    this.frameHandle = null;
                    resolve();
                });
            });
            if (this.disposed) {
                return;
            }
            redrawTerm(this.term);
        } finally {
            this.pending = false;
        }
    }

    dispose(): void {
        this.disposed = true;
        if (this.frameHandle != null) {
            this.cancelFrame(this.frameHandle);
            this.frameHandle = null;
        }
    }
}
