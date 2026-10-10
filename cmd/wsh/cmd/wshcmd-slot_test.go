// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

type fakeSlotOps struct {
	blocks  map[string]string
	created []wshrpc.CommandCreateBlockData
	deleted []string
	next    int
	failOn  string
}

func newFakeSlotOps() *fakeSlotOps {
	return &fakeSlotOps{blocks: make(map[string]string)}
}

func (f *fakeSlotOps) SlotBlocks(tabId string) (map[string]string, error) {
	out := make(map[string]string, len(f.blocks))
	for k, v := range f.blocks {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSlotOps) CreateBlock(d wshrpc.CommandCreateBlockData) (string, error) {
	if f.failOn != "" {
		return "", fmt.Errorf("%s", f.failOn)
	}
	f.next++
	id := fmt.Sprintf("b%d", f.next)
	f.created = append(f.created, d)
	slot, _ := d.BlockDef.Meta[SlotMetaKey].(string)
	f.blocks[slot] = id
	return id, nil
}

func (f *fakeSlotOps) DeleteBlock(id string) error {
	f.deleted = append(f.deleted, id)
	for k, v := range f.blocks {
		if v == id {
			delete(f.blocks, k)
		}
	}
	return nil
}

func mustDefaultLayout(t *testing.T) *slotLayout {
	t.Helper()
	l, err := loadSlotLayout("")
	if err != nil {
		t.Fatalf("default layout: %v", err)
	}
	return l
}

func TestDefaultLayoutIsValidAndHasTheFourSlots(t *testing.T) {
	l := mustDefaultLayout(t)
	got := strings.Join(l.names(), ",")
	if got != "left,middle,right-top,right-bottom" {
		t.Errorf("slots = %s", got)
	}
}

func TestBuildDashboardThenMiddleOnDemand(t *testing.T) {
	l := mustDefaultLayout(t)
	ops := newFakeSlotOps()
	steps := []struct{ slot, view, wantAction, wantTarget string }{
		{"left", "term", "insert", ""},
		{"right-top", "web", SlotActionSplitRight, "b1"},
		{"right-bottom", "term", SlotActionSplitDown, "b2"},
		{"middle", "term", SlotActionSplitRight, "b1"},
	}
	for _, s := range steps {
		res, err := openInSlot(ops, l, "tab1", s.slot, s.view, nil, false)
		if err != nil {
			t.Fatalf("open %s: %v", s.slot, err)
		}
		if res.Action != s.wantAction || res.Target != s.wantTarget {
			t.Errorf("open %s: action=%s target=%s, want %s %s", s.slot, res.Action, res.Target, s.wantAction, s.wantTarget)
		}
	}
	for i, c := range ops.created {
		if c.Focused {
			t.Errorf("create %d was focused although focused=false was passed", i)
		}
		if c.BlockDef.Meta[SlotMetaKey] == nil || c.BlockDef.Meta[waveobj.MetaKey_View] == nil {
			t.Errorf("create %d lacks slot or view meta: %v", i, c.BlockDef.Meta)
		}
	}
}

func TestOpenInFilledSlotReplacesIt(t *testing.T) {
	l := mustDefaultLayout(t)
	ops := newFakeSlotOps()
	if _, err := openInSlot(ops, l, "tab1", "middle", "web", map[string]any{"url": "http://a/"}, true); err != nil {
		t.Fatal(err)
	}
	res, err := openInSlot(ops, l, "tab1", "middle", "web", map[string]any{"url": "http://b/"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != SlotActionReplace || res.Target != "b1" {
		t.Errorf("second open = %+v, want replace of b1", res)
	}
	last := ops.created[len(ops.created)-1]
	if last.TargetAction != SlotActionReplace || last.TargetBlockId != "b1" || !last.Focused {
		t.Errorf("request = %+v", last)
	}
	if last.BlockDef.Meta["url"] != "http://b/" {
		t.Errorf("url meta lost: %v", last.BlockDef.Meta)
	}
}

func TestMiddleFindsRightColumnWhenLeftIsEmpty(t *testing.T) {
	l := mustDefaultLayout(t)
	got, err := planSlotPlacement(l, "middle", map[string]string{"right-top": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != SlotActionSplitLeft || got.TargetBlockId != "x" {
		t.Errorf("plan = %+v", got)
	}
}

func TestEmptyTabInsertsNormally(t *testing.T) {
	l := mustDefaultLayout(t)
	got, err := planSlotPlacement(l, "right-bottom", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != "" || got.TargetBlockId != "" {
		t.Errorf("plan = %+v, want plain insert", got)
	}
}

func TestUnknownSlotIsRefusedAndNothingIsCreated(t *testing.T) {
	l := mustDefaultLayout(t)
	ops := newFakeSlotOps()
	_, err := openInSlot(ops, l, "tab1", "centre", "term", nil, true)
	if err == nil || !strings.Contains(err.Error(), "unknown slot") {
		t.Fatalf("err = %v", err)
	}
	if len(ops.created) != 0 {
		t.Errorf("created %d blocks for an unknown slot", len(ops.created))
	}
}

func TestSlotUserMetaCannotOverrideSlotOrView(t *testing.T) {
	l := mustDefaultLayout(t)
	ops := newFakeSlotOps()
	if _, err := openInSlot(ops, l, "tab1", "left", "term", map[string]any{"slot": "middle", "view": "web"}, true); err != nil {
		t.Fatal(err)
	}
	m := ops.created[0].BlockDef.Meta
	if m[SlotMetaKey] != "left" || m[waveobj.MetaKey_View] != "term" {
		t.Errorf("meta = %v", m)
	}
}

func TestCloseSlotDeletesItsBlockAndFreesTheSlot(t *testing.T) {
	l := mustDefaultLayout(t)
	ops := newFakeSlotOps()
	openInSlot(ops, l, "tab1", "left", "term", nil, true)
	openInSlot(ops, l, "tab1", "middle", "web", nil, true)
	id, err := closeSlot(ops, l, "tab1", "middle")
	if err != nil || id != "b2" {
		t.Fatalf("close = %q, %v", id, err)
	}
	if len(ops.deleted) != 1 || ops.deleted[0] != "b2" {
		t.Errorf("deleted = %v", ops.deleted)
	}
	again, err := closeSlot(ops, l, "tab1", "middle")
	if err != nil || again != "" {
		t.Errorf("second close = %q, %v; want empty and no error", again, err)
	}
	res, _ := openInSlot(ops, l, "tab1", "middle", "web", nil, true)
	if res.Action != SlotActionSplitRight || res.Target != "b1" {
		t.Errorf("reopen = %+v, want a fresh split next to left", res)
	}
}

func TestCreateFailureIsReported(t *testing.T) {
	l := mustDefaultLayout(t)
	ops := newFakeSlotOps()
	ops.failOn = "backend down"
	if _, err := openInSlot(ops, l, "tab1", "left", "term", nil, true); err == nil || !strings.Contains(err.Error(), "backend down") {
		t.Errorf("err = %v", err)
	}
}

func TestSlotsFromBlocksFiltersByTabAndKeepsFirst(t *testing.T) {
	mk := func(tab, id, slot string) wshrpc.BlocksListEntry {
		meta := waveobj.MetaMapType{}
		if slot != "" {
			meta[SlotMetaKey] = slot
		}
		return wshrpc.BlocksListEntry{TabId: tab, BlockId: id, Meta: meta}
	}
	got := slotsFromBlocks([]wshrpc.BlocksListEntry{
		mk("t1", "a", "left"), mk("t2", "b", "left"), mk("t1", "c", ""), mk("t1", "d", "left"), mk("t1", "e", "middle"),
	}, "t1")
	if len(got) != 2 || got["left"] != "a" || got["middle"] != "e" {
		t.Errorf("got %v", got)
	}
}

func TestParseSlotLayoutRejectsBadData(t *testing.T) {
	cases := map[string]string{
		"not json":       `{`,
		"no slots":       `{"slots":[]}`,
		"duplicate":      `{"slots":[{"name":"a"},{"name":"a"}]}`,
		"unknown anchor": `{"slots":[{"name":"a","anchors":[{"slot":"b","action":"splitright"}]}]}`,
		"self anchor":    `{"slots":[{"name":"a","anchors":[{"slot":"a","action":"splitright"}]}]}`,
		"bad action":     `{"slots":[{"name":"a","anchors":[{"slot":"b","action":"explode"}]},{"name":"b"}]}`,
		"empty name":     `{"slots":[{"name":" "}]}`,
	}
	for name, in := range cases {
		if _, err := parseSlotLayout([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCustomLayoutNamesAreData(t *testing.T) {
	l, err := parseSlotLayout([]byte(`{"slots":[{"name":"main"},{"name":"side","anchors":[{"slot":"main","action":"splitright"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := planSlotPlacement(l, "side", map[string]string{"main": "m1"})
	if err != nil || got.Action != SlotActionSplitRight || got.TargetBlockId != "m1" {
		t.Errorf("plan = %+v, %v", got, err)
	}
}
