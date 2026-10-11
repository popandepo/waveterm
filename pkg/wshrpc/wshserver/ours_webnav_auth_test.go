// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

func TestCheckWebNavCaller(t *testing.T) {
	if err := checkWebNavCaller(wshutil.DefaultRoute, "", "", ""); err != nil {
		t.Fatalf("the server itself is trusted: %v", err)
	}
	if err := checkWebNavCaller("proc:a", "blkA", "wsA", "wsA"); err != nil {
		t.Fatalf("same workspace must pass: %v", err)
	}
	cases := []struct {
		name                                                 string
		route, callerBlock, callerWorkspace, targetWorkspace string
	}{
		{"another workspace", "proc:a", "blkA", "wsA", "wsB"},
		{"no block behind the route", "proc:a", "", "wsA", "wsA"},
		{"caller workspace unknown", "proc:a", "blkA", "", "wsA"},
		{"target workspace unknown", "proc:a", "blkA", "wsA", ""},
		{"both unknown", "proc:a", "blkA", "", ""},
		{"electron is not the server", "electron", "", "", ""},
	}
	for _, c := range cases {
		if err := checkWebNavCaller(c.route, c.callerBlock, c.callerWorkspace, c.targetWorkspace); err == nil {
			t.Errorf("%s must be refused", c.name)
		}
	}
}

func withWebNavFakes(t *testing.T, routes map[string]string, workspaces map[string]string) {
	oldBlock, oldWs, oldSource := webNavCallerBlockFn, webNavWorkspaceOfFn, webNavSourceFn
	webNavSourceFn = func(ctx context.Context) string { return wshutil.ElectronRoute }
	webNavCallerBlockFn = func(route string) string { return routes[route] }
	webNavWorkspaceOfFn = func(ctx context.Context, blockId string) (string, error) {
		if ws, ok := workspaces[blockId]; ok {
			return ws, nil
		}
		return "", fmt.Errorf("no block %s", blockId)
	}
	t.Cleanup(func() { webNavCallerBlockFn, webNavWorkspaceOfFn, webNavSourceFn = oldBlock, oldWs, oldSource })
}

// The caller is the block behind the route that the router stamped, never an id in the request: a call from workspace A for a tile in workspace B is
// refused, whatever the request says.
func TestWebNavAuthorizeBindsTheRequestToTheCallersWorkspace(t *testing.T) {
	withWebNavFakes(t,
		map[string]string{"proc:inA": "blockA", "proc:inB": "blockB"},
		map[string]string{"blockA": "wsA", "tileA": "wsA", "blockB": "wsB", "tileB": "wsB"})
	ws := &WshServer{}
	ctx := context.Background()
	if err := ws.WebNavAuthorizeCommand(ctx, wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:inA", BlockId: "tileA"}); err != nil {
		t.Fatalf("a call from A for a tile in A: %v", err)
	}
	if err := ws.WebNavAuthorizeCommand(ctx, wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:inA", BlockId: "tileB"}); err == nil {
		t.Fatal("a call from workspace A for a tile in workspace B must be refused")
	}
	if err := ws.WebNavAuthorizeCommand(ctx, wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:inB", BlockId: "tileA"}); err == nil {
		t.Fatal("a call from workspace B for a tile in workspace A must be refused")
	}
	if err := ws.WebNavAuthorizeCommand(ctx, wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:nobody", BlockId: "tileA"}); err == nil {
		t.Fatal("a route that no block authenticated with must be refused")
	}
	if err := ws.WebNavAuthorizeCommand(ctx, wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:inA", BlockId: "no-such-tile"}); err == nil {
		t.Fatal("a tile that does not exist must be refused")
	}
	if err := ws.WebNavAuthorizeCommand(ctx, wshrpc.CommandWebNavAuthorizeData{CallerRoute: wshutil.DefaultRoute, BlockId: "tileB"}); err != nil {
		t.Fatalf("the server's own call (AI tools) is trusted: %v", err)
	}
}

func TestTheRouterRecordsTheBlockOnlyAfterTheRouteIsBound(t *testing.T) {
	src, err := os.ReadFile("../../wshutil/wshrouter_controlimpl.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (impl *WshRouterControlImpl) bindLeafIdentity(")
	if start < 0 {
		t.Fatal("bindLeafIdentity not found")
	}
	body := text[start:]
	body = body[:strings.Index(body, "\n}\n")]
	guard := strings.Index(body, `curRoute != "" && (curRoute != routeId || curBlock != blockId)`)
	bind := strings.Index(body, "impl.Router.bindRoute(linkId, routeId, true); err != nil")
	set := strings.Index(body, "impl.Router.SetLinkAuthBlockId(linkId, blockId)")
	if guard < 0 || bind < 0 || set < 0 || !(guard < bind && bind < set) {
		t.Fatalf("order must be: identity guard, bind (error checked), record (guard %d, bind %d, set %d)", guard, bind, set)
	}
	if !strings.Contains(body, "already authenticated as another route or block") {
		t.Fatal("the refusal text is missing")
	}
	// both ways of authenticating go through it: the JWT (every wsh command but "wsh token") and the swap token
	if n := strings.Count(text, "impl.bindLeafIdentity("); n != 2 {
		t.Fatalf("the JWT path and the token path must both call bindLeafIdentity, found %d calls", n)
	}
}

// One text for a tile that does not exist and for a tile in another workspace: a caller cannot tell which ids exist elsewhere.
func TestADeniedTileAnswersTheSameWhetherItExistsOrNot(t *testing.T) {
	withWebNavFakes(t,
		map[string]string{"proc:inA": "blockA"},
		map[string]string{"blockA": "wsA", "tileB": "wsB"})
	ws := &WshServer{}
	other := ws.WebNavAuthorizeCommand(context.Background(), wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:inA", BlockId: "tileB"})
	missing := ws.WebNavAuthorizeCommand(context.Background(), wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:inA", BlockId: "no-such-tile"})
	if other == nil || missing == nil {
		t.Fatalf("both must be refused: %v %v", other, missing)
	}
	if other.Error() != missing.Error() || other.Error() != WebNavDenied {
		t.Fatalf("the two answers must be the same text: %q and %q", other.Error(), missing.Error())
	}
}

func TestOnlyEmainMayNameAnotherCallersRoute(t *testing.T) {
	if got := callerRouteOf("electron", "proc:inA"); got != "proc:inA" {
		t.Fatalf("emain asks in the name of the caller: %q", got)
	}
	if got := callerRouteOf("proc:evil", "wavesrv"); got != "proc:evil" {
		t.Fatalf("a session cannot ask in the name of the server: %q", got)
	}
	if got := callerRouteOf("proc:evil", "proc:inA"); got != "proc:evil" {
		t.Fatalf("a session cannot ask in the name of another route: %q", got)
	}
	if got := callerRouteOf("proc:inA", ""); got != "proc:inA" {
		t.Fatalf("a session that names nothing asks about itself: %q", got)
	}
}

func TestASessionAskingInTheNameOfTheServerIsRefused(t *testing.T) {
	withWebNavFakes(t,
		map[string]string{"proc:inA": "blockA"},
		map[string]string{"blockA": "wsA", "tileB": "wsB"})
	old := webNavSourceFn
	webNavSourceFn = func(ctx context.Context) string { return "proc:inA" }
	t.Cleanup(func() { webNavSourceFn = old })
	ws := &WshServer{}
	if err := ws.WebNavAuthorizeCommand(context.Background(), wshrpc.CommandWebNavAuthorizeData{CallerRoute: wshutil.DefaultRoute, BlockId: "tileB"}); err == nil {
		t.Fatal("a session that claims to be the server must still be judged as itself")
	}
	if err := ws.WebNavAuthorizeCommand(context.Background(), wshrpc.CommandWebNavAuthorizeData{CallerRoute: "proc:inB", BlockId: "blockA"}); err != nil {
		t.Fatalf("a session asking about its own workspace is allowed whatever it claims: %v", err)
	}
}
