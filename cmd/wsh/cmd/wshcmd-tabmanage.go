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
standard output. The tab is NOT shown and the active tab does not change unless --activate is given; its page is
loaded in the background so programs started in it run. The tab starts with Wave's default layout (one terminal)
unless --empty is given. With no name the next default name is used.`,
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
running in them, are closed without a confirmation. A tab must be named explicitly; there is no default. A tab that holds blocks is NOT closed unless --force is given (the command says how many blocks it holds); there is no dialog on this route.`,
	Args:    cobra.ExactArgs(1),
	RunE:    tabCloseRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	tabNewWorkspace string
	tabNewActivate  bool
	tabNewEmpty     bool
	tabRenameTab    string
)

func init() {
	tabNewCmd.Flags().StringVar(&tabNewWorkspace, "workspace", "", "workspace id (defaults to WAVETERM_WORKSPACEID)")
	tabNewCmd.Flags().BoolVar(&tabNewActivate, "activate", false, "switch to the new tab (default: leave the active tab alone)")
	tabNewCmd.Flags().BoolVar(&tabNewEmpty, "empty", false, "create the tab with no blocks instead of the default layout")
	tabRenameCmd.Flags().StringVar(&tabRenameTab, "tab", "", "tab to rename (default: the tab this command runs in)")
	tabCommand.AddCommand(tabNewCmd)
	tabCommand.AddCommand(tabRenameCmd)
	tabCommand.AddCommand(tabSwitchCmd)
	tabCloseCmd.Flags().BoolVar(&tabCloseForce, "force", false, "close the tab even if it holds blocks (their programs are stopped)")
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
	workspaces, err := wshclient.WorkspaceListAllCommand(RpcClient, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return "", "", fmt.Errorf("listing workspaces: %w", err)
	}
	wsId, err := findWorkspaceIdForTab(workspaces, tabId)
	if err != nil {
		return "", "", err
	}
	return tabId, wsId, nil
}

// tabCreator is the part of the RPC client that creating a tab needs; tests replace it.
type tabCreator interface {
	CreateTab(data wshrpc.CommandCreateTabData) (string, error)
	PreloadTab(data wshrpc.CommandElectronTabData) error
	SetActiveTab(data wshrpc.CommandElectronTabData) error
}

type rpcTabCreator struct{}

func (rpcTabCreator) CreateTab(data wshrpc.CommandCreateTabData) (string, error) {
	return wshclient.CreateTabCommand(RpcClient, data, &wshrpc.RpcOpts{Timeout: 15000})
}

func (rpcTabCreator) PreloadTab(data wshrpc.CommandElectronTabData) error {
	return wshclient.ElectronPreloadTabCommand(RpcClient, data, &wshrpc.RpcOpts{Timeout: 30000, Route: wshutil.ElectronRoute})
}

func (rpcTabCreator) SetActiveTab(data wshrpc.CommandElectronTabData) error {
	return wshclient.ElectronSetActiveTabCommand(RpcClient, data, &wshrpc.RpcOpts{Timeout: 15000, Route: wshutil.ElectronRoute})
}

// createBackgroundTab creates the tab, loads its page hidden, and switches to it only when activate is set. When a
// step after the creation fails it still returns the tab id, so the caller can print it and manage or close the tab.
func createBackgroundTab(c tabCreator, wsId string, name string, empty bool, activate bool) (string, error) {
	tabId, err := c.CreateTab(wshrpc.CommandCreateTabData{WorkspaceId: wsId, Name: name, Empty: empty})
	if err != nil {
		return "", fmt.Errorf("creating tab: %w", err)
	}
	tabData := wshrpc.CommandElectronTabData{WorkspaceId: wsId, TabId: tabId}
	if err := c.PreloadTab(tabData); err != nil {
		return tabId, fmt.Errorf("tab %s was created but its page could not be loaded in the background: %w", tabId, err)
	}
	if activate {
		if err := c.SetActiveTab(tabData); err != nil {
			return tabId, fmt.Errorf("tab %s was created but could not be activated: %w", tabId, err)
		}
	}
	return tabId, nil
}

func resolveWorkspaceIdForNewTab(flagValue string) (string, error) {
	wsId := flagValue
	if wsId == "" {
		wsId = os.Getenv("WAVETERM_WORKSPACEID")
	}
	if wsId == "" {
		return "", fmt.Errorf("no workspace id specified (use --workspace or set WAVETERM_WORKSPACEID)")
	}
	return wsId, nil
}

func tabNewRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("tabnew", rtnErr == nil)
	}()
	wsId, err := resolveWorkspaceIdForNewTab(tabNewWorkspace)
	if err != nil {
		return err
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	tabId, err := createBackgroundTab(rpcTabCreator{}, wsId, name, tabNewEmpty, tabNewActivate)
	if tabId != "" {
		WriteStdout("%s\n", tabId)
	}
	return err
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

// A tab that holds blocks is not closed by a command unless --force is given: closing deletes the tab and stops its
// programs and cannot be undone. No dialog is used on this route, because the callers are scripts and agents that
// would wait for it forever.
func checkTabCloseAllowed(blockCount int, countErr error, force bool) error {
	if force {
		return nil
	}
	if countErr != nil {
		return fmt.Errorf("could not read the tab to see whether it holds blocks (%v); use --force to close it anyway", countErr)
	}
	if blockCount == 0 {
		return nil
	}
	what := fmt.Sprintf("%d blocks", blockCount)
	if blockCount == 1 {
		what = "1 block"
	}
	return fmt.Errorf("the tab holds %s; closing it stops the programs in them and cannot be undone: use --force to close it anyway", what)
}

var tabBlockCount = func(tabId string) (int, error) {
	tab, err := wshclient.GetTabCommand(RpcClient, tabId, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return 0, err
	}
	return len(tab.BlockIds), nil
}

var tabCloseForce bool

func tabCloseRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("tabclose", rtnErr == nil)
	}()
	tabId, wsId, err := resolveTabAndWorkspace(args[0])
	if err != nil {
		return err
	}
	count, countErr := tabBlockCount(tabId)
	if err := checkTabCloseAllowed(count, countErr, tabCloseForce); err != nil {
		return err
	}
	err = wshclient.ElectronCloseTabCommand(RpcClient, wshrpc.CommandElectronTabData{WorkspaceId: wsId, TabId: tabId}, &wshrpc.RpcOpts{Timeout: 15000, Route: wshutil.ElectronRoute})
	if err != nil {
		return fmt.Errorf("closing tab: %w", err)
	}
	WriteStdout("tab closed\n")
	return nil
}
