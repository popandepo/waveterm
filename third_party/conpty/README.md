# Bundled ConPTY (Windows)

Why: the ConPTY that Windows 10 ships (kernel32 `CreatePseudoConsole`, run by conhost) drops the program's mouse-tracking sequences
(`?1000h`, `?1002h`, `?1003h`, `?1006h`) and handles the alternate screen (`?1049h`) itself, so a program in a Wave terminal never
gets a mouse click, and a fullscreen program is redrawn by conhost (stale and repeated text). The newer ConPTY passes them through.
Measured 31 Oct 2026 (reports in the MalfestioMaster repo, `reports\wave-conpty-mouse-2026-10-31.md`).

What: `wavesrv` loads `conpty.dll` from the folder of its own executable (`dist\bin`, unpacked from the app by electron-builder
`asarUnpack: dist/bin/**/*`). `conpty.dll` starts `OpenConsole.exe` from the same folder. `task build:conpty:windows` copies both
from here. If either is missing, opening a terminal fails with a message that names the file; there is NO fallback to the inbox
ConPTY. The pty code is `third_party/pty` (a copy of github.com/photostorm/pty v1.1.19-0.20230903182454-31354506054b, MIT, with
the three pseudo console calls taken from the bundled dll; `go.mod` replaces `github.com/creack/pty` with it).

Source (pinned): the release **microsoft/terminal v1.24.12741.0** (stable, 2 Oct 2026), asset
`Microsoft.Windows.Console.ConPTY.1.24.261001001.nupkg` (the official ConPTY package of that release; not taken from pywinpty).

| file | from inside the package | sha256 | signature |
| --- | --- | --- | --- |
| nupkg | `Microsoft.Windows.Console.ConPTY.1.24.261001001.nupkg` | `eceaafe3bdcc85e95d18666eb647e0b8e7e00af45dbcad7034cdc61c4a29fe74` | |
| `x64/conpty.dll` | `runtimes/win-x64/native/conpty.dll` | `5afea6c9480e2c7dfd195376dd10996b0ae25cdc6f014e6247fd8aedc8c34852` | Microsoft Corporation, valid |
| `x64/OpenConsole.exe` | `build/native/runtimes/x64/OpenConsole.exe` | `6f8e68dec4e8e5e15a54ecc8afa127b0e62139edc004ffbb4892125ff01d5737` | Microsoft Corporation, valid |

License: MIT (Microsoft Corporation). Works on Windows 10 1809 (build 17763) and later.

Update: download the same asset name of a newer release, put the two files here, change the version and the three hashes above in the
same commit, and run `tools/ours/Test-BundledConpty.ps1`. Check: `tools/ours/Test-BundledConpty.ps1` also verifies the hashes.
