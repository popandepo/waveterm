// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package attention

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type vectors struct {
	Check []struct {
		Level       string `json:"level"`
		Reason      string `json:"reason"`
		Ok          bool   `json:"ok"`
		ReasonOut   string `json:"reason_out"`
		RepeatText  string `json:"repeat_text"`
		RepeatCount int    `json:"repeat_count"`
	} `json:"check"`
	Highest []struct {
		Levels []string `json:"levels"`
		Want   string   `json:"want"`
	} `json:"highest"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	data, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCheckAgainstTheSharedVectors(t *testing.T) {
	v := loadVectors(t)
	if len(v.Check) < 10 {
		t.Fatalf("the vectors did not load")
	}
	for _, c := range v.Check {
		reason := c.Reason
		if c.RepeatCount > 0 {
			reason = strings.Repeat(c.RepeatText, c.RepeatCount)
		}
		got, err := Check(c.Level, reason)
		if c.Ok != (err == nil) {
			t.Errorf("Check(%q, %q): ok=%v err=%v", c.Level, c.Reason, c.Ok, err)
			continue
		}
		if c.Ok && c.RepeatCount == 0 && got != c.ReasonOut {
			t.Errorf("Check(%q, %q) reason %q, want %q", c.Level, c.Reason, got, c.ReasonOut)
		}
	}
}

func TestHighestAgainstTheSharedVectors(t *testing.T) {
	v := loadVectors(t)
	for _, c := range v.Highest {
		if got := Highest(c.Levels...); got != c.Want {
			t.Errorf("Highest(%v) = %q, want %q", c.Levels, got, c.Want)
		}
	}
}

func TestOfReadsMetaValuesAndRefusesWhatCheckRefuses(t *testing.T) {
	if l, r := Of("urgent", " why "); l != LevelUrgent || r != "why" {
		t.Fatalf("got %q %q", l, r)
	}
	if l, _ := Of("urgent", ""); l != LevelNone {
		t.Fatalf("an urgent mark without a reason, written by a program around the command, counts as none")
	}
	if l, _ := Of(5, "x"); l != LevelNone {
		t.Fatalf("a number is not a level")
	}
	if l, r := Of(nil, nil); l != LevelNone || r != "" {
		t.Fatalf("nothing set is no attention")
	}
}
