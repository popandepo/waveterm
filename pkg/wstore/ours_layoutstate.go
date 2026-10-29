// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wstore

import (
	"context"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// DBUpdateLayoutStateFromView stores a layout state the view saved. The view saves the whole object, and it clears
// the actions it has processed by leaving them out. Only the backend queues actions, so a save can keep or drop
// stored actions but never add one: a stored action stays if the save lists it, or if it was queued after the
// version the view saved from (the view has not seen it). Anything else was in the view's copy and left out, so
// the view processed it. Saves from one view can arrive out of order; an action a later save already cleared is
// not stored any more and an earlier save that still lists it cannot bring it back.
func DBUpdateLayoutStateFromView(ctx context.Context, ls *waveobj.LayoutState) error {
	return WithTx(ctx, func(tx *TxWrap) error {
		current, err := DBMustGet[*waveobj.LayoutState](tx.Context(), ls.OID)
		if err != nil {
			return err
		}
		ls.PendingBackendActions = keepStoredActions(ls.PendingBackendActions, current.PendingBackendActions, ls.Version)
		return DBUpdate(tx.Context(), ls)
	})
}

func keepStoredActions(saved *[]waveobj.LayoutActionData, stored *[]waveobj.LayoutActionData, savedVersion int) *[]waveobj.LayoutActionData {
	if stored == nil {
		return nil
	}
	listed := make(map[string]bool)
	if saved != nil {
		for _, action := range *saved {
			listed[action.ActionId] = true
		}
	}
	kept := []waveobj.LayoutActionData{}
	for _, action := range *stored {
		if listed[action.ActionId] || action.QueuedAtVersion > savedVersion {
			kept = append(kept, action)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return &kept
}
