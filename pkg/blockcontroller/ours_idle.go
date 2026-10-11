// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/wavetermdev/waveterm/pkg/blockstatus"
	"github.com/wavetermdev/waveterm/pkg/remote/conncontroller"
)

const (
	maxLookDepth = 16
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
	if lookDepthExceeded(depth) {
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
	parentCreated, parentErr := p.CreateTime()
	for _, c := range children {
		// Windows keeps the parent id of a process after the parent has ended and reuses freed ids, so a process can name a parent that is a
		// different, newer process. A real child is always created after its parent: one that is not is not below this process.
		childCreated, childErr := c.CreateTime()
		if !plausibleChild(parentCreated, parentErr, childCreated, childErr) {
			continue
		}
		name, nameErr := c.Name()
		if nameErr != nil {
			// a child that ended between the two calls is not there any more; one that is alive and cannot be named, or whose state cannot
			// be read either, makes the look fail (the question is asked)
			running, runErr := c.IsRunning()
			if runErr != nil {
				return runErr
			}
			if !running {
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

// lookDepthExceeded: exactly maxLookDepth levels below the shell are allowed; the process at the deepest level is still asked for its children.
func lookDepthExceeded(depth int) bool {
	return depth > maxLookDepth
}

// observeOutput and observeInput are the two feeds of the prompt tracker: what the shell prints, and what is written to it. The controller's read and
// write loops call them (a test pins both calls).
func (sc *ShellController) observeOutput(b []byte) {
	sc.promptTracker.Output(b)
}

func (sc *ShellController) observeInput(b []byte) {
	sc.promptTracker.Input(b)
}

// plausibleChild: a child must have been created at or after its parent. When either creation time cannot be read the child counts (the safe
// side: it can only make the shell look busy).
func plausibleChild(parentCreated int64, parentErr error, childCreated int64, childErr error) bool {
	if parentErr != nil || childErr != nil {
		return true
	}
	return childCreated >= parentCreated
}

type lookError string

func (e lookError) Error() string { return string(e) }

const errTooMany = lookError("too many processes below the shell")

// the folder whose conhost.exe is the console host of the shell and not a program of the user: the Windows system folder only. Wave's own
// install folder is user-writable and would let a program placed there pass for a helper.
var trustedConsoleDirs = func() []string {
	var dirs []string
	if root := os.Getenv("SystemRoot"); root != "" {
		dirs = append(dirs, filepath.Join(root, "System32"))
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

// isIdle: the shell of this block runs on this machine (not WSL, not SSH), is running, has printed its prompt marker with no Enter sent to it
// since (PromptTracker: this is what separates a prompt from a command that runs inside the shell itself, such as Start-Sleep, which has no
// process below the shell), and has nothing below it but console helpers from the system folder. Anything else (not running, a remote shell,
// no marker ever seen, a look that failed or took too long) is not idle.
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
	atPrompt, generation := sc.promptTracker.AtPrompt()
	if !atPrompt {
		return false
	}
	below, err := lookBelowWithTimeout(pid)
	// the look can take up to two seconds, so the prompt is asked again afterwards: an Enter or a marker during the look changes the generation
	atPromptAfter, generationAfter := sc.promptTracker.AtPrompt()
	if !atPromptAfter || generationAfter != generation {
		return false
	}
	return blockstatus.ShellIsIdle(below, err != nil, trustedConsoleDirs())
}

// newPromptToken makes the token that a PowerShell shell gets in its environment and puts into its prompt markers.
func newPromptToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "" // no token, no tracking: the shell asks
	}
	return hex.EncodeToString(b)
}

// GetShellIdle says whether the block's shell is waiting at its prompt. false for every block that is not a local shell controller.
func GetShellIdle(blockId string) bool {
	sc, ok := getController(blockId).(*ShellController)
	if !ok || sc == nil {
		return false
	}
	return sc.isIdle()
}
