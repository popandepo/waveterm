// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

const (
	tabTwoFull = "22222222-2222-2222-2222-222222222222"
	blkOneFull = "33333333-3333-3333-3333-333333333333"
)

func TestChooseCreateTab(t *testing.T) {
	resolve := func(ref string) (string, error) {
		switch ref {
		case "tab:2":
			return "tab-two-id", nil
		case tabTwoFull, "tab:" + tabTwoFull:
			return tabTwoFull, nil
		case "tab":
			return "env-tab", nil
		}
		return "", errors.New("no such tab")
	}
	if got, err := chooseCreateTab(false, "", "env-tab", resolve); err != nil || got != "env-tab" {
		t.Fatalf("without --tab the tab of the calling block is used, got %q %v", got, err)
	}
	if got, err := chooseCreateTab(true, "tab:2", "env-tab", resolve); err != nil || got != "tab-two-id" {
		t.Fatalf("--tab wins over the environment, got %q %v", got, err)
	}
	if got, err := chooseCreateTab(true, tabTwoFull, "", resolve); err != nil || got != tabTwoFull {
		t.Fatalf("a full tab id works without a calling block's tab, got %q %v", got, err)
	}
	if got, err := chooseCreateTab(true, "tab:"+tabTwoFull, "", resolve); err != nil || got != tabTwoFull {
		t.Fatalf("tab:<full id> works, got %q %v", got, err)
	}
	if got, err := chooseCreateTab(true, "tab", "env-tab", resolve); err != nil || got != "env-tab" {
		t.Fatalf("the word tab names the caller's own tab, got %q %v", got, err)
	}
	if _, err := chooseCreateTab(false, "", "", resolve); err == nil || !strings.Contains(err.Error(), "WAVETERM_TABID") {
		t.Fatalf("no flag and no environment is the old error, got %v", err)
	}
	if _, err := chooseCreateTab(true, "tab:9", "env-tab", resolve); err == nil || !strings.Contains(err.Error(), "--tab") {
		t.Fatalf("an unknown tab is refused and says --tab, got %v", err)
	}
}

func TestAGivenButEmptyTabFlagIsRefusedNotIgnored(t *testing.T) {
	resolve := func(ref string) (string, error) { return "should-not-be-asked", nil }
	for _, empty := range []string{"", " ", "\t"} {
		if got, err := chooseCreateTab(true, empty, "env-tab", resolve); err == nil || got != "" {
			t.Fatalf("--tab %q must be refused, got %q %v", empty, got, err)
		}
	}
}

func TestIdPrefixesAndOtherFormsAreRefusedBeforeAnyLookup(t *testing.T) {
	asked := false
	resolve := func(ref string) (string, error) { asked = true; return "x", nil }
	for _, bad := range []string{"22222222", "2222", "block:" + blkOneFull, "workspace", "this", "tab:1000", "tab:-1", "../x", "tab:22222222"} {
		if _, err := chooseCreateTab(true, bad, "env-tab", resolve); err == nil {
			t.Errorf("--tab %q must be refused", bad)
		}
	}
	if asked {
		t.Fatalf("a refused form must not reach the resolver")
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

// The command path: with the create request replaced, a command with --tab and a stack or replace target in ANOTHER tab sends nothing.
func stubTabEnv(t *testing.T, blockTab string) *[]wshrpc.CommandCreateBlockData {
	t.Helper()
	t.Setenv("WAVETERM_TABID", "caller-tab")
	var creates []wshrpc.CommandCreateBlockData
	oldCreate, oldBlockRef, oldResolveTab, oldBlockTab, oldTarget, oldFocus, oldTabRef := createBlockRPC, resolveBlockRef, resolveTabFn, blockTabFn, stackTarget, noFocus, createTabRef
	createBlockRPC = func(d wshrpc.CommandCreateBlockData) (waveobj.ORef, error) {
		creates = append(creates, d)
		return waveobj.ORef{OType: waveobj.OType_Block, OID: "blk"}, nil
	}
	resolveBlockRef = func(ref string) (string, error) { return blkOneFull, nil }
	resolveTabFn = func(ref string) (string, error) { return tabTwoFull, nil }
	blockTabFn = func(blockId string) (string, error) { return blockTab, nil }
	stackTarget, noFocus = "", false
	t.Cleanup(func() {
		createBlockRPC, resolveBlockRef, resolveTabFn, blockTabFn, stackTarget, noFocus, createTabRef = oldCreate, oldBlockRef, oldResolveTab, oldBlockTab, oldTarget, oldFocus, oldTabRef
	})
	return &creates
}

type tabCase struct {
	name  string
	cmd   *cobra.Command
	args  []string
	setup func() func()
	// the flag that names the target block
	targetFlag string
}

func tabCases() []tabCase {
	setRunCommand := func() func() {
		runCmd.Flags().Set("command", "echo hi")
		return func() { runCmd.Flags().Set("command", "") }
	}
	return []tabCase{
		{"createblock", createBlockCmd, []string{"term"}, nil, stackFlagName},
		{"term", termCmd, nil, nil, stackFlagName},
		{"run", runCmd, nil, setRunCommand, stackFlagName},
		{"web open", webOpenCmd, []string{"http://127.0.0.1:1/"}, nil, stackFlagName},
		{"web open --replace", webOpenCmd, []string{"http://127.0.0.1:1/"}, nil, "replace"},
	}
}

func runWithTab(t *testing.T, tc tabCase, tabValue string, target string) error {
	t.Helper()
	if tc.setup != nil {
		defer tc.setup()()
	}
	if err := tc.cmd.Flags().Set(tabFlagName, tabValue); err != nil {
		t.Fatal(err)
	}
	defer func() {
		tc.cmd.Flags().Lookup(tabFlagName).Changed = false
		createTabRef = ""
	}()
	if target != "" {
		if err := tc.cmd.Flags().Set(tc.targetFlag, target); err != nil {
			t.Fatal(err)
		}
		defer func() {
			tc.cmd.Flags().Lookup(tc.targetFlag).Changed = false
			stackTarget, webOpenReplaceBlock = "", ""
		}()
	}
	return tc.cmd.RunE(tc.cmd, tc.args)
}

func TestACrossTabStackOrReplaceTargetSendsNoCreateRequest(t *testing.T) {
	for _, tc := range tabCases() {
		creates := stubTabEnv(t, "some-other-tab")
		err := runWithTab(t, tc, tabTwoFull, "2")
		if err == nil || !strings.Contains(err.Error(), "some-other-tab") {
			t.Errorf("%s: a target in another tab must be refused and name it, got %v", tc.name, err)
		}
		if len(*creates) != 0 {
			t.Errorf("%s: a refused request must not be sent, but %d went out", tc.name, len(*creates))
		}
	}
}

func TestATargetInTheChosenTabIsSentWithThatTab(t *testing.T) {
	for _, tc := range tabCases() {
		creates := stubTabEnv(t, tabTwoFull)
		if err := runWithTab(t, tc, tabTwoFull, "2"); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(*creates) != 1 || (*creates)[0].TabId != tabTwoFull || (*creates)[0].TargetBlockId != blkOneFull {
			t.Errorf("%s: want one request in tab %s targeting %s, got %+v", tc.name, tabTwoFull, blkOneFull, *creates)
		}
	}
}

func TestAnEmptyTabFlagOnTheCommandSendsNothing(t *testing.T) {
	for _, tc := range tabCases()[:4] {
		creates := stubTabEnv(t, tabTwoFull)
		if err := runWithTab(t, tc, "", ""); err == nil {
			t.Errorf("%s: an empty --tab must be refused", tc.name)
		}
		if len(*creates) != 0 {
			t.Errorf("%s: an empty --tab must not create a block in the caller's tab", tc.name)
		}
	}
}

func TestWithoutTheFlagTheCallersTabIsUsed(t *testing.T) {
	for _, tc := range tabCases()[:4] {
		creates := stubTabEnv(t, "caller-tab")
		if tc.setup != nil {
			defer tc.setup()()
		}
		if err := tc.cmd.RunE(tc.cmd, tc.args); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(*creates) != 1 || (*creates)[0].TabId != "caller-tab" {
			t.Errorf("%s: want one request in the caller's tab, got %+v", tc.name, *creates)
		}
	}
}

func TestTheFourCommandsHaveTheTabFlagThatDefaultsToEmpty(t *testing.T) {
	for _, c := range []struct {
		name string
		file string
		cmd  *cobra.Command
	}{{"createblock", "wshcmd-createblock.go", createBlockCmd}, {"term", "wshcmd-term.go", termCmd}, {"run", "wshcmd-run.go", runCmd}, {"web open", "wshcmd-web.go", webOpenCmd}} {
		f := c.cmd.Flags().Lookup(tabFlagName)
		if f == nil || f.DefValue != "" {
			t.Errorf("%s: --tab must exist and default to empty", c.name)
		}
		src, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "getTabIdFromEnv()") {
			t.Errorf("%s: still reads the environment's tab directly", c.name)
		}
	}
}
