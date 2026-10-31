# third_party/pty in the waveterm fork

A copy of github.com/photostorm/pty v1.1.19-0.20230903182454-31354506054b (the module `go.mod` replaced github.com/creack/pty with until
31 Oct 2026), MIT, see LICENSE. Copied: every `.go` file, `go.mod`, `go.sum`, `LICENSE`. Not copied: the upstream tests, `.github`,
`.gitignore`, the Dockerfiles, `README.md`, `mktypes.bash`, `test_crosscompile.sh`, `asm_solaris_amd64.s` (nothing here builds for Solaris).

Changed against upstream, and only this:
- `pty_windows.go`: the three pseudo console calls (`CreatePseudoConsole`, `ClosePseudoConsole`, `ResizePseudoConsole`) are
  `bundledProc` values instead of `kernel32DLL.NewProc(...)` (three lines), and `procCreatePseudoConsole` calls
  `createPseudoConsole.CallChecked(...)` and returns its error before it reads the result (a failed verification used to come back as a
  zero result, which the caller took for success: a pty with a zero handle).
- `bundled_conpty_windows.go` (new): loads the pinned `conpty.dll` from the folder of the executable after both it and `OpenConsole.exe`
  match their pinned sha256 (again before every new console); no environment override; no fallback to the inbox ConPTY. Pins and source:
  `../conpty/README.md`.
- `bundled_conpty_windows_test.go` (new): `go test github.com/creack/pty` from the repo root; a bundled file changed after the dll was
  loaded must make `open()` fail.

Check that nothing else differs (Go module cache path as on the build machine; prints only the files above):

    diff -rq --exclude='*_test.go' <GOMODCACHE>/github.com/photostorm/pty@v1.1.19-0.20230903182454-31354506054b third_party/pty

Known upstream issues in code this fork takes unchanged, not fixed here (a follow-up PR is worth it: wavesrv runs all day):
- `run_windows.go`: the process wait is raced with `cmd.Wait`; the attribute list is not deleted when starting fails; `pi.Process` is never
  closed on success, so one process handle leaks per terminal.
- `pty_windows.go`: the pipes are not all closed when closing one fails in `open`.
