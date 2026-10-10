// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "testing"

func TestNoFocusFlagOnBlockCommands(t *testing.T) {
	cmds := map[string]bool{
		"createblock": createBlockCmd.Flags().Lookup(noFocusFlagName) != nil,
		"run":         runCmd.Flags().Lookup(noFocusFlagName) != nil,
		"term":        termCmd.Flags().Lookup(noFocusFlagName) != nil,
		"web open":    webOpenCmd.Flags().Lookup(noFocusFlagName) != nil,
	}
	for name, has := range cmds {
		if !has {
			t.Errorf("%s has no --%s flag", name, noFocusFlagName)
		}
	}
}

func TestNewBlockFocusedFollowsFlag(t *testing.T) {
	old := noFocus
	defer func() { noFocus = old }()

	noFocus = false
	if !newBlockFocused() {
		t.Errorf("default must keep the upstream behaviour: new block is focused")
	}
	if err := createBlockCmd.Flags().Set(noFocusFlagName, "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	defer createBlockCmd.Flags().Set(noFocusFlagName, "false")
	if newBlockFocused() {
		t.Errorf("--no-focus must make the new block unfocused")
	}
}
