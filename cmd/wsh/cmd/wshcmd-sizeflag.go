// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

const (
	sizeFlagName = "size"
	MinNodeSize  = 2
	MaxNodeSize  = 50
)

var createNodeSize uint

// addSizeFlag adds --size to a command that creates a block. The layout gives every tile a weight (10 for a tile that nobody resized) and
// splits the space in proportion to the weights, so a new tile of weight 7 next to a tile of weight 10 gets 7 of 17 parts (about 41 percent).
func addSizeFlag(c *cobra.Command) {
	c.Flags().UintVar(&createNodeSize, sizeFlagName, 0, "weight of the new tile in the layout, 2 to 50 (a tile that nobody resized has 10; 7 next to such a tile gives it about 41 percent of the width). It has an effect only when the block is added as a new tile in the layout, not with --replace or --stack, and not for an ephemeral block")
}

// chooseNodeSize turns the value of --size into the field of the request: nothing when the flag was not given, an error for a value outside 2 to 50
// (a weight of 0 would give the tile no space, a huge one all of it).
func chooseNodeSize(given bool, v uint) (*uint, error) {
	if !given {
		return nil, nil
	}
	if v < MinNodeSize || v > MaxNodeSize {
		return nil, fmt.Errorf("--%s must be between %d and %d, got %d", sizeFlagName, MinNodeSize, MaxNodeSize, v)
	}
	return &v, nil
}

func applyCreateSize(cmd *cobra.Command, data *wshrpc.CommandCreateBlockData) error {
	size, err := chooseNodeSize(cmd.Flags().Changed(sizeFlagName), createNodeSize)
	if err != nil {
		return err
	}
	data.NodeSize = size
	return nil
}
