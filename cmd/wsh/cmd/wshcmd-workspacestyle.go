// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var workspaceStyleCmd = &cobra.Command{
	Use:   "style",
	Short: "Set the accent colour of a workspace",
	Long: `Set the accent colour of the whole Wave frame for a workspace. It is used while the active tab has no accent of its
own (wsh tab style --accent). Without any accent a workspace uses its own icon colour; --clear removes the accent.`,
	Example: `  wsh workspace style --accent crimson
  wsh workspace style --workspace <id> --accent "#2D7DD2"
  wsh workspace style --clear`,
	Args:    cobra.NoArgs,
	RunE:    workspaceStyleRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	workspaceStyleWorkspace string
	workspaceStyleAccent    string
	workspaceStyleClear     bool
)

func init() {
	workspaceStyleCmd.Flags().StringVar(&workspaceStyleWorkspace, "workspace", "", "workspace id (defaults to WAVETERM_WORKSPACEID)")
	workspaceStyleCmd.Flags().StringVar(&workspaceStyleAccent, "accent", "", "CSS colour name or hex")
	workspaceStyleCmd.Flags().BoolVar(&workspaceStyleClear, "clear", false, "remove the workspace accent")
	workspaceCommand.AddCommand(workspaceStyleCmd)
}

func buildWorkspaceStyleMeta(accent string, clear bool) (map[string]any, error) {
	accent = strings.TrimSpace(accent)
	if clear {
		if accent != "" {
			return nil, fmt.Errorf("--clear cannot be combined with --accent")
		}
		return map[string]any{WorkspaceMetaAccent: nil}, nil
	}
	if accent == "" {
		return nil, fmt.Errorf("nothing to set: give --accent or --clear")
	}
	if err := validateColor(accent); err != nil {
		return nil, err
	}
	return map[string]any{WorkspaceMetaAccent: accent}, nil
}

func workspaceStyleRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("workspacestyle", rtnErr == nil)
	}()
	meta, err := buildWorkspaceStyleMeta(workspaceStyleAccent, workspaceStyleClear)
	if err != nil {
		return err
	}
	wsId := workspaceStyleWorkspace
	if wsId == "" {
		wsId = os.Getenv("WAVETERM_WORKSPACEID")
	}
	if wsId == "" {
		return fmt.Errorf("no workspace id specified (use --workspace or set WAVETERM_WORKSPACEID)")
	}
	err = wshclient.SetMetaCommand(RpcClient, wshrpc.CommandSetMetaData{ORef: waveobj.MakeORef(waveobj.OType_Workspace, wsId), Meta: meta}, &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return fmt.Errorf("setting workspace accent: %w", err)
	}
	WriteStdout("workspace style set\n")
	return nil
}
