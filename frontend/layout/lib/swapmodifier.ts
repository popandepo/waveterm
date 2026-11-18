// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// react-dnd's hover callback has no access to the event, but every native dragover event carries altKey, and key events are not delivered
// to the page while a drag is in progress. So the state is read from dragover and reset when the drag ends.
const Capture = { capture: true };

export type SwapModifierTracker = {
    held: () => boolean;
    dispose: () => void;
};

export function makeSwapModifierTracker(target: EventTarget): SwapModifierTracker {
    let held = false;
    const onDragOver = (e: Event) => {
        held = (e as DragEvent).altKey === true;
    };
    const onEnd = () => {
        held = false;
    };
    target.addEventListener("dragover", onDragOver, Capture);
    target.addEventListener("dragend", onEnd, Capture);
    target.addEventListener("drop", onEnd, Capture);
    return {
        held: () => held,
        dispose: () => {
            target.removeEventListener("dragover", onDragOver, Capture);
            target.removeEventListener("dragend", onEnd, Capture);
            target.removeEventListener("drop", onEnd, Capture);
        },
    };
}
