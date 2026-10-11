// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshutil

import (
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
