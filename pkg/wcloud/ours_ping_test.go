// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wcloud

import "testing"

func TestDiagnosticPingNeedsTelemetryOptIn(t *testing.T) {
	if DiagnosticPingAllowed(false) {
		t.Fatalf("the diagnostic ping must not be sent while usage telemetry is off")
	}
	if !DiagnosticPingAllowed(true) {
		t.Fatalf("the ping is allowed once the user has opted in")
	}
}
