// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"github.com/shirou/gopsutil/v4/process"
	"github.com/wavetermdev/waveterm/pkg/blockstatus"
)

// the names of the child processes of a process; a variable so that a test can stand in for a failing look
var listChildNames = func(pid int) ([]string, error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil, err
	}
	children, err := p.Children()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, c := range children {
		name, err := c.Name()
		if err != nil {
			// a child that ended between the two calls is not there any more; any other failure counts as a child we cannot name
			names = append(names, "?")
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// isIdle: the shell of this block runs on this machine, is running, and waits at its prompt (no child process but console helpers).
// Anything else (not running, a remote shell, a look that failed) is not idle.
func (sc *ShellController) isIdle() bool {
	sc.Lock.Lock()
	status := sc.ProcStatus
	proc := sc.ShellProc
	sc.Lock.Unlock()
	if status != Status_Running {
		return false
	}
	pid, ok := proc.LocalPid()
	if !ok {
		return false
	}
	names, err := listChildNames(pid)
	return blockstatus.ShellIsIdle(names, err != nil)
}

// GetShellIdle says whether the block's shell is waiting at its prompt. false for every block that is not a local shell controller.
func GetShellIdle(blockId string) bool {
	sc, ok := getController(blockId).(*ShellController)
	if !ok || sc == nil {
		return false
	}
	return sc.isIdle()
}
