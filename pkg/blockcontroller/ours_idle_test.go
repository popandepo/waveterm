// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/blockstatus"
	"github.com/wavetermdev/waveterm/pkg/shellexec"
)

// The helper process: the test binary started again. WAVE_TEST_HELPER=sleep waits; =parent starts a waiting child of its own and waits.
// Every helper waits until the pipe on its stdin is closed (the test holds the only write end and the helpers share the read end, which each
// child inherits from its parent), or for a safety limit. So the tests never need to find a process to end it: closing the pipe ends them all, and
// only the process that the test started itself is ended through its own handle. NO process is ever found by its parent id: Windows keeps the
// parent id of a process after the parent has ended and reuses freed ids, so that finds unrelated programs.
func waitForEOF() {
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 64)
		for {
			if _, err := os.Stdin.Read(buf); err != nil {
				close(done)
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
	}
}

func TestIdleHelperProcess(t *testing.T) {
	switch os.Getenv("WAVE_TEST_HELPER") {
	case "":
		return
	case "viacopy":
		// the shell: starts the console-helper copy (WAVE_TEST_COPY) in the mode WAVE_TEST_COPYMODE
		c := exec.Command(os.Getenv("WAVE_TEST_COPY"), "-test.run=TestIdleHelperProcess")
		c.Env = append(os.Environ(), "WAVE_TEST_HELPER="+os.Getenv("WAVE_TEST_COPYMODE"))
		c.Stdin = os.Stdin
		if err := c.Start(); err != nil {
			os.Exit(3)
		}
		waitForEOF()
	case "parent", "grand":
		next := "sleep"
		if os.Getenv("WAVE_TEST_HELPER") == "grand" {
			next = "parent"
		}
		self := os.Args[0]
		if real := os.Getenv("WAVE_TEST_REAL"); real != "" {
			self = real
		}
		c := exec.Command(self, "-test.run=TestIdleHelperProcess")
		c.Env = append(os.Environ(), "WAVE_TEST_HELPER="+next)
		c.Stdin = os.Stdin
		if err := c.Start(); err != nil {
			os.Exit(3)
		}
		waitForEOF()
	case "sleep":
		waitForEOF()
	}
}

func startHelper(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=TestIdleHelperProcess")
	c.Env = append(os.Environ(), "WAVE_TEST_HELPER="+mode)
	endHelper := startWithPipe(t, c)
	t.Cleanup(endHelper)
	return c
}

// startWithPipe starts a helper with a pipe on its stdin and returns the function that ends it: closing the pipe ends the helper and everything it
// started (they share the read end), the helper itself is also ended through its own handle, and a short wait lets its children go.
func startWithPipe(t *testing.T, c *exec.Cmd) func() {
	t.Helper()
	w, err := c.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	return func() {
		w.Close()
		c.Process.Kill()
		c.Wait()
		time.Sleep(300 * time.Millisecond)
	}
}

func controllerFor(status string, c *exec.Cmd) *ShellController {
	sc := &ShellController{Lock: &sync.Mutex{}, RunLock: &atomic.Bool{}, ProcStatus: status}
	if c != nil {
		sc.ShellProc = &shellexec.ShellProc{Cmd: shellexec.MakeCmdWrap(c, nil, true)}
	}
	// the shell has printed its prompt marker and nothing was sent to it since, unless a test says otherwise
	sc.promptTracker.SetToken(testToken)
	sc.promptTracker.Output(promptMarker('P'))
	return sc
}

const testToken = "tok"

func promptMarker(kind byte) []byte {
	return []byte("\x1b]16162;" + string(kind) + ";" + testToken + "\a")
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

// a copy of the test binary named conhost.exe in a folder of its own, which the test declares trusted
func consoleHelperCopy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dir, "conhost.exe")
	if err := os.WriteFile(copyPath, data, 0o755); err != nil {
		t.Fatal(err)
	}
	old := trustedConsoleDirs
	trustedConsoleDirs = func() []string { return []string{dir} }
	t.Cleanup(func() { trustedConsoleDirs = old })
	return copyPath
}

func shellViaCopy(t *testing.T, copyPath, copyMode string) *ShellController {
	c := exec.Command(os.Args[0], "-test.run=TestIdleHelperProcess")
	c.Env = append(os.Environ(), "WAVE_TEST_HELPER=viacopy", "WAVE_TEST_COPY="+copyPath, "WAVE_TEST_COPYMODE="+copyMode, "WAVE_TEST_REAL="+os.Args[0])
	endHelper := startWithPipe(t, c)
	t.Cleanup(func() {
		endHelper()
		// the copy must be gone before the folder is removed
		for i := 0; i < 50; i++ {
			if err := os.Remove(copyPath); err == nil || os.IsNotExist(err) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	return controllerFor(Status_Running, c)
}

func TestAConsoleHelperAloneBelowTheShellIsIdle(t *testing.T) {
	copyPath := consoleHelperCopy(t)
	sc := shellViaCopy(t, copyPath, "sleep")
	waitUntil(t, "the helper copy is below the shell", func() bool {
		below, err := listBelow(sc.ShellProc.Cmd.(shellexec.CmdWrap).Cmd.Process.Pid)
		return err == nil && len(below) == 1
	})
	if !sc.isIdle() {
		t.Fatalf("a trusted console helper alone below the shell is idle")
	}
}

func TestAProgramBelowAConsoleHelperIsFoundOnlyByLookingDeeper(t *testing.T) {
	// shell > conhost.exe (trusted, allowed) > a program: the immediate child alone would say idle
	copyPath := consoleHelperCopy(t)
	sc := shellViaCopy(t, copyPath, "parent")
	waitUntil(t, "the program below the helper exists", func() bool {
		below, err := listBelow(sc.ShellProc.Cmd.(shellexec.CmdWrap).Cmd.Process.Pid)
		return err == nil && len(below) >= 2
	})
	if sc.isIdle() {
		t.Fatalf("a program below the console helper must make the shell not idle")
	}
}

func TestALookThatFinishedAfterTheLimitIsRefusedEvenWithItsResultReady(t *testing.T) {
	oldTimeout := lookTimeout
	defer func() { lookTimeout = oldTimeout }()
	lookTimeout = 2 * time.Second
	if _, err := acceptLook(2*time.Second+time.Millisecond, nil, nil); err == nil {
		t.Fatalf("a look that took longer than the limit must be refused")
	}
	if _, err := acceptLook(time.Second, nil, nil); err != nil {
		t.Fatalf("a look inside the limit is accepted: %v", err)
	}
	failed := errors.New("access denied")
	if _, err := acceptLook(time.Second, nil, failed); err != failed {
		t.Fatalf("a failed look keeps its error")
	}
}

func TestAShellThatNeverPrintedThePromptMarkerIsNeverIdleEvenWithNothingBelowIt(t *testing.T) {
	sc := controllerFor(Status_Running, startHelper(t, "sleep"))
	sc.promptTracker.Reset()
	if sc.isIdle() {
		t.Fatalf("no marker seen (integration off, an old shell, cmd.exe): the question is asked")
	}
}

func TestACommandThatRunsInsideTheShellItselfIsNotIdle(t *testing.T) {
	// Start-Sleep, a loop of cmdlets, Copy-Item, Read-Host: no process below the shell, but an Enter was sent after the last prompt marker
	sc := controllerFor(Status_Running, startHelper(t, "sleep"))
	if !sc.isIdle() {
		t.Fatalf("at the prompt with nothing below it is idle")
	}
	sc.promptTracker.Input([]byte("Start-Sleep 20\r"))
	if sc.isIdle() {
		t.Fatalf("a command accepted after the last prompt marker keeps the shell busy although nothing runs below it")
	}
	sc.promptTracker.Output(promptMarker('B'))
	sc.promptTracker.Output(promptMarker('P'))
	if !sc.isIdle() {
		t.Fatalf("the next prompt makes it idle again")
	}
}

func TestOnlyTheWindowsSystemFolderIsTrustedForTheConsoleHost(t *testing.T) {
	t.Setenv("SystemRoot", "C:\\Windows")
	dirs := trustedConsoleDirs()
	if len(dirs) != 1 || !strings.EqualFold(filepath.ToSlash(dirs[0]), "C:/Windows/System32") {
		t.Fatalf("only the system folder is trusted, got %v", dirs)
	}
	exe, _ := os.Executable()
	for _, d := range dirs {
		if strings.EqualFold(d, filepath.Dir(exe)) {
			t.Fatalf("the folder of wavesrv must not be trusted: %v", dirs)
		}
	}
}

func TestAnEnterDuringTheProcessLookMakesTheAnswerNotIdle(t *testing.T) {
	sc := controllerFor(Status_Running, startHelper(t, "sleep"))
	oldList := listBelow
	defer func() { listBelow = oldList }()
	listBelow = func(pid int) ([]blockstatus.ChildProc, error) {
		sc.promptTracker.Input([]byte("Start-Sleep 20\r")) // the owner (or a script) presses Enter while the look is running
		return nil, nil
	}
	if sc.isIdle() {
		t.Fatalf("an Enter during the look must make the answer not idle: the prompt is asked again after the look")
	}
}

func TestAMarkerDuringTheLookAlsoMakesTheAnswerNotIdle(t *testing.T) {
	sc := controllerFor(Status_Running, startHelper(t, "sleep"))
	oldList := listBelow
	defer func() { listBelow = oldList }()
	listBelow = func(pid int) ([]blockstatus.ChildProc, error) {
		sc.promptTracker.Output(promptMarker('P'))
		return nil, nil
	}
	if sc.isIdle() {
		t.Fatalf("any prompt event during the look changes the generation: the answer is not trusted")
	}
}

func TestAChildThatCannotBeNamedOrCheckedFailsTheLookByTheSource(t *testing.T) {
	src, err := os.ReadFile("ours_idle.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	i := strings.Index(text, "name, nameErr := c.Name()")
	if i < 0 {
		t.Fatalf("the naming code moved")
	}
	block := text[i : i+700]
	if !strings.Contains(block, "running, runErr := c.IsRunning()") || !strings.Contains(block, "return runErr") {
		t.Fatalf("a live child that cannot be named, and whose state cannot be read either, must fail the look: %s", block)
	}
}

func TestAShellWithoutATokenIsNeverIdle(t *testing.T) {
	sc := controllerFor(Status_Running, startHelper(t, "sleep"))
	sc.promptTracker.SetToken("") // bash, zsh, cmd.exe, a remote shell: no token was given
	sc.promptTracker.Output(promptMarker('P'))
	if sc.isIdle() {
		t.Fatalf("a shell without a token is never at prompt")
	}
}

func TestOnlyAChildCreatedAfterItsParentIsBelowTheShell(t *testing.T) {
	cases := []struct {
		name                string
		parent, child       int64
		parentErr, childErr error
		want                bool
	}{
		{"created after", 1000, 1500, nil, nil, true},
		{"created in the same millisecond", 1000, 1000, nil, nil, true},
		{"created before: a reused parent id", 1000, 400, nil, nil, false},
		{"child time unknown: counts", 1000, 0, nil, errors.New("access denied"), true},
		{"parent time unknown: counts", 0, 500, errors.New("access denied"), nil, true},
	}
	for _, c := range cases {
		if got := plausibleChild(c.parent, c.parentErr, c.child, c.childErr); got != c.want {
			t.Errorf("%s: plausibleChild = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTheLookAllowsExactlySixteenLevels(t *testing.T) {
	if lookDepthExceeded(16) {
		t.Fatalf("a chain of exactly 16 levels must be looked at, the deepest still asked for its children")
	}
	if !lookDepthExceeded(17) {
		t.Fatalf("a 17th level is over the limit")
	}
}

func TestTheControllerFeedsTheTrackerFromBothLoops(t *testing.T) {
	// the feeds themselves
	sc := &ShellController{Lock: &sync.Mutex{}, RunLock: &atomic.Bool{}, ProcStatus: Status_Running}
	sc.promptTracker.SetToken(testToken)
	sc.observeOutput(promptMarker('P'))
	if at, _ := sc.promptTracker.AtPrompt(); !at {
		t.Fatalf("output must reach the tracker")
	}
	sc.observeInput([]byte("ls\r"))
	if at, _ := sc.promptTracker.AtPrompt(); at {
		t.Fatalf("input must reach the tracker")
	}
	// the loops call them: the output before it is appended to the block file, the input before it is written to the shell
	src, err := os.ReadFile("shellcontroller.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	out := strings.Index(text, "bc.observeOutput(buf[:nr])")
	appendCall := strings.Index(text, "HandleAppendBlockFile(bc.BlockId, wavebase.BlockFile_Term, buf[:nr])")
	if out < 0 || appendCall < 0 || out > appendCall {
		t.Errorf("the read loop must feed the tracker with every chunk the shell prints")
	}
	in := strings.Index(text, "bc.observeInput(ic.InputData)")
	write := strings.Index(text, "shellProc.Cmd.Write(ic.InputData)")
	if in < 0 || write < 0 || in > write {
		t.Errorf("the write loop must feed the tracker BEFORE the input is written to the shell")
	}
	if strings.Count(text, "bc.promptTracker.SetToken(promptToken)") != 1 || strings.Count(text, "bc.promptTracker.Reset()") != 1 {
		t.Errorf("the start path must set the token and reset the tracker")
	}
}
