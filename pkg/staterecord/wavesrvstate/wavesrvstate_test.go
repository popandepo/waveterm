// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wavesrvstate

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/staterecord"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wps"
	"github.com/wavetermdev/waveterm/pkg/wstore"
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

func initTestStore(t *testing.T) context.Context {
	t.Helper()
	dataDir := t.TempDir()
	wavebase.DataHome_VarCache = dataDir
	if err := os.MkdirAll(filepath.Join(dataDir, wavebase.WaveDBDir), 0700); err != nil {
		t.Fatalf("db dir: %v", err)
	}
	if err := wstore.InitWStore(); err != nil {
		t.Fatalf("init wstore: %v", err)
	}
	t.Cleanup(func() { _ = wstore.CloseDB() })
	return context.Background()
}

// The whole chain with a real store: a stored block is changed through the store, the update event that the services send
// is published, and the file on disk shows the change. Nothing about the record is mocked.
func TestAStoredObjectChangeReachesTheFile(t *testing.T) {
	ctx := initTestStore(t)
	block := &waveobj.Block{OID: uuid.NewString(), Meta: waveobj.MetaMapType{"view": "term", "cmd": "/usr/bin/claude --resume SECRET"}}
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "T1", BlockIds: []string{block.OID}}
	ws := &waveobj.Workspace{OID: uuid.NewString(), Name: "Work", Icon: "flask", Color: "#FF453A", TabIds: []string{tab.OID}, ActiveTabId: tab.OID}
	for _, obj := range []waveobj.WaveObj{block, tab, ws} {
		if err := wstore.DBInsert(ctx, obj); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "state", "wave-state.json")
	StartWith(path, func() (staterecord.Record, error) {
		snap, err := loadSnapshot()
		if err != nil {
			return staterecord.Record{}, err
		}
		return staterecord.BuildRecord(snap), nil
	}, 10*time.Millisecond)

	waitFor(t, "the first record to name the program", func() bool {
		data, _ := os.ReadFile(path)
		return contains(string(data), `"claude"`)
	})
	data, _ := os.ReadFile(path)
	if contains(string(data), "SECRET") {
		t.Fatalf("an argument reached the file: %s", data)
	}

	mutateCtx := waveobj.ContextWithUpdates(ctx)
	if err := wstore.UpdateObjectMeta(mutateCtx, waveobj.MakeORef(waveobj.OType_Block, block.OID), waveobj.MetaMapType{"cmd": "pwsh -File x.ps1"}, false); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	wps.Broker.SendUpdateEvents(waveobj.ContextGetUpdatesRtn(mutateCtx))
	waitFor(t, "the file to show the changed program", func() bool {
		data, _ := os.ReadFile(path)
		return contains(string(data), `"pwsh"`) && !contains(string(data), `"claude"`)
	})
}
