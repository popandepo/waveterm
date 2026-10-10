// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

const stackTargetFull = "11111111-1111-1111-1111-111111111111"

func stubStackEnv(t *testing.T) (*[]wshrpc.CommandCreateBlockData, *[]wshrpc.CommandStackBlockData) {
	t.Helper()
	t.Setenv("WAVETERM_TABID", "tab-1")
	var creates []wshrpc.CommandCreateBlockData
	var stacks []wshrpc.CommandStackBlockData
	oldCreate, oldStack, oldResolve, oldTarget, oldFocus := createBlockRPC, stackBlockRPC, resolveBlockRef, stackTarget, noFocus
	createBlockRPC = func(d wshrpc.CommandCreateBlockData) (waveobj.ORef, error) {
		creates = append(creates, d)
		return waveobj.ORef{OType: waveobj.OType_Block, OID: "blk"}, nil
	}
	stackBlockRPC = func(d wshrpc.CommandStackBlockData) error {
		stacks = append(stacks, d)
		return nil
	}
	resolveBlockRef = func(ref string) (string, error) {
		if ref == "2" || ref == "term" || ref == "this" {
			return stackTargetFull, nil
		}
		return ref, nil
	}
	stackTarget, noFocus = "", false
	t.Cleanup(func() {
		createBlockRPC, stackBlockRPC, resolveBlockRef, stackTarget, noFocus = oldCreate, oldStack, oldResolve, oldTarget, oldFocus
	})
	return &creates, &stacks
}

func TestCreateCommandsAcceptStack(t *testing.T) {
	setRunCommand := func() func() {
		runCmd.Flags().Set("command", "echo hi")
		return func() { runCmd.Flags().Set("command", "") }
	}
	cases := []struct {
		name  string
		cmd   *cobra.Command
		args  []string
		setup func() func()
	}{
		{"createblock", createBlockCmd, []string{"term"}, nil},
		{"term", termCmd, nil, nil},
		{"web open", webOpenCmd, []string{"http://127.0.0.1:1/"}, nil},
		{"run", runCmd, nil, setRunCommand},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			creates, _ := stubStackEnv(t)
			if tc.setup != nil {
				defer tc.setup()()
			}
			if err := tc.cmd.Flags().Set(stackFlagName, "2"); err != nil {
				t.Fatalf("--%s is not a flag of %s: %v", stackFlagName, tc.name, err)
			}
			defer tc.cmd.Flags().Set(stackFlagName, "")
			if err := tc.cmd.RunE(tc.cmd, tc.args); err != nil {
				t.Fatal(err)
			}
			if len(*creates) != 1 {
				t.Fatalf("%d create requests", len(*creates))
			}
			got := (*creates)[0]
			if got.TargetBlockId != stackTargetFull || got.TargetAction != wshrpc.CreateBlockAction_Stack {
				t.Errorf("target %q action %q", got.TargetBlockId, got.TargetAction)
			}
		})
	}
}

func TestCreateWithoutStackFlagHasNoTarget(t *testing.T) {
	creates, _ := stubStackEnv(t)
	if err := createBlockCmd.RunE(createBlockCmd, []string{"term"}); err != nil {
		t.Fatal(err)
	}
	if got := (*creates)[0]; got.TargetBlockId != "" || got.TargetAction != "" {
		t.Fatalf("a plain create got a target: %+v", got)
	}
}

func TestStackConflictsAreRefusedBeforeAnyRequest(t *testing.T) {
	creates, _ := stubStackEnv(t)
	webOpenCmd.Flags().Set(stackFlagName, "2")
	defer webOpenCmd.Flags().Set(stackFlagName, "")
	webOpenCmd.Flags().Set("replace", "block:22222222-2222-2222-2222-222222222222")
	defer webOpenCmd.Flags().Set("replace", "")
	err := webOpenCmd.RunE(webOpenCmd, []string{"http://127.0.0.1:1/"})
	if err == nil || !strings.Contains(err.Error(), "--stack") {
		t.Fatalf("--stack with --replace should be refused, got %v", err)
	}
	webOpenCmd.Flags().Set("replace", "")
	webOpenCmd.Flags().Set("magnified", "true")
	defer webOpenCmd.Flags().Set("magnified", "false")
	err = webOpenCmd.RunE(webOpenCmd, []string{"http://127.0.0.1:1/"})
	if err == nil || !strings.Contains(err.Error(), "--stack") {
		t.Fatalf("--stack with --magnified should be refused, got %v", err)
	}
	if len(*creates) != 0 {
		t.Fatalf("a refused command still sent %d requests", len(*creates))
	}
}

func runGroup(t *testing.T, c *cobra.Command, setFlags map[string]string) error {
	t.Helper()
	for k, v := range setFlags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("flag %s: %v", k, err)
		}
	}
	defer func() {
		for k := range setFlags {
			c.Flags().Set(k, c.Flags().Lookup(k).DefValue)
		}
	}()
	return c.RunE(c, nil)
}

func TestGroupAddSendsStackWithoutFocusByDefault(t *testing.T) {
	_, stacks := stubStackEnv(t)
	groupBlockOverride = "block-one"
	defer func() { groupBlockOverride = "" }()
	if err := runGroup(t, groupAddCmd, map[string]string{"to": "2"}); err != nil {
		t.Fatal(err)
	}
	got := (*stacks)[0]
	if got.Action != wshrpc.StackBlockAction_Stack || got.BlockId != "block-one" || got.TargetBlockId != stackTargetFull || got.Focused {
		t.Fatalf("request %+v", got)
	}
	if err := runGroup(t, groupAddCmd, map[string]string{"to": "2", "focus": "true"}); err != nil {
		t.Fatal(err)
	}
	if !(*stacks)[1].Focused {
		t.Fatalf("--focus must ask for focus")
	}
}

func TestGroupRemoveAndActivate(t *testing.T) {
	_, stacks := stubStackEnv(t)
	groupBlockOverride = "block-one"
	defer func() { groupBlockOverride = "" }()
	if err := runGroup(t, groupRemoveCmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := runGroup(t, groupActivateCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(*stacks) != 2 || (*stacks)[0].Action != wshrpc.StackBlockAction_Unstack || (*stacks)[1].Action != wshrpc.StackBlockAction_Activate {
		t.Fatalf("requests %+v", *stacks)
	}
	if (*stacks)[0].Focused {
		t.Fatalf("remove must not take focus by default")
	}
}

func TestGroupAddNeedsATarget(t *testing.T) {
	_, stacks := stubStackEnv(t)
	groupBlockOverride = "block-one"
	defer func() { groupBlockOverride = "" }()
	if err := runGroup(t, groupAddCmd, nil); err == nil {
		t.Fatalf("group add without --to should be refused")
	}
	if len(*stacks) != 0 {
		t.Fatalf("a refused command sent a request")
	}
}

func TestQueuedMessageSaysNotConfirmed(t *testing.T) {
	msg := queuedMessage("stack", "blk")
	if !strings.Contains(msg, "queued") || !strings.Contains(msg, "not confirmed") || strings.Contains(strings.ToLower(msg), "done") {
		t.Fatalf("a queued change must not read as applied: %q", msg)
	}
}
