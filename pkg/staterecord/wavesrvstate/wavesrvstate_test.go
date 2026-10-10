// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wavesrvstate

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/staterecord"
	"github.com/wavetermdev/waveterm/pkg/wps"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The writer is wired to the event stream: a change to an object that the record is built from produces a new file, and an
// event about something else does not cause another build. This fails if Start never registers its observer.
func TestAnObjectUpdateReachesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "wave-state.json")
	var builds int32
	name := "first"
	StartWith(path, func() (staterecord.Record, error) {
		atomic.AddInt32(&builds, 1)
		return staterecord.Record{Version: staterecord.RecordVersion, Wave: staterecord.WaveInfo{Version: name}}, nil
	}, 10*time.Millisecond)

	waitFor(t, "the first write at start", func() bool { _, err := os.Stat(path); return err == nil })
	buildsAtStart := atomic.LoadInt32(&builds)

	name = "second"
	wps.Broker.Publish(wps.WaveEvent{Event: wps.Event_WaveObjUpdate, Scopes: []string{"block:abc"}})
	waitFor(t, "the file to change after an object update", func() bool {
		data, _ := os.ReadFile(path)
		return len(data) > 0 && contains(string(data), "second")
	})

	before := atomic.LoadInt32(&builds)
	wps.Broker.Publish(wps.WaveEvent{Event: "sysinfo", Scopes: []string{"block:abc"}})
	wps.Broker.Publish(wps.WaveEvent{Event: wps.Event_WaveObjUpdate, Scopes: []string{"client:c1"}})
	time.Sleep(100 * time.Millisecond)
	if after := atomic.LoadInt32(&builds); after != before {
		t.Fatalf("events about other things caused %d builds", after-before)
	}
	if buildsAtStart < 1 {
		t.Fatalf("the record must be written once at start")
	}
}

func contains(s string, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
