// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package workspaceservice

import (
	"strings"
	"testing"
)

func TestCloseTabCheckedRefusesATabThatGainedBlocks(t *testing.T) {
	if err := checkTabWithinApproved(3, 3); err != nil {
		t.Fatalf("same count is fine: %v", err)
	}
	if err := checkTabWithinApproved(2, 3); err != nil {
		t.Fatalf("fewer blocks than approved is fine: %v", err)
	}
	if err := checkTabWithinApproved(0, 0); err != nil {
		t.Fatalf("an empty tab that stayed empty closes: %v", err)
	}
	err := checkTabWithinApproved(4, 3)
	if err == nil || !strings.Contains(err.Error(), "gained") || !strings.Contains(err.Error(), "4") {
		t.Fatalf("a tab that grew after the approval must not be closed, and the error says so: %v", err)
	}
	if err := checkTabWithinApproved(9, -1); err != nil {
		t.Fatalf("a negative limit means no limit (force or an unreadable count that the owner confirmed): %v", err)
	}
}

func TestCountUnknownIsNotALimitAndMatchesTheWindow(t *testing.T) {
	if CloseTabCountUnknown != -2 {
		t.Fatalf("emain/closetabhandler.ts sends -2 for an approval without a count, got %d here", CloseTabCountUnknown)
	}
	if err := checkTabWithinApproved(9, CloseTabCountUnknown); err != nil {
		t.Fatalf("an approval without a count has no limit to compare with: %v", err)
	}
}
