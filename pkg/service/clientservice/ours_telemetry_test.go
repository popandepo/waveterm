// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package clientservice

import (
	"context"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wconfig"
)

func TestTelemetryUpdateIsRefusedBothWays(t *testing.T) {
	cs := &ClientService{}
	for _, val := range []bool{true, false} {
		err := cs.TelemetryUpdate(context.Background(), val)
		if err == nil || !strings.Contains(err.Error(), wconfig.OwnerOnlyKeysRefused) {
			t.Fatalf("TelemetryUpdate(%v) must be refused with the stated reason, got %v", val, err)
		}
	}
}
