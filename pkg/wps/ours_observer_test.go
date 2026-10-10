// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wps

import "testing"

func TestObserversSeeEveryPublishedEvent(t *testing.T) {
	var seen []string
	AddObserver(func(event WaveEvent) { seen = append(seen, event.Event) })
	Broker.Publish(WaveEvent{Event: Event_WaveObjUpdate, Scopes: []string{"block:abc"}})
	Broker.Publish(WaveEvent{Event: "other"})
	if len(seen) != 2 || seen[0] != Event_WaveObjUpdate || seen[1] != "other" {
		t.Fatalf("seen: %v", seen)
	}
}
