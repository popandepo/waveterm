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
	Long:    "Delete a block. Optionally specify a block reference (block:oid, uuid, block number, or keyword like 'this'). If no block is specified, deletes the current block. Can also use --block/-b flag.",
	Args:    cobra.MaximumNArgs(1),
	RunE:    deleteBlockRun,
	PreRunE: preRunSetupRpcClient,
}

var deleteBlockForce bool

func init() {
	deleteBlockCmd.Flags().BoolVar(&deleteBlockForce, "force", false, "delete even if a program is running in the block (or its state cannot be read)")
	rootCmd.AddCommand(deleteBlockCmd)
}

// deleteBlockRefusal says why a block must not be deleted without --force, or "" when it may be. Deleting stops the program in it, and a
// script that does it by accident loses a session, so the command is silent only when the answer is known: the block has no program
// (its view is not one that runs one), or its program has ended. A running program, a program that is starting, a missing or unknown
// state all refuse.
func deleteBlockRefusal(view string, status string, statusErr error, force bool) string {
	if force {
		return ""
	}
	if view != "term" && view != "tsunami" {
		return ""
	}
	if statusErr != nil {
		return fmt.Sprintf("the state of the program in the block could not be read (%v); use --force to delete it anyway", statusErr)
	}
	if status == "done" {
		return ""
	}
	return fmt.Sprintf("a program may still be running in the block (state %q); deleting it stops the program and cannot be undone: use --force to delete it anyway", status)
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
	if !deleteBlockForce {
		meta, err := wshclient.GetMetaCommand(RpcClient, wshrpc.CommandGetMetaData{ORef: *fullORef}, &wshrpc.RpcOpts{Timeout: 2000})
		if err != nil {
			return fmt.Errorf("reading the block before deleting it failed (use --force to delete anyway): %v", err)
		}
		view, _ := meta["view"].(string)
		status := ""
		var statusErr error
		if view == "term" || view == "tsunami" {
			st, err := wshclient.ControllerStatusCommand(RpcClient, fullORef.OID, &wshrpc.RpcOpts{Timeout: 2000})
			if err != nil {
				statusErr = err
			} else {
				status = st.ShellProcStatus
			}
		}
		if refusal := deleteBlockRefusal(view, status, statusErr, false); refusal != "" {
			return fmt.Errorf("not deleted: %s", refusal)
		}
	}
	deleteBlockData := &wshrpc.CommandDeleteBlockData{
		BlockId: fullORef.OID,
	}
	err = wshclient.DeleteBlockCommand(RpcClient, *deleteBlockData, &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return fmt.Errorf("delete block failed: %v", err)
	}
	WriteStdout("block deleted\n")
	return nil
}
