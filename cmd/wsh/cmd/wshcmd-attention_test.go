// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"
	"testing"
)

func TestAttentionMetaRequiresAReasonForTheOrangeLevels(t *testing.T) {
	for _, level := range []string{"need", "urgent"} {
		if _, err := attentionMeta(level, ""); err == nil {
			t.Errorf("%s without a reason must be refused", level)
		}
		if _, err := attentionMeta(level, "  "); err == nil {
			t.Errorf("%s with a blank reason must be refused", level)
		}
	}
	meta, err := attentionMeta("info", "")
	if err != nil || meta["attention:level"] != "info" || meta["attention:reason"] != nil {
		t.Fatalf("info needs no reason and clears an old one: %v %v", meta, err)
	}
	meta, err = attentionMeta("urgent", "  cannot go on  ")
	if err != nil || meta["attention:level"] != "urgent" || meta["attention:reason"] != "cannot go on" {
		t.Fatalf("urgent with a reason is written trimmed: %v %v", meta, err)
	}
}

func TestAttentionMetaRefusesWhatTheWindowWouldNotDraw(t *testing.T) {
	if _, err := attentionMeta("need", "a\nb"); err == nil {
		t.Fatalf("a line break in the reason must be refused")
	}
	if _, err := attentionMeta("need", strings.Repeat("x", 121)); err == nil {
		t.Fatalf("a reason over 120 characters must be refused")
	}
	if _, err := attentionMeta("loud", "x"); err == nil {
		t.Fatalf("an unknown level must be refused")
	}
}

func TestAttentionClearRemovesBothKeys(t *testing.T) {
	m := attentionClearMeta()
	v1, ok1 := m["attention:level"]
	v2, ok2 := m["attention:reason"]
	if !ok1 || !ok2 || v1 != nil || v2 != nil || len(m) != 2 {
		t.Fatalf("clear must set exactly the two keys to null: %v", m)
	}
}

func TestTheCommandHasTheThreeLevelsAndClear(t *testing.T) {
	names := map[string]bool{}
	for _, c := range attentionCmd.Commands() {
		names[c.Name()] = true
		if c.Name() != "clear" && c.Flags().Lookup("reason") == nil {
			t.Errorf("%s has no --reason", c.Name())
		}
	}
	for _, want := range []string{"info", "need", "urgent", "clear"} {
		if !names[want] {
			t.Errorf("wsh attention %s is missing", want)
		}
	}
}
