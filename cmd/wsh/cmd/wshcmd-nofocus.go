// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

// noFocusFlagName is the flag that stops a newly created block from taking focus.
const noFocusFlagName = "no-focus"

// noFocus is set by --no-focus. A wsh process runs one command, so one variable is enough.
var noFocus bool

// createBlockRPC is a variable so tests can look at the request a command sends.
var createBlockRPC = func(data wshrpc.CommandCreateBlockData) (waveobj.ORef, error) {
	return wshclient.CreateBlockCommand(RpcClient, data, nil)
}

// addNoFocusFlag registers --no-focus on a command that creates a block.
func addNoFocusFlag(c *cobra.Command) {
	c.Flags().BoolVar(&noFocus, noFocusFlagName, false, "create the block without moving focus to it")
}

// newBlockFocused is the value for the Focused field of a create-block request.
func newBlockFocused() bool {
	return !noFocus
}
