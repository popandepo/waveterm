# Boot fixes: what to build and swap for the real run

Written 18 Oct 2026 by the wave session. All four boot fixes are merged in Master at `464e143d`. None of them runs until a NEW
wavesrv and a NEW wsh are installed and Wave is restarted; the Wave that runs now is `Programs\WaveFork\3ee658d4` and has none of them.

## What changes after the swap

| PR | Where it runs | Effect |
| --- | --- | --- |
| 81 | wavesrv | Every shell/cmd block with `cmd:runonstart` true and no `cmd:runonce` is started at backend start, tab shown or not. A short command that ends before its view mounts is not run again by the first mount. |
| 82 | wavesrv | `wsh deleteblock` on the last block of a tab no longer answers with a nil-pointer panic. |
| 83 | wsh | `wsh session start` (new tab, without `--stack`) creates the block before the tab's page is loaded, so the page's orphan clean-up cannot delete it. `Wave-Sessions.ps1 open` stops losing its block. |

wsh needs no separate step: Wave copies its bundled wsh to `%LOCALAPPDATA%\WaveFork\profile\data\bin\wsh.exe` at every start
(`initCustomShellStartupFilesInternal` in `pkg/util/shellutil/shellutil.go`), so the new install brings the new wsh with it.

## Before: the one thing that must be done first

The Tether agent runs now as a hidden background process (`tether.exe`, started by hand). The restart block for the agent
(block `82227bcd-d38d-45ab-ab44-1d24c5c9a151`, Tether tab) will start `tether.exe` at the first start of the new wavesrv. Two agents cannot
share port 47474, so the hand-started one must be stopped first, as late as possible (his phone link is down from then until the new
wavesrv starts the block, a minute or two): find it with `Get-Process tether`, stop that one process.

## Build (does not touch the running Wave; heavy on CPU, so not while he plays)

```
pwsh -File C:\Projects\Personal\wave\tools\Build-WaveFork.ps1
```

This uses the fork's own toolchain (Go 1.26.8, zig as the C compiler, task), builds wavesrv and wsh (`task build:backend`), the frontend
(`npm run build:prod`) and the unpacked app (`electron-builder --dir`). Output: `C:\Projects\Personal\wave\waveterm\make\win-unpacked`.
The main checkout must be on Master at or after `464e143d` (`git -C C:\Projects\Personal\wave\waveterm log -1 --oneline`).
The script prints `commit: <id>`; use that id below.

## Install (a new folder; nothing is overwritten or deleted)

```
pwsh -File C:\Projects\Personal\wave\tools\Install-WaveFork.ps1 -Commit 464e143d
```

It copies the build to `%LOCALAPPDATA%\Programs\WaveFork\464e143d\` and points the Desktop shortcut "Wave (fork)" at it. The old folders stay.
Rollback: point the shortcut at `Programs\WaveFork\3ee658d4\WaveFork.exe` again and restart Wave.

## Restart

Through the launcher only (Wave.exe started directly opens the empty default profile):

```
pwsh -File C:\Projects\Personal\MalfestioMaster\tools\Restart-Wave.ps1 -DryRun
pwsh -File C:\Projects\Personal\MalfestioMaster\tools\Restart-Wave.ps1 -WaitForPid <manager claude pid>
```

or a planned PC restart (`Prepare-Restart.ps1`); both start Wave through `Get-WaveLauncher.ps1`, which follows the shortcut.
Every session in Wave ends at the restart and comes back through its own restart block.

## After: what to look at (each line says what would be red)

1. Which wavesrv runs: `Get-CimInstance Win32_Process -Filter "Name='wavesrv.x64.exe'" | Select ExecutablePath`; red if the path still contains `3ee658d4`.
2. wsh was replaced: `(Get-Item $env:LOCALAPPDATA\WaveFork\profile\data\bin\wsh.exe).LastWriteTime` is the time of the start; red if it is older.
3. Step 1, restart blocks start without their tab shown: `Select-String '\[bootstart\]' $env:LOCALAPPDATA\WaveFork\profile\data\waveapp.log` has one `started restart block <id>` line per restart block, and `wsh session list` shows those blocks `running` while the tabs were never displayed; red if a restart block has status `none`.
4. The Tether agent: block 82227bcd is `running`, `Get-Process tether` shows exactly ONE process, `Get-NetTCPConnection -LocalPort 47474 -State Listen` has an owner, the phone reconnects. Red: two agents, or 82227bcd `exited` with a "port in use" message (the hand-started agent was not stopped).
5. No command ran twice: for a restart block that ends quickly, the terminal shows one run, not two.
6. Step 3, from a manager block: `Wave-Sessions.ps1 open -Cwd <repo> -Name <name> -Account auto -Prompt "..."`; the new tab keeps its program block after 60 seconds and `starter.log` gets a line for it. Red: the block is replaced by a bare shell at `C:/Users/popan` (the old fault). One success proves little, the fault was a race: repeat it three times.
7. Step 4: `wsh deleteblock --force -b <the only block of a throwaway tab>` answers without "panic in handleRequest".

## Not verified before this report

All four fixes have unit tests that pass with the fork toolchain (`go test` in `pkg/blockcontroller`, `pkg/wshrpc/wshserver`, `cmd/wsh/cmd`). None of them has been run against a real Wave start: that is what the steps above do. Known limits, written in the PRs: a metadata edit made during the boot pass itself, a second mount of a finished restart block (restarts, by design), restart blocks on a remote connection that is not up (skipped, no retry), and a person opening a just-created background tab during the block RPC of `wsh session start`.
