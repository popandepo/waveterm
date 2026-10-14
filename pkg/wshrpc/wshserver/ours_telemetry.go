// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

// WaveAIEnableTelemetryRefused is the answer to the AI panel's "enable telemetry and continue" request. In this fork nothing switches usage telemetry on
// except the owner editing the setting himself; the AI panel's cloud mode is not available.
const WaveAIEnableTelemetryRefused = "usage telemetry cannot be switched on from the AI panel in this fork (it stays off unless telemetry:enabled is set by hand); Wave AI Cloud is not available, use a model of your own"
