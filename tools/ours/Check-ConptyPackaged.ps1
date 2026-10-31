# Fails (exit 1, names the file) when a packaged Wave app lacks the bundled ConPTY next to wavesrv, or holds a different file than the pinned one.
#   pwsh -NoProfile -File tools\ours\Check-ConptyPackaged.ps1 -Output <repo>\make\win-unpacked
# The pin (sha256) is read from third_party\conpty\README.md, the only copy of it. Reads only; changes nothing.
param([Parameter(Mandatory)][string]$Output)
$ErrorActionPreference = 'Stop'
$repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$readme = Get-Content -LiteralPath (Join-Path $repo 'third_party\conpty\README.md') -Raw
$bin = Join-Path $Output 'resources\app.asar.unpacked\dist\bin'
$bad = 0
if (-not (Test-Path -LiteralPath $bin -PathType Container)) { "FAIL the packaged wavesrv folder is missing: $bin"; exit 1 }
if (-not (Test-Path -LiteralPath (Join-Path $bin 'wavesrv.x64.exe'))) { "FAIL wavesrv.x64.exe is not in $bin"; $bad++ }
foreach ($f in 'conpty.dll', 'OpenConsole.exe') {
    $pinned = [regex]::Match($readme, '`x64/' + [regex]::Escape($f) + '`[^\r\n]*?`([0-9a-f]{64})`').Groups[1].Value
    $p = Join-Path $bin $f
    if (-not $pinned) { "FAIL no pinned sha256 for $f in the README"; $bad++; continue }
    if (-not (Test-Path -LiteralPath $p -PathType Leaf)) { "FAIL $f is missing next to wavesrv: $p"; $bad++; continue }
    $actual = (Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $pinned) { "FAIL $f is not the pinned file (sha256 $actual, pinned $pinned)"; $bad++; continue }
    "OK   $f next to wavesrv, sha256 matches the pin"
}
if ($bad) { "failures: $bad"; exit 1 }
'packaged app holds both files'
exit 0
