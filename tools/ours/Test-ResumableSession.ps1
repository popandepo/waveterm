# Self test for Start-ResumableSession.ps1: a made-up config folder with and without a transcript. Starts nothing.
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'Start-ResumableSession.ps1'
$id = '11111111-2222-3333-4444-555555555555'
$root = Join-Path ([IO.Path]::GetTempPath()) ("resumable-test-" + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path (Join-Path $root 'projects\some-folder') -Force
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }

$firstOut = & $script -SessionId $id -ConfigDir $root -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
$first = $firstOut[0]
Check 'the folder that is searched is the folder Claude is told to use' ($firstOut[1] -eq "config=$root") ($firstOut -join ' | ')
Check 'first start creates the session under the id' ($first -match "--session-id $id" -and $first -notmatch '--resume') $first
Check 'first start carries model, effort, mode, name, prompt' ($first -match '--model m1' -and $first -match '--effort low' -and $first -match '--permission-mode auto' -and $first -match '--remote-control n1' -and $first -match 'hello') $first

Set-Content (Join-Path $root "projects\some-folder\$id.jsonl") '{}'
$later = & $script -SessionId $id -ConfigDir $root -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
Check 'a later start resumes the same id' ($later[0] -match "--resume $id" -and $later[0] -notmatch '--session-id') $later[0]
Check 'a resumed start does not repeat the first prompt' ($later[0] -notmatch 'hello') $later[0]
Check 'model and effort are kept on resume' ($later[0] -match '--model m1' -and $later[0] -match '--effort low') $later[0]

$other = & $script -SessionId '99999999-2222-3333-4444-555555555555' -ConfigDir $root -DryRun
Check 'another id is not mistaken for it' ($other[0] -match '--session-id 99999999') $other[0]
$threw = $false; try { & $script -SessionId 'not-a-guid' -ConfigDir $root -DryRun } catch { $threw = $true }
Check 'a session id that is not a guid is refused' $threw ''


# The launch itself: a stub stands in for claude and records its arguments and the environment it was started with.
$stub = Join-Path $root 'stub.ps1'
$rec = Join-Path $root 'record.txt'
Set-Content $stub @'
param()
$lines = @("ARGS: " + ($args -join ' '))
Get-ChildItem env: | Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -in 'NO_COLOR', 'FORCE_COLOR' } | ForEach-Object { $lines += "ENV: $($_.Name)=$($_.Value)" }
$lines += 'PATH0: ' + (($env:PATH -split ';')[0])
$lines += 'PATHCOUNT: ' + (@($env:PATH -split ';' | Where-Object { $_ -ieq $env:RESUMABLE_TEST_WSHDIR }).Count)
$found = Get-Command wsh -ErrorAction SilentlyContinue
$lines += 'WSH: ' + $(if ($found) { $found.Source } else { 'none' })
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
if ($PathMode -eq 'set') { $env:WAVETERM_WSHBINDIR = $WshDir } elseif ($PathMode -eq 'missing') { $env:WAVETERM_WSHBINDIR = $WshDir + '-nope' } elseif ($PathMode -eq 'already') { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = $WshDir + ';' + $env:PATH } else { Remove-Item Env:WAVETERM_WSHBINDIR -ErrorAction SilentlyContinue }
& $Script -SessionId $Id -ConfigDir $Root -Command $Stub -Model m1
'@
$shellBefore = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR', 'FORCE_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
$wshDir = Join-Path $root 'wshbin'
$null = New-Item -ItemType Directory -Path $wshDir
Set-Content (Join-Path $wshDir 'wsh.cmd') '@echo off'
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode set | Out-Null
$recorded = @(Get-Content -LiteralPath $rec)
$shellAfter = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR', 'FORCE_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
Check 'this shell is left as it was (the launch ran in a child process)' (($shellBefore -join '|') -eq ($shellAfter -join '|')) (($shellBefore -join '|') + ' -> ' + ($shellAfter -join '|'))
Check 'the launched program gets the searched config folder, not the inherited one' ($recorded -contains "ENV: CLAUDE_CONFIG_DIR=$root") ($recorded -join ' | ')
Check 'the inherited child-session markers are gone' (-not ($recorded -match 'ENV: (CLAUDE_CODE_CHILD_SESSION|CLAUDECODE)=')) ($recorded -join ' | ')
Check 'NO_COLOR is gone' (-not ($recorded -match 'ENV: NO_COLOR=')) ($recorded -join ' | ')
Check 'the launched program gets the resume arguments' (($recorded -join ' ') -match "ARGS: --resume $id") ($recorded -join ' | ')
Check 'the folder of wsh is first on the PATH of the launched program' ($recorded -contains "PATH0: $wshDir") ($recorded -join ' | ')
Check 'wsh can be run from there' (($recorded -join ' ') -match [regex]::Escape("WSH: $wshDir")) ($recorded -join ' | ')

foreach ($mode in 'missing', 'unset') {
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode $mode | Out-Null
    $r = @(Get-Content -LiteralPath $rec)
    Check "with WAVETERM_WSHBINDIR $mode the PATH is left alone" (-not ($r -contains "PATH0: $wshDir") -and ($r -contains 'PATHCOUNT: 0')) ($r -join ' | ')
}
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Id $id -Root $root -Stub $stub -Record $rec -WshDir $wshDir -PathMode already | Out-Null
$r = @(Get-Content -LiteralPath $rec)
Check 'a folder that is already on the PATH is not added again' ($r -contains 'PATHCOUNT: 1') ($r -join ' | ')
Remove-Item $stub, $rec, $child, (Join-Path $wshDir 'wsh.cmd'), $wshDir -ErrorAction SilentlyContinue
Remove-Item (Join-Path $root "projects\some-folder\$id.jsonl"), (Join-Path $root 'projects\some-folder'), (Join-Path $root 'projects'), $root
if ($fail -gt 0) { exit 1 }
