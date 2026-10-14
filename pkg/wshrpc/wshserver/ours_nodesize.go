// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"fmt"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

const (
	MinCreateNodeSize = 2
	MaxCreateNodeSize = 50
)

// checkNodeSize validates the weight that a client asks for its new tile (CommandCreateBlockData.NodeSize) before anything is created: absent
// means the default weight (nil), 2 to 50 is kept, anything else (0 included) is refused. The request comes from any client of the RPC, not only
// from wsh, whose own check is a convenience.
func checkNodeSize(size *uint) (*uint, error) {
	if size == nil {
		return nil, nil
	}
	if *size < MinCreateNodeSize || *size > MaxCreateNodeSize {
		return nil, fmt.Errorf("nodesize must be between %d and %d (leave it out for the default), got %d", MinCreateNodeSize, MaxCreateNodeSize, *size)
	}
	return size, nil
}

// checkCreateNodeSize is checkNodeSize for a whole request: a weight is for a tile in the layout, and an ephemeral block is not one (the layout
// would drop the weight without a word), so a weight with ephemeral is refused.
func checkCreateNodeSize(data wshrpc.CommandCreateBlockData) (*uint, error) {
	if data.Ephemeral && data.NodeSize != nil {
		return nil, fmt.Errorf("nodesize applies to tiles in the layout only, not to an ephemeral block")
	}
	return checkNodeSize(data.NodeSize)
}
