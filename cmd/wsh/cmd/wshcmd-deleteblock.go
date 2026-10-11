// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var deleteBlockCmd = &cobra.Command{
	Use:     "deleteblock [block-ref]",
	Short:   "delete a block",
	Long:    "Delete a block. Optionally specify a block reference (block:oid, uuid, block number, or keyword like 'this'). If no block is specified, deletes the current block. Can also use --block/-b flag. A block whose program may still be running (or whose state or view is not known) is not deleted unless --force is given; the check is made by wavesrv in the same call that deletes.",
	Args:    cobra.MaximumNArgs(1),
	RunE:    deleteBlockRun,
	PreRunE: preRunSetupRpcClient,
}

var deleteBlockForce bool

func init() {
	deleteBlockCmd.Flags().BoolVar(&deleteBlockForce, "force", false, "delete even if a program is running in the block (or its state cannot be read)")
	rootCmd.AddCommand(deleteBlockCmd)
}

// makeDeleteBlockRequest is the request that wsh deleteblock sends: without --force wavesrv is told to refuse a block whose program may
// still be running, and it makes that check in the same call that deletes (nothing can start a program between the check and the delete
// except in the few microseconds of the database write).
func makeDeleteBlockRequest(blockId string, force bool) wshrpc.CommandDeleteBlockData {
	return wshrpc.CommandDeleteBlockData{BlockId: blockId, RefuseRunning: !force}
}

func deleteBlockRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("deleteblock", rtnErr == nil)
	}()
	var blockRef string
	if len(args) > 0 {
		blockRef = args[0]
	}
	fullORef, err := resolveBlockArgWithOverride(blockRef)
	if err != nil {
		return err
	}
	if fullORef.OType != "block" {
		return fmt.Errorf("object reference is not a block")
	}
	err = wshclient.DeleteBlockCommand(RpcClient, makeDeleteBlockRequest(fullORef.OID, deleteBlockForce), &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return fmt.Errorf("delete block failed: %v", err)
	}
	WriteStdout("block deleted\n")
	return nil
}
