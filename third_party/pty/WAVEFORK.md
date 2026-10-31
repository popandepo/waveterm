# third_party/pty in the waveterm fork

A copy of github.com/photostorm/pty v1.1.19-0.20230903182454-31354506054b (the module `go.mod` replaced github.com/creack/pty with until
31 Oct 2026), MIT, see LICENSE. Copied: every `.go` file, `go.mod`, `go.sum`, `LICENSE`. Not copied: the upstream tests, `.github`,
`.gitignore`, the Dockerfiles, `README.md`, `mktypes.bash`, `test_crosscompile.sh`, `asm_solaris_amd64.s` (nothing here builds for Solaris).

Changed against upstream, and only this:
- `pty_windows.go`: the three pseudo console calls (`CreatePseudoConsole`, `ClosePseudoConsole`, `ResizePseudoConsole`) are
  `bundledProc` values instead of `kernel32DLL.NewProc(...)` (three lines).
- `bundled_conpty_windows.go` (new): loads the pinned `conpty.dll` from the folder of the executable after both it and `OpenConsole.exe`
  match their pinned sha256; no environment override; no fallback to the inbox ConPTY. Pins and source: `../conpty/README.md`.

Check that nothing else differs (Go module cache path as on the build machine; prints only the files above):

    diff -rq --exclude='*_test.go' <GOMODCACHE>/github.com/photostorm/pty@v1.1.19-0.20230903182454-31354506054b third_party/pty

Known upstream issues in code this fork takes unchanged, not fixed here: `run_windows.go` (process wait raced with `cmd.Wait`; the
attribute list is not deleted when starting fails) and `pty_windows.go` (pipes not all closed when closing one fails in `open`).
