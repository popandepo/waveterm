// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wconfig

import (
	"os"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

func TestOwnerOnlyKeysAreRefused(t *testing.T) {
	for _, key := range []string{"telemetry:enabled", "telemetry:*", "autoupdate:enabled", "autoupdate:channel", "autoupdate:*", "Telemetry:Enabled", " autoupdate:enabled"} {
		meta := waveobj.MetaMapType{key: true}
		err := CheckNoOwnerOnlyKeys(meta)
		if err == nil || !strings.Contains(err.Error(), OwnerOnlyKeysRefused) {
			t.Fatalf("%q must be refused with the stated reason, got %v", key, err)
		}
	}
	mixed := waveobj.MetaMapType{"term:cursor": "block", "telemetry:enabled": true}
	if err := CheckNoOwnerOnlyKeys(mixed); err == nil {
		t.Fatal("a request that carries one such key among others must be refused as a whole")
	}
}

func TestOtherKeysAreNotRefused(t *testing.T) {
	for _, key := range []string{"term:cursor", "autoupdate:intervalms", "autoupdate:installonquit", "app:confirmquit", "telemetry"} {
		if err := CheckNoOwnerOnlyKeys(waveobj.MetaMapType{key: true}); err != nil {
			t.Fatalf("%q must not be refused: %v", key, err)
		}
	}
	if err := CheckNoOwnerOnlyKeys(nil); err != nil {
		t.Fatalf("an empty request must not be refused: %v", err)
	}
}

// The refusal sits in the one function that writes the settings file, as its first statement: every caller (the setconfig command, the client service, anything added later) is covered.
func TestTheSettingsWriterRefusesBeforeItReadsTheFile(t *testing.T) {
	src, err := os.ReadFile("settingsconfig.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	i := strings.Index(text, "func SetBaseConfigValue(toMerge waveobj.MetaMapType) error {")
	if i < 0 {
		t.Fatal("SetBaseConfigValue not found")
	}
	body := text[i+len("func SetBaseConfigValue(toMerge waveobj.MetaMapType) error {"):]
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "if err := CheckNoOwnerOnlyKeys(toMerge); err != nil {") {
		t.Fatalf("the refusal must be the first statement, the function starts with %q", body[:80])
	}
}

func TestOwnerOnlyKeysAreTheKeysOfTheSettingsType(t *testing.T) {
	for _, key := range ownerOnlyKeys {
		if key != ConfigKey_TelemetryClear && key != ConfigKey_AutoUpdateClear && getConfigKeyType(key) == nil {
			t.Fatalf("%q is no config key: the list is out of date", key)
		}
	}
}
