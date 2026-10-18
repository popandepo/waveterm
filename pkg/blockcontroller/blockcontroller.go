// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/blocklogger"
	"github.com/wavetermdev/waveterm/pkg/filestore"
	"github.com/wavetermdev/waveterm/pkg/jobcontroller"
	"github.com/wavetermdev/waveterm/pkg/remote"
	"github.com/wavetermdev/waveterm/pkg/remote/conncontroller"
	"github.com/wavetermdev/waveterm/pkg/util/ds"
	"github.com/wavetermdev/waveterm/pkg/util/shellutil"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wconfig"
	"github.com/wavetermdev/waveterm/pkg/wps"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wslconn"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const (
	BlockController_Shell   = "shell"
	BlockController_Cmd     = "cmd"
	BlockController_Tsunami = "tsunami"
)

const (
	Status_Running = "running"
	Status_Done    = "done"
	Status_Init    = "init"
)

const (
	DefaultTermMaxFileSize = 2 * 1024 * 1024
	DefaultHtmlMaxFileSize = 256 * 1024
	MaxInitScriptSize      = 50 * 1024
)

const DefaultTimeout = 2 * time.Second
const DefaultGracefulKillWait = 400 * time.Millisecond

type BlockInputUnion struct {
	InputData []byte            `json:"inputdata,omitempty"`
	SigName   string            `json:"signame,omitempty"`
	TermSize  *waveobj.TermSize `json:"termsize,omitempty"`
}

type BlockControllerRuntimeStatus struct {
	BlockId           string `json:"blockid"`
	Version           int64  `json:"version"`
	ShellProcStatus   string `json:"shellprocstatus,omitempty"`
	ShellProcConnName string `json:"shellprocconnname,omitempty"`
	ShellProcExitCode int    `json:"shellprocexitcode"`
	TsunamiPort       int    `json:"tsunamiport,omitempty"`
}

// Controller interface that all block controllers must implement
type Controller interface {
	Start(ctx context.Context, blockMeta waveobj.MetaMapType, rtOpts *waveobj.RuntimeOpts, force bool) error
	Stop(graceful bool, newStatus string, destroy bool)
	GetRuntimeStatus() *BlockControllerRuntimeStatus // does not return nil
	GetConnName() string
	SendInput(input *BlockInputUnion) error
}

// Registry for all controllers
var (
	controllerRegistry  = make(map[string]Controller)
	registryLock        sync.RWMutex
	blockResyncMutexMap = ds.MakeSyncMap[*sync.Mutex]()
)

func getBlockResyncMutex(blockId string) *sync.Mutex {
	return blockResyncMutexMap.GetOrCreate(blockId, func() *sync.Mutex {
		return &sync.Mutex{}
	})
}

// Registry operations
func getController(blockId string) Controller {
	registryLock.RLock()
	defer registryLock.RUnlock()
	return controllerRegistry[blockId]
}

func registerController(blockId string, controller Controller) {
	var existingController Controller

	registryLock.Lock()
	existing, exists := controllerRegistry[blockId]
	if exists {
		existingController = existing
	}
	controllerRegistry[blockId] = controller
	registryLock.Unlock()

	if existingController != nil {
		existingController.Stop(false, Status_Done, true)
		wstore.DeleteRTInfo(waveobj.MakeORef(waveobj.OType_Block, blockId))
	}
}

func deleteController(blockId string) {
	registryLock.Lock()
	defer registryLock.Unlock()
	delete(controllerRegistry, blockId)
}

func getAllControllers() map[string]Controller {
	registryLock.RLock()
	defer registryLock.RUnlock()
	// Return a copy to avoid lock issues
	result := make(map[string]Controller)
	for k, v := range controllerRegistry {
		result[k] = v
	}
	return result
}

func InitBlockController() {
	rpcClient := wshclient.GetBareRpcClient()
	rpcClient.EventListener.On(wps.Event_BlockClose, handleBlockCloseEvent)
	wshclient.EventSubCommand(rpcClient, wps.SubscriptionRequest{
		Event:     wps.Event_BlockClose,
		AllScopes: true,
	}, nil)
}

func handleBlockCloseEvent(event *wps.WaveEvent) {
	blockId, ok := event.Data.(string)
	if !ok {
		log.Printf("[blockclose] invalid event data type")
		return
	}
	go DestroyBlockController(blockId)
}

// Public API Functions

var makeShellControllerFn = MakeShellController

// Test seam: runs after the boot pass has checked a block and before it starts it.
var afterBootEligibilityCheck = func(blockId string) {}

// Restart blocks started since backend start, by anyone, that no mounted view has resynced yet. A command that ends before
// its view mounts leaves a done controller, and the first mount must not destroy and rerun it.
var restartStartedBlocks = ds.MakeSyncMap[bool]()

// StartRestartBlocks starts every restart block (cmd:runonstart, not run once) at backend start. Without it a block waits
// for the frontend to mount its view, which never happens for a tab nobody shows after a reboot.
func StartRestartBlocks(ctx context.Context) {
	blocks, err := wstore.DBGetAllObjsByType[*waveobj.Block](ctx, waveobj.OType_Block)
	if err != nil {
		log.Printf("[bootstart] error listing blocks: %v\n", err)
		return
	}
	for _, block := range blocks {
		if !isRestartBlock(block) {
			continue
		}
		started, err := bootStartBlock(ctx, block.OID)
		if err != nil {
			log.Printf("[bootstart] block %s: %v\n", block.OID, err)
			continue
		}
		if started {
			log.Printf("[bootstart] started restart block %s\n", block.OID)
		}
	}
}

// bootStartBlock decides under the block's resync lock, from the block as it is now: the list the boot pass read may be
// stale, and a frontend resync may already have given the block a controller.
func bootStartBlock(ctx context.Context, blockId string) (bool, error) {
	mu := getBlockResyncMutex(blockId)
	mu.Lock()
	defer mu.Unlock()

	block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
	if err != nil {
		return false, fmt.Errorf("error getting block: %w", err)
	}
	if block == nil || !isRestartBlock(block) {
		return false, nil
	}
	if getController(blockId) != nil {
		return false, nil
	}
	tabId, err := wstore.DBFindTabForBlockId(ctx, blockId)
	if err != nil {
		return false, fmt.Errorf("no tab: %w", err)
	}
	tab, err := wstore.DBGet[*waveobj.Tab](ctx, tabId)
	if err != nil {
		return false, fmt.Errorf("error getting tab: %w", err)
	}
	if tab == nil {
		return false, fmt.Errorf("tab %s is gone", tabId)
	}
	afterBootEligibilityCheck(blockId)
	err = resyncControllerLocked(ctx, tabId, blockId, nil, false, true, false)
	if err != nil {
		return false, err
	}
	return getController(blockId) != nil, nil
}

// Only an explicit runonstart counts: for the shell controller an unset runonstart defaults to true, which would start every terminal.
func isRestartBlock(block *waveobj.Block) bool {
	controllerName := block.Meta.GetString(waveobj.MetaKey_Controller, "")
	if controllerName != BlockController_Shell && controllerName != BlockController_Cmd {
		return false
	}
	return getBoolFromMeta(block.Meta, waveobj.MetaKey_CmdRunOnStart, false) && !getBoolFromMeta(block.Meta, waveobj.MetaKey_CmdRunOnce, false)
}

func ResyncController(ctx context.Context, tabId string, blockId string, rtOpts *waveobj.RuntimeOpts, force bool) error {
	if tabId == "" || blockId == "" {
		return fmt.Errorf("invalid tabId or blockId passed to ResyncController")
	}

	mu := getBlockResyncMutex(blockId)
	mu.Lock()
	defer mu.Unlock()

	_, sparedDone := restartStartedBlocks.GetEx(blockId)
	if force || isViewResync(rtOpts) {
		defer restartStartedBlocks.Delete(blockId)
	}
	return resyncControllerLocked(ctx, tabId, blockId, rtOpts, force, false, sparedDone)
}

// The terminal view always sends its size; a caller without one is not a view mounting, and must not use up the exemption.
func isViewResync(rtOpts *waveobj.RuntimeOpts) bool {
	return rtOpts != nil && rtOpts.TermSize.Rows > 0 && rtOpts.TermSize.Cols > 0
}

// bootStart: called by the boot pass, which starts the block only if it is still a restart block when this reads it.
// sparedDone: this restart block was started since backend start and no view has resynced it yet, so a done controller is left as it is.
func resyncControllerLocked(ctx context.Context, tabId string, blockId string, rtOpts *waveobj.RuntimeOpts, force bool, bootStart bool, sparedDone bool) error {
	blockData, err := wstore.DBMustGet[*waveobj.Block](ctx, blockId)
	if err != nil {
		return fmt.Errorf("error getting block: %w", err)
	}
	if bootStart && !isRestartBlock(blockData) {
		return nil
	}

	controllerName := blockData.Meta.GetString(waveobj.MetaKey_Controller, "")
	connName := blockData.Meta.GetString(waveobj.MetaKey_Connection, "")

	// Get existing controller
	existing := getController(blockId)

	// Check for connection change FIRST - always destroy on conn change
	if existing != nil {
		existingConnName := existing.GetConnName()
		if existingConnName != connName {
			log.Printf("stopping blockcontroller %s due to conn change (from %q to %q)\n", blockId, existingConnName, connName)
			DestroyBlockController(blockId)
			time.Sleep(100 * time.Millisecond)
			existing = nil
		}
	}

	// If no controller needed, stop existing if present
	if controllerName == "" {
		if existing != nil {
			DestroyBlockController(blockId)
		}
		return nil
	}

	// Determine if we should use DurableShellController vs ShellController
	shouldUseDurableShellController := controllerName == BlockController_Shell && jobcontroller.IsBlockIdTermDurable(blockId)

	// Check if we need to morph controller type
	if existing != nil {
		needsReplace := false

		switch existing.(type) {
		case *ShellController:
			if controllerName != BlockController_Shell && controllerName != BlockController_Cmd {
				needsReplace = true
			} else if shouldUseDurableShellController {
				needsReplace = true
			}
		case *DurableShellController:
			if !shouldUseDurableShellController {
				needsReplace = true
			}
		case *TsunamiController:
			if controllerName != BlockController_Tsunami {
				needsReplace = true
			}
		}

		if needsReplace {
			log.Printf("stopping blockcontroller %s due to controller type change\n", blockId)
			DestroyBlockController(blockId)
			time.Sleep(100 * time.Millisecond)
			existing = nil
		}
	}

	// Force restart if requested
	if force && existing != nil {
		DestroyBlockController(blockId)
		time.Sleep(100 * time.Millisecond)
		existing = nil
	}

	// Destroy done controllers before restarting
	if existing != nil {
		status := existing.GetRuntimeStatus()
		if status.ShellProcStatus == Status_Done {
			if sparedDone && !force {
				return nil
			}
			log.Printf("destroying blockcontroller %s with done status before restart\n", blockId)
			DestroyBlockController(blockId)
			time.Sleep(100 * time.Millisecond)
			existing = nil
		}
	}

	// Create or restart controller
	var controller Controller
	if existing != nil {
		controller = existing
	} else {
		// Create new controller based on type
		switch controllerName {
		case BlockController_Shell, BlockController_Cmd:
			if shouldUseDurableShellController {
				controller = MakeDurableShellController(tabId, blockId, controllerName, connName)
			} else {
				controller = makeShellControllerFn(tabId, blockId, controllerName, connName)
			}
			registerController(blockId, controller)

		case BlockController_Tsunami:
			controller = MakeTsunamiController(tabId, blockId, connName)
			registerController(blockId, controller)

		default:
			return fmt.Errorf("unknown controller type %q", controllerName)
		}
	}

	// Check if we need to start/restart
	status := controller.GetRuntimeStatus()
	if status.ShellProcStatus == Status_Init {
		// For shell/cmd, check connection status first (for non-local connections)
		if controllerName == BlockController_Shell || controllerName == BlockController_Cmd {
			if !conncontroller.IsLocalConnName(connName) {
				err = CheckConnStatus(blockId)
				if err != nil {
					return fmt.Errorf("cannot start shellproc: %w", err)
				}
			}
		}

		// Start controller
		err = controller.Start(ctx, blockData.Meta, rtOpts, force)
		if err != nil {
			return fmt.Errorf("error starting controller: %w", err)
		}
		if isRestartBlock(blockData) {
			restartStartedBlocks.Set(blockId, true)
		}
	}

	return nil
}

func GetBlockControllerRuntimeStatus(blockId string) *BlockControllerRuntimeStatus {
	controller := getController(blockId)
	if controller == nil {
		return nil
	}
	return controller.GetRuntimeStatus()
}

func DestroyBlockController(blockId string) {
	controller := getController(blockId)
	if controller == nil {
		return
	}
	controller.Stop(true, Status_Done, true)
	wstore.DeleteRTInfo(waveobj.MakeORef(waveobj.OType_Block, blockId))
	deleteController(blockId)
}

func sendConnMonitorInputNotification(controller Controller) {
	connName := controller.GetConnName()
	if connName == "" || conncontroller.IsLocalConnName(connName) || conncontroller.IsWslConnName(connName) {
		return
	}

	connOpts, parseErr := remote.ParseOpts(connName)
	if parseErr != nil {
		return
	}
	sshConn := conncontroller.MaybeGetConn(connOpts)
	if sshConn != nil {
		monitor := sshConn.GetMonitor()
		if monitor != nil {
			monitor.NotifyInput()
		}
	}
}

func SendInput(blockId string, inputUnion *BlockInputUnion) error {
	controller := getController(blockId)
	if controller == nil {
		return fmt.Errorf("no controller found for block %s", blockId)
	}
	sendConnMonitorInputNotification(controller)
	return controller.SendInput(inputUnion)
}

// only call this on shutdown
func StopAllBlockControllersForShutdown() {
	controllers := getAllControllers()
	for blockId, controller := range controllers {
		status := controller.GetRuntimeStatus()
		if status != nil && status.ShellProcStatus == Status_Running {
			go func(id string, c Controller) {
				c.Stop(true, Status_Done, false)
				wstore.DeleteRTInfo(waveobj.MakeORef(waveobj.OType_Block, id))
			}(blockId, controller)
		}
	}
}

func getBoolFromMeta(meta map[string]any, key string, def bool) bool {
	ival, found := meta[key]
	if !found || ival == nil {
		return def
	}
	if val, ok := ival.(bool); ok {
		return val
	}
	return def
}

func getTermSize(bdata *waveobj.Block) waveobj.TermSize {
	if bdata.RuntimeOpts != nil {
		return bdata.RuntimeOpts.TermSize
	} else {
		return waveobj.TermSize{
			Rows: 25,
			Cols: 80,
		}
	}
}

func HandleAppendBlockFile(blockId string, blockFile string, data []byte) error {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	err := filestore.WFS.AppendData(ctx, blockId, blockFile, data)
	if err != nil {
		return fmt.Errorf("error appending to blockfile: %w", err)
	}
	wps.Broker.Publish(wps.WaveEvent{
		Event: wps.Event_BlockFile,
		Scopes: []string{
			waveobj.MakeORef(waveobj.OType_Block, blockId).String(),
		},
		Data: &wps.WSFileEventData{
			ZoneId:   blockId,
			FileName: blockFile,
			FileOp:   wps.FileOp_Append,
			Data64:   base64.StdEncoding.EncodeToString(data),
		},
	})
	return nil
}

func HandleTruncateBlockFile(blockId string) error {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	err := filestore.WFS.WriteFile(ctx, blockId, wavebase.BlockFile_Term, nil)
	if err == fs.ErrNotExist {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error truncating blockfile: %w", err)
	}
	err = filestore.WFS.DeleteFile(ctx, blockId, wavebase.BlockFile_Cache)
	if err == fs.ErrNotExist {
		err = nil
	}
	if err != nil {
		log.Printf("error deleting cache file (continuing): %v\n", err)
	}
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_BlockFile,
		Scopes: []string{waveobj.MakeORef(waveobj.OType_Block, blockId).String()},
		Data: &wps.WSFileEventData{
			ZoneId:   blockId,
			FileName: wavebase.BlockFile_Term,
			FileOp:   wps.FileOp_Truncate,
		},
	})
	return nil

}

func debugLog(ctx context.Context, fmtStr string, args ...interface{}) {
	blocklogger.Infof(ctx, "[conndebug] "+fmtStr, args...)
	log.Printf(fmtStr, args...)
}

func CheckConnStatus(blockId string) error {
	bdata, err := wstore.DBMustGet[*waveobj.Block](context.Background(), blockId)
	if err != nil {
		return fmt.Errorf("error getting block: %w", err)
	}
	connName := bdata.Meta.GetString(waveobj.MetaKey_Connection, "")
	if conncontroller.IsLocalConnName(connName) {
		return nil
	}
	if strings.HasPrefix(connName, "wsl://") {
		distroName := strings.TrimPrefix(connName, "wsl://")
		conn := wslconn.GetWslConn(distroName)
		connStatus := conn.DeriveConnStatus()
		if connStatus.Status != conncontroller.Status_Connected {
			return fmt.Errorf("not connected: %s", connStatus.Status)
		}
		return nil
	}
	opts, err := remote.ParseOpts(connName)
	if err != nil {
		return fmt.Errorf("error parsing connection name: %w", err)
	}
	conn := conncontroller.MaybeGetConn(opts)
	if conn == nil {
		return fmt.Errorf("no connection found")
	}
	connStatus := conn.DeriveConnStatus()
	if connStatus.Status != conncontroller.Status_Connected {
		return fmt.Errorf("not connected: %s", connStatus.Status)
	}
	return nil
}

func makeSwapToken(ctx context.Context, logCtx context.Context, blockId string, blockMeta waveobj.MetaMapType, remoteName string, shellType string) *shellutil.TokenSwapEntry {
	token := &shellutil.TokenSwapEntry{
		Token: uuid.New().String(),
		Env:   make(map[string]string),
		Exp:   time.Now().Add(5 * time.Minute),
	}
	token.Env["TERM_PROGRAM"] = "waveterm"
	token.Env["WAVETERM_BLOCKID"] = blockId
	token.Env["WAVETERM_VERSION"] = wavebase.WaveVersion
	token.Env["WAVETERM"] = "1"
	tabId, err := wstore.DBFindTabForBlockId(ctx, blockId)
	if err != nil {
		log.Printf("error finding tab for block: %v\n", err)
	} else {
		token.Env["WAVETERM_TABID"] = tabId
	}
	if tabId != "" {
		wsId, err := wstore.DBFindWorkspaceForTabId(ctx, tabId)
		if err != nil {
			log.Printf("error finding workspace for tab: %v\n", err)
		} else {
			token.Env["WAVETERM_WORKSPACEID"] = wsId
		}
	}
	token.Env["WAVETERM_CLIENTID"] = wstore.GetClientId()
	token.Env["WAVETERM_CONN"] = remoteName
	for k, v := range shellutil.PerBlockHistoryEnv(wconfig.GetWatcher().GetFullConfig().Settings.TermPerBlockHistory, shellType, remoteName, blockId) {
		token.Env[k] = v
	}
	envMap, err := resolveEnvMap(blockId, blockMeta, remoteName)
	if err != nil {
		log.Printf("error resolving env map: %v\n", err)
	}
	for k, v := range envMap {
		token.Env[k] = v
	}
	token.ScriptText = getCustomInitScript(logCtx, blockMeta, remoteName, shellType)
	return token
}
