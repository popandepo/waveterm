// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

var sessionCommand = &cobra.Command{
	Use:   "session",
	Short: "Run programs as sessions, each in its own background tab",
	Long: `A session is a tab with one block running one program. These commands are for a controller (a person or a
program running inside Wave) that opens sessions, looks at them and closes them without showing them or taking
focus.`,
}

var sessionStartCmd = &cobra.Command{
	Use:   "start [flags] [-- command [args...]]",
	Short: "Start a program in a new background tab and print the tab and block ids",
	Long: `Create a tab, load it in the background, and start the program in a terminal block in it. The active tab does not
change and nothing takes keyboard focus unless --activate is given. The program is started directly, with the
arguments exactly as given: there is no shell and no launcher in between, so characters such as ; & | " in an
argument reach the program unchanged. Use --args-file for a JSON array of strings if even the command line of wsh
itself must not be trusted to carry the arguments.

With --stack <block> no tab is created: the program block is created in the tab of that block and put into its group (stack), as
wsh group add does. --name, --color and --symbol then go on the block (frame:title, icon:color and frame:bordercolor, frame:icon),
because there is no tab of its own to carry them. --stack cannot be combined with --activate. The group is applied when the tab
of that block is open (the back end only queues it, as for wsh group add).

The output is one JSON object: {"tabid":"...","blockid":"..."}, with "stackedwith":"<block>" added for --stack.`,
	Example: `  wsh session start --name tether-notes --color "#2D7DD2" --symbol star --cwd C:\work -- pwsh -NoLogo -File start.ps1 -Prompt "a; b"
  wsh session start --name w1 --args-file cmd.json
  wsh session start --stack 3 --name build --color "#2D7DD2" --symbol star --args-file cmd.json`,
	RunE:    sessionStartRun,
	PreRunE: preRunSetupRpcClient,
}

var sessionListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List tabs with their blocks and whether the program in each is running",
	Args:    cobra.NoArgs,
	RunE:    sessionListRun,
	PreRunE: preRunSetupRpcClient,
}

var sessionCloseCmd = &cobra.Command{
	Use:   "close <tab>",
	Short: "Close a tab whose program has ended",
	Long: `Close the tab (tab id, first 8 characters, tab:<n> or tab:<uuid>) only if every terminal block in it is known to have
finished its program. A block whose status is unknown (starting, restarting, or not started since the app launched)
counts as not finished. The check and the close happen in one backend call. Use --force to close anyway.`,
	Args:    cobra.ExactArgs(1),
	RunE:    sessionCloseRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	sessionName      string
	sessionColor     string
	sessionSymbol    string
	sessionCwd       string
	sessionEnv       []string
	sessionArgsFile  string
	sessionWorkspace string
	sessionActivate  bool
	sessionRestart   bool
	sessionMeta      []string
	sessionForce     bool
)

func init() {
	f := sessionStartCmd.Flags()
	f.StringVar(&sessionName, "name", "", "tab name")
	f.StringVar(&sessionColor, "color", "", "tab colour (CSS name or hex)")
	f.StringVar(&sessionSymbol, "symbol", "", "tab symbol (icon name or one or two characters)")
	f.StringVar(&sessionCwd, "cwd", "", "working directory (default: the caller's)")
	f.StringArrayVar(&sessionEnv, "env", nil, "KEY=VALUE for the program, repeatable")
	f.StringVar(&sessionArgsFile, "args-file", "", "JSON array of strings: the command and its arguments")
	f.StringVar(&sessionWorkspace, "workspace", "", "workspace id (defaults to WAVETERM_WORKSPACEID)")
	f.BoolVar(&sessionRestart, "restart", false, "start the program again every time Wave starts; the command must resume its own work. Best effort: --session-id and inline code (pwsh -Command, node -e, python -c, wsl ...) are refused, what a script file does cannot be checked")
	f.StringArrayVar(&sessionMeta, "meta", nil, "session:KEY=VALUE written on the block for outside tools (session:id, session:model ...), repeatable")
	f.BoolVar(&sessionActivate, "activate", false, "switch to the new tab (default: leave the active tab alone)")
	f.StringVar(&sessionStack, "stack", "", "no new tab: put the program block into the group (stack) of this block (block id or number); --name, --color and --symbol then go on the block")
	sessionCloseCmd.Flags().BoolVar(&sessionForce, "force", false, "close even if a program is still running")
	sessionCommand.AddCommand(sessionStartCmd)
	sessionCommand.AddCommand(sessionListCmd)
	sessionCommand.AddCommand(sessionCloseCmd)
	rootCmd.AddCommand(sessionCommand)
}

type sessionBlockInfo struct {
	BlockId  string `json:"blockid"`
	Title    string `json:"title,omitempty"`
	View     string `json:"view"`
	Status   string `json:"status"`
	ExitCode int    `json:"exitcode"`
}

type sessionTabInfo struct {
	TabId       string             `json:"tabid"`
	Name        string             `json:"name"`
	WorkspaceId string             `json:"workspaceid"`
	Color       string             `json:"color,omitempty"`
	Symbol      string             `json:"symbol,omitempty"`
	Running     bool               `json:"running"`
	Blocks      []sessionBlockInfo `json:"blocks"`
}

// splitSessionCommand returns the command and arguments from the text after "--" or from a JSON file.
func splitSessionCommand(dashArgs []string, argsFileData []byte) (string, []string, error) {
	if len(argsFileData) > 0 {
		if len(dashArgs) > 0 {
			return "", nil, fmt.Errorf("give the command after -- or in --args-file, not both")
		}
		var parts []string
		if err := json.Unmarshal(argsFileData, &parts); err != nil {
			return "", nil, fmt.Errorf("--args-file must be a JSON array of strings: %w", err)
		}
		dashArgs = parts
	}
	if len(dashArgs) == 0 || dashArgs[0] == "" {
		return "", nil, fmt.Errorf("no command given: put it after -- or in --args-file")
	}
	return dashArgs[0], dashArgs[1:], nil
}

var sessionMetaKeyRegexp = regexp.MustCompile(`^session:[A-Za-z0-9._-]+$`)

// validateSessionStart checks everything that can be checked before the first tab is created, so a refused request
// leaves nothing behind, and returns the parsed session meta. With --restart Wave starts the SAME command again at
// every start of the app, so the command must be able to resume its own work: a bare "--session-id <guid>" (Claude) would
// fail or start an empty session the second time. Only literal text can be seen here: the check refuses any argument that
// contains "--session-id" and any inline shell command (-Command, -c, /c, -EncodedCommand), because the flag could be
// inside it; it cannot see a value that a variable expands to inside a script. Use a starter script that makes the choice
// (tools/ours/Start-ResumableSession.ps1).
func validateSessionStart(restart bool, command string, cmdArgs []string, metaPairs []string) (map[string]any, error) {
	meta, err := parseSessionMeta(metaPairs)
	if err != nil {
		return nil, err
	}
	if !restart {
		return meta, nil
	}
	all := append([]string{command}, cmdArgs...)
	for _, a := range all {
		if strings.Contains(a, "--session-id") {
			return nil, fmt.Errorf("--restart cannot be used with --session-id in the command: the second start must resume the session (--resume), not create it again; use a starter script that makes that choice, for example tools/ours/Start-ResumableSession.ps1")
		}
	}
	if isInlineShellCommand(command, cmdArgs) {
		return nil, fmt.Errorf("--restart cannot be used with an inline shell command (%s ...): the flags that matter could be hidden inside it; start a script file instead, for example tools/ours/Start-ResumableSession.ps1", command)
	}
	return meta, nil
}

// inlineCodeFlags lists, per interpreter, the flags that make it run code given on the command line. Best effort: these
// are the common ones, not every interpreter there is, and nothing here can see what a script file does.
var inlineCodeFlags = map[string][]string{
	"pwsh":       {"-command", "-c", "-encodedcommand", "-ec", "-e", "-commandwithargs", "-cwa"},
	"powershell": {"-command", "-c", "-encodedcommand", "-ec", "-e", "-commandwithargs", "-cwa"},
	"cmd":        {"/c", "/k"},
	"bash":       {"-c", "-lc", "-ic", "-cl"},
	"sh":         {"-c", "-lc", "-ic", "-cl"},
	"zsh":        {"-c", "-lc", "-ic", "-cl"},
	"fish":       {"-c", "-lc", "-ic", "-cl"},
	"dash":       {"-c"},
	"nu":         {"-c", "--commands"},
	"node":       {"-e", "--eval", "-p", "--print"},
	"python":     {"-c"},
	"python3":    {"-c"},
	"py":         {"-c"},
	"ruby":       {"-e"},
	"perl":       {"-e", "-E"},
}

// wsl runs any Linux command line it is given, so with --restart it is refused whatever its arguments are.
func isInlineShellCommand(command string, args []string) bool {
	base := strings.ToLower(command)
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.TrimSuffix(base, ".exe")
	if base == "wsl" {
		return true
	}
	flags, ok := inlineCodeFlags[base]
	if !ok {
		return false
	}
	for _, a := range args {
		for _, f := range flags {
			if strings.EqualFold(a, f) {
				return true
			}
		}
	}
	return false
}

// parseSessionMeta accepts only keys under "session:". The block meta also decides what program runs and where, so
// no other key may be set from here.
func parseSessionMeta(pairs []string) (map[string]any, error) {
	meta := make(map[string]any)
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 || !sessionMetaKeyRegexp.MatchString(kv[0]) {
			return nil, fmt.Errorf("invalid --meta %q: want session:KEY=VALUE", p)
		}
		for _, r := range kv[1] {
			if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 {
				return nil, fmt.Errorf("invalid --meta %q: a value cannot contain control characters (line breaks, tabs, escape sequences)", kv[0])
			}
		}
		if _, dup := meta[kv[0]]; dup {
			return nil, fmt.Errorf("--meta %s given twice", kv[0])
		}
		meta[kv[0]] = kv[1]
	}
	return meta, nil
}

// applySessionOptions adds the optional parts to the block meta. Restart mode makes Wave start the program at every
// start of the app (the controller runs a block whose runonstart is true and which is not marked run once) and keeps
// the earlier output instead of clearing it.
func applySessionOptions(meta map[string]any, restart bool, extra map[string]any) {
	if restart {
		meta[waveobj.MetaKey_CmdRunOnce] = false
		meta[waveobj.MetaKey_CmdRunOnStart] = true
		meta[waveobj.MetaKey_CmdClearOnStart] = false
	}
	for k, v := range extra {
		meta[k] = v
	}
}

func parseSessionEnv(pairs []string) (map[string]any, error) {
	env := make(map[string]any)
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 || kv[0] == "" {
			return nil, fmt.Errorf("invalid --env %q: want KEY=VALUE", p)
		}
		env[kv[0]] = kv[1]
	}
	return env, nil
}

func sessionStartRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("sessionstart", rtnErr == nil)
	}()
	var fileData []byte
	if sessionArgsFile != "" {
		var err error
		fileData, err = os.ReadFile(sessionArgsFile)
		if err != nil {
			return fmt.Errorf("reading --args-file: %w", err)
		}
	}
	cmdArgsAfterDash := args
	command, cmdArgs, err := splitSessionCommand(cmdArgsAfterDash, fileData)
	if err != nil {
		return err
	}
	extraMeta, err := validateSessionStart(sessionRestart, command, cmdArgs, sessionMeta)
	if err != nil {
		return err
	}
	if err := validateSessionStackOptions(cmd.Flags().Changed("stack"), sessionStack, sessionActivate); err != nil {
		return err
	}
	var styleMeta, memberLook map[string]any
	if sessionStack != "" {
		memberLook, err = buildSessionMemberLook(sessionName, sessionColor, sessionSymbol)
	} else {
		styleMeta, err = buildSessionStyleMeta(sessionColor, sessionSymbol)
	}
	if err != nil {
		return err
	}
	envMap, err := parseSessionEnv(sessionEnv)
	if err != nil {
		return err
	}
	wsId, err := resolveWorkspaceIdForNewTab(sessionWorkspace)
	if err != nil {
		return err
	}
	cwd := sessionCwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
	}
	if sessionStack != "" {
		blockMeta := buildSessionBlockMeta(command, cmdArgs, cwd, envMap, RpcContext.Conn)
		applySessionOptions(blockMeta, sessionRestart, extraMeta)
		out, err := startSessionInStack(wsId, sessionStack, blockMeta, memberLook)
		if err != nil {
			return err
		}
		WriteStdout("%s\n", out)
		return nil
	}
	blockMeta := buildSessionBlockMeta(command, cmdArgs, cwd, envMap, RpcContext.Conn)
	applySessionOptions(blockMeta, sessionRestart, extraMeta)
	tabId, blockId, err := createSessionTab(rpcSessionOps{}, wsId, sessionName, styleMeta, blockMeta, sessionActivate)
	if err != nil {
		return err
	}
	WriteStdout("%s\n", sessionStartOutput(tabId, blockId, ""))
	return nil
}

type sessionTabOps interface {
	tabCreator
	SetTabStyle(tabId string, meta map[string]any) error
	CreateBlock(tabId string, meta map[string]any) (string, error)
	CloseTab(wsId string, tabId string) error
}

type rpcSessionOps struct {
	rpcTabCreator
}

func (rpcSessionOps) SetTabStyle(tabId string, meta map[string]any) error {
	return wshclient.SetMetaCommand(RpcClient, wshrpc.CommandSetMetaData{ORef: waveobj.MakeORef(waveobj.OType_Tab, tabId), Meta: meta}, &wshrpc.RpcOpts{Timeout: 2000})
}

func (rpcSessionOps) CloseTab(wsId string, tabId string) error {
	rtn, err := wshclient.SessionCloseTabCommand(RpcClient, wshrpc.CommandSessionCloseTabData{WorkspaceId: wsId, TabId: tabId, Force: true}, &wshrpc.RpcOpts{Timeout: 15000})
	if err != nil {
		return err
	}
	return wshclient.ElectronTabClosedCommand(RpcClient, wshrpc.CommandElectronTabClosedData{
		WorkspaceId: wsId, TabId: tabId, NewActiveTabId: rtn.NewActiveTabId, CloseWindow: rtn.CloseWindow,
	}, &wshrpc.RpcOpts{Timeout: 15000, Route: wshutil.ElectronRoute})
}

func (rpcSessionOps) CreateBlock(tabId string, meta map[string]any) (string, error) {
	oref, err := wshclient.CreateBlockCommand(RpcClient, wshrpc.CommandCreateBlockData{
		TabId:    tabId,
		BlockDef: &waveobj.BlockDef{Meta: meta},
		Focused:  false,
	}, &wshrpc.RpcOpts{Timeout: 15000})
	if err != nil {
		return "", err
	}
	return oref.OID, nil
}

// closeLeftoverTab closes the empty tab this call just made, so a failed start leaves nothing behind. When that fails too, the
// tab id is returned so the caller can close it by hand.
func closeLeftoverTab(c sessionTabOps, wsId string, tabId string, cause error) (string, string, error) {
	if err := c.CloseTab(wsId, tabId); err != nil {
		return tabId, "", fmt.Errorf("%w; the empty tab %s could not be closed again: %v", cause, tabId, err)
	}
	return "", "", fmt.Errorf("%w; the empty tab was closed again", cause)
}

// createSessionTab makes the tab and its program block and loads the tab's page LAST. The page queues a clean-up of orphaned
// blocks (blocks of the tab that are not in its layout) when it first loads; a block created after that is in the tab's block
// list before its layout insert has run, and the clean-up deletes it, leaving a bare shell. With the block there first, its
// insert is already queued when the page loads. When a step fails, the tab id is still returned so the caller can close the tab.
func createSessionTab(c sessionTabOps, wsId string, name string, styleMeta map[string]any, blockMeta map[string]any, activate bool) (string, string, error) {
	tabId, err := c.CreateTab(wshrpc.CommandCreateTabData{WorkspaceId: wsId, Name: name, Empty: true})
	if err != nil {
		return "", "", fmt.Errorf("creating tab: %w", err)
	}
	if len(styleMeta) > 0 {
		if err := c.SetTabStyle(tabId, styleMeta); err != nil {
			return closeLeftoverTab(c, wsId, tabId, fmt.Errorf("tab %s was created but styling it failed: %w", tabId, err))
		}
	}
	blockId, err := c.CreateBlock(tabId, blockMeta)
	if err != nil {
		return closeLeftoverTab(c, wsId, tabId, fmt.Errorf("tab %s was created but the block could not be: %w", tabId, err))
	}
	tabData := wshrpc.CommandElectronTabData{WorkspaceId: wsId, TabId: tabId}
	if err := c.PreloadTab(tabData); err != nil {
		return tabId, blockId, fmt.Errorf("tab %s was created with block %s but its page could not be loaded in the background: %w", tabId, blockId, err)
	}
	if activate {
		if err := c.SetActiveTab(tabData); err != nil {
			return tabId, blockId, fmt.Errorf("tab %s was created but could not be activated: %w", tabId, err)
		}
	}
	return tabId, blockId, nil
}

// buildSessionBlockMeta is the whole description of the program block. cmd:shell is false and the arguments are a list,
// so the program receives every argument exactly as given.
func buildSessionBlockMeta(command string, cmdArgs []string, cwd string, env map[string]any, conn string) map[string]any {
	meta := map[string]any{
		waveobj.MetaKey_View:            "term",
		waveobj.MetaKey_Controller:      "cmd",
		waveobj.MetaKey_Cmd:             command,
		waveobj.MetaKey_CmdArgs:         cmdArgs,
		waveobj.MetaKey_CmdShell:        false,
		waveobj.MetaKey_CmdCwd:          cwd,
		waveobj.MetaKey_CmdRunOnce:      true,
		waveobj.MetaKey_CmdRunOnStart:   true,
		waveobj.MetaKey_CmdClearOnStart: true,
	}
	if len(env) > 0 {
		meta[waveobj.MetaKey_CmdEnv] = env
	}
	if conn != "" {
		meta[waveobj.MetaKey_Connection] = conn
	}
	return meta
}

// buildSessionStyleMeta reuses the tab style rules; both values are optional here.
func buildSessionStyleMeta(color string, symbol string) (map[string]any, error) {
	if strings.TrimSpace(color) == "" && strings.TrimSpace(symbol) == "" {
		return nil, nil
	}
	return buildTabStyleMeta(color, symbol, false)
}

func isRunningStatus(status string) bool {
	return status == "running" || status == "init"
}

func collectSessionTabs() ([]sessionTabInfo, error) {
	workspaces, err := wshclient.WorkspaceListAllCommand(RpcClient, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	var tabs []sessionTabInfo
	for _, w := range workspaces {
		if w.WorkspaceData == nil {
			continue
		}
		blocks, err := wshclient.BlocksListCommand(RpcClient, wshrpc.BlocksListRequest{WorkspaceId: w.WorkspaceData.OID}, &wshrpc.RpcOpts{Timeout: 5000})
		if err != nil {
			return nil, fmt.Errorf("listing blocks of workspace %s: %w", w.WorkspaceData.OID, err)
		}
		for _, tabId := range w.WorkspaceData.TabIds {
			tab, err := wshclient.GetTabCommand(RpcClient, tabId, &wshrpc.RpcOpts{Timeout: 2000})
			if err != nil {
				return nil, fmt.Errorf("reading tab %s: %w", tabId, err)
			}
			info := sessionTabInfo{TabId: tabId, Name: tab.Name, WorkspaceId: w.WorkspaceData.OID, Blocks: []sessionBlockInfo{}}
			if c, ok := tab.Meta[TabMetaFlagColor].(string); ok {
				info.Color = c
			}
			if s, ok := tab.Meta[TabMetaSymbol].(string); ok {
				info.Symbol = s
			}
			for _, b := range blocks {
				if b.TabId != tabId {
					continue
				}
				view, _ := b.Meta[waveobj.MetaKey_View].(string)
				title, _ := b.Meta[waveobj.MetaKey_FrameTitle].(string)
				bi := sessionBlockInfo{BlockId: b.BlockId, Title: title, View: view}
				if view == "term" {
					st, err := wshclient.ControllerStatusCommand(RpcClient, b.BlockId, &wshrpc.RpcOpts{Timeout: 2000})
					if err != nil {
						return nil, fmt.Errorf("status of block %s: %w", b.BlockId, err)
					}
					bi.Status = st.ShellProcStatus
					bi.ExitCode = st.ShellProcExitCode
					if isRunningStatus(st.ShellProcStatus) {
						info.Running = true
					}
				}
				info.Blocks = append(info.Blocks, bi)
			}
			tabs = append(tabs, info)
		}
	}
	return tabs, nil
}

func sessionListRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("sessionlist", rtnErr == nil)
	}()
	tabs, err := collectSessionTabs()
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(tabs, "", "  ")
	if err != nil {
		return err
	}
	WriteStdout("%s\n", string(out))
	return nil
}

func sessionCloseRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("sessionclose", rtnErr == nil)
	}()
	tabId, wsId, err := resolveTabAndWorkspace(args[0])
	if err != nil {
		return err
	}
	// The check and the delete are one backend call. A block with no controller status counts as not finished.
	rtn, err := wshclient.SessionCloseTabCommand(RpcClient, wshrpc.CommandSessionCloseTabData{WorkspaceId: wsId, TabId: tabId, Force: sessionForce}, &wshrpc.RpcOpts{Timeout: 15000})
	if err != nil {
		return fmt.Errorf("not closed: %w", err)
	}
	err = wshclient.ElectronTabClosedCommand(RpcClient, wshrpc.CommandElectronTabClosedData{
		WorkspaceId: wsId, TabId: tabId, NewActiveTabId: rtn.NewActiveTabId, CloseWindow: rtn.CloseWindow,
	}, &wshrpc.RpcOpts{Timeout: 15000, Route: wshutil.ElectronRoute})
	if err != nil {
		return fmt.Errorf("tab %s was closed but the window could not be told: %w", tabId, err)
	}
	WriteStdout("tab closed\n")
	return nil
}
