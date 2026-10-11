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

// A program exits: the controller publishes a controllerstatus event with the block in its scopes, and the file on disk then
// shows the exit. Drives the real event path with a real store; only the program's state is stood in for.
func TestAnExitEventReachesTheFile(t *testing.T) {
	ctx := initTestStore(t)
	block := &waveobj.Block{OID: uuid.NewString(), Meta: waveobj.MetaMapType{"view": "term", "cmd": "claude --resume x"}}
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "T1", BlockIds: []string{block.OID}}
	ws := &waveobj.Workspace{OID: uuid.NewString(), Name: "Work", Icon: "flask", Color: "#FF453A", TabIds: []string{tab.OID}, ActiveTabId: tab.OID}
	for _, obj := range []waveobj.WaveObj{block, tab, ws} {
		if err := wstore.DBInsert(ctx, obj); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	var state atomic.Value
	state.Store("running")
	old := procStatusOf
	procStatusOf = func(blockId string) (string, *int) {
		if state.Load().(string) == "exited" {
			code := 5
			return "exited", &code
		}
		return "running", nil
	}
	t.Cleanup(func() { procStatusOf = old })
	path := filepath.Join(t.TempDir(), "state", "wave-state.json")
	StartWith(path, func() (staterecord.Record, error) {
		snap, err := loadSnapshot()
		if err != nil {
			return staterecord.Record{}, err
		}
		return staterecord.BuildRecord(snap), nil
	}, 10*time.Millisecond)
	waitFor(t, "the first record to say running", func() bool {
		data, _ := os.ReadFile(path)
		return contains(compact(data), `"status":"running"`)
	})

	state.Store("exited")
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_ControllerStatus,
		Scopes: []string{waveobj.MakeORef(waveobj.OType_Tab, tab.OID).String(), waveobj.MakeORef(waveobj.OType_Block, block.OID).String()},
	})
	waitFor(t, "the file to show the exit", func() bool {
		data, _ := os.ReadFile(path)
		return contains(compact(data), `"status":"exited"`) && contains(compact(data), `"exitcode":5`)
	})
}

func compact(data []byte) string {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
			out = append(out, b)
		}
	}
	return string(out)
}

// Look and attention through the whole chain, read from the file: a mark written to a stored block reaches the file; a change of the
// reason rewrites it; a value that the window would not draw (markup in an icon, a colour with style text, a hidden character in a
// reason) never reaches the file.
func TestLookAndAttentionReachTheFileAndHostileValuesDoNot(t *testing.T) {
	ctx := initTestStore(t)
	block := &waveobj.Block{OID: uuid.NewString(), Meta: waveobj.MetaMapType{"view": "term", "frame:icon": "rocket", "icon:color": "#2D7DD2"}}
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "T1", BlockIds: []string{block.OID}, Meta: waveobj.MetaMapType{"tab:flagcolor": "#ff8800"}}
	ws := &waveobj.Workspace{OID: uuid.NewString(), Name: "Work", TabIds: []string{tab.OID}, ActiveTabId: tab.OID}
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
	read := func() string {
		data, _ := os.ReadFile(path)
		return string(data)
	}
	setMeta := func(oType string, oid string, meta waveobj.MetaMapType) {
		mutateCtx := waveobj.ContextWithUpdates(ctx)
		if err := wstore.UpdateObjectMeta(mutateCtx, waveobj.MakeORef(oType, oid), meta, false); err != nil {
			t.Fatalf("update meta: %v", err)
		}
		wps.Broker.SendUpdateEvents(waveobj.ContextGetUpdatesRtn(mutateCtx))
	}

	waitFor(t, "the file to carry the icon, its colour and the tab colour", func() bool {
		s := read()
		return contains(s, `"icon": "rocket"`) && contains(s, `"iconcolor": "#2D7DD2"`) && contains(s, `"flagcolor": "#ff8800"`)
	})
	setMeta(waveobj.OType_Block, block.OID, waveobj.MetaMapType{"attention:level": "need", "attention:reason": "the build is waiting"})
	waitFor(t, "the file to carry the mark", func() bool {
		s := read()
		return contains(s, `"attentionlevel": "need"`) && contains(s, `"attentionreason": "the build is waiting"`)
	})
	setMeta(waveobj.OType_Block, block.OID, waveobj.MetaMapType{"attention:reason": "now it is urgent text"})
	waitFor(t, "a changed reason to rewrite the file", func() bool {
		s := read()
		return contains(s, "now it is urgent text") && !contains(s, "the build is waiting")
	})
	setMeta(waveobj.OType_Block, block.OID, waveobj.MetaMapType{
		"frame:icon": "<img src=x onerror=alert(1)>", "icon:color": "red;background:url(http://x/a.png)", "attention:reason": "a" + string(rune(0x200b)) + "b",
	})
	setMeta(waveobj.OType_Tab, tab.OID, waveobj.MetaMapType{"tab:flagcolor": "url(http://x/a.png)"})
	waitFor(t, "the file to drop the values that the window would not draw", func() bool {
		s := read()
		return !contains(s, `"icon"`) && !contains(s, `"iconcolor"`) && !contains(s, `"flagcolor"`) && !contains(s, "attentionlevel") && !contains(s, "attentionreason")
	})
	s := read()
	for _, forbidden := range []string{"<img", "onerror", "url(", "background"} {
		if contains(s, forbidden) {
			t.Fatalf("%q reached the file: %s", forbidden, s)
		}
	}
}
