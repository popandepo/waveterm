// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"os"
	"strings"
	"testing"
)

func uintp(v uint) *uint { return &v }

func TestCheckNodeSize(t *testing.T) {
	if got, err := checkNodeSize(nil); got != nil || err != nil {
		t.Fatalf("absent means the default: %v %v", got, err)
	}
	if got, err := checkNodeSize(uintp(0)); got != nil || err != nil {
		t.Fatalf("0 means the default: %v %v", got, err)
	}
	for _, good := range []uint{1, 7, 10, 100} {
		got, err := checkNodeSize(uintp(good))
		if err != nil || got == nil || *got != good {
			t.Fatalf("%d must be kept: %v %v", good, got, err)
		}
	}
	for _, bad := range []uint{101, 1000, 1 << 31, ^uint(0)} {
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
