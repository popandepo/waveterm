// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wstore

import (
	"context"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// attentionClearBeforeWrite is a seam for the tests: it runs inside the transaction, after the compare and before the write.
var attentionClearBeforeWrite func()

// matchesAttention says whether the block's mark is exactly the one given: the level and the reason, as the strings that were planned for. A value
// that is not a string never matches (and a block with no mark matches only the empty pair, which the caller never plans for).
func matchesAttention(meta waveobj.MetaMapType, level string, reason string) bool {
	curLevel, levelOk := meta[waveobj.MetaKey_AttentionLevel].(string)
	curReason, reasonOk := meta[waveobj.MetaKey_AttentionReason].(string)
	return levelOk && reasonOk && curLevel == level && curReason == reason
}

// ClearBlockAttentionIf removes the attention mark of a block (both keys) only if the mark is exactly (level, reason) at the moment of the write. The
// read, the compare and the write are one transaction, so a mark that a program writes between the plan of a clear and its arrival is never erased
// (a compare in the window, followed by a plain write, leaves the round trip open). Returns whether the mark was cleared.
func ClearBlockAttentionIf(ctx context.Context, blockId string, level string, reason string) (bool, error) {
	// an empty or missing expected value never matches: a mark that draws nothing is not planned for, and an empty string must not clear another mark
	if level == "" || reason == "" {
		return false, nil
	}
	cleared, err := WithTxRtn(ctx, func(tx *TxWrap) (bool, error) {
		block, err := DBGet[*waveobj.Block](tx.Context(), blockId)
		if err != nil {
			return false, err
		}
		if block == nil {
			return false, ErrNotFound
		}
		if !matchesAttention(block.Meta, level, reason) {
			return false, nil
		}
		if attentionClearBeforeWrite != nil {
			attentionClearBeforeWrite()
		}
		meta := make(waveobj.MetaMapType, len(block.Meta))
		for k, v := range block.Meta {
			if k == waveobj.MetaKey_AttentionLevel || k == waveobj.MetaKey_AttentionReason {
				continue
			}
			meta[k] = v
		}
		block.Meta = meta
		if err := DBUpdate(tx.Context(), block); err != nil {
			return false, err
		}
		return true, nil
	})
	return cleared, err
}
