// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"os"
	"strings"
	"testing"
)

func uintp(v uint) *uint { return &v }

func TestCheckNodeSize(t *testing.T) {
	if got, err := checkNodeSize(nil); got != nil || err != nil {
		t.Fatalf("absent means the default: %v %v", got, err)
	}
	// only an absent field is the default; a client that sends 0 sent a weight, and 0 is outside 2 to 50
	if got, err := checkNodeSize(uintp(0)); err == nil || got != nil {
		t.Fatalf("a supplied 0 must be refused: %v %v", got, err)
	}
	for _, good := range []uint{2, 7, 10, 50} {
		got, err := checkNodeSize(uintp(good))
		if err != nil || got == nil || *got != good {
			t.Fatalf("%d must be kept: %v %v", good, got, err)
		}
	}
	for _, bad := range []uint{1, 51, 101, 1000, 1 << 31, ^uint(0)} {
		if got, err := checkNodeSize(uintp(bad)); err == nil || got != nil {
			t.Fatalf("%d must be refused: %v %v", bad, got, err)
		}
	}
}

func TestTheWeightIsCheckedBeforeAnythingIsCreated(t *testing.T) {
	src, err := os.ReadFile("wshserver.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (ws *WshServer) CreateBlockCommand(")
	if start < 0 {
		t.Fatal("CreateBlockCommand not found")
	}
	body := text[start:]
	check := strings.Index(body, "checkNodeSize(data.NodeSize)")
	create := strings.Index(body, "wcore.CreateBlock(")
	if check < 0 || create < 0 || check > create {
		t.Fatalf("checkNodeSize must run before wcore.CreateBlock (check at %d, create at %d)", check, create)
	}
	if !strings.Contains(body, "data.NodeSize = nodeSize") {
		t.Fatal("the checked weight must replace the requested one")
	}
}

// The handler itself: a weight outside the range is an error and nothing is made (the answer is the weight's own error, not a later one from the
// store, which this test does not even set up).
func TestTheHandlerRefusesAWeightOutOfRangeBeforeItTouchesAnything(t *testing.T) {
	ws := &WshServer{}
	for _, bad := range []uint{1, 51, 100, 1 << 31} {
		rtn, err := ws.CreateBlockCommand(context.Background(), wshrpc.CommandCreateBlockData{TabId: "tab-that-does-not-exist", NodeSize: uintp(bad)})
		if err == nil || rtn != nil {
			t.Fatalf("nodesize %d must be refused: %v %v", bad, rtn, err)
		}
		if !strings.Contains(err.Error(), "nodesize must be between") {
			t.Fatalf("nodesize %d: the refusal must be the weight's own (it came first), got %v", bad, err)
		}
	}
}
