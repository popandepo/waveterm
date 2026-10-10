// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// A key that is absent from the merged settings (a user file that drops it, a defaults file that loses it)
// must read as false, never as the upstream default.
func TestAbsentKeysReadAsOff(t *testing.T) {
	var settings SettingsType
	if err := json.Unmarshal([]byte(`{}`), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.TelemetryEnabled {
		t.Fatalf("telemetry:enabled is on when the key is absent")
	}
	if settings.AutoUpdateEnabled {
		t.Fatalf("autoupdate:enabled is on when the key is absent")
	}
}

func TestExplicitOptInStillWorks(t *testing.T) {
	var settings SettingsType
	if err := json.Unmarshal([]byte(`{"telemetry:enabled": true, "autoupdate:enabled": true}`), &settings); err != nil {
		t.Fatal(err)
	}
	if !settings.TelemetryEnabled || !settings.AutoUpdateEnabled {
		t.Fatalf("an explicit opt-in was lost: %+v", settings)
	}
}

// The updater is TypeScript, outside the Go type: pin its fallback for an absent key in the source.
func TestUpdaterFallbackIsOff(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "emain", "updater.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `settings["autoupdate:enabled"] ?? false`) {
		t.Fatalf("emain/updater.ts must read an absent autoupdate:enabled as false")
	}
}
