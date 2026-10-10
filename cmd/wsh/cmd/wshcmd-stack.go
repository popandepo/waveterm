// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

const stackFlagName = "stack"

// stackTarget is set by --stack on the commands that create a block.
var stackTarget string

// groupBlockOverride lets tests name the block without the id-resolving RPC.
var groupBlockOverride string

var (
	groupTo    string
	groupFocus bool
)

var stackBlockRPC = func(data wshrpc.CommandStackBlockData) error {
	return wshclient.StackBlockCommand(RpcClient, data, nil)
}

var resolveBlockRef = func(ref string) (string, error) {
	oref, err := resolveSimpleId(ref)
	if err != nil {
		return "", err
	}
	return oref.OID, nil
}

func addStackFlag(c *cobra.Command) {
	c.Flags().StringVar(&stackTarget, stackFlagName, "", "put the new block into the tab group of this block (block id or number) instead of next to it")
}

// applyStackTarget turns --stack into the target fields of a create-block request. A stack member is never
// magnified and never replaces another block, so those combinations are refused before anything is sent.
func applyStackTarget(data *wshrpc.CommandCreateBlockData) error {
	if stackTarget == "" {
		return nil
	}
	if data.TargetBlockId != "" {
		return fmt.Errorf("--%s cannot be combined with --replace", stackFlagName)
	}
	if data.Magnified {
		return fmt.Errorf("--%s cannot be combined with --magnified", stackFlagName)
	}
	id, err := resolveBlockRef(stackTarget)
	if err != nil {
		return fmt.Errorf("resolving --%s: %w", stackFlagName, err)
	}
	data.TargetBlockId = id
	data.TargetAction = wshrpc.CreateBlockAction_Stack
	return nil
}

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Tab groups: several blocks as tabs inside one tile",
	Long: `A group (stack) shows one block of several in the same tile, with a tab strip to switch. Every member is still an
ordinary block with its own id: wsh send, screen, focusblock and the rest address it as before. Blocks must be in the
same tab. A change never takes keyboard focus unless --focus is given.`,
}

var groupAddCmd = &cobra.Command{
	Use:     "add --to <block>",
	Short:   "Put a block into the group of another block (or make a group of the two)",
	Args:    cobra.NoArgs,
	RunE:    groupAddRun,
	PreRunE: preRunSetupRpcClient,
}

var groupRemoveCmd = &cobra.Command{
	Use:     "remove",
	Short:   "Take a block out of its group and place it next to the group",
	Args:    cobra.NoArgs,
	RunE:    groupRemoveRun,
	PreRunE: preRunSetupRpcClient,
}

var groupActivateCmd = &cobra.Command{
	Use:     "activate",
	Short:   "Show a block's tab in its group without taking keyboard focus",
	Args:    cobra.NoArgs,
	RunE:    groupActivateRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	groupAddCmd.Flags().StringVar(&groupTo, "to", "", "the block whose group to join (block id or number)")
	groupAddCmd.Flags().BoolVar(&groupFocus, "focus", false, "also move keyboard focus to the block")
	groupRemoveCmd.Flags().BoolVar(&groupFocus, "focus", false, "also move keyboard focus to the block")
	groupCmd.AddCommand(groupAddCmd, groupRemoveCmd, groupActivateCmd)
	rootCmd.AddCommand(groupCmd)
}

// The back end only queues the change for the page of that tab, so success means "accepted and queued", not
// "applied": if the tab is closed, or the layout changed meanwhile, the page drops it.
func queuedMessage(action string, blockId string) string {
	return fmt.Sprintf("queued: %s %s (applied when the tab is open; not confirmed)\n", action, blockId)
}

func sendStack(data wshrpc.CommandStackBlockData) error {
	if err := stackBlockRPC(data); err != nil {
		return err
	}
	WriteStdout("%s", queuedMessage(data.Action, data.BlockId))
	return nil
}

func groupBlockId() (string, error) {
	if groupBlockOverride != "" {
		return groupBlockOverride, nil
	}
	oref, err := resolveBlockArg()
	if err != nil {
		return "", err
	}
	return oref.OID, nil
}

func groupAddRun(cmd *cobra.Command, args []string) error {
	if groupTo == "" {
		return fmt.Errorf("--to is required: the block whose group to join")
	}
	blockId, err := groupBlockId()
	if err != nil {
		return err
	}
	targetId, err := resolveBlockRef(groupTo)
	if err != nil {
		return fmt.Errorf("resolving --to: %w", err)
	}
	return sendStack(wshrpc.CommandStackBlockData{
		BlockId:       blockId,
		TargetBlockId: targetId,
		Action:        wshrpc.StackBlockAction_Stack,
		Focused:       groupFocus,
	})
}

func groupRemoveRun(cmd *cobra.Command, args []string) error {
	blockId, err := groupBlockId()
	if err != nil {
		return err
	}
	return sendStack(wshrpc.CommandStackBlockData{BlockId: blockId, Action: wshrpc.StackBlockAction_Unstack, Focused: groupFocus})
}

func groupActivateRun(cmd *cobra.Command, args []string) error {
	blockId, err := groupBlockId()
	if err != nil {
		return err
	}
	return sendStack(wshrpc.CommandStackBlockData{BlockId: blockId, Action: wshrpc.StackBlockAction_Activate})
}
