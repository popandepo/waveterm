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

export async function redrawAfterSettle(
    term: RedrawableTerm,
    fontsReady: Promise<unknown>,
    nextFrame: (cb: () => void) => void
): Promise<void> {
    try {
        await fontsReady;
    } catch (_) {
        /* repaint anyway */
    }
    await new Promise<void>((resolve) => nextFrame(resolve));
    try {
        redrawTerm(term);
    } catch (_) {
        /* terminal was disposed while waiting */
    }
}
