// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wps

import "sync"

// Observers see every event that is published, inside the process. They are for fork features that must react to changes
// (the state record); they do not go through routes and subscriptions. An observer must return quickly and never publish.
var (
	observerLock sync.Mutex
	observers    []func(event WaveEvent)
)

func AddObserver(fn func(event WaveEvent)) {
	observerLock.Lock()
	defer observerLock.Unlock()
	observers = append(observers, fn)
}

func notifyObservers(event WaveEvent) {
	observerLock.Lock()
	list := append([]func(event WaveEvent){}, observers...)
	observerLock.Unlock()
	for _, fn := range list {
		fn(event)
	}
}
