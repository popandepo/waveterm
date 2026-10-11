// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/wavetermdev/waveterm/pkg/blockstatus"
	"github.com/wavetermdev/waveterm/pkg/remote/conncontroller"
)

const (
	maxLookDepth = 8
	maxLookProcs = 500
	// the look must not hold a close up: a look that takes longer counts as failed (the question is asked)
	idleLookTimeout = 2 * time.Second
)

// the look at the processes below a pid; a variable so that a test can stand in for a failing or a slow look
var listBelow = func(pid int) ([]blockstatus.ChildProc, error) {
	var found []blockstatus.ChildProc
	err := collectBelow(int32(pid), 0, &found)
	return found, err
}

var lookTimeout = idleLookTimeout

// collectBelow adds the children of a process, and their children and so on, to found. A look that goes deeper or wider than the limits is an
// error (so: not idle). A child that has ended between the two calls is not there any more and is skipped.
func collectBelow(pid int32, depth int, found *[]blockstatus.ChildProc) error {
	if depth >= maxLookDepth {
		return errTooMany
	}
	p, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	children, err := p.Children()
	if err != nil {
		return err
	}
	for _, c := range children {
		name, nameErr := c.Name()
		if nameErr != nil {
			if running, _ := c.IsRunning(); !running {
				continue
			}
			name = "?"
		}
		exe, _ := c.Exe()
		*found = append(*found, blockstatus.ChildProc{Name: name, Exe: exe})
		if len(*found) > maxLookProcs {
			return errTooMany
		}
		if err := collectBelow(c.Pid, depth+1, found); err != nil {
			return err
		}
	}
	return nil
}

type lookError string

func (e lookError) Error() string { return string(e) }

const errTooMany = lookError("too many processes below the shell")

// the folders whose conhost.exe and OpenConsole.exe are the console host of the shell and not a program of the user
var trustedConsoleDirs = func() []string {
	var dirs []string
	if root := os.Getenv("SystemRoot"); root != "" {
		dirs = append(dirs, filepath.Join(root, "System32"))
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	return dirs
}

// the timer and the result can both be ready when the select runs, and select may take either: a look that took longer than the limit is never
// trusted, whichever it took
func acceptLook(elapsed time.Duration, below []blockstatus.ChildProc, err error) ([]blockstatus.ChildProc, error) {
	if elapsed > lookTimeout {
		return nil, lookError("the look at the processes took too long")
	}
	return below, err
}

func lookBelowWithTimeout(pid int) ([]blockstatus.ChildProc, error) {
	type result struct {
		below []blockstatus.ChildProc
		err   error
	}
	ch := make(chan result, 1)
	start := time.Now()
	go func() {
		below, err := listBelow(pid)
		ch <- result{below, err}
	}()
	select {
	case r := <-ch:
		return acceptLook(time.Since(start), r.below, r.err)
	case <-time.After(lookTimeout):
		return nil, lookError("the look at the processes took too long")
	}
}

// isIdle: the shell of this block runs on this machine (not WSL, not SSH), is running, and waits at its prompt: nothing below it but console
// helpers from a trusted folder. Anything else (not running, a remote shell, a look that failed or took too long) is not idle.
func (sc *ShellController) isIdle() bool {
	sc.Lock.Lock()
	status := sc.ProcStatus
	proc := sc.ShellProc
	sc.Lock.Unlock()
	if status != Status_Running || proc == nil || !conncontroller.IsLocalConnName(proc.ConnName) {
		return false
	}
	pid, ok := proc.LocalPid()
	if !ok {
		return false
	}
	below, err := lookBelowWithTimeout(pid)
	return blockstatus.ShellIsIdle(below, err != nil, trustedConsoleDirs())
}

// GetShellIdle says whether the block's shell is waiting at its prompt. false for every block that is not a local shell controller.
func GetShellIdle(blockId string) bool {
	sc, ok := getController(blockId).(*ShellController)
	if !ok || sc == nil {
		return false
	}
	return sc.isIdle()
}
