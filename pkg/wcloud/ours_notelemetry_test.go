// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wcloud

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

type countingTransport struct{ calls atomic.Int32 }

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("no network in this test")
}

// Changing the telemetry setting must not send the client id and the new value to the vendor, in either direction.
func TestNoTelemetryUpdateSendsNothing(t *testing.T) {
	ct := &countingTransport{}
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = ct
	t.Cleanup(func() { http.DefaultClient.Transport = old })
	for _, val := range []bool{true, false} {
		if err := SendNoTelemetryUpdate(context.Background(), "client-id-1", val); err != nil {
			t.Fatalf("a no-op must not fail (%v): %v", val, err)
		}
	}
	if n := ct.calls.Load(); n != 0 {
		t.Fatalf("the no-telemetry update made %d network calls", n)
	}
	if NoTelemetryUpdateAllowed() {
		t.Fatal("the update must not be allowed in this fork")
	}
}
