// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wcore

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// two block deletes at the same moment each queue a layout action; both must be in the list afterwards
func TestQueueLayoutActionKeepsEveryConcurrentAction(t *testing.T) {
	ctx := initTestWStore(t)
	t.Cleanup(func() { _ = wstore.CloseDB() })
	ls := &waveobj.LayoutState{OID: uuid.NewString()}
	if err := wstore.DBInsert(ctx, ls); err != nil {
		t.Fatalf("insert layout: %v", err)
	}
	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			action := waveobj.LayoutActionData{ActionType: "delete", ActionId: fmt.Sprintf("a%d", i), BlockId: fmt.Sprintf("b%d", i)}
			if err := QueueLayoutAction(ctx, ls.OID, action); err != nil {
				t.Errorf("queue %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	got, err := wstore.DBGet[*waveobj.LayoutState](ctx, ls.OID)
	if err != nil || got == nil {
		t.Fatalf("get layout: %v", err)
	}
	count := 0
	if got.PendingBackendActions != nil {
		count = len(*got.PendingBackendActions)
	}
	if count != n {
		t.Fatalf("pending actions = %d, want %d", count, n)
	}
}
