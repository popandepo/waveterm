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
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

var tabNewCmd = &cobra.Command{
	Use:   "new [name]",
	Short: "Create a tab and print its id",
	Long: `Create a new tab in the workspace of the calling block (or --workspace) and print the new tab id on
standard output. The tab is activated unless --no-activate is given. With no name the next default name is used.`,
	Args:    cobra.MaximumNArgs(1),
	RunE:    tabNewRun,
	PreRunE: preRunSetupRpcClient,
}

var tabRenameCmd = &cobra.Command{
	Use:   "rename <name>",
	Short: "Rename a tab",
	Long: `Rename the tab this command runs in, or the tab given by --tab (tab id, first 8 characters, tab:<n> or
tab:<uuid>). The tab bar shows at most 14 characters.`,
	Args:    cobra.ExactArgs(1),
	RunE:    tabRenameRun,
	PreRunE: preRunSetupRpcClient,
}

var tabSwitchCmd = &cobra.Command{
	Use:     "switch <tab>",
	Short:   "Make a tab the active tab of its window",
	Long:    "Show the given tab (tab id, first 8 characters, tab:<n> or tab:<uuid>). This switches the tab view of the window; it does not raise the window.",
	Args:    cobra.ExactArgs(1),
	RunE:    tabSwitchRun,
	PreRunE: preRunSetupRpcClient,
}

var tabCloseCmd = &cobra.Command{
	Use:   "close <tab>",
	Short: "Close a tab and the blocks in it",
	Long: `Close the given tab (tab id, first 8 characters, tab:<n> or tab:<uuid>). Its blocks, and the processes
running in them, are closed without a confirmation. A tab must be named explicitly; there is no default.`,
	Args:    cobra.ExactArgs(1),
	RunE:    tabCloseRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	tabNewWorkspace  string
	tabNewNoActivate bool
	tabRenameTab     string
)

func init() {
	tabNewCmd.Flags().StringVar(&tabNewWorkspace, "workspace", "", "workspace id (defaults to WAVETERM_WORKSPACEID)")
	tabNewCmd.Flags().BoolVar(&tabNewNoActivate, "no-activate", false, "create the tab without switching to it")
	tabRenameCmd.Flags().StringVar(&tabRenameTab, "tab", "", "tab to rename (default: the tab this command runs in)")
	tabCommand.AddCommand(tabNewCmd)
	tabCommand.AddCommand(tabRenameCmd)
	tabCommand.AddCommand(tabSwitchCmd)
	tabCommand.AddCommand(tabCloseCmd)
}

func findWorkspaceIdForTab(workspaces []wshrpc.WorkspaceInfoData, tabId string) (string, error) {
	for _, w := range workspaces {
		if w.WorkspaceData == nil {
			continue
		}
		for _, id := range w.WorkspaceData.TabIds {
			if id == tabId {
				return w.WorkspaceData.OID, nil
			}
		}
	}
	return "", fmt.Errorf("no workspace found containing tab %s", tabId)
}

func resolveTabId(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "tab"
	}
	oref, err := resolveSimpleId(ref)
	if err != nil {
		return "", fmt.Errorf("resolving tab: %w", err)
	}
	if oref.OType != waveobj.OType_Tab {
		return "", fmt.Errorf("%q is a %s, not a tab", ref, oref.OType)
	}
	return oref.OID, nil
}

func resolveTabAndWorkspace(ref string) (string, string, error) {
	tabId, err := resolveTabId(ref)
	if err != nil {
		return "", "", err
	}
	workspaces, err := wshclient.WorkspaceListCommand(RpcClient, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return "", "", fmt.Errorf("listing workspaces: %w", err)
	}
	wsId, err := findWorkspaceIdForTab(workspaces, tabId)
	if err != nil {
		return "", "", err
	}
	return tabId, wsId, nil
}

func tabNewRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("tabnew", rtnErr == nil)
	}()
	wsId := tabNewWorkspace
	if wsId == "" {
		wsId = os.Getenv("WAVETERM_WORKSPACEID")
	}
	if wsId == "" {
		return fmt.Errorf("no workspace id specified (use --workspace or set WAVETERM_WORKSPACEID)")
	}
	data := wshrpc.CommandElectronCreateTabData{WorkspaceId: wsId, NoActivate: tabNewNoActivate}
	if len(args) > 0 {
		data.Name = args[0]
	}
	tabId, err := wshclient.ElectronCreateTabCommand(RpcClient, data, &wshrpc.RpcOpts{Timeout: 15000, Route: wshutil.ElectronRoute})
	if err != nil {
		return fmt.Errorf("creating tab: %w", err)
	}
	WriteStdout("%s\n", tabId)
	return nil
}

func tabRenameRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("tabrename", rtnErr == nil)
	}()
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("tab name must not be empty")
	}
	tabId, err := resolveTabId(tabRenameTab)
	if err != nil {
		return err
	}
	err = wshclient.UpdateTabNameCommand(RpcClient, tabId, name, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return fmt.Errorf("renaming tab: %w", err)
	}
	WriteStdout("tab renamed\n")
	return nil
}

func tabSwitchRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("tabswitch", rtnErr == nil)
	}()
	tabId, wsId, err := resolveTabAndWorkspace(args[0])
	if err != nil {
		return err
	}
	err = wshclient.ElectronSetActiveTabCommand(RpcClient, wshrpc.CommandElectronTabData{WorkspaceId: wsId, TabId: tabId}, &wshrpc.RpcOpts{Timeout: 15000, Route: wshutil.ElectronRoute})
	if err != nil {
		return fmt.Errorf("switching tab: %w", err)
	}
	WriteStdout("tab switched\n")
	return nil
}

func tabCloseRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("tabclose", rtnErr == nil)
	}()
	tabId, wsId, err := resolveTabAndWorkspace(args[0])
	if err != nil {
		return err
	}
	err = wshclient.ElectronCloseTabCommand(RpcClient, wshrpc.CommandElectronTabData{WorkspaceId: wsId, TabId: tabId}, &wshrpc.RpcOpts{Timeout: 15000, Route: wshutil.ElectronRoute})
	if err != nil {
		return fmt.Errorf("closing tab: %w", err)
	}
	WriteStdout("tab closed\n")
	return nil
}
