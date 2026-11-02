// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package objectservice

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func initLayoutTestStore(t *testing.T) context.Context {
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

func newLayoutState(t *testing.T, ctx context.Context) *waveobj.LayoutState {
	t.Helper()
	ls := &waveobj.LayoutState{OID: uuid.NewString()}
	if err := wstore.DBInsert(ctx, ls); err != nil {
		t.Fatalf("insert layout: %v", err)
	}
	return ls
}

// what the view holds: a copy of the object as it was when the view last heard about it
func viewCopy(t *testing.T, ctx context.Context, id string) *waveobj.LayoutState {
	t.Helper()
	ls, err := wstore.DBGet[*waveobj.LayoutState](ctx, id)
	if err != nil || ls == nil {
		t.Fatalf("get layout: %v", err)
	}
	return ls
}

func queue(t *testing.T, ctx context.Context, id string, actionId string) {
	t.Helper()
	action := waveobj.LayoutActionData{ActionType: "delete", ActionId: actionId, BlockId: "b-" + actionId}
	if err := wcore.QueueLayoutAction(ctx, id, action); err != nil {
		t.Fatalf("queue: %v", err)
	}
}

func pendingIds(t *testing.T, ctx context.Context, id string) []string {
	t.Helper()
	ids := []string{}
	ls := viewCopy(t, ctx, id)
	if ls.PendingBackendActions == nil {
		return ids
	}
	for _, a := range *ls.PendingBackendActions {
		ids = append(ids, a.ActionId)
	}
	return ids
}

func save(t *testing.T, ctx context.Context, ls *waveobj.LayoutState) {
	t.Helper()
	svc := &ObjectService{}
	if _, err := svc.UpdateObject(waveobj.UIContext{}, ls, false); err != nil {
		t.Fatalf("update object: %v", err)
	}
}

func equalIds(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// the lost Remove of 27 Oct 2026: the backend queued the layout action after the view read the object,
// and the view's next save (still the old copy, nothing pending) replaced the whole object
func TestViewSaveOfAnOlderCopyKeepsAnActionQueuedAfterItWasRead(t *testing.T) {
	ctx := initLayoutTestStore(t)
	ls := newLayoutState(t, ctx)
	stale := viewCopy(t, ctx, ls.OID)

	queue(t, ctx, ls.OID, "remove-1")

	stale.FocusedNodeId = "node-from-the-view"
	save(t, ctx, stale)

	if got := pendingIds(t, ctx, ls.OID); !equalIds(got, []string{"remove-1"}) {
		t.Fatalf("pending actions after the view save = %v, want [remove-1]", got)
	}
	if got := viewCopy(t, ctx, ls.OID).FocusedNodeId; got != "node-from-the-view" {
		t.Fatalf("the view's own change was not saved: focusednodeid = %q", got)
	}
}

// the view has seen the actions it was sent and clears them by saving without them
func TestViewSaveOfTheCurrentCopyClearsTheActionsItProcessed(t *testing.T) {
	ctx := initLayoutTestStore(t)
	ls := newLayoutState(t, ctx)
	queue(t, ctx, ls.OID, "a1")

	current := viewCopy(t, ctx, ls.OID)
	current.PendingBackendActions = nil
	save(t, ctx, current)

	if got := pendingIds(t, ctx, ls.OID); len(got) != 0 {
		t.Fatalf("pending actions = %v, want none", got)
	}
}

func TestViewSaveOfAnOlderCopyDoesNotDuplicateActionsItStillCarries(t *testing.T) {
	ctx := initLayoutTestStore(t)
	ls := newLayoutState(t, ctx)
	queue(t, ctx, ls.OID, "a1")
	stale := viewCopy(t, ctx, ls.OID)
	queue(t, ctx, ls.OID, "a2")

	save(t, ctx, stale)

	if got := pendingIds(t, ctx, ls.OID); !equalIds(got, []string{"a1", "a2"}) {
		t.Fatalf("pending actions = %v, want [a1 a2]", got)
	}
}

// the view read Remove A, processed it and left it out of its save; B was queued before that save arrived
func TestViewSaveOfAnOlderCopyDoesNotBringBackAnActionItProcessed(t *testing.T) {
	ctx := initLayoutTestStore(t)
	ls := newLayoutState(t, ctx)
	queue(t, ctx, ls.OID, "a")
	seenByView := viewCopy(t, ctx, ls.OID)
	queue(t, ctx, ls.OID, "b")

	seenByView.PendingBackendActions = nil
	save(t, ctx, seenByView)

	if got := pendingIds(t, ctx, ls.OID); !equalIds(got, []string{"b"}) {
		t.Fatalf("pending actions = %v, want [b]", got)
	}
}

// two saves from one copy reach the server in the wrong order: the later one (A processed) first, the earlier one
// (A still listed) second. A was cleared already, so the earlier save must not queue it again.
func TestAnEarlierSaveArrivingLateDoesNotQueueAnActionAlreadyCleared(t *testing.T) {
	ctx := initLayoutTestStore(t)
	ls := newLayoutState(t, ctx)
	queue(t, ctx, ls.OID, "a")
	copyWithA := viewCopy(t, ctx, ls.OID)
	copyProcessed := viewCopy(t, ctx, ls.OID)
	copyProcessed.PendingBackendActions = nil

	save(t, ctx, copyProcessed)
	save(t, ctx, copyWithA)

	if got := pendingIds(t, ctx, ls.OID); len(got) != 0 {
		t.Fatalf("pending actions = %v, want none", got)
	}
}
