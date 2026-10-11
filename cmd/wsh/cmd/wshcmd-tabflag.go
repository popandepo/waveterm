// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

const tabFlagName = "tab"

var createTabRef string

// addTabFlag adds --tab to a command that creates a block. Without it the block goes into the tab of the calling block, as before.
func addTabFlag(c *cobra.Command) {
	c.Flags().StringVar(&createTabRef, tabFlagName, "", "create the block in this tab (a tab id, tab:<n>, or any reference that names a tab) instead of the tab of the calling block")
}

// chooseCreateTab decides the tab of a new block. flag is the value of --tab ("" when not given), envTab the tab of the calling block.
// Only the flag is resolved; a missing flag and a missing environment is the old error.
func chooseCreateTab(flag string, envTab string, resolve func(ref string) (string, error)) (string, error) {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		if envTab == "" {
			return "", fmt.Errorf("no WAVETERM_TABID env var set")
		}
		return envTab, nil
	}
	tabId, err := resolve(flag)
	if err != nil {
		return "", fmt.Errorf("resolving --%s: %w", tabFlagName, err)
	}
	return tabId, nil
}

// createTabId is the tab for a new block: --tab when given, else the tab of the calling block.
func createTabId() (string, error) {
	return chooseCreateTab(createTabRef, getTabIdFromEnv(), resolveTabId)
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
func checkCreateTarget(data *wshrpc.CommandCreateBlockData) error {
	if strings.TrimSpace(createTabRef) == "" {
		return nil
	}
	return checkTargetInTab(data.TabId, data.TargetBlockId, blockTabViaRPC)
}
