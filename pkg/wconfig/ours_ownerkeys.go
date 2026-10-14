// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wconfig

import (
	"fmt"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// OwnerOnlyKeysRefused is the answer to any request that would change a setting that decides whether this app talks to the vendor.
const OwnerOnlyKeysRefused = "this setting can only be changed by editing the settings file by hand in this fork (telemetry and automatic updates stay off unless the owner sets them)"

// The keys that decide whether the app sends usage data or looks for updates at the vendor. They are changed only by the owner editing the settings file:
// no command, service or screen of the app may write them (including the "clear" forms, which would fall back to the upstream default).
var ownerOnlyKeys = []string{
	ConfigKey_TelemetryEnabled,
	ConfigKey_TelemetryClear,
	ConfigKey_AutoUpdateEnabled,
	ConfigKey_AutoUpdateChannel,
	ConfigKey_AutoUpdateClear,
}

func OwnerOnlyKeysIn(meta waveobj.MetaMapType) []string {
	var found []string
	for key := range meta {
		for _, owned := range ownerOnlyKeys {
			if strings.EqualFold(strings.TrimSpace(key), owned) {
				found = append(found, key)
				break
			}
		}
	}
	return found
}

func CheckNoOwnerOnlyKeys(meta waveobj.MetaMapType) error {
	if found := OwnerOnlyKeysIn(meta); len(found) > 0 {
		return fmt.Errorf("%s: %s", OwnerOnlyKeysRefused, strings.Join(found, ", "))
	}
	return nil
}
