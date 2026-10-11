// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

const tabFlagName = "tab"

var createTabRef string

// the forms of a tab reference that --tab accepts: the caller's own tab, the n-th tab of the caller's workspace, and a full tab id.
// An 8-character prefix is NOT accepted: the server takes the first object that starts with it, whatever its type, and does not notice
// that more than one matches.
var (
	tabRefNum  = regexp.MustCompile(`^tab:\d{1,3}$`)
	tabRefUUID = regexp.MustCompile(`^(tab:)?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// seams for tests
var (
	resolveTabFn = resolveTabId
	blockTabFn   = blockTabViaRPC
)

// addTabFlag adds --tab to a command that creates a block. Without it the block goes into the tab of the calling block, as before.
func addTabFlag(c *cobra.Command) {
	c.Flags().StringVar(&createTabRef, tabFlagName, "", "create the block in this tab: a full tab id (any workspace or window), tab:<n> (the n-th tab of the caller's workspace) or tab (the caller's own); default: the tab of the calling block")
}

// chooseCreateTab decides the tab of a new block. given says whether --tab was on the command line (its presence counts, not its value: an
// empty value is an error, never "not given"); flag is its value, envTab the tab of the calling block.
func chooseCreateTab(given bool, flag string, envTab string, resolve func(ref string) (string, error)) (string, error) {
	if !given {
		if envTab == "" {
			return "", fmt.Errorf("no WAVETERM_TABID env var set")
		}
		return envTab, nil
	}
	ref := strings.TrimSpace(flag)
	if ref == "" {
		return "", fmt.Errorf("--%s needs a tab: a full tab id, tab:<n> or tab", tabFlagName)
	}
	if ref != "tab" && !tabRefNum.MatchString(ref) && !tabRefUUID.MatchString(ref) {
		return "", fmt.Errorf("--%s %q is not accepted: use a full tab id, tab:<n> or tab (an id prefix can match more than one object)", tabFlagName, ref)
	}
	tabId, err := resolve(ref)
	if err != nil {
		return "", fmt.Errorf("resolving --%s: %w", tabFlagName, err)
	}
	return tabId, nil
}

// createTabId is the tab for a new block: --tab when given, else the tab of the calling block.
func createTabId(cmd *cobra.Command) (string, error) {
	return chooseCreateTab(cmd.Flags().Changed(tabFlagName), createTabRef, getTabIdFromEnv(), resolveTabFn)
}

// checkTargetInTab refuses a request whose target block (--stack, --replace) is in another tab than the new block: the layout of the tab
// that the block is created in cannot place a block next to, into the stack of, or in place of a block that is not in it.
func checkTargetInTab(tabId string, targetBlockId string, blockTab func(blockId string) (string, error)) error {
	if targetBlockId == "" {
		return nil
	}
	targetTab, err := blockTab(targetBlockId)
	if err != nil {
		return fmt.Errorf("looking up the tab of the target block: %w", err)
	}
	if targetTab != tabId {
		return fmt.Errorf("the target block is in tab %s, not in tab %s: --%s needs the stack or the block to replace to be in that tab", targetTab, tabId, tabFlagName)
	}
	return nil
}

func blockTabViaRPC(blockId string) (string, error) {
	info, err := wshclient.BlockInfoCommand(RpcClient, blockId, nil)
	if err != nil {
		return "", err
	}
	return info.TabId, nil
}

// checkCreateTarget runs after the target fields are set: with --tab the target must be in that tab.
func checkCreateTarget(cmd *cobra.Command, data *wshrpc.CommandCreateBlockData) error {
	if !cmd.Flags().Changed(tabFlagName) {
		return nil
	}
	return checkTargetInTab(data.TabId, data.TargetBlockId, blockTabFn)
}
