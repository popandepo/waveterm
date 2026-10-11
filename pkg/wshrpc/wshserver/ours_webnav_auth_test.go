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
	oldBlock, oldWs := webNavCallerBlockFn, webNavWorkspaceOfFn
	webNavCallerBlockFn = func(route string) string { return routes[route] }
	webNavWorkspaceOfFn = func(ctx context.Context, blockId string) (string, error) {
		if ws, ok := workspaces[blockId]; ok {
			return ws, nil
		}
		return "", fmt.Errorf("no block %s", blockId)
	}
	t.Cleanup(func() { webNavCallerBlockFn, webNavWorkspaceOfFn = oldBlock, oldWs })
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

func TestTheRouterRecordsTheBlockAtAuthentication(t *testing.T) {
	src, err := os.ReadFile("../../wshutil/wshrouter_controlimpl.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	bind := strings.Index(text, "impl.Router.bindRoute(linkId, rtnData.RouteId, true)")
	set := strings.Index(text, "impl.Router.SetLinkAuthBlockId(linkId, rtnData.RpcContext.BlockId)")
	if bind < 0 || set < bind {
		t.Fatalf("the block of the token must be recorded right after the route is bound (bind at %d, set at %d)", bind, set)
	}
}
