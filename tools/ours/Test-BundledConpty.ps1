# Tests the pty layer of wavesrv with the bundled ConPTY (third_party\conpty) through cmd\conptyprobe, which opens the same pty module.
# The files are only ever taken from the folder of the running executable (no override), so every case copies the probe into a folder of its own.
# Needs the portable toolchain (C:\Projects\Personal\wave\toolchain\go). Nothing here starts Wave or touches its data folders.
#   pwsh -NoProfile -File tools\ours\Test-BundledConpty.ps1 [-Probe <exe>]   (builds the probe when -Probe is not given)
# Exit code = number of failed checks.
param([string]$Probe = '')
$ErrorActionPreference = 'Stop'
$repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$bundled = Join-Path $repo 'third_party\conpty\x64'
$fail = 0
function Check([string]$name, [bool]$ok, [string]$detail = '') {
    if ($ok) { "GREEN $name" } else { "RED   $name : $detail"; $script:fail++ }
}

if (-not $Probe) {
    $env:Path = 'C:\Projects\Personal\wave\toolchain\go\bin;' + $env:Path
    $env:CGO_ENABLED = '0'
    $Probe = Join-Path $repo 'dist\conptyprobe.exe'
    Push-Location $repo
    try { & go build -o $Probe ./cmd/conptyprobe; if ($LASTEXITCODE -ne 0) { throw "go build failed ($LASTEXITCODE)" } } finally { Pop-Location }
}

$work = Join-Path ([IO.Path]::GetTempPath()) ('conpty-test-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $work

# A folder with a copy of the probe, the named pinned files, and the named files altered by one appended byte.
function New-ProbeDir([string]$name, [string[]]$copy = @(), [string[]]$alter = @()) {
    $d = Join-Path $work $name
    $null = New-Item -ItemType Directory -Path $d
    Copy-Item -LiteralPath $Probe -Destination (Join-Path $d 'conptyprobe.exe')
    foreach ($f in $copy) { Copy-Item -LiteralPath (Join-Path $bundled $f) -Destination $d }
    foreach ($f in $alter) { [IO.File]::AppendAllText((Join-Path $d $f), 'x') }
    return $d
}
function Run([string[]]$probeArgs, [string]$dir) {
    $line = (& (Join-Path $dir 'conptyprobe.exe') @probeArgs 2>&1 | Out-String).Trim()
    $json = $null
    try { $json = $line | ConvertFrom-Json } catch { }
    return [pscustomobject]@{ Code = $LASTEXITCODE; Json = $json; Raw = $line }
}
$both = @('conpty.dll', 'OpenConsole.exe')
$good = New-ProbeDir 'good' $both

# 0. the pin: the hashes in third_party\conpty\README.md, the constants in the Go code and the files in the repo are the same, and git tracks both files
$readme = Get-Content -LiteralPath (Join-Path $repo 'third_party\conpty\README.md') -Raw
$goSrc = Get-Content -LiteralPath (Join-Path $repo 'third_party\pty\bundled_conpty_windows.go') -Raw
$goPins = @{
    'conpty.dll'      = [regex]::Match($goSrc, 'pinnedConptyDLL\s*=\s*"([0-9a-f]{64})"').Groups[1].Value
    'OpenConsole.exe' = [regex]::Match($goSrc, 'pinnedOpenConsole\s*=\s*"([0-9a-f]{64})"').Groups[1].Value
}
foreach ($f in $both) {
    $pinned = [regex]::Match($readme, '`x64/' + [regex]::Escape($f) + '`[^\r\n]*?`([0-9a-f]{64})`').Groups[1].Value
    $actual = (Get-FileHash -LiteralPath (Join-Path $bundled $f) -Algorithm SHA256).Hash.ToLower()
    Check "$f in the repo is the file the README pins" ($pinned -and $pinned -eq $actual) "pinned=$pinned actual=$actual"
    Check "$f : the hash in the Go code equals the README's" ($goPins[$f] -and $goPins[$f] -eq $pinned) "go=$($goPins[$f]) readme=$pinned"
}
$tracked = @(& git -C $repo ls-files -- 'third_party/conpty/x64') | ForEach-Object { Split-Path $_ -Leaf }
Check 'git tracks both binaries (a *.exe ignore rule does not drop OpenConsole.exe)' (($both | Where-Object { $tracked -notcontains $_ }).Count -eq 0) ($tracked -join ',')

# 1. the mouse-tracking and alternate-screen modes of the program reach the reader (the inbox ConPTY drops them: 31 Oct 2026)
$mouse = Join-Path $work 'mouse.ps1'
Set-Content -LiteralPath $mouse -Encoding utf8 -Value ('$e = [char]27' + "`n" + '[Console]::Out.Write("${e}[?1049h${e}[?1000h${e}[?1002h${e}[?1003h${e}[?1006hHELLO"); Start-Sleep 1')
$r = Run @('-wait', '5000', '--', 'pwsh', '-NoProfile', '-File', $mouse) $good
$modes = if ($r.Json) { $r.Json.modes } else { $null }
$allModes = $modes -and $modes.'1049h' -and $modes.'1000h' -and $modes.'1002h' -and $modes.'1003h' -and $modes.'1006h'
Check 'the alternate screen and all four mouse modes reach the reader' ([bool]$allModes -and $r.Json.text -match 'HELLO') $r.Raw

# 2. no fallback: nothing next to the executable fails, says why, and runs nothing
$empty = New-ProbeDir 'empty'
$r = Run @('-wait', '3000', '--', 'pwsh', '-NoProfile', '-File', $mouse) $empty
Check 'without conpty.dll the pty fails with a message naming it, no fallback, nothing runs' ($r.Code -eq 1 -and $r.Json.error -match 'conpty\.dll' -and $r.Json.error -match 'no fallback' -and $r.Json.bytes -eq 0) $r.Raw

# 3. conpty.dll without OpenConsole.exe is not enough either
$half = New-ProbeDir 'half' @('conpty.dll')
$r = Run @('-wait', '3000', '--', 'pwsh', '-NoProfile', '-File', $mouse) $half
Check 'conpty.dll without OpenConsole.exe fails with a message naming OpenConsole.exe' ($r.Code -eq 1 -and $r.Json.error -match 'OpenConsole\.exe') $r.Raw

# 4. an altered file is refused before anything is loaded or run
foreach ($f in $both) {
    $d = New-ProbeDir ('altered-' + ($f -replace '\W', '')) $both @($f)
    $r = Run @('-wait', '3000', '--', 'pwsh', '-NoProfile', '-File', $mouse) $d
    Check "an altered $f next to the executable fails the start, naming the file and the hash, and runs nothing" ($r.Code -eq 1 -and $r.Json.error -match [regex]::Escape($f) -and $r.Json.error -match 'sha256' -and $r.Json.bytes -eq 0) $r.Raw
}

# 5. there is no environment override: a folder named in the environment is ignored
$env:WAVETERM_CONPTY_DIR = $good
try { $r = Run @('-wait', '3000', '--', 'pwsh', '-NoProfile', '-File', $mouse) $empty } finally { Remove-Item Env:WAVETERM_CONPTY_DIR -ErrorAction SilentlyContinue }
Check 'WAVETERM_CONPTY_DIR is ignored: the executable still needs its own files' ($r.Code -eq 1 -and $r.Json.error -match 'conpty\.dll' -and $r.Json.bytes -eq 0) $r.Raw

# 6. a resize reaches the program
$size = Join-Path $work 'size.ps1'
# the program prints its size at once (start time varies), then again every 400 ms; the resize comes 5 s after the start of the probe
Set-Content -LiteralPath $size -Encoding utf8 -Value ('$start = Get-Date; while (((Get-Date) - $start).TotalSeconds -lt 9) { "SIZE=" + $Host.UI.RawUI.WindowSize.Width + "x" + $Host.UI.RawUI.WindowSize.Height; Start-Sleep -Milliseconds 400 }')
$r = Run @('-cols', '120', '-rows', '30', '-wait', '13000', '-resize', '100x40', '-resize-after', '5000', '--', 'pwsh', '-NoProfile', '-File', $size) $good
Check 'a resize is seen by the program (120x30 before, 100x40 after)' ($r.Json.text -match 'SIZE=120x30' -and $r.Json.text -match 'SIZE=100x40') $r.Raw

# 7. Unicode, wide and box-drawing characters come through as UTF-8
$uni = Join-Path $work 'uni.ps1'
$u8 = [Text.UTF8Encoding]::new($true)
[IO.File]::WriteAllText($uni, '[Console]::OutputEncoding = [Text.Encoding]::UTF8; [Console]::Out.Write("UNI:h' + [char]0x00E9 + 'llo " + [char]0x4E16 + [char]0x754C + " " + [char]::ConvertFromUtf32(0x1F600) + " " + [char]0x2591 + [char]0x2592 + " " + [char]0x250C + [char]0x2500 + [char]0x2510 + " END"); Start-Sleep -Milliseconds 800', $u8)
$rawFile = Join-Path $work 'uni.raw'
$r = Run @('-wait', '5000', '-raw', $rawFile, '--', 'pwsh', '-NoProfile', '-File', $uni) $good
$rawText = [Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes($rawFile))
$want = "UNI:h$([char]0x00E9)llo $([char]0x4E16)$([char]0x754C) $([char]::ConvertFromUtf32(0x1F600)) $([char]0x2591)$([char]0x2592) $([char]0x250C)$([char]0x2500)$([char]0x2510) END"
Check 'accents, wide CJK, an emoji and box-drawing characters arrive intact' ($rawText.Contains($want)) ($rawText.Length.ToString() + ' chars')

# 8. an interactive shell: input typed to the pty is run and the shell ends cleanly
$r = Run @('-wait', '8000', '-send', 'echo SHELL-OK\r', '-send-after', '1500', '--', 'cmd.exe') $good
$r2 = Run @('-wait', '8000', '-send', 'echo SHELL-OK\rexit\r', '-send-after', '1500', '--', 'cmd.exe') $good
Check 'input reaches an interactive shell and its output comes back' ($r.Json.text -match 'SHELL-OK') $r.Raw
Check 'exit typed into the shell ends it' ($r2.Json.exited -eq $true) $r2.Raw

# clean up: named files first, then the empty folders from the deepest up (no recursive force delete)
Get-ChildItem -LiteralPath $work -Recurse -File -Force | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue }
Get-ChildItem -LiteralPath $work -Recurse -Directory -Force | Sort-Object { $_.FullName.Length } -Descending | ForEach-Object { Remove-Item -LiteralPath $_.FullName -ErrorAction SilentlyContinue }
Remove-Item -LiteralPath $work -ErrorAction SilentlyContinue
"failures: $fail"
exit $fail
