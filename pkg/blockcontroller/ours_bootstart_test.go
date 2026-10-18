// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// fakeShell stands in for the shell controller: Start counts as one process start, and with endsAtOnce the command is
// over before anyone looks (a short command).
type fakeShell struct {
	lock       sync.Mutex
	blockId    string
	status     string
	starts     int
	stops      int
	endsAtOnce bool
}

func (f *fakeShell) Start(ctx context.Context, blockMeta waveobj.MetaMapType, rtOpts *waveobj.RuntimeOpts, force bool) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.starts++
	f.status = Status_Running
	if f.endsAtOnce {
		f.status = Status_Done
	}
	return nil
}

func (f *fakeShell) Stop(graceful bool, newStatus string, destroy bool) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.stops++
	f.status = newStatus
}

func (f *fakeShell) GetRuntimeStatus() *BlockControllerRuntimeStatus {
	f.lock.Lock()
	defer f.lock.Unlock()
	return &BlockControllerRuntimeStatus{BlockId: f.blockId, ShellProcStatus: f.status}
}

func (f *fakeShell) GetConnName() string                    { return "" }
func (f *fakeShell) SendInput(input *BlockInputUnion) error { return nil }

type bootTestEnv struct {
	ctx   context.Context
	fakes map[string]*fakeShell
	made  int
	ends  bool
}

func initBootTest(t *testing.T) *bootTestEnv {
	t.Helper()
	dataDir := t.TempDir()
	old := wavebase.DataHome_VarCache
	wavebase.DataHome_VarCache = dataDir
	t.Cleanup(func() { wavebase.DataHome_VarCache = old })
	if err := os.MkdirAll(filepath.Join(dataDir, wavebase.WaveDBDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := wstore.InitWStore(); err != nil {
		t.Fatalf("init wstore: %v", err)
	}
	t.Cleanup(func() { _ = wstore.CloseDB() })
	env := &bootTestEnv{ctx: context.Background(), fakes: map[string]*fakeShell{}}
	oldMake := makeShellControllerFn
	t.Cleanup(func() { makeShellControllerFn = oldMake })
	makeShellControllerFn = func(tabId string, blockId string, controllerType string, connName string) Controller {
		f := &fakeShell{blockId: blockId, status: Status_Init, endsAtOnce: env.ends}
		env.fakes[blockId] = f
		env.made++
		return f
	}
	t.Cleanup(func() {
		for id := range env.fakes {
			deleteController(id)
		}
	})
	return env
}

func (e *bootTestEnv) insertTab(t *testing.T) string {
	t.Helper()
	tab := &waveobj.Tab{OID: uuid.NewString(), Name: "T"}
	if err := wstore.DBInsert(e.ctx, tab); err != nil {
		t.Fatal(err)
	}
	return tab.OID
}

func (e *bootTestEnv) insertBlock(t *testing.T, tabId string, meta waveobj.MetaMapType) string {
	t.Helper()
	block := &waveobj.Block{OID: uuid.NewString(), ParentORef: waveobj.MakeORef(waveobj.OType_Tab, tabId).String(), Meta: meta}
	if err := wstore.DBInsert(e.ctx, block); err != nil {
		t.Fatal(err)
	}
	return block.OID
}

// What a mounted terminal view sends with its resync.
func viewSize() *waveobj.RuntimeOpts {
	return &waveobj.RuntimeOpts{TermSize: waveobj.TermSize{Rows: 24, Cols: 80}}
}

func restartMeta() waveobj.MetaMapType {
	return waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": "x", "cmd:runonstart": true}
}

func (e *bootTestEnv) starts(blockId string) int {
	f := e.fakes[blockId]
	if f == nil {
		return 0
	}
	return f.starts
}

// A restart block (runonstart, not run once) in a tab that nobody ever shows is started by the backend itself at start.
// Other blocks wait for their view, as before; a block whose tab is gone is not started.
func TestRestartBlocksStartAtBackendStartWithoutTheirTabBeingShown(t *testing.T) {
	env := initBootTest(t)
	hiddenTab := env.insertTab(t)
	restart := env.insertBlock(t, hiddenTab, restartMeta())
	plain := env.insertBlock(t, hiddenTab, waveobj.MetaMapType{"view": "term", "controller": "shell"})
	once := env.insertBlock(t, hiddenTab, waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": "x", "cmd:runonstart": true, "cmd:runonce": true})
	off := env.insertBlock(t, hiddenTab, waveobj.MetaMapType{"view": "term", "controller": "cmd", "cmd": "x", "cmd:runonstart": false})
	web := env.insertBlock(t, hiddenTab, waveobj.MetaMapType{"view": "web", "cmd:runonstart": true})
	orphan := env.insertBlock(t, uuid.NewString(), restartMeta())

	StartRestartBlocks(env.ctx)

	if env.starts(restart) != 1 {
		t.Fatalf("the restart block must be started once, got %d", env.starts(restart))
	}
	for name, id := range map[string]string{"plain": plain, "runonce": once, "runonstart false": off, "web": web, "orphan (tab gone)": orphan} {
		if env.starts(id) != 0 {
			t.Fatalf("%s must not be started at backend start", name)
		}
	}
}

// The frontend mounts the view later and resyncs: a running program is left alone.
func TestALaterMountDoesNotStartASecondProcess(t *testing.T) {
	env := initBootTest(t)
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())

	StartRestartBlocks(env.ctx)
	if err := ResyncController(env.ctx, tab, id, viewSize(), false); err != nil {
		t.Fatal(err)
	}
	if env.starts(id) != 1 {
		t.Fatalf("one process only, got %d starts", env.starts(id))
	}
}

// A short command is over before its view mounts. The first mount must not destroy the done controller and run it again.
func TestAShortCommandIsNotRunAgainByTheFirstMount(t *testing.T) {
	env := initBootTest(t)
	env.ends = true
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())

	StartRestartBlocks(env.ctx)
	first := env.fakes[id]
	if first == nil || first.starts != 1 {
		t.Fatalf("the boot pass must start the command once")
	}
	if err := ResyncController(env.ctx, tab, id, viewSize(), false); err != nil {
		t.Fatal(err)
	}
	if env.fakes[id] != first || first.starts != 1 || first.stops != 0 {
		t.Fatalf("the first mount must leave the done controller alone (starts=%d stops=%d)", first.starts, first.stops)
	}
}

// Only the first mount is excused: a later mount of an ended restart block behaves as it always did.
func TestTheSecondMountOfAnEndedBlockRestartsAsBefore(t *testing.T) {
	env := initBootTest(t)
	env.ends = true
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())

	StartRestartBlocks(env.ctx)
	_ = ResyncController(env.ctx, tab, id, viewSize(), false)
	_ = ResyncController(env.ctx, tab, id, viewSize(), false)
	if env.made != 2 || env.fakes[id].starts != 1 {
		t.Fatalf("the second mount must create a fresh controller and start it, got %d controllers (starts on the new one: %d)", env.made, env.fakes[id].starts)
	}
}

// The list the boot pass reads can be stale: eligibility is checked again when the block is started.
func TestABlockThatStoppedBeingARestartBlockIsNotStarted(t *testing.T) {
	env := initBootTest(t)
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())
	if err := wstore.UpdateObjectMeta(env.ctx, waveobj.MakeORef(waveobj.OType_Block, id), map[string]any{"cmd:runonce": true}, false); err != nil {
		t.Fatal(err)
	}

	started, err := bootStartBlock(env.ctx, id)
	if err != nil || started || env.starts(id) != 0 {
		t.Fatalf("a block marked run once meanwhile must not be started (started=%v err=%v)", started, err)
	}
}

// A mount that got there first has already given the block a controller: the boot pass does not touch it.
func TestTheBootPassLeavesABlockThatAMountAlreadyStarted(t *testing.T) {
	env := initBootTest(t)
	env.ends = true
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())

	if err := ResyncController(env.ctx, tab, id, viewSize(), false); err != nil {
		t.Fatal(err)
	}
	StartRestartBlocks(env.ctx)
	if env.starts(id) != 1 || env.made != 1 {
		t.Fatalf("one start from the mount only, got starts=%d controllers=%d", env.starts(id), env.made)
	}
}

// A resync that does not come from a mounted view (no terminal size) must not use up the exemption: the view's own first
// resync is the one that is spared.
func TestAResyncThatIsNotAViewMountKeepsTheExemptionForTheMount(t *testing.T) {
	env := initBootTest(t)
	env.ends = true
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())

	StartRestartBlocks(env.ctx)
	if err := ResyncController(env.ctx, tab, id, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := ResyncController(env.ctx, tab, id, viewSize(), false); err != nil {
		t.Fatal(err)
	}
	if env.made != 1 || env.starts(id) != 1 {
		t.Fatalf("one run only: controllers=%d starts=%d", env.made, env.starts(id))
	}
}

// The block can lose runonstart after the boot pass checked it and before it starts: it is read again at the start.
func TestRunOnStartRemovedBetweenTheCheckAndTheStartIsHonoured(t *testing.T) {
	env := initBootTest(t)
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())
	old := afterBootEligibilityCheck
	t.Cleanup(func() { afterBootEligibilityCheck = old })
	afterBootEligibilityCheck = func(blockId string) {
		err := wstore.UpdateObjectMeta(env.ctx, waveobj.MakeORef(waveobj.OType_Block, blockId), map[string]any{"cmd:runonstart": false}, false)
		if err != nil {
			t.Error(err)
		}
	}

	started, err := bootStartBlock(env.ctx, id)
	if err != nil || started || env.made != 0 {
		t.Fatalf("a block that lost runonstart in between must not be started (started=%v err=%v controllers=%d)", started, err, env.made)
	}
}

// A resync that is not a view mount starts the short command before the boot pass gets to the block. The command ends, the
// boot pass finds a controller and leaves it, and the first mount must still not run it again.
func TestACommandStartedBeforeTheBootPassIsNotRunAgainByTheFirstMount(t *testing.T) {
	env := initBootTest(t)
	env.ends = true
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())

	if err := ResyncController(env.ctx, tab, id, nil, false); err != nil {
		t.Fatal(err)
	}
	StartRestartBlocks(env.ctx)
	if err := ResyncController(env.ctx, tab, id, viewSize(), false); err != nil {
		t.Fatal(err)
	}
	if env.made != 1 || env.starts(id) != 1 {
		t.Fatalf("one run only: controllers=%d starts=%d", env.made, env.starts(id))
	}
}

// Non-view start, then the first mount, then the boot pass: one run, whatever the order.
func TestAMountBetweenANonViewStartAndTheBootPassDoesNotRerunTheCommand(t *testing.T) {
	env := initBootTest(t)
	env.ends = true
	tab := env.insertTab(t)
	id := env.insertBlock(t, tab, restartMeta())

	if err := ResyncController(env.ctx, tab, id, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := ResyncController(env.ctx, tab, id, viewSize(), false); err != nil {
		t.Fatal(err)
	}
	StartRestartBlocks(env.ctx)
	if env.made != 1 || env.starts(id) != 1 {
		t.Fatalf("one run only: controllers=%d starts=%d", env.made, env.starts(id))
	}
}
