// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func TestChooseNodeSize(t *testing.T) {
	if got, err := chooseNodeSize(false, 0); got != nil || err != nil {
		t.Fatalf("not given must give nothing: %v %v", got, err)
	}
	for _, bad := range []uint{0, 101, 100000} {
		if got, err := chooseNodeSize(true, bad); err == nil || got != nil {
			t.Fatalf("%d must be refused: %v %v", bad, got, err)
		}
	}
	for _, good := range []uint{1, 7, 10, 100} {
		got, err := chooseNodeSize(true, good)
		if err != nil || got == nil || *got != good {
			t.Fatalf("%d must be accepted: %v %v", good, got, err)
		}
	}
}

func TestApplyCreateSizeSetsTheRequestField(t *testing.T) {
	c := &cobra.Command{Use: "x"}
	addSizeFlag(c)
	data := &wshrpc.CommandCreateBlockData{}
	if err := applyCreateSize(c, data); err != nil || data.NodeSize != nil {
		t.Fatalf("without the flag nothing is set: %v %v", err, data.NodeSize)
	}
	if err := c.Flags().Set(sizeFlagName, "7"); err != nil {
		t.Fatal(err)
	}
	if err := applyCreateSize(c, data); err != nil || data.NodeSize == nil || *data.NodeSize != 7 {
		t.Fatalf("--size 7: %v %v", err, data.NodeSize)
	}
	if err := c.Flags().Set(sizeFlagName, "0"); err != nil {
		t.Fatal(err)
	}
	if err := applyCreateSize(c, data); err == nil {
		t.Fatal("--size 0 must be refused")
	}
}

func TestEveryBlockCreatingCommandHasTheFlagAndTheCheckAppliesIt(t *testing.T) {
	for _, c := range []*cobra.Command{createBlockCmd, runCmd, termCmd, webOpenCmd} {
		if c.Flags().Lookup(sizeFlagName) == nil {
			t.Errorf("%s has no --%s", c.Use, sizeFlagName)
		}
	}
	src, err := os.ReadFile("wshcmd-tabflag.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "applyCreateSize(cmd, data)") {
		t.Error("checkCreateTarget must apply the size")
	}
}

func TestTheServerPassesTheWeightToEveryInsertAndSplit(t *testing.T) {
	src, err := os.ReadFile("../../../pkg/wshrpc/wshserver/wshserver.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "NodeSize:      data.NodeSize"); n != 4 {
		t.Errorf("the four splits must carry NodeSize, found %d", n)
	}
	if n := strings.Count(string(src), "NodeSize:   data.NodeSize"); n != 1 {
		t.Errorf("the plain insert must carry NodeSize, found %d", n)
	}
	replace := string(src)[strings.Index(string(src), `case "replace":`):strings.Index(string(src), `case "splitright":`)]
	if strings.Contains(replace, "NodeSize") {
		t.Error("a replace keeps the tile of the block it replaces: no NodeSize there")
	}
}
