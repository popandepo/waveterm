# Self test for Start-ResumableSession.ps1: a made-up config folder with and without a transcript. Starts nothing.
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'Start-ResumableSession.ps1'
$id = '11111111-2222-3333-4444-555555555555'
$root = Join-Path ([IO.Path]::GetTempPath()) ("resumable-test-" + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path (Join-Path $root 'projects\some-folder') -Force
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }

$firstOut = & $script -SessionId $id -ConfigDir $root -ThrowawayConfig -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
$first = $firstOut[0]
Check 'the folder that is searched is the folder Claude is told to use' ($firstOut[1] -eq "config=$root") ($firstOut -join ' | ')
Check 'first start creates the session under the id' ($first -match "--session-id $id" -and $first -notmatch '--resume') $first
Check 'first start carries model, effort, mode, name, prompt' ($first -match '--model m1' -and $first -match '--effort low' -and $first -match '--permission-mode auto' -and $first -match '--remote-control n1' -and $first -match 'hello') $first

Set-Content (Join-Path $root "projects\some-folder\$id.jsonl") '{}'
$later = & $script -SessionId $id -ConfigDir $root -ThrowawayConfig -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
Check 'a later start resumes the same id' ($later[0] -match "--resume $id" -and $later[0] -notmatch '--session-id') $later[0]
Check 'a resumed start does not repeat the first prompt' ($later[0] -notmatch 'hello') $later[0]
Check 'model and effort are kept on resume' ($later[0] -match '--model m1' -and $later[0] -match '--effort low') $later[0]

$other = & $script -SessionId '99999999-2222-3333-4444-555555555555' -ConfigDir $root -ThrowawayConfig -DryRun
Check 'another id is not mistaken for it' ($other[0] -match '--session-id 99999999') $other[0]
$threw = $false; try { & $script -SessionId 'not-a-guid' -ConfigDir $root -ThrowawayConfig -DryRun } catch { $threw = $true }
Check 'a session id that is not a guid is refused' $threw ''


# The launch itself: a stub stands in for claude and records its arguments and the environment it was started with.
$stub = Join-Path $root 'stub.ps1'
$rec = Join-Path $root 'record.txt'
Set-Content $stub @'
param()
$lines = @("ARGS: " + ($args -join ' '))
Get-ChildItem env: | Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -in 'NO_COLOR', 'FORCE_COLOR' } | ForEach-Object { $lines += "ENV: $($_.Name)=$($_.Value)" }
$lines += 'PATHLAST: ' + (@($env:PATH -split ';' | Where-Object { $_ })[-1])
$lines += 'PATHCOUNT: ' + (@($env:PATH -split ';' | Where-Object { $_ -ieq $env:RESUMABLE_TEST_WSHDIR }).Count)
$found = Get-Command wsh -ErrorAction SilentlyContinue
$lines += 'WSH: ' + $(if ($found) { $found.Source } else { 'none' })
$lines += 'WSHOUT: ' + $(if ($found) { (& wsh 2>&1 | Out-String).Trim() } else { 'none' })
Set-Content -LiteralPath $env:RESUMABLE_TEST_RECORD $lines
'@
# The launch runs in a child PowerShell process: the starter removes CLAUDE* and colour variables from the environment of the
# process it runs in, and this test must not leave the developer's own shell changed.
$child = Join-Path $root 'child.ps1'
Set-Content $child @'
param($Script, $Id, $Root, $Stub, $Record, $WshDir, $PathMode)
$env:CLAUDE_CONFIG_DIR = 'C:\somewhere\else'; $env:CLAUDE_CODE_CHILD_SESSION = '1'; $env:CLAUDECODE = '1'; $env:NO_COLOR = '1'
$env:RESUMABLE_TEST_RECORD = $Record
$env:RESUMABLE_TEST_WSHDIR = $WshDir
if ($PathMode -eq 'set') { $env:WAVETERM_WSHBINDIR = $WshDir } elseif ($PathMode -eq 'missing') { $env:WAVETERM_WSHBINDIR = $WshDir + '-nope' } elseif ($PathMode -eq 'already') { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = $WshDir + ';' + $env:PATH } elseif ($PathMode -eq 'spelling') { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = $WshDir.ToUpper() + '\..\' + (Split-Path $WshDir -Leaf) + '\;' + $env:PATH } elseif ($PathMode -eq 'relative') { $env:WAVETERM_WSHBINDIR = 'wshbin' } elseif ($PathMode -eq 'other') { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = $env:RESUMABLE_TEST_OTHERDIR + ';' + $env:PATH } else { Remove-Item Env:WAVETERM_WSHBINDIR -ErrorAction SilentlyContinue }
& $Script -SessionId $Id -ConfigDir $Root -ThrowawayConfig -Command $Stub -Model m1
'@
$shellBefore = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR', 'FORCE_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
$wshDir = Join-Path $root 'wshbin'
$null = New-Item -ItemType Directory -Path $wshDir
Set-Content (Join-Path $wshDir 'wsh.cmd') '@echo wsh-from-wave'
$otherDir = Join-Path $root 'otherbin'
$null = New-Item -ItemType Directory -Path $otherDir
Set-Content (Join-Path $otherDir 'wsh.cmd') '@echo wsh-already-there'
$env:RESUMABLE_TEST_OTHERDIR = $otherDir
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode set | Out-Null
$recorded = @(Get-Content -LiteralPath $rec)
$shellAfter = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR', 'FORCE_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
Check 'this shell is left as it was (the launch ran in a child process)' (($shellBefore -join '|') -eq ($shellAfter -join '|')) (($shellBefore -join '|') + ' -> ' + ($shellAfter -join '|'))
Check 'the launched program gets the searched config folder, not the inherited one' ($recorded -contains "ENV: CLAUDE_CONFIG_DIR=$root") ($recorded -join ' | ')
Check 'the inherited child-session markers are gone' (-not ($recorded -match 'ENV: (CLAUDE_CODE_CHILD_SESSION|CLAUDECODE)=')) ($recorded -join ' | ')
Check 'NO_COLOR is gone' (-not ($recorded -match 'ENV: NO_COLOR=')) ($recorded -join ' | ')
Check 'the launched program gets the resume arguments' (($recorded -join ' ') -match "ARGS: --resume $id") ($recorded -join ' | ')
Check 'the folder of wsh is put AFTER the existing PATH entries' ($recorded -contains "PATHLAST: $wshDir") ($recorded -join ' | ')
Check 'wsh can be run from there, and the stub prints what wsh printed' ($recorded -contains 'WSHOUT: wsh-from-wave') ($recorded -join ' | ')

foreach ($mode in 'missing', 'unset', 'relative') {
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode $mode | Out-Null
    $r = @(Get-Content -LiteralPath $rec)
    Check "with WAVETERM_WSHBINDIR $mode the PATH is left alone" (-not ($r -contains "PATH0: $wshDir") -and ($r -contains 'PATHCOUNT: 0')) ($r -join ' | ')
}
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode already | Out-Null
$r = @(Get-Content -LiteralPath $rec)
Check 'a folder that is already on the PATH is not added again' ($r -contains 'PATHCOUNT: 1') ($r -join ' | ')
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode spelling | Out-Null
$r = @(Get-Content -LiteralPath $rec)
Check 'the same folder under another spelling (case, dot dot, trailing slash) is not added again' (-not ($r -contains "PATHLAST: $wshDir")) ($r -join ' | ')
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode other | Out-Null
$r = @(Get-Content -LiteralPath $rec)
Check 'a wsh that is already on the PATH wins over the one Wave names' ($r -contains 'WSHOUT: wsh-already-there' -and ($r -contains "PATHLAST: $wshDir")) ($r -join ' | ')

# The config folder rules: no default, and .claude / .claude-work are refused with exit code 2 before the program is started.
$refuse = Join-Path $root 'refuse.ps1'
Set-Content $refuse @'
param($Script, $Id, $Stub, $Record, $Mode)
Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
$env:RESUMABLE_TEST_RECORD = $Record
switch ($Mode) {
    'unset' { & $Script -SessionId $Id -Command $Stub }
    'empty' { & $Script -SessionId $Id -ConfigDir '' -Command $Stub }
    'old' { & $Script -SessionId $Id -ConfigDir (Join-Path $env:USERPROFILE '.claude') -ThrowawayConfig -Command $Stub }
    'oldsub' { & $Script -SessionId $Id -ConfigDir (Join-Path $env:USERPROFILE '.claude\projects') -ThrowawayConfig -Command $Stub }
    'work' { & $Script -SessionId $Id -ConfigDir (Join-Path $env:USERPROFILE '.claude-work') -ThrowawayConfig -Command $Stub }
    'dots' { & $Script -SessionId $Id -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal\..\.claude') -Command $Stub }
    'elsewhere' { & $Script -SessionId $Id -ConfigDir (Join-Path $env:TEMP 'not-a-throwaway') -Command $Stub }
    'envold' { $env:CLAUDE_CONFIG_DIR = Join-Path $env:USERPROFILE '.claude'; & $Script -SessionId $Id -Command $Stub }
}
exit $LASTEXITCODE
'@
$ErrorActionPreference = 'Continue'
foreach ($mode in 'unset', 'empty', 'old', 'oldsub', 'work', 'dots', 'elsewhere', 'envold') {
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $refuse -Script $script -Id $id -Stub $stub -Record $rec -Mode $mode 2>$null | Out-Null
    Check "config folder case '$mode' is refused with exit code 2 and nothing is started" (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE record=$(Test-Path -LiteralPath $rec)"
}
$ErrorActionPreference = 'Stop'
$personal = Join-Path $env:USERPROFILE '.claude-personal\anything'
$dry = & $script -SessionId $id -ConfigDir $personal -DryRun
Check 'a folder under .claude-personal is accepted without the throwaway switch' ($dry[1] -eq "config=$personal") ($dry -join ' | ')
Remove-Item $stub, $rec, $child, $refuse, (Join-Path $wshDir 'wsh.cmd'), (Join-Path $otherDir 'wsh.cmd'), $otherDir, $wshDir -ErrorAction SilentlyContinue
Remove-Item (Join-Path $root "projects\some-folder\$id.jsonl"), (Join-Path $root 'projects\some-folder'), (Join-Path $root 'projects'), $root
if ($fail -gt 0) { exit 1 }
