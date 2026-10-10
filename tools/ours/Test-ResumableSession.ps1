# Self test for Start-ResumableSession.ps1: a made-up config folder with and without a transcript. Starts nothing.
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'Start-ResumableSession.ps1'
$id = '11111111-2222-3333-4444-555555555555'
$root = Join-Path ([IO.Path]::GetTempPath()) ("resumable-test-" + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path (Join-Path $root 'projects\some-folder') -Force
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }

$first = & $script -SessionId $id -ConfigDir $root -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
Check 'first start creates the session under the id' ($first -match "--session-id $id" -and $first -notmatch '--resume') $first
Check 'first start carries model, effort, mode, name, prompt' ($first -match '--model m1' -and $first -match '--effort low' -and $first -match '--permission-mode auto' -and $first -match '--remote-control n1' -and $first -match 'hello') $first

Set-Content (Join-Path $root "projects\some-folder\$id.jsonl") '{}'
$later = & $script -SessionId $id -ConfigDir $root -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
Check 'a later start resumes the same id' ($later -match "--resume $id" -and $later -notmatch '--session-id') $later
Check 'a resumed start does not repeat the first prompt' ($later -notmatch 'hello') $later
Check 'model and effort are kept on resume' ($later -match '--model m1' -and $later -match '--effort low') $later

$other = & $script -SessionId '99999999-2222-3333-4444-555555555555' -ConfigDir $root -DryRun
Check 'another id is not mistaken for it' ($other -match '--session-id 99999999') $other
$threw = $false; try { & $script -SessionId 'not-a-guid' -ConfigDir $root -DryRun } catch { $threw = $true }
Check 'a session id that is not a guid is refused' $threw ''
Remove-Item (Join-Path $root "projects\some-folder\$id.jsonl"), (Join-Path $root 'projects\some-folder'), (Join-Path $root 'projects'), $root
if ($fail -gt 0) { exit 1 }
