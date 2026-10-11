// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellexec

// LocalPid is the process id of a shell that runs on this machine under a pty of its own. A remote or WSL shell has none that means anything here.
func (sp *ShellProc) LocalPid() (int, bool) {
	if sp == nil {
		return 0, false
	}
	cw, ok := sp.Cmd.(CmdWrap)
	if !ok || cw.Cmd == nil || cw.Cmd.Process == nil {
		return 0, false
	}
	return cw.Cmd.Process.Pid, true
}
