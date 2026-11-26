// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func decodeLayoutJson(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func tilesText(tiles []wshrpc.TabLayoutTile) string {
	var parts []string
	for _, tile := range tiles {
		text := strings.Join(tile.BlockIds, "+")
		if len(tile.BlockIds) > 1 {
			text += "(active " + tile.Active + ")"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

func TestTabLayoutTilesListsTilesInOrderWithGroupsAsOneTile(t *testing.T) {
	tree := decodeLayoutJson(t, `{"id":"root","children":[
		{"id":"l1","data":{"blockId":"a"}},
		{"id":"col","children":[
			{"id":"s1","kind":"stack","activeChildId":"l3","children":[{"id":"l2","data":{"blockId":"b"}},{"id":"l3","data":{"blockId":"c"}}]},
			{"id":"l4","data":{"blockId":"d"}}]}]}`)
	if got := tilesText(tabLayoutTiles(tree)); got != "a b+c(active c) d" {
		t.Fatalf("tiles = %q", got)
	}
}

func TestTabLayoutTilesOfASingleLeafRootAndOfNothing(t *testing.T) {
	root := decodeLayoutJson(t, `{"id":"l1","data":{"blockId":"only"}}`)
	if got := tilesText(tabLayoutTiles(root)); got != "only" {
		t.Fatalf("a root that is a leaf is one tile, got %q", got)
	}
	if got := tabLayoutTiles(nil); len(got) != 0 {
		t.Fatalf("no layout has no tiles, got %+v", got)
	}
}

func TestGetTabLayoutCommandReadsTheSavedLayoutAndNamesWhatIsNotAppliedYet(t *testing.T) {
	ctx := initDeleteTestStore(t)
	block := insertBlockInTab(t, ctx, "term")
	tabId, _ := wstore.DBFindTabForBlockId(ctx, block)
	tab, _ := wstore.DBMustGet[*waveobj.Tab](ctx, tabId)
	var root any
	if err := json.Unmarshal([]byte(`{"id":"r","children":[{"id":"l1","data":{"blockId":"x"}},{"id":"l2","data":{"blockId":"y"}}]}`), &root); err != nil {
		t.Fatal(err)
	}
	layout, _ := wstore.DBMustGet[*waveobj.LayoutState](ctx, tab.LayoutState)
	layout.RootNode = root
	pending := []waveobj.LayoutActionData{{ActionType: "swap", ActionId: uuid.NewString(), BlockId: "x", TargetBlockId: "y"}}
	layout.PendingBackendActions = &pending
	if err := wstore.DBUpdate(ctx, layout); err != nil {
		t.Fatal(err)
	}

	got, err := (&WshServer{}).GetTabLayoutCommand(ctx, wshrpc.CommandGetTabLayoutData{TabId: tabId})
	if err != nil {
		t.Fatal(err)
	}
	if got.TabId != tabId || tilesText(got.Tiles) != "x y" {
		t.Fatalf("got %+v", got)
	}
	if got.PendingActions != 1 {
		t.Fatalf("a queued action that the page has not applied must be counted, got %d", got.PendingActions)
	}
}

func TestGetTabLayoutCommandRefusesAMissingTab(t *testing.T) {
	ctx := initDeleteTestStore(t)
	if _, err := (&WshServer{}).GetTabLayoutCommand(ctx, wshrpc.CommandGetTabLayoutData{TabId: uuid.NewString()}); err == nil {
		t.Fatalf("an unknown tab must be refused")
	}
	if _, err := (&WshServer{}).GetTabLayoutCommand(ctx, wshrpc.CommandGetTabLayoutData{}); err == nil {
		t.Fatalf("an empty tab id must be refused")
	}
}
