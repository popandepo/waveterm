// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

var webUrlJson bool
var webInfoJson bool

var webReloadCmd = &cobra.Command{
	Use:   "reload [-b block]",
	Short: "reload a web tile",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return webNavRun(wshrpc.WebNavAction_Reload) },
}

var webBackCmd = &cobra.Command{
	Use:   "back [-b block]",
	Short: "go back one page in a web tile",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return webNavRun(wshrpc.WebNavAction_Back) },
}

var webForwardCmd = &cobra.Command{
	Use:   "forward [-b block]",
	Short: "go forward one page in a web tile",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return webNavRun(wshrpc.WebNavAction_Forward) },
}

var webUrlCmd = &cobra.Command{
	Use:   "url [--json] [-b block]",
	Short: "print the current address and title of a web tile",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return webNavRun(wshrpc.WebNavAction_Url) },
}

var webInfoCmd = &cobra.Command{
	Use:   "info [--json] [-b block]",
	Short: "print the address, title, load state (loading, ready or failed) and history state of a web tile",
	Long: `Reads only: nothing is navigated and nothing takes focus. The state is "loading" while the page is loading, "failed" when the
tile shows the error page of a navigation that failed, otherwise "ready". It is the state at the moment of the call: ask again
after reload, back or forward. Same limits as the other web commands: a tile of the caller's own workspace only.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error { return webNavRun(wshrpc.WebNavAction_Info) },
}

func init() {
	webInfoCmd.Flags().BoolVar(&webInfoJson, "json", false, "print the address, title, state and history state as JSON")
	webUrlCmd.Flags().BoolVar(&webUrlJson, "json", false, "print the address, title and history state as JSON")
	webCmd.AddCommand(webReloadCmd, webBackCmd, webForwardCmd, webUrlCmd, webInfoCmd)
}

// the one answer for a tile that the caller may not act on, whether it exists or not (the same text as the server's)
const webNavDenied = "that web tile is not available to this caller"

// seams for tests
var (
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) {
		return wshclient.BlockInfoCommand(RpcClient, blockId, nil)
	}
	// asks the server (which knows who is calling from the route the router stamped) whether the caller may act on the block
	webNavAuthorize = func(blockId string) error {
		return wshclient.WebNavAuthorizeCommand(RpcClient, wshrpc.CommandWebNavAuthorizeData{BlockId: blockId}, nil)
	}
	webNavRPC = func(data wshrpc.CommandWebNavData) (*wshrpc.CommandWebNavRtnData, error) {
		return wshclient.WebNavCommand(RpcClient, data, &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: 5000})
	}
)

// makeWebNavRequest builds the request for a block, refusing a block that is not a web tile. The tab and workspace come from the block itself,
// so the tile may be in any tab the caller can name (-b), not only the caller's own.
func makeWebNavRequest(info *wshrpc.BlockInfoData, blockId string, action string) (wshrpc.CommandWebNavData, error) {
	if info == nil || info.Block == nil {
		return wshrpc.CommandWebNavData{}, fmt.Errorf("block %s could not be read", blockId)
	}
	if info.Block.Meta.GetString(waveobj.MetaKey_View, "") != "web" {
		return wshrpc.CommandWebNavData{}, fmt.Errorf("block %s is not a web block", blockId)
	}
	if info.TabId == "" || info.WorkspaceId == "" {
		return wshrpc.CommandWebNavData{}, fmt.Errorf("block %s has no tab or workspace", blockId)
	}
	return wshrpc.CommandWebNavData{WorkspaceId: info.WorkspaceId, TabId: info.TabId, BlockId: blockId, Action: action}, nil
}

// The state comes from the window, but only these three words are ever printed: anything else is "unknown".
func webLoadStateOrUnknown(state string) string {
	switch state {
	case "loading", "ready", "failed", "":
		return state
	}
	return "unknown"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// formatWebNav is what the command prints: for url the address and then the title (or JSON); for the others a short line with the address the
// tile had when the command was handled (the page loads after that: ask again with url).
func formatWebNav(action string, res *wshrpc.CommandWebNavRtnData, asJson bool) (string, error) {
	if res == nil {
		return "", fmt.Errorf("no answer from the window")
	}
	// the title and the address come from a web page and are printed as they are in no case (see safeText), JSON included
	shown := *res
	shown.Url = safeText(res.Url, maxPrintedUrlRunes)
	shown.Title = safeText(res.Title, maxPrintedTitleRunes)
	shown.State = webLoadStateOrUnknown(res.State)
	res = &shown
	if asJson {
		barr, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return "", fmt.Errorf("json encoding: %w", err)
		}
		return string(barr), nil
	}
	if action == wshrpc.WebNavAction_Url {
		return res.Url + "\n" + res.Title, nil
	}
	if action == wshrpc.WebNavAction_Info {
		return fmt.Sprintf("address: %s\ntitle: %s\nstate: %s\nback: %s\nforward: %s", res.Url, res.Title, res.State, yesNo(res.CanGoBack), yesNo(res.CanGoForward)), nil
	}
	return fmt.Sprintf("%s requested (the tile showed %s)", strings.ToLower(action), res.Url), nil
}

func webNavRun(action string) error {
	ref := blockArg
	if ref == "" {
		ref = "this"
	}
	blockId, err := resolveBlockRef(ref)
	if err != nil {
		if blockArg != "" {
			return errors.New(webNavDenied)
		}
		return fmt.Errorf("resolving blockid: %w", err)
	}
	// the question of who may act on which tile is asked FIRST, before anything about the tile is read: a tile that does not exist and a tile in another
	// workspace get the same answer
	if err := webNavAuthorize(blockId); err != nil {
		return errors.New(webNavDenied)
	}
	info, err := webNavBlockInfo(blockId)
	if err != nil {
		return fmt.Errorf("getting block info: %w", err)
	}
	data, err := makeWebNavRequest(info, blockId, action)
	if err != nil {
		return err
	}
	res, err := webNavRPC(data)
	if err != nil {
		if strings.Contains(err.Error(), webNavDenied) {
			return errors.New(webNavDenied)
		}
		return err
	}
	text, err := formatWebNav(action, res, (webUrlJson && action == wshrpc.WebNavAction_Url) || (webInfoJson && action == wshrpc.WebNavAction_Info))
	if err != nil {
		return err
	}
	WriteStdout("%s\n", text)
	return nil
}
