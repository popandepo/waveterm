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
Set-Content -LiteralPath $env:RESUMABLE_TEST_RECORD $lines
'@
$saved = @{}
foreach ($n in 'CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR', 'RESUMABLE_TEST_RECORD') { $saved[$n] = [Environment]::GetEnvironmentVariable($n) }
try {
    $env:CLAUDE_CONFIG_DIR = 'C:\somewhere\else'; $env:CLAUDE_CODE_CHILD_SESSION = '1'; $env:CLAUDECODE = '1'; $env:NO_COLOR = '1'
    $env:RESUMABLE_TEST_RECORD = $rec
    & $script -SessionId $id -ConfigDir $root -Command $stub -Model m1
    $recorded = @(Get-Content -LiteralPath $rec)
} finally {
    foreach ($n in $saved.Keys) { [Environment]::SetEnvironmentVariable($n, $saved[$n]) }
}
Check 'the launched program gets the searched config folder, not the inherited one' ($recorded -contains "ENV: CLAUDE_CONFIG_DIR=$root") ($recorded -join ' | ')
Check 'the inherited child-session markers are gone' (-not ($recorded -match 'ENV: (CLAUDE_CODE_CHILD_SESSION|CLAUDECODE)=')) ($recorded -join ' | ')
Check 'NO_COLOR is gone' (-not ($recorded -match 'ENV: NO_COLOR=')) ($recorded -join ' | ')
Check 'the launched program gets the resume arguments' (($recorded -join ' ') -match "ARGS: --resume $id") ($recorded -join ' | ')
Remove-Item $stub, $rec -ErrorAction SilentlyContinue
if ($fail -gt 0) { exit 1 }
Remove-Item $stub, $rec -ErrorAction SilentlyContinue
Remove-Item (Join-Path $root "projects\some-folder\$id.jsonl"), (Join-Path $root 'projects\some-folder'), (Join-Path $root 'projects'), $root
if ($fail -gt 0) { exit 1 }
