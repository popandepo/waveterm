// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"os"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// The restart mode of `wsh session start` relies on exactly this decision: a block that is marked runonstart and not
// run once is started again whenever its status is init, which is what every start of the app finds.
func TestRestartModeBlocksStartAtEveryAppStart(t *testing.T) {
	if !shouldRunCommand(false, true, Status_Init, false) {
		t.Fatalf("runonstart without runonce must start at init")
	}
	if shouldRunCommand(false, true, Status_Running, false) {
		t.Fatalf("a running program must not be started a second time")
	}
	if shouldRunCommand(false, true, Status_Done, false) {
		t.Fatalf("a program that has ended is not restarted by the controller on its own")
	}
}

func TestOneShotBlocksAndPlainShellsKeepTheirRules(t *testing.T) {
	if !shouldRunCommand(true, false, Status_Init, false) {
		t.Fatalf("run once starts at init")
	}
	if shouldRunCommand(false, false, Status_Init, false) {
		t.Fatalf("neither flag: no automatic start")
	}
	if !shouldRunCommand(false, false, Status_Done, true) {
		t.Fatalf("a forced start always runs")
	}
}

// The meta that `wsh session start --restart` writes, read back the way the controller reads it, must lead to a start at init.
func TestRestartMetaLeadsToAStartThroughTheControllersOwnReader(t *testing.T) {
	meta := waveobj.MetaMapType{
		waveobj.MetaKey_CmdRunOnce:    false,
		waveobj.MetaKey_CmdRunOnStart: true,
	}
	runOnce := getBoolFromMeta(meta, waveobj.MetaKey_CmdRunOnce, false)
	runOnStart := getBoolFromMeta(meta, waveobj.MetaKey_CmdRunOnStart, true)
	if !shouldRunCommand(runOnce, runOnStart, Status_Init, false) {
		t.Fatalf("the restart meta must start the block at init")
	}
	once := waveobj.MetaMapType{waveobj.MetaKey_CmdRunOnce: true, waveobj.MetaKey_CmdRunOnStart: true}
	if !shouldRunCommand(getBoolFromMeta(once, waveobj.MetaKey_CmdRunOnce, false), getBoolFromMeta(once, waveobj.MetaKey_CmdRunOnStart, true), Status_Init, false) {
		t.Fatalf("the first start of a one-shot block still runs")
	}
}

// A test of the decision function alone would stay green if the controller stopped calling it. The full start path
// needs a database and a process, so this pins the call site in the source: a change there makes this test fail.
func TestControllerStartPathUsesTheDecisionFunction(t *testing.T) {
	src, err := os.ReadFile("shellcontroller.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "if shouldRunCommand(runOnce, runOnStart, curStatus.ShellProcStatus, force) {") {
		t.Fatalf("the shell controller must start blocks through shouldRunCommand")
	}
	if strings.Contains(string(src), "(runOnStart || runOnce) && curStatus.ShellProcStatus == Status_Init") {
		t.Fatalf("the old inline condition is back: the decision would exist twice")
	}
}
