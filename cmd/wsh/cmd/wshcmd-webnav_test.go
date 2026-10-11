// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

const webBlockId = "44444444-4444-4444-4444-444444444444"

func webInfo(view string) *wshrpc.BlockInfoData {
	return &wshrpc.BlockInfoData{
		BlockId:     webBlockId,
		TabId:       "tab-other",
		WorkspaceId: "ws-1",
		Block:       &waveobj.Block{OID: webBlockId, Meta: waveobj.MetaMapType{"view": view}},
	}
}

func TestTheRequestCarriesTheTabAndWorkspaceOfTheBlockItself(t *testing.T) {
	data, err := makeWebNavRequest(webInfo("web"), webBlockId, wshrpc.WebNavAction_Reload)
	if err != nil {
		t.Fatal(err)
	}
	if data.TabId != "tab-other" || data.WorkspaceId != "ws-1" || data.BlockId != webBlockId || data.Action != "reload" {
		t.Fatalf("the request must name the block's own tab, not the caller's: %+v", data)
	}
}

func TestOnlyAWebTileIsNavigated(t *testing.T) {
	for _, view := range []string{"term", "preview", "", "webx"} {
		if _, err := makeWebNavRequest(webInfo(view), webBlockId, "reload"); err == nil || !strings.Contains(err.Error(), "not a web block") {
			t.Errorf("view %q must be refused, got %v", view, err)
		}
	}
	if _, err := makeWebNavRequest(nil, webBlockId, "reload"); err == nil {
		t.Errorf("a block that cannot be read is refused")
	}
	noTab := webInfo("web")
	noTab.TabId = ""
	if _, err := makeWebNavRequest(noTab, webBlockId, "reload"); err == nil {
		t.Errorf("a block with no tab is refused")
	}
}

func TestTheFourCommandsExistAndSendTheirAction(t *testing.T) {
	t.Setenv("WAVETERM_TABID", "caller-tab")
	oldInfo, oldRPC, oldBlockArg := webNavBlockInfo, webNavRPC, blockArg
	defer func() { webNavBlockInfo, webNavRPC, blockArg = oldInfo, oldRPC, oldBlockArg }()
	var sent []wshrpc.CommandWebNavData
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { return webInfo("web"), nil }
	webNavRPC = func(d wshrpc.CommandWebNavData) (*wshrpc.CommandWebNavRtnData, error) {
		sent = append(sent, d)
		return &wshrpc.CommandWebNavRtnData{Url: "http://127.0.0.1:1/", Title: "t"}, nil
	}
	oldResolve := resolveBlockRef
	defer func() { resolveBlockRef = oldResolve }()
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	for _, c := range []struct {
		name   string
		action string
		run    func() error
	}{
		{"reload", "reload", func() error { return webReloadCmd.RunE(webReloadCmd, nil) }},
		{"back", "back", func() error { return webBackCmd.RunE(webBackCmd, nil) }},
		{"forward", "forward", func() error { return webForwardCmd.RunE(webForwardCmd, nil) }},
		{"url", "url", func() error { return webUrlCmd.RunE(webUrlCmd, nil) }},
	} {
		sent = nil
		if err := c.run(); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(sent) != 1 || sent[0].Action != c.action || sent[0].TabId != "tab-other" {
			t.Errorf("%s: want one request with action %q for the block's own tab, got %+v", c.name, c.action, sent)
		}
	}
}

func TestAnErrorFromTheWindowIsReturnedAndNothingIsPrinted(t *testing.T) {
	oldInfo, oldRPC := webNavBlockInfo, webNavRPC
	defer func() { webNavBlockInfo, webNavRPC = oldInfo, oldRPC }()
	oldResolve := resolveBlockRef
	defer func() { resolveBlockRef = oldResolve }()
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { return webInfo("web"), nil }
	webNavRPC = func(d wshrpc.CommandWebNavData) (*wshrpc.CommandWebNavRtnData, error) {
		return nil, errors.New("nothing to go back to")
	}
	if err := webBackCmd.RunE(webBackCmd, nil); err == nil || !strings.Contains(err.Error(), "nothing to go back to") {
		t.Fatalf("the window's refusal must come back to the caller, got %v", err)
	}
}

func TestFormatWebNav(t *testing.T) {
	res := &wshrpc.CommandWebNavRtnData{Url: "file:///C:/x/y.html", Title: "Y", CanGoBack: true}
	text, err := formatWebNav("url", res, false)
	if err != nil || text != "file:///C:/x/y.html\nY" {
		t.Fatalf("url prints the address and then the title, got %q %v", text, err)
	}
	text, err = formatWebNav("url", res, true)
	if err != nil || !strings.Contains(text, `"cangoback": true`) || !strings.Contains(text, `"url": "file:///C:/x/y.html"`) {
		t.Fatalf("json carries the state, got %q %v", text, err)
	}
	text, err = formatWebNav("reload", res, false)
	if err != nil || !strings.HasPrefix(text, "reload requested") {
		t.Fatalf("the others print a short line, got %q %v", text, err)
	}
	if _, err := formatWebNav("url", nil, false); err == nil {
		t.Fatalf("no answer is an error")
	}
}
