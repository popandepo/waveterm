// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import "testing"

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
