// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

func initAttentionTestStore(t *testing.T) context.Context {
	t.Helper()
	dataDir := t.TempDir()
	wavebase.DataHome_VarCache = dataDir
	if err := os.MkdirAll(filepath.Join(dataDir, wavebase.WaveDBDir), 0700); err != nil {
		t.Fatalf("db dir: %v", err)
	}
	if err := InitWStore(); err != nil {
		t.Fatalf("init wstore: %v", err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	return context.Background()
}

func insertBlock(t *testing.T, ctx context.Context, meta waveobj.MetaMapType) *waveobj.Block {
	t.Helper()
	block := &waveobj.Block{OID: uuid.NewString(), Meta: meta}
	if err := DBInsert(ctx, block); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return block
}

func getBlock(t *testing.T, ctx context.Context, id string) *waveobj.Block {
	t.Helper()
	b, err := DBGet[*waveobj.Block](ctx, id)
	if err != nil || b == nil {
		t.Fatalf("get block: %v", err)
	}
	return b
}

func setMeta(t *testing.T, ctx context.Context, id string, meta waveobj.MetaMapType) {
	t.Helper()
	if err := UpdateObjectMeta(waveobj.ContextWithUpdates(ctx), waveobj.MakeORef(waveobj.OType_Block, id), meta, false); err != nil {
		t.Fatalf("update meta: %v", err)
	}
}

func TestClearIfRemovesBothKeysAndOnlyThose(t *testing.T) {
	ctx := initAttentionTestStore(t)
	b := insertBlock(t, ctx, waveobj.MetaMapType{"view": "term", "frame:title": "keep", "attention:level": "need", "attention:reason": "why"})
	cleared, err := ClearBlockAttentionIf(ctx, b.OID, "need", "why")
	if err != nil || !cleared {
		t.Fatalf("cleared=%v err=%v", cleared, err)
	}
	got := getBlock(t, ctx, b.OID).Meta
	if _, ok := got["attention:level"]; ok {
		t.Fatalf("level still there: %v", got)
	}
	if _, ok := got["attention:reason"]; ok {
		t.Fatalf("reason still there: %v", got)
	}
	if got["view"] != "term" || got["frame:title"] != "keep" {
		t.Fatalf("other keys must stay: %v", got)
	}
}

func TestClearIfLeavesAnyOtherMarkAndWritesNothing(t *testing.T) {
	ctx := initAttentionTestStore(t)
	cases := []struct {
		name string
		meta waveobj.MetaMapType
	}{
		{"another reason", waveobj.MetaMapType{"attention:level": "need", "attention:reason": "other"}},
		{"another level", waveobj.MetaMapType{"attention:level": "urgent", "attention:reason": "why"}},
		{"reason with a trailing space", waveobj.MetaMapType{"attention:level": "need", "attention:reason": "why "}},
		{"no reason", waveobj.MetaMapType{"attention:level": "need"}},
		{"no mark at all", waveobj.MetaMapType{"view": "term"}},
		{"a level that is not a string", waveobj.MetaMapType{"attention:level": 3, "attention:reason": "why"}},
	}
	for _, c := range cases {
		b := insertBlock(t, ctx, c.meta)
		before := getBlock(t, ctx, b.OID)
		cleared, err := ClearBlockAttentionIf(ctx, b.OID, "need", "why")
		if err != nil || cleared {
			t.Fatalf("%s: cleared=%v err=%v", c.name, cleared, err)
		}
		after := getBlock(t, ctx, b.OID)
		if after.Version != before.Version {
			t.Fatalf("%s: a refused clear must not write (version %d to %d)", c.name, before.Version, after.Version)
		}
		for k, v := range c.meta {
			if fmt.Sprint(after.Meta[k]) != fmt.Sprint(v) {
				t.Fatalf("%s: %s changed: %v", c.name, k, after.Meta)
			}
		}
	}
}

func TestClearIfOfAMissingBlockIsAnError(t *testing.T) {
	ctx := initAttentionTestStore(t)
	if cleared, err := ClearBlockAttentionIf(ctx, uuid.NewString(), "need", "why"); err == nil || cleared {
		t.Fatalf("a block that does not exist: cleared=%v err=%v", cleared, err)
	}
}

// The interleaving that the old way (read in the window, write later) lost: a clear is planned for (need, "x"), a program writes a new mark, the clear arrives.
func TestAMarkWrittenAfterThePlanIsNotErased(t *testing.T) {
	ctx := initAttentionTestStore(t)
	b := insertBlock(t, ctx, waveobj.MetaMapType{"attention:level": "need", "attention:reason": "x"})
	setMeta(t, ctx, b.OID, waveobj.MetaMapType{"attention:reason": "y"})
	cleared, err := ClearBlockAttentionIf(ctx, b.OID, "need", "x")
	if err != nil || cleared {
		t.Fatalf("the stale clear must not clear: cleared=%v err=%v", cleared, err)
	}
	got := getBlock(t, ctx, b.OID).Meta
	if got["attention:level"] != "need" || got["attention:reason"] != "y" {
		t.Fatalf("the newer mark must be intact: %v", got)
	}
	if cleared, err := ClearBlockAttentionIf(ctx, b.OID, "need", "y"); err != nil || !cleared {
		t.Fatalf("the clear that matches must work: cleared=%v err=%v", cleared, err)
	}
}

// A real race: a clear for the old mark and the write of a new mark at the same time. Whatever the order, the new mark must be there at the end: a
// compare followed by a separate write would let the clear overwrite the new mark.
func TestAClearRacingWithANewMarkNeverEratesTheNewMark(t *testing.T) {
	ctx := initAttentionTestStore(t)
	for round := 0; round < 150; round++ {
		b := insertBlock(t, ctx, waveobj.MetaMapType{"attention:level": "need", "attention:reason": "old"})
		var wg sync.WaitGroup
		wg.Add(2)
		start := make(chan struct{})
		go func() {
			defer wg.Done()
			<-start
			if _, err := ClearBlockAttentionIf(ctx, b.OID, "need", "old"); err != nil {
				t.Errorf("round %d: clear: %v", round, err)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			if err := UpdateObjectMeta(waveobj.ContextWithUpdates(ctx), waveobj.MakeORef(waveobj.OType_Block, b.OID), waveobj.MetaMapType{"attention:level": "need", "attention:reason": "new"}, false); err != nil {
				t.Errorf("round %d: write: %v", round, err)
			}
		}()
		close(start)
		wg.Wait()
		got := getBlock(t, ctx, b.OID).Meta
		if got["attention:level"] != "need" || got["attention:reason"] != "new" {
			t.Fatalf("round %d: the new mark was lost: %v", round, got)
		}
	}
}

func TestAnEmptyExpectedValueNeverMatches(t *testing.T) {
	ctx := initAttentionTestStore(t)
	b := insertBlock(t, ctx, waveobj.MetaMapType{"attention:level": "need", "attention:reason": ""})
	for _, c := range [][2]string{{"", ""}, {"need", ""}, {"", "x"}} {
		if cleared, err := ClearBlockAttentionIf(ctx, b.OID, c[0], c[1]); err != nil || cleared {
			t.Fatalf("(%q, %q) must not clear: cleared=%v err=%v", c[0], c[1], cleared, err)
		}
	}
	if got := getBlock(t, ctx, b.OID).Meta; got["attention:level"] != "need" {
		t.Fatalf("the mark must be intact: %v", got)
	}
}

// A controlled interleaving: inside the transaction of the clear, after the compare and before the write, another writer tries to write a new mark. If
// the compare and the write are one step the rival is held back until the clear is done and the new mark is the last word; if they were two steps the
// rival would get through in between (and the clear would then overwrite it).
func TestTheCompareAndTheWriteAreOneStep(t *testing.T) {
	ctx := initAttentionTestStore(t)
	b := insertBlock(t, ctx, waveobj.MetaMapType{"attention:level": "need", "attention:reason": "old"})
	attempting := make(chan struct{})
	rivalDone := make(chan struct{})
	rivalErr := make(chan error, 1)
	var rivalFinishedInsideTheStep bool
	attentionClearBeforeWrite = func() {
		go func() {
			defer close(rivalDone)
			close(attempting)
			rivalErr <- UpdateObjectMeta(waveobj.ContextWithUpdates(ctx), waveobj.MakeORef(waveobj.OType_Block, b.OID), waveobj.MetaMapType{"attention:level": "need", "attention:reason": "new"}, false)
		}()
		// the barrier: the rival has started its write attempt before the clear is allowed to go on
		select {
		case <-attempting:
		case <-time.After(5 * time.Second):
			t.Error("the rival never started its write attempt")
			return
		}
		select {
		case <-rivalDone:
			rivalFinishedInsideTheStep = true
		case <-time.After(300 * time.Millisecond):
		}
	}
	t.Cleanup(func() { attentionClearBeforeWrite = nil })
	cleared, err := ClearBlockAttentionIf(ctx, b.OID, "need", "old")
	if err != nil || !cleared {
		t.Fatalf("cleared=%v err=%v", cleared, err)
	}
	if rivalFinishedInsideTheStep {
		t.Fatal("a rival write got through between the compare and the write: they are not one step")
	}
	select {
	case <-rivalDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the rival write never finished")
	}
	if err := <-rivalErr; err != nil {
		t.Fatalf("the rival's write must succeed once the clear is done: %v", err)
	}
	got := getBlock(t, ctx, b.OID).Meta
	if got["attention:level"] != "need" || got["attention:reason"] != "new" {
		t.Fatalf("the rival's mark must be the last word: %v", got)
	}
}
