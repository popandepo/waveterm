// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshutil

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// The signature is verified BEFORE any identity is bound: a token that is not signed by this server leaves the link untrusted, with no source route and no
// block, whatever its claims say. Each case is a way to forge a block id.
func TestAForgedJwtBindsNothing(t *testing.T) {
	useTestKeys(t)
	good := jwtFor(t, wshrpc.RpcContext{BlockId: "blockA", ProcRoute: true})
	other := jwtFor(t, wshrpc.RpcContext{BlockId: "blockB", ProcRoute: true})
	goodParts := strings.Split(good, ".")
	otherParts := strings.Split(other, ".")
	if len(goodParts) != 3 || len(otherParts) != 3 {
		t.Fatalf("expected three-part tokens, got %d and %d parts", len(goodParts), len(otherParts))
	}
	flipped := []byte(goodParts[2])
	if flipped[0] == 'A' {
		flipped[0] = 'B'
	} else {
		flipped[0] = 'A'
	}
	cases := []struct{ name, token string }{
		{"a changed signature", goodParts[0] + "." + goodParts[1] + "." + string(flipped)},
		{"claims of B under A's signature", goodParts[0] + "." + otherParts[1] + "." + goodParts[2]},
		{"alg none, no signature", b64(`{"alg":"none","typ":"JWT"}`) + "." + otherParts[1] + "."},
		{"alg none, A's signature", b64(`{"alg":"none","typ":"JWT"}`) + "." + otherParts[1] + "." + goodParts[2]},
		{"a cut signature", goodParts[0] + "." + goodParts[1] + "." + goodParts[2][:10]},
		{"two parts", goodParts[0] + "." + goodParts[1]},
		{"junk", "not a token at all"},
		{"empty", ""},
	}
	for _, c := range cases {
		name, token := c.name, c.token
		router, impl := routerWithUntrustedLink(1)
		if _, err := impl.AuthenticateCommand(authCtx(1), token); err == nil {
			t.Errorf("%s must be refused", name)
		}
		route, block := router.linkAuthIdentity(1)
		if route != "" || block != "" {
			t.Errorf("%s: a refused token left route %q and block %q on the link", name, route, block)
		}
		if router.linkMap[1].trusted {
			t.Errorf("%s: a refused token left the link trusted", name)
		}
		if got := router.BlockIdForSourceRoute("proc:forged"); got != "" {
			t.Errorf("%s: a block was found behind a route that was never bound: %q", name, got)
		}
	}
	router, impl := routerWithUntrustedLink(1)
	if _, err := impl.AuthenticateCommand(authCtx(1), good); err != nil {
		t.Fatalf("the control (the real token) must work: %v", err)
	}
	if route, block := router.linkAuthIdentity(1); route == "" || block != "blockA" {
		t.Fatalf("the control did not bind: route %q block %q", route, block)
	}
}
