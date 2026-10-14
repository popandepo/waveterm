// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshutil

import (
	"context"

	"github.com/wavetermdev/waveterm/pkg/wavejwt"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/baseds"
)

func routerWithLeaf(linkId baseds.LinkId, kind string, sourceRoute string) *WshRouter {
	router := NewWshRouter()
	router.linkMap[linkId] = &linkMeta{linkId: linkId, trusted: true, linkKind: kind, sourceRouteId: sourceRoute}
	router.routeMap[sourceRoute] = linkId
	return router
}

func TestBlockIdForSourceRouteIsTheBlockOfTheToken(t *testing.T) {
	router := routerWithLeaf(1, LinkKind_Leaf, "proc:aaa")
	if got := router.BlockIdForSourceRoute("proc:aaa"); got != "" {
		t.Fatalf("a link that has not recorded a block says %q", got)
	}
	router.SetLinkAuthBlockId(1, "block-1")
	if got := router.BlockIdForSourceRoute("proc:aaa"); got != "block-1" {
		t.Fatalf("got %q", got)
	}
}

func TestBlockIdForSourceRouteRefusesWhatIsNotALeafSourceRoute(t *testing.T) {
	router := routerWithLeaf(1, LinkKind_Leaf, "proc:aaa")
	router.SetLinkAuthBlockId(1, "block-1")
	if got := router.BlockIdForSourceRoute("proc:unknown"); got != "" {
		t.Fatalf("an unknown route must give nothing, got %q", got)
	}
	// a route that points at a link, but is not that link's own source route (a route announced by a router behind it)
	router.routeMap["proc:other"] = 1
	if got := router.BlockIdForSourceRoute("proc:other"); got != "" {
		t.Fatalf("a route that is not the source route of the link must give nothing, got %q", got)
	}
	// a router link never carries a block of its own, even if one was set by mistake
	rr := routerWithLeaf(2, LinkKind_Router, "proc:bbb")
	rr.SetLinkAuthBlockId(2, "block-2")
	if got := rr.BlockIdForSourceRoute("proc:bbb"); got != "" {
		t.Fatalf("a router link must give nothing, got %q", got)
	}
	// a link that is gone
	delete(router.linkMap, 1)
	if got := router.BlockIdForSourceRoute("proc:aaa"); got != "" {
		t.Fatalf("a removed link must give nothing, got %q", got)
	}
}

func TestALinkIsRecordedAsOneBlockOnly(t *testing.T) {
	router := routerWithLeaf(1, LinkKind_Leaf, "proc:aaa")
	if err := router.SetLinkAuthBlockId(1, "block-1"); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := router.SetLinkAuthBlockId(1, "block-1"); err != nil {
		t.Fatalf("the same block again changes nothing: %v", err)
	}
	if err := router.SetLinkAuthBlockId(1, "block-2"); err == nil {
		t.Fatal("another block on the same link must be refused")
	}
	if got := router.BlockIdForSourceRoute("proc:aaa"); got != "block-1" {
		t.Fatalf("the recorded block must stay block-1, got %q", got)
	}
	route, block := router.linkAuthIdentity(1)
	if route != "proc:aaa" || block != "block-1" {
		t.Fatalf("identity: %q %q", route, block)
	}
}

func TestABlockIsNotRecordedOnALinkWithoutASourceRoute(t *testing.T) {
	router := NewWshRouter()
	router.linkMap[1] = &linkMeta{linkId: 1, trusted: true, linkKind: LinkKind_Leaf}
	if err := router.SetLinkAuthBlockId(1, "block-1"); err == nil {
		t.Fatal("a leaf link whose route was never bound must refuse a block")
	}
	if err := router.SetLinkAuthBlockId(9, "block-1"); err == nil {
		t.Fatal("a link that does not exist must refuse a block")
	}
	route, block := router.linkAuthIdentity(1)
	if route != "" || block != "" {
		t.Fatalf("nothing recorded: %q %q", route, block)
	}
}

type stubClient struct{}

func (stubClient) GetPeerInfo() string                                             { return "stub" }
func (stubClient) SendRpcMessage(msg []byte, ingress baseds.LinkId, d string) bool { return true }
func (stubClient) RecvRpcMessage() ([]byte, bool)                                  { return nil, false }

func authCtx(linkId baseds.LinkId) context.Context {
	return context.WithValue(context.Background(), wshRpcRespHandlerContextKey{}, &RpcResponseHandler{ingressLinkId: linkId})
}

func routerWithUntrustedLink(linkId baseds.LinkId) (*WshRouter, *WshRouterControlImpl) {
	router := NewWshRouter()
	router.linkMap[linkId] = &linkMeta{linkId: linkId, client: stubClient{}}
	return router, &WshRouterControlImpl{Router: router}
}

func useTestKeys(t *testing.T) {
	t.Helper()
	kp, err := wavejwt.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := wavejwt.SetPublicKey(kp.PublicKey); err != nil {
		t.Fatal(err)
	}
	if err := wavejwt.SetPrivateKey(kp.PrivateKey); err != nil {
		t.Fatal(err)
	}
}

func jwtFor(t *testing.T, rpcCtx wshrpc.RpcContext) string {
	t.Helper()
	tok, err := MakeClientJWTToken(rpcCtx)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// The way wsh authenticates for every command but "wsh token": the JWT in WAVETERM_JWT, through AuthenticateCommand on the link. The block of the JWT must be the
// block that the router knows behind the route it bound.
func TestAJwtAuthenticatedLinkIsKnownByItsBlock(t *testing.T) {
	useTestKeys(t)
	router, impl := routerWithUntrustedLink(1)
	rtn, err := impl.AuthenticateCommand(authCtx(1), jwtFor(t, wshrpc.RpcContext{BlockId: "blockA", ProcRoute: true}))
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got := router.BlockIdForSourceRoute(rtn.RouteId); got != "blockA" {
		t.Fatalf("the route %q must lead to blockA, got %q", rtn.RouteId, got)
	}
}

func TestASecondJwtOnTheSameLinkCannotChangeTheIdentity(t *testing.T) {
	useTestKeys(t)
	router, impl := routerWithUntrustedLink(1)
	first, err := impl.AuthenticateCommand(authCtx(1), jwtFor(t, wshrpc.RpcContext{BlockId: "blockA", RouteId: "tab:route-a"}))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := impl.AuthenticateCommand(authCtx(1), jwtFor(t, wshrpc.RpcContext{BlockId: "blockB", RouteId: "tab:route-b"})); err == nil {
		t.Fatal("a second JWT for another route and block on the same link must be refused")
	}
	if _, err := impl.AuthenticateCommand(authCtx(1), jwtFor(t, wshrpc.RpcContext{BlockId: "blockB", RouteId: "tab:route-a"})); err == nil {
		t.Fatal("the same route with another block must be refused")
	}
	if got := router.BlockIdForSourceRoute(first.RouteId); got != "blockA" {
		t.Fatalf("the identity must stay blockA, got %q", got)
	}
	if _, err := impl.AuthenticateCommand(authCtx(1), jwtFor(t, wshrpc.RpcContext{BlockId: "blockA", RouteId: "tab:route-a"})); err != nil {
		t.Fatalf("the same route and block again changes nothing and is allowed: %v", err)
	}
}

func TestAnUnknownRouteOfAnotherLinkHasNoBlock(t *testing.T) {
	useTestKeys(t)
	router, impl := routerWithUntrustedLink(1)
	router.linkMap[2] = &linkMeta{linkId: 2, client: stubClient{}}
	a, _ := impl.AuthenticateCommand(authCtx(1), jwtFor(t, wshrpc.RpcContext{BlockId: "blockA", ProcRoute: true}))
	b, _ := impl.AuthenticateCommand(authCtx(2), jwtFor(t, wshrpc.RpcContext{BlockId: "blockB", ProcRoute: true}))
	if router.BlockIdForSourceRoute(a.RouteId) != "blockA" || router.BlockIdForSourceRoute(b.RouteId) != "blockB" {
		t.Fatalf("each link must lead to its own block: %q %q", router.BlockIdForSourceRoute(a.RouteId), router.BlockIdForSourceRoute(b.RouteId))
	}
}
