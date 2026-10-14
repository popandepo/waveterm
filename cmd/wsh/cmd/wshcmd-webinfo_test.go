// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func TestWebInfoSendsTheInfoActionForTheBlocksOwnTab(t *testing.T) {
	oldAuth, oldInfo, oldRPC, oldResolve := webNavAuthorize, webNavBlockInfo, webNavRPC, resolveBlockRef
	defer func() { webNavAuthorize, webNavBlockInfo, webNavRPC, resolveBlockRef = oldAuth, oldInfo, oldRPC, oldResolve }()
	var sent []wshrpc.CommandWebNavData
	webNavAuthorize = func(blockId string) error { return nil }
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { return webInfo("web"), nil }
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	webNavRPC = func(d wshrpc.CommandWebNavData) (*wshrpc.CommandWebNavRtnData, error) {
		sent = append(sent, d)
		return &wshrpc.CommandWebNavRtnData{Url: "http://127.0.0.1:1/", Title: "t", State: "ready"}, nil
	}
	if err := webInfoCmd.RunE(webInfoCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0].Action != wshrpc.WebNavAction_Info || sent[0].TabId != "tab-other" {
		t.Fatalf("want one info request for the block's own tab, got %+v", sent)
	}
}

func TestWebInfoIsRefusedForAForeignTileBeforeAnythingIsRead(t *testing.T) {
	oldAuth, oldInfo, oldRPC, oldResolve := webNavAuthorize, webNavBlockInfo, webNavRPC, resolveBlockRef
	defer func() { webNavAuthorize, webNavBlockInfo, webNavRPC, resolveBlockRef = oldAuth, oldInfo, oldRPC, oldResolve }()
	read := false
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	webNavAuthorize = func(blockId string) error { return errString("no") }
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { read = true; return webInfo("web"), nil }
	webNavRPC = func(d wshrpc.CommandWebNavData) (*wshrpc.CommandWebNavRtnData, error) { read = true; return nil, nil }
	err := webInfoCmd.RunE(webInfoCmd, nil)
	if err == nil || err.Error() != webNavDenied {
		t.Fatalf("want the one denial text, got %v", err)
	}
	if read {
		t.Fatal("nothing about the tile may be read before the question is answered")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestFormatWebInfoText(t *testing.T) {
	res := &wshrpc.CommandWebNavRtnData{Url: "https://example.org/", Title: "Ex", CanGoBack: true, State: "failed"}
	text, err := formatWebNav(wshrpc.WebNavAction_Info, res, false)
	want := "address: https://example.org/\ntitle: Ex\nstate: failed\nback: yes\nforward: no"
	if err != nil || text != want {
		t.Fatalf("got %q %v want %q", text, err, want)
	}
}

func TestFormatWebInfoJsonCarriesTheState(t *testing.T) {
	res := &wshrpc.CommandWebNavRtnData{Url: "https://example.org/", State: "loading"}
	text, err := formatWebNav(wshrpc.WebNavAction_Info, res, true)
	if err != nil || !strings.Contains(text, `"state": "loading"`) || !strings.Contains(text, `"cangoback": false`) {
		t.Fatalf("got %q %v", text, err)
	}
}

func TestOnlyThreeStatesAreEverPrinted(t *testing.T) {
	for _, bad := range []string{"\x1b[31mred", "ready\nstate: x", "READY", "loading "} {
		res := &wshrpc.CommandWebNavRtnData{Url: "https://example.org/", State: bad}
		text, err := formatWebNav(wshrpc.WebNavAction_Info, res, false)
		if err != nil || !strings.Contains(text, "state: unknown\n") {
			t.Errorf("state %q must print as unknown, got %q %v", bad, text, err)
		}
		if strings.ContainsAny(text, "\x1b") {
			t.Errorf("an escape reached the output for %q", bad)
		}
	}
	for _, ok := range []string{"loading", "ready", "failed"} {
		text, _ := formatWebNav(wshrpc.WebNavAction_Info, &wshrpc.CommandWebNavRtnData{State: ok}, false)
		if !strings.Contains(text, "state: "+ok+"\n") {
			t.Errorf("state %s must print as it is, got %q", ok, text)
		}
	}
}

func TestAnEmptyOrMissingStateIsUnknownInBothModes(t *testing.T) {
	res := &wshrpc.CommandWebNavRtnData{Url: "https://example.org/", State: ""}
	text, err := formatWebNav(wshrpc.WebNavAction_Info, res, false)
	if err != nil || !strings.Contains(text, "state: unknown\n") {
		t.Fatalf("an empty state must print as unknown, got %q %v", text, err)
	}
	text, err = formatWebNav(wshrpc.WebNavAction_Info, res, true)
	if err != nil || !strings.Contains(text, `"state": "unknown"`) {
		t.Fatalf("an empty state must be \"unknown\" in JSON, not left out, got %q %v", text, err)
	}
}

func TestUrlJsonCarriesNoState(t *testing.T) {
	for _, st := range []string{"", "ready"} {
		text, err := formatWebNav(wshrpc.WebNavAction_Url, &wshrpc.CommandWebNavRtnData{Url: "https://example.org/", State: st}, true)
		if err != nil || strings.Contains(text, "state") {
			t.Fatalf("url --json has address, title and history only (state %q), got %q %v", st, text, err)
		}
	}
}

func TestUrlAndTheOthersDoNotPrintAState(t *testing.T) {
	res := &wshrpc.CommandWebNavRtnData{Url: "https://example.org/", Title: "Ex", State: "ready"}
	text, err := formatWebNav(wshrpc.WebNavAction_Url, res, false)
	if err != nil || strings.Contains(text, "state") {
		t.Fatalf("url prints address and title only, got %q %v", text, err)
	}
}
