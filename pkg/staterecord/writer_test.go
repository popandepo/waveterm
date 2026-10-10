// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package staterecord

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func makeTestWriter(t *testing.T, build func() (Record, error)) (*Writer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state", "wave-state.json")
	return MakeWriter(path, build, 20*time.Millisecond), path
}

func staticBuild(rec Record) func() (Record, error) {
	return func() (Record, error) { return rec, nil }
}

func readRecord(t *testing.T, path string) Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("the file is not valid json: %v", err)
	}
	return rec
}

func listDir(t *testing.T, path string) []string {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestFlushWritesTheRecordAndNothingElse(t *testing.T) {
	w, path := makeTestWriter(t, staticBuild(BuildRecord(fixture(t))))
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	rec := readRecord(t, path)
	if rec.Sequence != 1 || rec.WrittenAt == "" || len(rec.Workspaces) != 1 || len(rec.Sessions) != 2 {
		t.Fatalf("record: %+v", rec)
	}
	if names := listDir(t, path); len(names) != 1 || names[0] != "wave-state.json" {
		t.Fatalf("files in the folder: %v", names)
	}
}

func TestAnUnchangedStateIsNotRewritten(t *testing.T) {
	w, path := makeTestWriter(t, staticBuild(BuildRecord(fixture(t))))
	w.Flush()
	first, _ := os.Stat(path)
	time.Sleep(15 * time.Millisecond)
	w.Flush()
	second, _ := os.Stat(path)
	if !second.ModTime().Equal(first.ModTime()) || readRecord(t, path).Sequence != 1 {
		t.Fatalf("an unchanged state must not touch the file")
	}
}

func TestAChangedStateGetsTheNextSequence(t *testing.T) {
	snap := fixture(t)
	w, path := makeTestWriter(t, func() (Record, error) { return BuildRecord(snap), nil })
	w.Flush()
	snap.Tabs["tab1"].Name = "renamed"
	w.Flush()
	rec := readRecord(t, path)
	if rec.Sequence != 2 || rec.Workspaces[0].Tabs[0].Name != "renamed" {
		t.Fatalf("record: %+v", rec)
	}
}

func TestBurstsOfChangesAreCoalescedIntoOneBuild(t *testing.T) {
	var builds int32
	w, path := makeTestWriter(t, func() (Record, error) {
		atomic.AddInt32(&builds, 1)
		return BuildRecord(fixture(t)), nil
	})
	for i := 0; i < 25; i++ {
		w.Notify()
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(80 * time.Millisecond)
	if n := atomic.LoadInt32(&builds); n != 1 {
		t.Fatalf("25 notifications inside the debounce window gave %d builds", n)
	}
}

func TestNothingIsWrittenWithoutAChange(t *testing.T) {
	var builds int32
	_, path := makeTestWriter(t, func() (Record, error) {
		atomic.AddInt32(&builds, 1)
		return BuildRecord(fixture(t)), nil
	})
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&builds) != 0 {
		t.Fatalf("the writer must not poll or write on a timer")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("no file without a change")
	}
}

func TestAFailedRenameKeepsTheOldFileAndLeavesNoTempFile(t *testing.T) {
	snap := fixture(t)
	w, path := makeTestWriter(t, func() (Record, error) { return BuildRecord(snap), nil })
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	snap.Tabs["tab1"].Name = "changed"
	w.renameFn = func(oldPath string, newPath string) error { return errors.New("disk said no") }
	if err := w.Flush(); err == nil {
		t.Fatalf("the failure must be reported")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("the old file must stay whole")
	}
	if names := listDir(t, path); len(names) != 1 {
		t.Fatalf("temp file left behind: %v", names)
	}
	w.renameFn = os.Rename
	if err := w.Flush(); err != nil {
		t.Fatalf("the next flush must retry the change: %v", err)
	}
	if readRecord(t, path).Workspaces[0].Tabs[0].Name != "changed" {
		t.Fatalf("the change was lost after a failed write")
	}
}

func TestABuildErrorWritesNothingAndIsReported(t *testing.T) {
	w, path := makeTestWriter(t, func() (Record, error) { return Record{}, errors.New("db not ready") })
	if err := w.Flush(); err == nil {
		t.Fatalf("a build error must be reported")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("a record that could not be built must not be written")
	}
}

func TestTheFileIsPrivateWhereTheSystemAllowsIt(t *testing.T) {
	w, path := makeTestWriter(t, staticBuild(BuildRecord(fixture(t))))
	w.Flush()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("mode %v: others can read the record", mode)
	}
}

func TestOnlyChangesToTheObjectsOfTheRecordTriggerAWrite(t *testing.T) {
	yes := [][]string{{"workspace:w1"}, {"tab:t1"}, {"block:b1"}, {"layout:l1"}, {"window:w"}}
	for _, scopes := range yes {
		if !IsRelevantUpdate("waveobj:update", scopes) {
			t.Fatalf("%v should trigger a write", scopes)
		}
	}
	for _, c := range []struct {
		event  string
		scopes []string
	}{
		{"waveobj:update", []string{"client:c1"}},
		{"waveobj:update", []string{"temp:x"}},
		{"waveobj:update", nil},
		{"blockclose", []string{"block:b1"}},
		{"sysinfo", []string{"block:b1"}},
	} {
		if IsRelevantUpdate(c.event, c.scopes) {
			t.Fatalf("%s %v should not trigger a write", c.event, c.scopes)
		}
	}
}

func TestABuildErrorKeepsThePreviousFile(t *testing.T) {
	fail := false
	w, path := makeTestWriter(t, func() (Record, error) {
		if fail {
			return Record{}, errors.New("database read failed")
		}
		return BuildRecord(fixture(t)), nil
	})
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	fail = true
	if err := w.Flush(); err == nil {
		t.Fatalf("the failure must be reported")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("an incomplete read must not replace the last good record")
	}
}

func TestAReaderHoldingTheFileOpenOnlyDelaysTheReplace(t *testing.T) {
	snap := fixture(t)
	w, path := makeTestWriter(t, func() (Record, error) { return BuildRecord(snap), nil })
	w.retryDelay = 20 * time.Millisecond
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snap.Tabs["tab1"].Name = "changed while read"
	go func() {
		time.Sleep(120 * time.Millisecond)
		reader.Close()
	}()
	if err := w.Flush(); err != nil {
		t.Fatalf("the write must succeed once the reader lets go: %v", err)
	}
	if readRecord(t, path).Workspaces[0].Tabs[0].Name != "changed while read" {
		t.Fatalf("the change was not written")
	}
	if names := listDir(t, path); len(names) != 1 {
		t.Fatalf("files left behind: %v", names)
	}
}

func TestARenameThatKeepsFailingGivesUpAfterTheBoundedRetries(t *testing.T) {
	snap := fixture(t)
	var tries int32
	w, _ := makeTestWriter(t, func() (Record, error) { return BuildRecord(snap), nil })
	w.retryDelay = time.Millisecond
	w.renameFn = func(oldPath string, newPath string) error {
		atomic.AddInt32(&tries, 1)
		return errors.New("locked")
	}
	if err := w.Flush(); err == nil {
		t.Fatalf("a rename that never works must be reported")
	}
	if n := atomic.LoadInt32(&tries); n < 2 || n > 30 {
		t.Fatalf("expected a bounded number of tries, got %d", n)
	}
}
