// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

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

The output is one JSON object: {"tabid":"...","blockid":"..."}.`,
	Example: `  wsh session start --name tether-notes --color "#2D7DD2" --symbol star --cwd C:\work -- pwsh -NoLogo -File start.ps1 -Prompt "a; b"
  wsh session start --name w1 --args-file cmd.json`,
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
	f.BoolVar(&sessionRestart, "restart", false, "start the program again every time Wave starts (use with a command that resumes its own session)")
	f.StringArrayVar(&sessionMeta, "meta", nil, "session:KEY=VALUE written on the block for outside tools (session:id, session:model ...), repeatable")
	f.BoolVar(&sessionActivate, "activate", false, "switch to the new tab (default: leave the active tab alone)")
	sessionCloseCmd.Flags().BoolVar(&sessionForce, "force", false, "close even if a program is still running")
	sessionCommand.AddCommand(sessionStartCmd)
	sessionCommand.AddCommand(sessionListCmd)
	sessionCommand.AddCommand(sessionCloseCmd)
	rootCmd.AddCommand(sessionCommand)
}

type sessionBlockInfo struct {
	BlockId  string `json:"blockid"`
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

// parseSessionMeta accepts only keys under "session:". The block meta also decides what program runs and where, so
// no other key may be set from here.
func parseSessionMeta(pairs []string) (map[string]any, error) {
	meta := make(map[string]any)
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 || !strings.HasPrefix(kv[0], "session:") || len(kv[0]) == len("session:") {
			return nil, fmt.Errorf("invalid --meta %q: want session:KEY=VALUE", p)
		}
		if strings.ContainsAny(kv[1], "\r\n") {
			return nil, fmt.Errorf("invalid --meta %q: a value cannot contain a line break", kv[0])
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
	styleMeta, err := buildSessionStyleMeta(sessionColor, sessionSymbol)
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
	tabId, err := createBackgroundTab(rpcTabCreator{}, wsId, sessionName, true, sessionActivate)
	if err != nil {
		return err
	}
	if len(styleMeta) > 0 {
		err = wshclient.SetMetaCommand(RpcClient, wshrpc.CommandSetMetaData{ORef: waveobj.MakeORef(waveobj.OType_Tab, tabId), Meta: styleMeta}, &wshrpc.RpcOpts{Timeout: 2000})
		if err != nil {
			return fmt.Errorf("tab %s was created but styling it failed: %w", tabId, err)
		}
	}
	extraMeta, err := parseSessionMeta(sessionMeta)
	if err != nil {
		return err
	}
	blockMeta := buildSessionBlockMeta(command, cmdArgs, cwd, envMap, RpcContext.Conn)
	applySessionOptions(blockMeta, sessionRestart, extraMeta)
	oref, err := wshclient.CreateBlockCommand(RpcClient, wshrpc.CommandCreateBlockData{
		TabId:    tabId,
		BlockDef: &waveobj.BlockDef{Meta: blockMeta},
		Focused:  false,
	}, &wshrpc.RpcOpts{Timeout: 15000})
	if err != nil {
		return fmt.Errorf("tab %s was created but the block could not be: %w", tabId, err)
	}
	out, _ := json.Marshal(map[string]string{"tabid": tabId, "blockid": oref.OID})
	WriteStdout("%s\n", string(out))
	return nil
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
				bi := sessionBlockInfo{BlockId: b.BlockId, View: view}
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
