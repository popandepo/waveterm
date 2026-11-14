// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellexec

import "os/exec"

// directCommand is the command of a cmd block whose meta says cmd:shell false: the program and its arguments, started as they are.
// Upstream starts every cmd block as "<shell> -c <command line>" even then (cmd:shell only changes how the line is quoted), so each
// session of ours had a shell process in front of the starter that did nothing but wait: 29 sessions held 29 extra pwsh (14 Nov 2026,
// about 1 GB private). The program name is looked up on the PATH of this process by exec.Command, as the shell did.
func directCommand(argv []string) *exec.Cmd {
	return exec.Command(argv[0], argv[1:]...)
}
