// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wconfig

import "testing"

// Fork rule: nothing is sent to the network before the user has chosen. Both settings must be off in the
// defaults that ship in the binary; an upstream merge that flips either one back turns this red.
func TestShippedDefaultsAreOptIn(t *testing.T) {
	defaults, errs := ReadDefaultsConfigFile("settings.json")
	if len(errs) != 0 {
		t.Fatalf("reading default settings: %v", errs)
	}
	for _, key := range []string{"telemetry:enabled", "autoupdate:enabled"} {
		val, ok := defaults[key]
		if !ok {
			t.Fatalf("%s is missing from the shipped defaults: a missing key is read as the upstream default", key)
		}
		if val != false {
			t.Fatalf("%s must be false in the shipped defaults, got %v", key, val)
		}
	}
}
