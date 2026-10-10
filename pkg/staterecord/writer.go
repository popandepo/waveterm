// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package staterecord

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Writer writes the record when something changed, never on a timer: Notify arms a short trailing debounce so that a burst
// of object updates (a drag moves many objects) gives one build and one write. The file is replaced atomically: the new
// content goes to a temp file in the same folder, is flushed to disk, and is renamed over the old file, so a reader sees the
// old record or the new one, never half of one. A write that fails leaves the old file in place and is retried at the
// next change or flush.
type Writer struct {
	path     string
	build    func() (Record, error)
	debounce time.Duration
	renameFn func(oldPath string, newPath string) error

	lock     sync.Mutex
	timer    *time.Timer
	sequence int64
	lastHash [32]byte
	hasLast  bool
}

func MakeWriter(path string, build func() (Record, error), debounce time.Duration) *Writer {
	return &Writer{path: path, build: build, debounce: debounce, renameFn: os.Rename}
}

// Notify says that something may have changed. It returns at once.
func (w *Writer) Notify() {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, func() {
		if err := w.Flush(); err != nil {
			log.Printf("state record not written: %v\n", err)
		}
	})
}

// Flush builds the record now and writes it if it differs from the last one written.
func (w *Writer) Flush() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	rec, err := w.build()
	if err != nil {
		return fmt.Errorf("building the state record: %w", err)
	}
	content, err := json.Marshal(rec.WithoutStamp())
	if err != nil {
		return err
	}
	hash := sha256.Sum256(content)
	if w.hasLast && hash == w.lastHash {
		return nil
	}
	rec.Sequence = w.sequence + 1
	rec.WrittenAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := w.writeAtomic(data); err != nil {
		return err
	}
	w.sequence = rec.Sequence
	w.lastHash = hash
	w.hasLast = true
	return nil
}

func (w *Writer) writeAtomic(data []byte) error {
	dir := filepath.Dir(w.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".wave-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := w.renameFn(tmpName, w.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replacing %s: %w", w.path, err)
	}
	return nil
}

// IsRelevantUpdate says whether an event is a change to an object that the record is built from.
func IsRelevantUpdate(eventName string, scopes []string) bool {
	if eventName != "waveobj:update" {
		return false
	}
	for _, scope := range scopes {
		for _, prefix := range []string{"workspace:", "tab:", "block:", "layout:", "window:"} {
			if len(scope) > len(prefix) && scope[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}
