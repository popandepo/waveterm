// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveapi"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

const apiDirName = "api"

type apiCaller interface {
	List(ctx context.Context) ([]waveapi.Workspace, error)
	Focus(ctx context.Context, a waveapi.Address) error
	Goto(ctx context.Context, a waveapi.Address, window bool) error
}

var makeApiCaller = func(dir string) (apiCaller, error) {
	return waveapi.MakeClient(dir)
}

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Use the local Wave API: list, focus, goto",
	Long: `Talks to the local Wave API (a unix socket in the "api" folder of the Wave data directory, guarded by a token
file that only the current user can read). Ids are full lowercase uuids as printed by "wsh api list".
None of these commands raises the OS window unless "goto --window" is given.`,
}

var apiListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces, tabs and blocks",
	Args:  cobra.NoArgs,
	RunE:  apiListRun,
}

var apiFocusCmd = &cobra.Command{
	Use:   "focus (--block <id> | --tab <id>)",
	Short: "Focus a block in the active tab, or make a tab the active tab",
	Args:  cobra.NoArgs,
	RunE:  apiFocusRun,
}

var apiGotoCmd = &cobra.Command{
	Use:   "goto --tab <id> [--workspace <id>] [--block <id>] [--window]",
	Short: "Go to a workspace, tab and block by address",
	Long: `Make the tab the active tab of its workspace and focus the block in it. The whole address is checked
first: a tab that is not in the workspace, or a block that is not in the tab, changes nothing. The workspace
defaults to the one of the calling block (WAVETERM_WORKSPACEID). The OS window is only raised with --window.`,
	Args: cobra.NoArgs,
	RunE: apiGotoRun,
}

var (
	apiDirFlag   string
	apiJson      bool
	apiWorkspace string
	apiTab       string
	apiBlock     string
	apiWindow    bool
)

func init() {
	apiCmd.PersistentFlags().StringVar(&apiDirFlag, "dir", "", "api folder (default: <wave data dir>/api)")
	apiListCmd.Flags().BoolVar(&apiJson, "json", false, "output as JSON")
	apiFocusCmd.Flags().StringVar(&apiTab, "tab", "", "tab id")
	apiFocusCmd.Flags().StringVar(&apiBlock, "block", "", "block id")
	apiGotoCmd.Flags().StringVar(&apiWorkspace, "workspace", "", "workspace id (default: the calling block's workspace)")
	apiGotoCmd.Flags().StringVar(&apiTab, "tab", "", "tab id")
	apiGotoCmd.Flags().StringVar(&apiBlock, "block", "", "block id")
	apiGotoCmd.Flags().BoolVar(&apiWindow, "window", false, "also raise the window of the workspace")
	apiCmd.AddCommand(apiListCmd, apiFocusCmd, apiGotoCmd)
	rootCmd.AddCommand(apiCmd)
}

func resolveApiDir(cmd *cobra.Command) (string, error) {
	if apiDirFlag != "" {
		return apiDirFlag, nil
	}
	if err := preRunSetupRpcClient(cmd, nil); err != nil {
		return "", err
	}
	dataDir, err := wshclient.PathCommand(RpcClient, wshrpc.PathCommandData{PathType: "data"}, nil)
	if err != nil {
		return "", fmt.Errorf("finding the wave data directory: %w", err)
	}
	return filepath.Join(dataDir, apiDirName), nil
}

func apiCallerFor(cmd *cobra.Command) (apiCaller, error) {
	dir, err := resolveApiDir(cmd)
	if err != nil {
		return nil, err
	}
	return makeApiCaller(dir)
}

func apiListRun(cmd *cobra.Command, args []string) error {
	c, err := apiCallerFor(cmd)
	if err != nil {
		return err
	}
	tree, err := c.List(cmd.Context())
	if err != nil {
		return err
	}
	if apiJson {
		out, err := json.MarshalIndent(tree, "", "  ")
		if err != nil {
			return err
		}
		WriteStdout("%s\n", out)
		return nil
	}
	for _, ws := range tree {
		WriteStdout("workspace %s %q window %s\n", ws.WorkspaceId, ws.Name, ws.WindowId)
		for _, tab := range ws.Tabs {
			active := ""
			if tab.TabId == ws.ActiveTabId {
				active = " (active)"
			}
			WriteStdout("  tab %s %q%s\n", tab.TabId, tab.Name, active)
			for _, b := range tab.Blocks {
				WriteStdout("    block %s %s slot=%s %q\n", b.BlockId, b.View, b.Slot, b.Title)
			}
		}
	}
	return nil
}

func apiFocusRun(cmd *cobra.Command, args []string) error {
	if apiBlock == "" && apiTab == "" {
		return fmt.Errorf("give --block or --tab")
	}
	c, err := apiCallerFor(cmd)
	if err != nil {
		return err
	}
	return c.Focus(cmd.Context(), waveapi.Address{TabId: apiTab, BlockId: apiBlock})
}

func apiGotoRun(cmd *cobra.Command, args []string) error {
	workspace := apiWorkspace
	if workspace == "" {
		workspace = os.Getenv("WAVETERM_WORKSPACEID")
	}
	if workspace == "" {
		return fmt.Errorf("no workspace: give --workspace (WAVETERM_WORKSPACEID is not set)")
	}
	if apiTab == "" {
		return fmt.Errorf("no tab: give --tab")
	}
	c, err := apiCallerFor(cmd)
	if err != nil {
		return err
	}
	return c.Goto(cmd.Context(), waveapi.Address{WorkspaceId: workspace, TabId: apiTab, BlockId: apiBlock}, apiWindow)
}
