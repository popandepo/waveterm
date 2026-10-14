// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf16"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

// The longest title the strip draws whole (the page counts UTF-16 units, see memberTitleValue in stacktablook.ts).
const maxSessionMemberTitleLen = 60

// sessionStack is set by --stack on "wsh session start".
var sessionStack string

// sessionTabOfBlock and sessionCreateBlock are the two calls that --stack needs from the back end; tests replace them.
var sessionTabOfBlock = func(wsId string, blockId string) (string, error) {
	entries, err := wshclient.BlocksListCommand(RpcClient, wshrpc.BlocksListRequest{WorkspaceId: wsId}, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return "", fmt.Errorf("listing blocks of workspace %s: %w", wsId, err)
	}
	return findBlockTab(entries, blockId, wsId)
}

var sessionCreateBlock = func(data wshrpc.CommandCreateBlockData) (string, error) {
	oref, err := wshclient.CreateBlockCommand(RpcClient, data, &wshrpc.RpcOpts{Timeout: 15000})
	if err != nil {
		return "", err
	}
	return oref.OID, nil
}

// validateSessionStackOptions refuses what cannot go together with --stack before anything is looked up or created. A stacked session is a tile in
// a tab that already exists, so there is no new tab to switch to.
func validateSessionStackOptions(stack string, activate bool) error {
	if stack == "" {
		return nil
	}
	if activate {
		return fmt.Errorf("--activate cannot be combined with --stack: it switches to a new tab and a stacked session has none; show the member with: wsh group activate -b <block>")
	}
	return nil
}

// buildSessionMemberLook is what --name, --color and --symbol become for a session that is a member of a stack. There is no tab of its own to
// carry them, so they go on the block: the name is frame:title (the label in the strip), the colour is icon:color (the strip tab and the colour of
// the member) and frame:bordercolor (the outline of its tile), the symbol is frame:icon (the strip tab). The same value checks as for a tab apply,
// because the page draws these keys whoever wrote them.
func buildSessionMemberLook(name string, color string, symbol string) (map[string]any, error) {
	look := make(map[string]any)
	name = strings.TrimSpace(name)
	color = strings.TrimSpace(color)
	symbol = strings.TrimSpace(symbol)
	if name != "" {
		if hasHiddenOrDirectionalRunes(name) {
			return nil, fmt.Errorf("invalid name: control, bidirectional and invisible format characters are not allowed")
		}
		if len(utf16.Encode([]rune(name))) > maxSessionMemberTitleLen {
			return nil, fmt.Errorf("invalid name: at most %d characters", maxSessionMemberTitleLen)
		}
		look[waveobj.MetaKey_FrameTitle] = name
	}
	if color != "" {
		if err := validateColor(color); err != nil {
			return nil, err
		}
		look[waveobj.MetaKey_IconColor] = color
		look[waveobj.MetaKey_FrameBorderColor] = color
	}
	if symbol != "" {
		if err := validateTabSymbol(symbol); err != nil {
			return nil, err
		}
		look[waveobj.MetaKey_FrameIcon] = symbol
	}
	return look, nil
}

// findBlockTab answers which tab a block is in, from the block list of one workspace.
func findBlockTab(entries []wshrpc.BlocksListEntry, blockId string, wsId string) (string, error) {
	for _, e := range entries {
		if e.BlockId == blockId {
			return e.TabId, nil
		}
	}
	return "", fmt.Errorf("block %s is not in workspace %s (give --workspace if it is in another)", blockId, wsId)
}

// buildSessionStackCreate is the whole create request of a stacked session. The target must be in TabId: the back end checks that before it creates
// anything. Focused is false: a stacked session never takes keyboard focus.
func buildSessionStackCreate(tabId string, targetBlockId string, blockMeta map[string]any) wshrpc.CommandCreateBlockData {
	return wshrpc.CommandCreateBlockData{
		TabId:         tabId,
		BlockDef:      &waveobj.BlockDef{Meta: blockMeta},
		TargetBlockId: targetBlockId,
		TargetAction:  wshrpc.CreateBlockAction_Stack,
		Focused:       false,
	}
}

// sessionStartOutput is the one JSON line of "wsh session start". A stacked session also names the block it was stacked with.
func sessionStartOutput(tabId string, blockId string, stackedWith string) string {
	out := map[string]string{"tabid": tabId, "blockid": blockId}
	if stackedWith != "" {
		out["stackedwith"] = stackedWith
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// checkStackRefWorkspace refuses a short reference (a number, "this") when the session goes into another workspace: those are resolved in the
// caller's own tab, so they would name a block there, not in the requested workspace. Only a full block id means the same block everywhere.
func checkStackRefWorkspace(wsId string, ref string) error {
	if wsId == os.Getenv("WAVETERM_WORKSPACEID") {
		return nil
	}
	if _, err := uuid.Parse(strings.TrimPrefix(ref, waveobj.OType_Block+":")); err == nil {
		return nil
	}
	return fmt.Errorf("--stack %q is counted in your own tab, not in workspace %s; give the full block id (wsh session list shows it)", ref, wsId)
}

// startSessionInStack creates the program block in the tab of the target block and puts it into the target's group. The back end only queues the
// layout change for the page of that tab, so the block exists at once and the group is applied when that tab is open (the same as wsh group add).
func startSessionInStack(wsId string, target string, blockMeta map[string]any, look map[string]any) (string, error) {
	if err := checkStackRefWorkspace(wsId, target); err != nil {
		return "", err
	}
	targetId, err := resolveBlockRef(target)
	if err != nil {
		return "", fmt.Errorf("resolving --stack: %w", err)
	}
	tabId, err := sessionTabOfBlock(wsId, targetId)
	if err != nil {
		return "", err
	}
	for k, v := range look {
		blockMeta[k] = v
	}
	blockId, err := sessionCreateBlock(buildSessionStackCreate(tabId, targetId, blockMeta))
	if err != nil {
		return "", fmt.Errorf("the block could not be created in the stack of %s: %w", targetId, err)
	}
	return sessionStartOutput(tabId, blockId, targetId), nil
}
