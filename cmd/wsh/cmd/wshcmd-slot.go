// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var (
	slotTab        string
	slotLayoutFile string
)

var slotCommand = &cobra.Command{
	Use:   "slot",
	Short: "open, replace and close blocks in named places of a tab",
	Long: `Slots give blocks a fixed place by name (left, middle, right-top, right-bottom by default).
"wsh slot open middle web url=http://127.0.0.1:18766/" always lands in the middle: it replaces what is
there, or opens next to the nearest filled slot. "wsh slot close middle" gives the space back.
The slot names and how each one finds its neighbour are data (--layout-file).`,
	PersistentPreRunE: preRunSetupRpcClient,
}

var slotOpenCmd = &cobra.Command{
	Use:   "open <slot> <viewname> [key=value ...]",
	Short: "open a block in a slot, replacing the block that fills it",
	Args:  cobra.MinimumNArgs(2),
	RunE:  slotOpenRun,
}

var slotCloseCmd = &cobra.Command{
	Use:   "close <slot>",
	Short: "close the block in a slot",
	Args:  cobra.ExactArgs(1),
	RunE:  slotCloseRun,
}

var slotListCmd = &cobra.Command{
	Use:   "list",
	Short: "show which block fills each slot of the tab (JSON)",
	Args:  cobra.NoArgs,
	RunE:  slotListRun,
}

func init() {
	slotCommand.PersistentFlags().StringVar(&slotTab, "tab", "", "tab to act on (default: the tab this command runs in)")
	slotCommand.PersistentFlags().StringVar(&slotLayoutFile, "layout-file", "", "JSON file with the slot names and anchors (default: built in)")
	addNoFocusFlag(slotOpenCmd)
	slotCommand.AddCommand(slotOpenCmd)
	slotCommand.AddCommand(slotCloseCmd)
	slotCommand.AddCommand(slotListCmd)
	rootCmd.AddCommand(slotCommand)
}

type slotOps interface {
	SlotBlocks(tabId string) (map[string]string, error)
	CreateBlock(data wshrpc.CommandCreateBlockData) (string, error)
	DeleteBlock(blockId string) error
}

type rpcSlotOps struct{}

func (rpcSlotOps) SlotBlocks(tabId string) (map[string]string, error) {
	blocks, err := wshclient.BlocksListCommand(RpcClient, wshrpc.BlocksListRequest{}, &wshrpc.RpcOpts{Timeout: 10000})
	if err != nil {
		return nil, fmt.Errorf("listing blocks: %w", err)
	}
	return slotsFromBlocks(blocks, tabId), nil
}

func (rpcSlotOps) CreateBlock(data wshrpc.CommandCreateBlockData) (string, error) {
	oref, err := wshclient.CreateBlockCommand(RpcClient, data, nil)
	if err != nil {
		return "", err
	}
	return oref.OID, nil
}

func (rpcSlotOps) DeleteBlock(blockId string) error {
	return wshclient.DeleteBlockCommand(RpcClient, wshrpc.CommandDeleteBlockData{BlockId: blockId}, nil)
}

// slotsFromBlocks keeps the first block found for each slot of the tab.
func slotsFromBlocks(blocks []wshrpc.BlocksListEntry, tabId string) map[string]string {
	out := make(map[string]string)
	for _, b := range blocks {
		if b.TabId != tabId {
			continue
		}
		name := b.Meta.GetString(SlotMetaKey, "")
		if name == "" || out[name] != "" {
			continue
		}
		out[name] = b.BlockId
	}
	return out
}

type slotOpenResult struct {
	Slot    string `json:"slot"`
	BlockId string `json:"blockid"`
	Action  string `json:"action"`
	Target  string `json:"target,omitempty"`
}

func openInSlot(ops slotOps, layout *slotLayout, tabId string, slot string, viewName string, meta map[string]any, focused bool) (*slotOpenResult, error) {
	occupied, err := ops.SlotBlocks(tabId)
	if err != nil {
		return nil, err
	}
	plan, err := planSlotPlacement(layout, slot, occupied)
	if err != nil {
		return nil, err
	}
	blockMeta := make(map[string]any, len(meta)+2)
	for k, v := range meta {
		blockMeta[k] = v
	}
	blockMeta[waveobj.MetaKey_View] = viewName
	blockMeta[SlotMetaKey] = slot
	data := wshrpc.CommandCreateBlockData{
		TabId:         tabId,
		BlockDef:      &waveobj.BlockDef{Meta: blockMeta},
		Focused:       focused,
		TargetBlockId: plan.TargetBlockId,
		TargetAction:  plan.Action,
	}
	blockId, err := ops.CreateBlock(data)
	if err != nil {
		return nil, fmt.Errorf("creating block in slot %q: %w", slot, err)
	}
	action := plan.Action
	if action == "" {
		action = "insert"
	}
	return &slotOpenResult{Slot: slot, BlockId: blockId, Action: action, Target: plan.TargetBlockId}, nil
}

// closeSlot returns the id of the closed block, or "" when the slot was already empty.
func closeSlot(ops slotOps, layout *slotLayout, tabId string, slot string) (string, error) {
	if layout.find(slot) == nil {
		return "", fmt.Errorf("unknown slot %q (known: %v)", slot, layout.names())
	}
	occupied, err := ops.SlotBlocks(tabId)
	if err != nil {
		return "", err
	}
	blockId := occupied[slot]
	if blockId == "" {
		return "", nil
	}
	if err := ops.DeleteBlock(blockId); err != nil {
		return "", fmt.Errorf("closing block in slot %q: %w", slot, err)
	}
	return blockId, nil
}

func slotTabId() (string, error) {
	if slotTab != "" {
		return resolveTabId(slotTab)
	}
	tabId := getTabIdFromEnv()
	if tabId == "" {
		return "", fmt.Errorf("no WAVETERM_TABID env var set (use --tab)")
	}
	return tabId, nil
}

func slotOpenRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("slot", rtnErr == nil)
	}()
	layout, err := loadSlotLayout(slotLayoutFile)
	if err != nil {
		return err
	}
	tabId, err := slotTabId()
	if err != nil {
		return err
	}
	meta, err := parseMetaSets(args[2:])
	if err != nil {
		return err
	}
	res, err := openInSlot(rpcSlotOps{}, layout, tabId, args[0], args[1], meta, newBlockFocused())
	if err != nil {
		return err
	}
	out, _ := json.Marshal(res)
	WriteStdout("%s\n", out)
	return nil
}

func slotCloseRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("slot", rtnErr == nil)
	}()
	layout, err := loadSlotLayout(slotLayoutFile)
	if err != nil {
		return err
	}
	tabId, err := slotTabId()
	if err != nil {
		return err
	}
	blockId, err := closeSlot(rpcSlotOps{}, layout, tabId, args[0])
	if err != nil {
		return err
	}
	if blockId == "" {
		WriteStdout("slot %s was already empty\n", args[0])
		return nil
	}
	WriteStdout("closed block %s in slot %s\n", blockId, args[0])
	return nil
}

func slotListRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("slot", rtnErr == nil)
	}()
	layout, err := loadSlotLayout(slotLayoutFile)
	if err != nil {
		return err
	}
	tabId, err := slotTabId()
	if err != nil {
		return err
	}
	occupied, err := rpcSlotOps{}.SlotBlocks(tabId)
	if err != nil {
		return err
	}
	rows := make([]map[string]string, 0, len(layout.Slots))
	for _, s := range layout.Slots {
		rows = append(rows, map[string]string{"slot": s.Name, "blockid": occupied[s.Name]})
	}
	out, _ := json.Marshal(rows)
	WriteStdout("%s\n", out)
	return nil
}
