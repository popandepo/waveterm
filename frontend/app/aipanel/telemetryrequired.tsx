// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { cn } from "@/util/util";

interface TelemetryRequiredMessageProps {
    className?: string;
}

// In this fork the AI panel has no button that switches usage telemetry on. Wave AI Cloud needs telemetry, so it is not available; a model of the
// owner's own (a custom mode) is used instead.
const TelemetryRequiredMessage = ({ className }: TelemetryRequiredMessageProps) => {
    return (
        <div className={cn("flex flex-col h-full", className)}>
            <div className="flex-grow"></div>
            <div className="flex items-center justify-center p-8 text-center">
                <div className="max-w-md space-y-4">
                    <i className="fa fa-sparkles text-accent text-5xl"></i>
                    <h2 className="text-2xl font-semibold text-foreground">Wave AI</h2>
                    <p className="text-secondary leading-relaxed">
                        Wave AI Cloud is not available in this fork: it needs usage telemetry, and this fork keeps
                        telemetry off. A model of your own (a custom mode, with your own key or a local model) works
                        without it.
                    </p>
                    <p className="text-xs text-secondary">
                        <a
                            href="https://docs.waveterm.dev/waveai-modes"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="!text-secondary hover:!text-accent/80 cursor-pointer"
                        >
                            https://docs.waveterm.dev/waveai-modes
                        </a>
                    </p>
                </div>
            </div>
            <div className="flex-grow-[2]"></div>
        </div>
    );
};

TelemetryRequiredMessage.displayName = "TelemetryRequiredMessage";

export { TelemetryRequiredMessage };
