// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestChooseCreateTab(t *testing.T) {
	resolve := func(ref string) (string, error) {
		if ref == "tab:2" {
			return "tab-two-id", nil
		}
		return "", errors.New("no such tab")
	}
	if got, err := chooseCreateTab("", "env-tab", resolve); err != nil || got != "env-tab" {
		t.Fatalf("without --tab the tab of the calling block is used, got %q %v", got, err)
	}
	if got, err := chooseCreateTab("  ", "env-tab", resolve); err != nil || got != "env-tab" {
		t.Fatalf("a blank --tab is not given, got %q %v", got, err)
	}
	if got, err := chooseCreateTab("tab:2", "env-tab", resolve); err != nil || got != "tab-two-id" {
		t.Fatalf("--tab wins over the environment, got %q %v", got, err)
	}
	if got, err := chooseCreateTab("tab:2", "", resolve); err != nil || got != "tab-two-id" {
		t.Fatalf("--tab works without a calling block's tab, got %q %v", got, err)
	}
	if _, err := chooseCreateTab("", "", resolve); err == nil || !strings.Contains(err.Error(), "WAVETERM_TABID") {
		t.Fatalf("no flag and no environment is the old error, got %v", err)
	}
	if _, err := chooseCreateTab("tab:9", "env-tab", resolve); err == nil || !strings.Contains(err.Error(), "--tab") {
		t.Fatalf("an unknown tab is refused and says --tab, got %v", err)
	}
}

func TestATargetBlockMustBeInTheTabOfTheNewBlock(t *testing.T) {
	lookup := func(id string) (string, error) {
		switch id {
		case "b-in-two":
			return "tab-two", nil
		case "b-in-one":
			return "tab-one", nil
		}
		return "", errors.New("unknown block")
	}
	if err := checkTargetInTab("tab-two", "", lookup); err != nil {
		t.Fatalf("no target, nothing to check: %v", err)
	}
	if err := checkTargetInTab("tab-two", "b-in-two", lookup); err != nil {
		t.Fatalf("a target in the same tab is fine: %v", err)
	}
	if err := checkTargetInTab("tab-two", "b-in-one", lookup); err == nil || !strings.Contains(err.Error(), "tab-one") {
		t.Fatalf("a target in another tab is refused and names it, got %v", err)
	}
	if err := checkTargetInTab("tab-two", "b-gone", lookup); err == nil {
		t.Fatalf("a target that cannot be looked up is refused")
	}
}

func TestTheFourCommandsHaveTheTabFlagAndUseOnlyTheSharedChoice(t *testing.T) {
	for _, c := range []struct {
		name string
		file string
	}{{"createblock", "wshcmd-createblock.go"}, {"term", "wshcmd-term.go"}, {"run", "wshcmd-run.go"}, {"web open", "wshcmd-web.go"}} {
		src, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		if !strings.Contains(text, "addTabFlag(") {
			t.Errorf("%s: no --tab flag", c.name)
		}
		if !strings.Contains(text, "createTabId()") {
			t.Errorf("%s: the tab is not chosen by createTabId", c.name)
		}
		if strings.Contains(text, "getTabIdFromEnv()") {
			t.Errorf("%s: still reads the environment's tab directly", c.name)
		}
		stack := strings.Index(text, "applyStackTarget(")
		check := strings.Index(text, "checkCreateTarget(")
		if stack < 0 || check < stack {
			t.Errorf("%s: the target tab is not checked after the stack target is applied", c.name)
		}
	}
}

func TestTheTabFlagDefaultsToTheCallingBlocksTab(t *testing.T) {
	for name, flagValue := range map[string]string{
		"createblock": createBlockCmd.Flags().Lookup(tabFlagName).DefValue,
		"term":        termCmd.Flags().Lookup(tabFlagName).DefValue,
		"run":         runCmd.Flags().Lookup(tabFlagName).DefValue,
		"web open":    webOpenCmd.Flags().Lookup(tabFlagName).DefValue,
	} {
		if flagValue != "" {
			t.Errorf("%s: --tab must default to empty (the calling block's tab), got %q", name, flagValue)
		}
	}
}
