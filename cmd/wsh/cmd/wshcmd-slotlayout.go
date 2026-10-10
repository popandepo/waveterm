// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SlotMetaKey is the block meta key that records which slot a block fills. It lives on the
// block, so a closed block frees its slot without any other bookkeeping.
const SlotMetaKey = "slot"

const (
	SlotActionSplitRight = "splitright"
	SlotActionSplitLeft  = "splitleft"
	SlotActionSplitUp    = "splitup"
	SlotActionSplitDown  = "splitdown"
	SlotActionReplace    = "replace"
)

// defaultSlotLayoutJSON is data, not code: a slot opens next to the first anchor whose slot is
// occupied. Replace it with --layout-file to use other names or another arrangement.
//
// "unless" skips an anchor while any listed slot is filled. Splitting a block that sits in a
// two-block column only cuts that one block, not the column, so the right-column anchors of the
// outer slots are skipped when both right slots are filled.
//
// Wanted picture: left | middle | right-top over right-bottom. Build left, right-top, right-bottom,
// then open middle on demand; it lands between left and the right column.
const defaultSlotLayoutJSON = `{
  "slots": [
    {"name": "left", "anchors": [
      {"slot": "middle", "action": "splitleft"},
      {"slot": "right-top", "action": "splitleft", "unless": ["right-bottom"]},
      {"slot": "right-bottom", "action": "splitleft", "unless": ["right-top"]}]},
    {"name": "middle", "anchors": [
      {"slot": "left", "action": "splitright"},
      {"slot": "right-top", "action": "splitleft", "unless": ["right-bottom"]},
      {"slot": "right-bottom", "action": "splitleft", "unless": ["right-top"]}]},
    {"name": "right-top", "anchors": [
      {"slot": "right-bottom", "action": "splitup"},
      {"slot": "middle", "action": "splitright"},
      {"slot": "left", "action": "splitright"}]},
    {"name": "right-bottom", "anchors": [
      {"slot": "right-top", "action": "splitdown"},
      {"slot": "middle", "action": "splitright"},
      {"slot": "left", "action": "splitright"}]}
  ]
}`

type slotAnchor struct {
	Slot   string   `json:"slot"`
	Action string   `json:"action"`
	Unless []string `json:"unless,omitempty"`
}

type slotDef struct {
	Name    string       `json:"name"`
	Anchors []slotAnchor `json:"anchors,omitempty"`
}

type slotLayout struct {
	Slots []slotDef `json:"slots"`
}

// slotPlacement says where a new block goes. An empty Action means no anchor exists and the
// block takes the normal insert position.
type slotPlacement struct {
	Action        string
	TargetBlockId string
}

func isSlotAction(action string) bool {
	switch action {
	case SlotActionSplitRight, SlotActionSplitLeft, SlotActionSplitUp, SlotActionSplitDown:
		return true
	}
	return false
}

func parseSlotLayout(data []byte) (*slotLayout, error) {
	var layout slotLayout
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, fmt.Errorf("slot layout is not valid JSON: %w", err)
	}
	if len(layout.Slots) == 0 {
		return nil, fmt.Errorf("slot layout has no slots")
	}
	names := make(map[string]bool)
	for _, s := range layout.Slots {
		if strings.TrimSpace(s.Name) == "" {
			return nil, fmt.Errorf("slot layout has a slot with no name")
		}
		if names[s.Name] {
			return nil, fmt.Errorf("slot %q is defined twice", s.Name)
		}
		names[s.Name] = true
	}
	for _, s := range layout.Slots {
		for _, a := range s.Anchors {
			if !names[a.Slot] {
				return nil, fmt.Errorf("slot %q has an anchor on unknown slot %q", s.Name, a.Slot)
			}
			if a.Slot == s.Name {
				return nil, fmt.Errorf("slot %q is anchored on itself", s.Name)
			}
			for _, u := range a.Unless {
				if !names[u] {
					return nil, fmt.Errorf("slot %q anchor on %q has unknown unless slot %q", s.Name, a.Slot, u)
				}
			}
			if !isSlotAction(a.Action) {
				return nil, fmt.Errorf("slot %q anchor on %q has unknown action %q (use splitright, splitleft, splitup or splitdown)", s.Name, a.Slot, a.Action)
			}
		}
	}
	return &layout, nil
}

func loadSlotLayout(path string) (*slotLayout, error) {
	if path == "" {
		return parseSlotLayout([]byte(defaultSlotLayoutJSON))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading slot layout: %w", err)
	}
	return parseSlotLayout(data)
}

func (l *slotLayout) find(name string) *slotDef {
	for i := range l.Slots {
		if l.Slots[i].Name == name {
			return &l.Slots[i]
		}
	}
	return nil
}

func (l *slotLayout) names() []string {
	out := make([]string, 0, len(l.Slots))
	for _, s := range l.Slots {
		out = append(out, s.Name)
	}
	return out
}

func firstFilled(names []string, occupied map[string]string) string {
	for _, n := range names {
		if occupied[n] != "" {
			return n
		}
	}
	return ""
}

// planSlotPlacement decides how to open a block in slot. occupied maps slot name to block id.
func planSlotPlacement(layout *slotLayout, slot string, occupied map[string]string) (slotPlacement, error) {
	def := layout.find(slot)
	if def == nil {
		return slotPlacement{}, fmt.Errorf("unknown slot %q (known: %s)", slot, strings.Join(layout.names(), ", "))
	}
	if blockId := occupied[slot]; blockId != "" {
		return slotPlacement{Action: SlotActionReplace, TargetBlockId: blockId}, nil
	}
	var skipped []string
	for _, a := range def.Anchors {
		blockId := occupied[a.Slot]
		if blockId == "" {
			continue
		}
		if blocker := firstFilled(a.Unless, occupied); blocker != "" {
			skipped = append(skipped, fmt.Sprintf("%s (its column also holds %s)", a.Slot, blocker))
			continue
		}
		return slotPlacement{Action: a.Action, TargetBlockId: blockId}, nil
	}
	if len(skipped) > 0 {
		return slotPlacement{}, fmt.Errorf("cannot place slot %q: the only neighbours are %s, and splitting one of them would not give a full-height block; fill another slot first", slot, strings.Join(skipped, ", "))
	}
	return slotPlacement{}, nil
}
