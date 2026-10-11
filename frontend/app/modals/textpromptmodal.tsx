// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Modal } from "@/app/modals/modal";
import { modalsModel } from "@/store/modalmodel";
import * as keyutil from "@/util/keyutil";
import { useState } from "react";

export interface TextPromptProps {
    title: string;
    label: string;
    initial: string;
    resolve: (answer: string | null) => void;
}

// One line of text for the owner of the window; resolves with the text, or null when it is cancelled.
const TextPromptModal = ({ title, label, initial, resolve }: TextPromptProps) => {
    const [text, setText] = useState(initial);
    const finish = (answer: string | null) => {
        modalsModel.popModal();
        resolve(answer);
    };
    return (
        <Modal className="pt-6 pb-4 px-5" onOk={() => finish(text)} onCancel={() => finish(null)} onClose={() => finish(null)}>
            <div className="font-bold text-primary mx-4 pb-2.5">{title}</div>
            <div className="flex flex-col gap-3 mx-4 mb-4 max-w-[500px] text-primary">
                <span>{label}</span>
                <input
                    type="text"
                    value={text}
                    maxLength={100}
                    autoFocus={true}
                    onChange={(e) => setText(e.target.value)}
                    onKeyDown={(e) =>
                        keyutil.keydownWrapper((ev: WaveKeyboardEvent) => {
                            if (keyutil.checkKeyPressed(ev, "Escape")) {
                                finish(null);
                                return true;
                            }
                            if (keyutil.checkKeyPressed(ev, "Enter")) {
                                finish(text);
                                return true;
                            }
                            return false;
                        })(e)
                    }
                    className="bg-panel rounded-md border border-border py-1.5 pl-4 min-h-[30px] text-inherit cursor-text focus:ring-2 focus:ring-accent focus:outline-none"
                />
            </div>
        </Modal>
    );
};
TextPromptModal.displayName = "TextPromptModal";

export { TextPromptModal };
