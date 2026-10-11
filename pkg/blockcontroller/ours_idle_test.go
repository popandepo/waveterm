// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/blockstatus"
	"github.com/wavetermdev/waveterm/pkg/shellexec"
)

// The helper process: the test binary started again. WAVE_TEST_HELPER=sleep sleeps; =parent starts a sleeping child of its own and sleeps.
func TestIdleHelperProcess(t *testing.T) {
	switch os.Getenv("WAVE_TEST_HELPER") {
	case "":
		return
	case "parent", "grand":
		next := "sleep"
		if os.Getenv("WAVE_TEST_HELPER") == "grand" {
			next = "parent"
		}
		c := exec.Command(os.Args[0], "-test.run=TestIdleHelperProcess")
		c.Env = append(os.Environ(), "WAVE_TEST_HELPER="+next)
		if err := c.Start(); err != nil {
			os.Exit(3)
		}
		time.Sleep(10 * time.Second)
	case "sleep":
		time.Sleep(10 * time.Second)
	}
}

func startHelper(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=TestIdleHelperProcess")
	c.Env = append(os.Environ(), "WAVE_TEST_HELPER="+mode)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.Process.Kill()
		c.Wait()
	})
	return c
}

func controllerFor(status string, c *exec.Cmd) *ShellController {
	sc := &ShellController{Lock: &sync.Mutex{}, RunLock: &atomic.Bool{}, ProcStatus: status}
	if c != nil {
		sc.ShellProc = &shellexec.ShellProc{Cmd: shellexec.MakeCmdWrap(c, nil, true)}
	}
	return sc
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func TestAShellWithNoChildIsIdleAndOneWithAChildIsNot(t *testing.T) {
	idle := controllerFor(Status_Running, startHelper(t, "sleep"))
	busy := controllerFor(Status_Running, startHelper(t, "parent"))
	waitUntil(t, "the busy shell has its child", func() bool { return !busy.isIdle() })
	if !idle.isIdle() {
		t.Fatalf("a shell with no child process must be idle")
	}
	if busy.isIdle() {
		t.Fatalf("a shell with a child process must not be idle")
	}
}

func TestAnythingThatIsNotAPlainRunningLocalShellIsNotIdle(t *testing.T) {
	c := startHelper(t, "sleep")
	if controllerFor(Status_Done, c).isIdle() || controllerFor(Status_Init, c).isIdle() {
		t.Fatalf("a shell that is not running is not idle")
	}
	if controllerFor(Status_Running, nil).isIdle() {
		t.Fatalf("a controller without a process is not idle")
	}
	remote := controllerFor(Status_Running, nil)
	remote.ShellProc = &shellexec.ShellProc{}
	if remote.isIdle() {
		t.Fatalf("a process that is not a local pty command is not idle")
	}
}

func TestAFailedLookIsNotIdle(t *testing.T) {
	old := listBelow
	defer func() { listBelow = old }()
	listBelow = func(pid int) ([]blockstatus.ChildProc, error) { return nil, errors.New("access denied") }
	if controllerFor(Status_Running, startHelper(t, "sleep")).isIdle() {
		t.Fatalf("a look that failed must not say idle")
	}
}

func TestGetShellIdleIsFalseWithoutAController(t *testing.T) {
	if GetShellIdle("00000000-0000-0000-0000-00000000beef") {
		t.Fatalf("no controller, no idle")
	}
}

func TestAProcessTwoLevelsBelowTheShellCounts(t *testing.T) {
	// the shell starts "parent", which starts "sleep": the shell has a child, and that child has a child
	deep := controllerFor(Status_Running, startHelper(t, "grand"))
	waitUntil(t, "the grandchild exists", func() bool {
		below, err := listBelow(deep.ShellProc.Cmd.(shellexec.CmdWrap).Cmd.Process.Pid)
		return err == nil && len(below) >= 2
	})
	if deep.isIdle() {
		t.Fatalf("a shell with a grandchild running must not be idle")
	}
}

func TestAShellOnAnotherConnectionIsNotIdleEvenWhenItHasAProcessId(t *testing.T) {
	for _, conn := range []string{"wsl://Ubuntu", "user@host", "ssh:somewhere"} {
		sc := controllerFor(Status_Running, startHelper(t, "sleep"))
		sc.ShellProc.ConnName = conn
		if sc.isIdle() {
			t.Fatalf("connection %q must not be idle", conn)
		}
	}
	for _, conn := range []string{"", "local", "local:gitbash"} {
		sc := controllerFor(Status_Running, startHelper(t, "sleep"))
		sc.ShellProc.ConnName = conn
		if !sc.isIdle() {
			t.Fatalf("connection %q is a local shell with nothing below it", conn)
		}
	}
}

func TestALookThatTakesTooLongIsNotIdle(t *testing.T) {
	oldList, oldTimeout := listBelow, lookTimeout
	defer func() { listBelow, lookTimeout = oldList, oldTimeout }()
	release := make(chan struct{})
	defer close(release)
	listBelow = func(pid int) ([]blockstatus.ChildProc, error) {
		<-release
		return nil, nil
	}
	lookTimeout = 50 * time.Millisecond
	start := time.Now()
	if controllerFor(Status_Running, startHelper(t, "sleep")).isIdle() {
		t.Fatalf("a look that did not finish must not say idle")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("the look held the caller up")
	}
}
