// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wstore

// CloseDB closes the object store. wavesrv never calls it; tests that open a store in a temporary folder do, so that the file
// is not held open (on Windows an open file cannot be removed).
func CloseDB() error {
	if globalDB == nil {
		return nil
	}
	err := globalDB.Close()
	globalDB = nil
	return err
}
