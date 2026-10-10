// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

var createBlockMagnified bool

var createBlockCmd = &cobra.Command{
	Use:     "createblock viewname key=value ...",
	Short:   "create a new block",
	Args:    cobra.MinimumNArgs(1),
	RunE:    createBlockRun,
	PreRunE: preRunSetupRpcClient,
	Hidden:  true,
}

func init() {
	addStackFlag(createBlockCmd)
	createBlockCmd.Flags().BoolVarP(&createBlockMagnified, "magnified", "m", false, "create block in magnified mode")
	addNoFocusFlag(createBlockCmd)
	rootCmd.AddCommand(createBlockCmd)
}

func createBlockRun(cmd *cobra.Command, args []string) error {
	viewName := args[0]
	var metaSetStrs []string
	if len(args) > 1 {
		metaSetStrs = args[1:]
	}
	tabId := getTabIdFromEnv()
	if tabId == "" {
		return fmt.Errorf("no WAVETERM_TABID env var set")
	}
	meta, err := parseMetaSets(metaSetStrs)
	if err != nil {
		return err
	}
	meta["view"] = viewName
	data := wshrpc.CommandCreateBlockData{
		TabId: tabId,
		BlockDef: &waveobj.BlockDef{
			Meta: meta,
		},
		Magnified: createBlockMagnified,
		Focused:   newBlockFocused(),
	}
	if err := applyStackTarget(&data); err != nil {
		return err
	}
	oref, err := createBlockRPC(data)
	if err != nil {
		return fmt.Errorf("create block failed: %v", err)
	}
	fmt.Printf("created block %s\n", oref.OID)
	return nil
}
