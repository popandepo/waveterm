// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
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

// Only blocks can be moved or stacked. A tab or workspace reference is refused here with a plain message.
func blockIdOfORef(oref waveobj.ORef) (string, error) {
	if oref.OType != waveobj.OType_Block {
		return "", fmt.Errorf("%s:%s is not a block; name a block (a tab or workspace is not supported as a target)", oref.OType, oref.OID)
	}
	return oref.OID, nil
}

var resolveBlockRef = func(ref string) (string, error) {
	oref, err := resolveSimpleId(ref)
	if err != nil {
		return "", err
	}
	return blockIdOfORef(*oref)
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

var (
	stackMoveFrom  string
	stackMoveTo    string
	stackMoveFocus bool
)

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Move whole tab groups: everything from one place into another",
}

var stackMoveCmd = &cobra.Command{
	Use:   "move --from <block|group> --to <block|group>",
	Short: "Move everything from FROM into the group at TO",
	Long: `FROM is a block, or any member of a group (then the whole group moves, in its order). TO is a block or a
group (or a member of one). Everything from FROM ends up as members of the group at TO, appended at the end; if TO is
a plain block the two become a new group. The two may be in different tabs of the same workspace: the blocks (and
their running processes) move to the tab of TO, and the leaf in the old tab is removed without closing the block.
A tab as target is not supported: name a block of that tab. The change is queued for the pages of the tabs
and is not confirmed. Keyboard focus moves only with --focus.`,
	Args:    cobra.NoArgs,
	RunE:    stackMoveRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	stackMoveCmd.Flags().StringVar(&stackMoveFrom, "from", "", "block or group to move (block id or number)")
	stackMoveCmd.Flags().StringVar(&stackMoveTo, "to", "", "block or group that receives everything")
	stackMoveCmd.Flags().BoolVar(&stackMoveFocus, "focus", false, "also move keyboard focus to the moved blocks")
	stackCmd.AddCommand(stackMoveCmd)
	rootCmd.AddCommand(stackCmd)
}

func stackMoveRun(cmd *cobra.Command, args []string) error {
	if stackMoveFrom == "" || stackMoveTo == "" {
		return fmt.Errorf("both --from and --to are required")
	}
	fromId, err := resolveBlockRef(stackMoveFrom)
	if err != nil {
		return fmt.Errorf("resolving --from: %w", err)
	}
	toId, err := resolveBlockRef(stackMoveTo)
	if err != nil {
		return fmt.Errorf("resolving --to: %w", err)
	}
	if fromId == toId {
		return fmt.Errorf("--from and --to are the same block")
	}
	return sendStack(wshrpc.CommandStackBlockData{
		BlockId:       fromId,
		TargetBlockId: toId,
		Action:        wshrpc.StackBlockAction_Move,
		Focused:       stackMoveFocus,
	})
}

var (
	stackSwapA string
	stackSwapB string
)

var stackSwapCmd = &cobra.Command{
	Use:   "swap --a <block> --b <block>",
	Short: "Make two tiles change places",
	Long: `A and B are blocks of the same tab. The two tiles change places and sizes; a block that is a member of a group means
its whole group (the group moves as one tile). Nothing is restarted: the processes keep running. Two members of the same
group cannot be swapped. The change is queued for the page of that tab and is not confirmed. A swap never takes keyboard
focus. With the mouse: hold Alt while dropping a tile on the middle of another one.`,
	Args:    cobra.NoArgs,
	RunE:    stackSwapRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	stackSwapCmd.Flags().StringVar(&stackSwapA, "a", "", "first block (block id or number)")
	stackSwapCmd.Flags().StringVar(&stackSwapB, "b", "", "second block (block id or number)")
	stackCmd.AddCommand(stackSwapCmd)
}

func stackSwapRun(cmd *cobra.Command, args []string) error {
	if stackSwapA == "" || stackSwapB == "" {
		return fmt.Errorf("both --a and --b are required")
	}
	aId, err := resolveBlockRef(stackSwapA)
	if err != nil {
		return fmt.Errorf("resolving --a: %w", err)
	}
	bId, err := resolveBlockRef(stackSwapB)
	if err != nil {
		return fmt.Errorf("resolving --b: %w", err)
	}
	if aId == bId {
		return fmt.Errorf("--a and --b are the same block")
	}
	return sendStack(wshrpc.CommandStackBlockData{
		BlockId:       aId,
		TargetBlockId: bId,
		Action:        wshrpc.StackBlockAction_Swap,
	})
}
