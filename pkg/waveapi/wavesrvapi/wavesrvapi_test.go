// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wavesrvapi

import (
	"errors"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/waveapi"
)

// Review finding F8: a start that hangs must not hold the caller; a failing or panicking start is not fatal
func TestStartInBackgroundDoesNotBlockTheCaller(t *testing.T) {
	release := make(chan struct{})
	returned := make(chan struct{})
	var done chan struct{}
	go func() {
		done = startInBackground(func() (*waveapi.Server, error) {
			<-release
			return nil, nil
		}, time.Hour)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatalf("startInBackground waited for a start that was still running")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("the background start never finished")
	}
}

func TestStartInBackgroundSurvivesAFailureAndAPanic(t *testing.T) {
	failed := startInBackground(func() (*waveapi.Server, error) { return nil, errors.New("no folder") }, time.Hour)
	panicked := startInBackground(func() (*waveapi.Server, error) { panic("boom") }, time.Hour)
	for _, c := range []chan struct{}{failed, panicked} {
		select {
		case <-c:
		case <-time.After(2 * time.Second):
			t.Fatalf("a failing start did not finish cleanly")
		}
	}
}

func TestABlockWithoutAControllerHasNoStatus(t *testing.T) {
	if s, c := statusOf("00000000-0000-0000-0000-000000000000"); s != "" || c != nil {
		t.Fatalf("no controller means no status: %q %v", s, c)
	}
}
