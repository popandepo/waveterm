// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wstore

import (
	"context"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// DBUpdateLayoutStateFromView stores a layout state the view saved. The view saves the whole object, and it clears
// the actions it has processed by leaving them out. When the stored object is newer than the copy the view saved
// from, the backend queued something the view has not seen yet, so those actions are kept instead of overwritten.
// An action the view already processed may stay one save longer; the view skips action ids it has already handled.
func DBUpdateLayoutStateFromView(ctx context.Context, ls *waveobj.LayoutState) error {
	return WithTx(ctx, func(tx *TxWrap) error {
		current, err := DBMustGet[*waveobj.LayoutState](tx.Context(), ls.OID)
		if err != nil {
			return err
		}
		if current.Version != ls.Version {
			ls.PendingBackendActions = mergeQueuedActions(ls.PendingBackendActions, current.PendingBackendActions)
		}
		return DBUpdate(tx.Context(), ls)
	})
}

func mergeQueuedActions(saved *[]waveobj.LayoutActionData, stored *[]waveobj.LayoutActionData) *[]waveobj.LayoutActionData {
	merged := []waveobj.LayoutActionData{}
	known := make(map[string]bool)
	if saved != nil {
		for _, action := range *saved {
			known[action.ActionId] = true
			merged = append(merged, action)
		}
	}
	if stored != nil {
		for _, action := range *stored {
			if !known[action.ActionId] {
				merged = append(merged, action)
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return &merged
}
