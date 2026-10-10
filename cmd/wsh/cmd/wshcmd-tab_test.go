// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestReorderTabIds(t *testing.T) {
	tests := []struct {
		name      string
		tabIds    []string
		oldIdx    int
		targetIdx int
		tabId     string
		expected  []string
	}{
		{
			name:      "move first to last",
			tabIds:    []string{"a", "b", "c", "d", "e"},
			oldIdx:    0,
			targetIdx: 4,
			tabId:     "a",
			expected:  []string{"b", "c", "d", "e", "a"},
		},
		{
			name:      "move last to first",
			tabIds:    []string{"a", "b", "c", "d", "e"},
			oldIdx:    4,
			targetIdx: 0,
			tabId:     "e",
			expected:  []string{"e", "a", "b", "c", "d"},
		},
		{
			name:      "move second to fourth (right)",
			tabIds:    []string{"a", "b", "c", "d", "e"},
			oldIdx:    1,
			targetIdx: 3,
			tabId:     "b",
			expected:  []string{"a", "c", "d", "b", "e"},
		},
		{
			name:      "move fourth to second (left)",
			tabIds:    []string{"a", "b", "c", "d", "e"},
			oldIdx:    3,
			targetIdx: 1,
			tabId:     "d",
			expected:  []string{"a", "d", "b", "c", "e"},
		},
		{
			name:      "move second to third (right by one)",
			tabIds:    []string{"a", "b", "c", "d", "e"},
			oldIdx:    1,
			targetIdx: 2,
			tabId:     "b",
			expected:  []string{"a", "c", "b", "d", "e"},
		},
		{
			name:      "move third to second (left by one)",
			tabIds:    []string{"a", "b", "c", "d", "e"},
			oldIdx:    2,
			targetIdx: 1,
			tabId:     "c",
			expected:  []string{"a", "c", "b", "d", "e"},
		},
		{
			name:      "two elements swap",
			tabIds:    []string{"x", "y"},
			oldIdx:    0,
			targetIdx: 1,
			tabId:     "x",
			expected:  []string{"y", "x"},
		},
		{
			name:      "move middle to end",
			tabIds:    []string{"a", "b", "c", "d", "e"},
			oldIdx:    2,
			targetIdx: 4,
			tabId:     "c",
			expected:  []string{"a", "b", "d", "e", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid mutating the test input
			input := make([]string, len(tt.tabIds))
			copy(input, tt.tabIds)
			result := reorderTabIds(input, tt.oldIdx, tt.targetIdx, tt.tabId)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("reorderTabIds(%v, %d, %d, %q) = %v, want %v",
					tt.tabIds, tt.oldIdx, tt.targetIdx, tt.tabId, result, tt.expected)
			}
			if len(result) != len(tt.tabIds) {
				t.Errorf("length changed: got %d, want %d", len(result), len(tt.tabIds))
			}
		})
	}
}

func TestTabCloseRefusesATabWithBlocksUnlessForced(t *testing.T) {
	err := checkTabCloseAllowed(3, nil, false)
	if err == nil || !strings.Contains(err.Error(), "3 blocks") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("a tab with 3 blocks must be refused and say how many and how to force: %v", err)
	}
	if err := checkTabCloseAllowed(1, nil, false); err == nil || !strings.Contains(err.Error(), "1 block") || strings.Contains(err.Error(), "1 blocks") {
		t.Fatalf("singular: %v", err)
	}
	if err := checkTabCloseAllowed(3, nil, true); err != nil {
		t.Fatalf("--force closes: %v", err)
	}
	if err := checkTabCloseAllowed(0, nil, false); err != nil {
		t.Fatalf("an empty tab closes without --force: %v", err)
	}
}

func TestTabCloseWhenTheCountCannotBeReadIsRefusedUnlessForced(t *testing.T) {
	cannot := fmt.Errorf("rpc down")
	if err := checkTabCloseAllowed(0, cannot, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("an unreadable tab must not be closed silently: %v", err)
	}
	if err := checkTabCloseAllowed(0, cannot, true); err != nil {
		t.Fatalf("--force skips the check: %v", err)
	}
}
