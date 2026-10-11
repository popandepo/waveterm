// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

var termMagnified bool

var termCmd = &cobra.Command{
	Use:     "term",
	Short:   "open a terminal in directory",
	Args:    cobra.RangeArgs(0, 1),
	RunE:    termRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	addStackFlag(termCmd)
	addTabFlag(termCmd)
	termCmd.Flags().BoolVarP(&termMagnified, "magnified", "m", false, "open view in magnified mode")
	addNoFocusFlag(termCmd)
	rootCmd.AddCommand(termCmd)
}

func termRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("term", rtnErr == nil)
	}()

	var cwd string
	if len(args) > 0 {
		cwd = args[0]
		cwdExpanded, err := wavebase.ExpandHomeDir(cwd)
		if err != nil {
			return err
		}
		cwd = cwdExpanded
	} else {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
	}
	var err error
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("getting absolute path: %w", err)
	}

	tabId, err := createTabId(cmd)
	if err != nil {
		return err
	}

	createMeta := map[string]any{
		waveobj.MetaKey_View:       "term",
		waveobj.MetaKey_CmdCwd:     cwd,
		waveobj.MetaKey_Controller: "shell",
	}
	if RpcContext.Conn != "" {
		createMeta[waveobj.MetaKey_Connection] = RpcContext.Conn
	}
	createBlockData := wshrpc.CommandCreateBlockData{
		TabId: tabId,
		BlockDef: &waveobj.BlockDef{
			Meta: createMeta,
		},
		Magnified: termMagnified,
		Focused:   newBlockFocused(),
	}
	if err := applyStackTarget(&createBlockData); err != nil {
		return err
	}
	if err := checkCreateTarget(cmd, &createBlockData); err != nil {
		return err
	}
	oref, err := createBlockRPC(createBlockData)
	if err != nil {
		return fmt.Errorf("creating new terminal block: %w", err)
	}
	WriteStdout("terminal block created: %s\n", oref)
	return nil
}
