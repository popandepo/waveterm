// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package staterecord keeps a plain JSON record of what runs where in Wave, for tools outside Wave that have to put
// things back after Wave was restarted. Words as the owner uses them: a workspace has tabs (the T1, T2 row), a tab has
// tiles, a tile can be a stack, and the tabs inside a stack are its group.
package staterecord

import (
	"strings"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

const (
	RecordVersion = 1
	KindTile      = "tile"
	KindStack     = "stack"
	sessionPrefix = "session:"
)

type Record struct {
	Version    int            `json:"version"`
	Sequence   int64          `json:"sequence"`
	WrittenAt  string         `json:"writtenat"`
	Wave       WaveInfo       `json:"wave"`
	Workspaces []WorkspaceRec `json:"workspaces"`
	Sessions   []SessionRec   `json:"sessions"`
}

type WaveInfo struct {
	Version string `json:"version"`
}

type WorkspaceRec struct {
	WorkspaceId string   `json:"workspaceid"`
	Name        string   `json:"name"`
	WindowId    string   `json:"windowid"`
	Tabs        []TabRec `json:"tabs"`
}

type TabRec struct {
	TabId    string    `json:"tabid"`
	Name     string    `json:"name"`
	Position int       `json:"position"`
	Active   bool      `json:"active"`
	Tiles    []TileRec `json:"tiles"`
}

// A tile is one rectangle of a tab. A plain tile has a group of one block. A stack tile has a group of several: the
// tabs inside the tile, in their order; ActiveBlockId is the one that is shown.
type TileRec struct {
	Position      int        `json:"position"`
	Kind          string     `json:"kind"`
	StackId       string     `json:"stackid,omitempty"`
	ActiveBlockId string     `json:"activeblockid"`
	Group         []BlockRec `json:"group"`
}

type BlockRec struct {
	Position   int               `json:"position"`
	BlockId    string            `json:"blockid"`
	Active     bool              `json:"active"`
	View       string            `json:"view,omitempty"`
	Title      string            `json:"title,omitempty"`
	Cwd        string            `json:"cwd,omitempty"`
	Program    string            `json:"program,omitempty"`
	Controller string            `json:"controller,omitempty"`
	Session    map[string]string `json:"session,omitempty"`
}

// SessionRec is the index of the blocks that carry a session id: where each one sits and what it was started with.
type SessionRec struct {
	BlockId       string            `json:"blockid"`
	WorkspaceId   string            `json:"workspaceid"`
	WorkspaceName string            `json:"workspacename"`
	TabId         string            `json:"tabid"`
	TabName       string            `json:"tabname"`
	TilePosition  int               `json:"tileposition"`
	Kind          string            `json:"kind"`
	StackId       string            `json:"stackid,omitempty"`
	GroupPosition int               `json:"groupposition"`
	Active        bool              `json:"active"`
	Title         string            `json:"title,omitempty"`
	Cwd           string            `json:"cwd,omitempty"`
	Session       map[string]string `json:"session"`
}

type WorkspaceSource struct {
	WindowId  string
	Workspace *waveobj.Workspace
}

// Snapshot is everything BuildRecord reads, so that it can be tested without a database.
type Snapshot struct {
	WaveVersion string
	Workspaces  []WorkspaceSource
	Tabs        map[string]*waveobj.Tab
	Blocks      map[string]*waveobj.Block
	Layouts     map[string]*waveobj.LayoutState
}

// WithoutStamp is the record with the fields that change on every write cleared, for comparing two states.
func (r Record) WithoutStamp() Record {
	r.Sequence = 0
	r.WrittenAt = ""
	return r
}

func metaString(meta waveobj.MetaMapType, key string) string {
	s, _ := meta[key].(string)
	return s
}

// Only the program name is kept, not its path or arguments, and no url, environment or other meta: the file is for
// restoring places, and command lines and addresses can hold secrets. Only the first token of the command is read (a leading
// quoted part counts as one token), then only its base name. Nothing past the first token is ever looked at, so an argument
// cannot end up in the name. When the first token cannot be identified safely (an unmatched quote, text glued to the closing quote, a Windows path with
// spaces that is not quoted, a name with characters a program name does not have) the program is left out.
const commandWhitespace = " \t\r\n\v\f"

func executableOf(cmd string) string {
	s := strings.TrimSpace(cmd)
	if s == "" {
		return ""
	}
	if s[0] == 39 || s[0] == 34 {
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return ""
		}
		after := 2 + end
		if after < len(s) && !strings.ContainsRune(commandWhitespace, rune(s[after])) {
			return ""
		}
		return s[1 : 1+end]
	}
	first := s
	if i := strings.IndexAny(s, commandWhitespace); i >= 0 {
		first = s[:i]
		if len(first) >= 3 && first[1] == ':' && (first[2] == '\\' || first[2] == '/') && !hasExecutableExtension(first) {
			return ""
		}
	}
	return first
}

func hasExecutableExtension(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".com", ".ps1"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

const maxProgramNameLen = 64

func plainProgramName(name string) bool {
	if name == "" || len(name) > maxProgramNameLen {
		return false
	}
	for _, c := range name {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' || c == '+'
		if !ok {
			return false
		}
	}
	return true
}

func programName(cmd string) string {
	exe := executableOf(cmd)
	if i := strings.LastIndexAny(exe, `/\`); i >= 0 {
		exe = exe[i+1:]
	}
	if !plainProgramName(exe) {
		return ""
	}
	return exe
}

// Only these session values reach the file: what is needed to put a session back, and the two status values that the
// manager's starter sets. Any other key (a token, a key, a note) is dropped, and so is a value that is not a plain short
// string.
var recordedSessionKeys = map[string]bool{
	"name": true, "id": true, "model": true, "effort": true, "permissionmode": true,
	"status-session": true, "status-tab": true,
}

const maxSessionValueLength = 200

func sessionMeta(meta waveobj.MetaMapType) map[string]string {
	var out map[string]string
	for k, v := range meta {
		if !strings.HasPrefix(k, sessionPrefix) {
			continue
		}
		key := strings.TrimPrefix(k, sessionPrefix)
		s, ok := v.(string)
		if !ok || !recordedSessionKeys[key] || len(s) > maxSessionValueLength {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[key] = s
	}
	return out
}

type tileSrc struct {
	kind     string
	stackId  string
	activeId string
	blockIds []string
}

// walkLayout lists the tiles of a saved layout tree (decoded JSON) in tree order. A node with kind "stack" is one tile
// whose children are its group; any other node with a block is a plain tile.
func walkLayout(node any, out *[]tileSrc) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	children, _ := m["children"].([]any)
	if m["kind"] == "stack" {
		t := tileSrc{kind: KindStack}
		t.stackId, _ = m["id"].(string)
		activeNodeId, _ := m["activeChildId"].(string)
		for _, c := range children {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			data, _ := cm["data"].(map[string]any)
			blockId, _ := data["blockId"].(string)
			if blockId == "" {
				continue
			}
			t.blockIds = append(t.blockIds, blockId)
			if id, _ := cm["id"].(string); id == activeNodeId {
				t.activeId = blockId
			}
		}
		if len(t.blockIds) > 0 {
			if t.activeId == "" {
				t.activeId = t.blockIds[0]
			}
			*out = append(*out, t)
		}
		return
	}
	if data, ok := m["data"].(map[string]any); ok {
		if blockId, _ := data["blockId"].(string); blockId != "" {
			*out = append(*out, tileSrc{kind: KindTile, blockIds: []string{blockId}, activeId: blockId})
		}
		return
	}
	for _, c := range children {
		walkLayout(c, out)
	}
}

func blockRec(blocks map[string]*waveobj.Block, blockId string, position int, active bool) (BlockRec, bool) {
	b := blocks[blockId]
	if b == nil {
		return BlockRec{}, false
	}
	return BlockRec{
		Position:   position,
		BlockId:    blockId,
		Active:     active,
		View:       metaString(b.Meta, "view"),
		Title:      metaString(b.Meta, "frame:title"),
		Cwd:        metaString(b.Meta, "cmd:cwd"),
		Program:    programName(metaString(b.Meta, "cmd")),
		Controller: metaString(b.Meta, "controller"),
		Session:    sessionMeta(b.Meta),
	}, true
}

func BuildRecord(snap Snapshot) Record {
	rec := Record{Version: RecordVersion, Wave: WaveInfo{Version: snap.WaveVersion}, Workspaces: []WorkspaceRec{}, Sessions: []SessionRec{}}
	for _, src := range snap.Workspaces {
		ws := src.Workspace
		if ws == nil {
			continue
		}
		wsRec := WorkspaceRec{WorkspaceId: ws.OID, Name: ws.Name, WindowId: src.WindowId, Tabs: []TabRec{}}
		for tabIndex, tabId := range ws.TabIds {
			tab := snap.Tabs[tabId]
			if tab == nil {
				continue
			}
			tabRec := TabRec{TabId: tabId, Name: tab.Name, Position: tabIndex, Active: tabId == ws.ActiveTabId, Tiles: []TileRec{}}
			var tiles []tileSrc
			if layout := snap.Layouts[tab.LayoutState]; layout != nil {
				walkLayout(layout.RootNode, &tiles)
			}
			// Only blocks that belong to the tab, each once: a layout can still name a block that was removed, or name one twice.
			inTab := map[string]bool{}
			for _, id := range tab.BlockIds {
				inTab[id] = true
			}
			shown := map[string]bool{}
			filtered := tiles[:0]
			for _, t := range tiles {
				var keep []string
				for _, id := range t.blockIds {
					if inTab[id] && !shown[id] {
						shown[id] = true
						keep = append(keep, id)
					}
				}
				if len(keep) == 0 {
					continue
				}
				t.blockIds = keep
				if !containsID(keep, t.activeId) {
					t.activeId = keep[0]
				}
				filtered = append(filtered, t)
			}
			tiles = filtered
			for _, id := range tab.BlockIds {
				if !shown[id] {
					shown[id] = true
					tiles = append(tiles, tileSrc{kind: KindTile, blockIds: []string{id}, activeId: id})
				}
			}
			tilePos := 0
			for _, t := range tiles {
				tileRec := TileRec{Position: tilePos, Kind: t.kind, StackId: t.stackId, Group: []BlockRec{}}
				for _, blockId := range t.blockIds {
					br, ok := blockRec(snap.Blocks, blockId, len(tileRec.Group), blockId == t.activeId)
					if !ok {
						continue
					}
					tileRec.Group = append(tileRec.Group, br)
					if br.Active {
						tileRec.ActiveBlockId = blockId
					}
				}
				if len(tileRec.Group) == 0 {
					continue
				}
				if tileRec.ActiveBlockId == "" {
					tileRec.ActiveBlockId = tileRec.Group[0].BlockId
					tileRec.Group[0].Active = true
				}
				tabRec.Tiles = append(tabRec.Tiles, tileRec)
				for _, br := range tileRec.Group {
					if br.Session["id"] != "" {
						rec.Sessions = append(rec.Sessions, SessionRec{
							BlockId: br.BlockId, WorkspaceId: ws.OID, WorkspaceName: ws.Name, TabId: tabId, TabName: tab.Name,
							TilePosition: tilePos, Kind: t.kind, StackId: t.stackId, GroupPosition: br.Position, Active: br.Active,
							Title: br.Title, Cwd: br.Cwd, Session: br.Session,
						})
					}
				}
				tilePos++
			}
			wsRec.Tabs = append(wsRec.Tabs, tabRec)
		}
		rec.Workspaces = append(rec.Workspaces, wsRec)
	}
	return rec
}

func containsID(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
