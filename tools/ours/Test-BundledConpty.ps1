# Tests the pty layer of wavesrv with the bundled ConPTY (third_party\conpty) through cmd\conptyprobe, which opens the same pty module.
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
$ESC = [char]27

function Run([string[]]$probeArgs, [string]$conptyDir = $bundled) {
    if ($conptyDir) { $env:WAVETERM_CONPTY_DIR = $conptyDir } else { Remove-Item Env:WAVETERM_CONPTY_DIR -ErrorAction SilentlyContinue }
    try {
        $line = (& $Probe @probeArgs 2>&1 | Out-String).Trim()
        $json = $null
        try { $json = $line | ConvertFrom-Json } catch { }
        return [pscustomobject]@{ Code = $LASTEXITCODE; Json = $json; Raw = $line }
    } finally { Remove-Item Env:WAVETERM_CONPTY_DIR -ErrorAction SilentlyContinue }
}

# 0. the two binaries are the pinned ones: the hashes in third_party\conpty\README.md are the only copy of the pin
$readme = Get-Content -LiteralPath (Join-Path $repo 'third_party\conpty\README.md') -Raw
foreach ($f in 'conpty.dll', 'OpenConsole.exe') {
    $pinned = [regex]::Match($readme, '`x64/' + [regex]::Escape($f) + '`[^\r\n]*?`([0-9a-f]{64})`').Groups[1].Value
    $actual = (Get-FileHash -LiteralPath (Join-Path $bundled $f) -Algorithm SHA256).Hash.ToLower()
    Check "$f is the pinned file (sha256 in the README)" ($pinned -and $pinned -eq $actual) "pinned=$pinned actual=$actual"
}

# 1. the mouse-tracking and alternate-screen modes of the program reach the reader (the inbox ConPTY drops them: 31 Oct 2026)
$mouse = Join-Path $work 'mouse.ps1'
Set-Content -LiteralPath $mouse -Encoding utf8 -Value ('$e = [char]27' + "`n" + '[Console]::Out.Write("${e}[?1049h${e}[?1000h${e}[?1002h${e}[?1003h${e}[?1006hHELLO"); Start-Sleep 1')
$r = Run @('-wait', '5000', '--', 'pwsh', '-NoProfile', '-File', $mouse)
$modes = if ($r.Json) { $r.Json.modes } else { $null }
$allModes = $modes -and $modes.'1049h' -and $modes.'1000h' -and $modes.'1002h' -and $modes.'1003h' -and $modes.'1006h'
Check 'the alternate screen and all four mouse modes reach the reader' ([bool]$allModes -and $r.Json.text -match 'HELLO') $r.Raw

# 2. no fallback: a folder without conpty.dll fails, says why, and runs nothing
$empty = Join-Path $work 'empty'; $null = New-Item -ItemType Directory -Path $empty
$r = Run @('-wait', '3000', '--', 'pwsh', '-NoProfile', '-File', $mouse) $empty
Check 'without conpty.dll the pty fails with a message naming it, no fallback, nothing runs' ($r.Code -eq 1 -and $r.Json.error -match 'conpty\.dll' -and $r.Json.error -match 'no fallback' -and $r.Json.bytes -eq 0) $r.Raw

# 3. conpty.dll without OpenConsole.exe is not enough either
$half = Join-Path $work 'half'; $null = New-Item -ItemType Directory -Path $half
Copy-Item -LiteralPath (Join-Path $bundled 'conpty.dll') -Destination $half
$r = Run @('-wait', '3000', '--', 'pwsh', '-NoProfile', '-File', $mouse) $half
Check 'conpty.dll without OpenConsole.exe fails with a message naming OpenConsole.exe' ($r.Code -eq 1 -and $r.Json.error -match 'OpenConsole\.exe') $r.Raw

# 4. with no folder given, the files must sit next to the executable (wavesrv)
$bare = Join-Path $work 'bare'; $null = New-Item -ItemType Directory -Path $bare
Copy-Item -LiteralPath $Probe -Destination (Join-Path $bare 'conptyprobe.exe')
$bareLine = (& (Join-Path $bare 'conptyprobe.exe') '-wait' '3000' '--' 'pwsh' '-NoProfile' '-File' $mouse 2>&1 | Out-String).Trim()
Check 'with no folder given and no files next to the executable it fails the same way' ($bareLine -match 'conpty\.dll' -and $bareLine -match 'no fallback') $bareLine
Copy-Item -LiteralPath (Join-Path $bundled 'conpty.dll'), (Join-Path $bundled 'OpenConsole.exe') -Destination $bare
$bareLine = (& (Join-Path $bare 'conptyprobe.exe') '-wait' '5000' '--' 'pwsh' '-NoProfile' '-File' $mouse 2>&1 | Out-String).Trim()
Check 'with the files next to the executable it works and the mouse modes arrive' ($bareLine -match '1000h' -and $bareLine -match 'HELLO') $bareLine

# 5. a resize reaches the program
$size = Join-Path $work 'size.ps1'
# the program prints its size at once (start time varies), then again every 500 ms; the resize comes 5 s after the start of the probe
Set-Content -LiteralPath $size -Encoding utf8 -Value ('$start = Get-Date; while (((Get-Date) - $start).TotalSeconds -lt 9) { "SIZE=" + $Host.UI.RawUI.WindowSize.Width + "x" + $Host.UI.RawUI.WindowSize.Height; Start-Sleep -Milliseconds 400 }')
$r = Run @('-cols', '120', '-rows', '30', '-wait', '13000', '-resize', '100x40', '-resize-after', '5000', '--', 'pwsh', '-NoProfile', '-File', $size)
Check 'a resize is seen by the program (120x30 before, 100x40 after)' ($r.Json.text -match 'SIZE=120x30' -and $r.Json.text -match 'SIZE=100x40') $r.Raw

# 6. Unicode, wide and box-drawing characters come through as UTF-8
$uni = Join-Path $work 'uni.ps1'
$u8 = [Text.UTF8Encoding]::new($true)
[IO.File]::WriteAllText($uni, '[Console]::OutputEncoding = [Text.Encoding]::UTF8; [Console]::Out.Write("UNI:h' + [char]0x00E9 + 'llo " + [char]0x4E16 + [char]0x754C + " " + [char]::ConvertFromUtf32(0x1F600) + " " + [char]0x2591 + [char]0x2592 + " " + [char]0x250C + [char]0x2500 + [char]0x2510 + " END"); Start-Sleep -Milliseconds 800', $u8)
$rawFile = Join-Path $work 'uni.raw'
$r = Run @('-wait', '5000', '-raw', $rawFile, '--', 'pwsh', '-NoProfile', '-File', $uni)
$rawText = [Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes($rawFile))
$want = "UNI:h$([char]0x00E9)llo $([char]0x4E16)$([char]0x754C) $([char]::ConvertFromUtf32(0x1F600)) $([char]0x2591)$([char]0x2592) $([char]0x250C)$([char]0x2500)$([char]0x2510) END"
Check 'accents, wide CJK, an emoji and box-drawing characters arrive intact' ($rawText.Contains($want)) ($rawText.Length.ToString() + ' chars')

# 7. an interactive shell: input typed to the pty is run and the shell ends cleanly
$r = Run @('-wait', '8000', '-send', 'echo SHELL-OK\r', '-send-after', '1500', '--', 'cmd.exe')
$r2 = Run @('-wait', '8000', '-send', 'echo SHELL-OK\rexit\r', '-send-after', '1500', '--', 'cmd.exe')
Check 'input reaches an interactive shell and its output comes back' ($r.Json.text -match 'SHELL-OK') $r.Raw
Check 'exit typed into the shell ends it' ($r2.Json.exited -eq $true) $r2.Raw

# clean up: named files first, then the empty folders from the deepest up (no recursive force delete)
Get-ChildItem -LiteralPath $work -Recurse -File -Force | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue }
Get-ChildItem -LiteralPath $work -Recurse -Directory -Force | Sort-Object { $_.FullName.Length } -Descending | ForEach-Object { Remove-Item -LiteralPath $_.FullName -ErrorAction SilentlyContinue }
Remove-Item -LiteralPath $work -ErrorAction SilentlyContinue
"failures: $fail"
exit $fail
