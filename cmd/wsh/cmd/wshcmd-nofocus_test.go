// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// payloadFor runs a command's RunE with the create-block call replaced and returns the request it sent.
func payloadFor(t *testing.T, c *cobra.Command, noFocusValue bool, args ...string) wshrpc.CommandCreateBlockData {
	t.Helper()
	t.Setenv("WAVETERM_TABID", "tab-1")
	var got []wshrpc.CommandCreateBlockData
	oldRPC := createBlockRPC
	createBlockRPC = func(d wshrpc.CommandCreateBlockData) (waveobj.ORef, error) {
		got = append(got, d)
		return waveobj.ORef{OType: waveobj.OType_Block, OID: "blk"}, nil
	}
	oldFlag := noFocus
	defer func() {
		createBlockRPC = oldRPC
		noFocus = oldFlag
	}()
	noFocus = false
	if noFocusValue {
		if err := c.Flags().Set(noFocusFlagName, "true"); err != nil {
			t.Fatalf("set --%s: %v", noFocusFlagName, err)
		}
		defer c.Flags().Set(noFocusFlagName, "false")
	}
	if err := c.RunE(c, args); err != nil {
		t.Fatalf("%s: %v", c.Name(), err)
	}
	if len(got) != 1 {
		t.Fatalf("%s sent %d create-block requests, want 1", c.Name(), len(got))
	}
	return got[0]
}

func TestBlockCommandsHonourNoFocus(t *testing.T) {
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
			if tc.setup != nil {
				defer tc.setup()()
			}
			if got := payloadFor(t, tc.cmd, true, tc.args...); got.Focused {
				t.Errorf("--no-focus sent Focused=true")
			}
			if got := payloadFor(t, tc.cmd, false, tc.args...); !got.Focused {
				t.Errorf("without --no-focus the request must stay Focused=true (upstream behaviour)")
			}
		})
	}
}
