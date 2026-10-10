# Replays our commit stack on the current upstream head, on a NEW branch, then builds, checks and reports.
#
# Never rewrites Master, never force pushes. The result arrives as a branch (and optionally a pull request
# into Master). ASCII only on purpose: Windows PowerShell 5.1 reads BOM-less files as ANSI.
#
# Exit codes: 0 ok, 1 error, 2 replay conflict (files listed), 3 build or checks failed (branch kept), 4 nothing to do.
#
# Usage (from anywhere inside the clone):
#   tools\ours\Update-FromUpstream.ps1                  fetch, replay, build, check, push branch, report
#   tools\ours\Update-FromUpstream.ps1 -OpenPr          also open the pull request into Master
#   tools\ours\Update-FromUpstream.ps1 -SkipBuild -NoPush   replay and report only (cheap rehearsal)
#   tools\ours\Update-FromUpstream.ps1 -UpstreamRef upstream/main~20 -NoPush   rehearse against an older upstream point
[CmdletBinding()]
param(
    [string]$UpstreamRemote = 'upstream',
    [string]$UpstreamBranch = 'main',
    [string]$UpstreamRef = '',
    [string]$OursBranch = 'Master',
    [string]$ToolchainDir = '',
    [string]$ReportDir = '',
    [switch]$SkipBuild,
    [switch]$NoPush,
    [switch]$OpenPr,
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = (& git rev-parse --show-toplevel).Trim()
Set-Location $repo
if (-not $ToolchainDir) { $ToolchainDir = Join-Path (Split-Path $repo -Parent) 'toolchain' }
if (-not $ReportDir) { $ReportDir = Join-Path (Split-Path $repo -Parent) 'reports' }
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$log = New-Object System.Collections.Generic.List[string]

function Say([string]$m) { Write-Host $m; $script:log.Add($m) }
function GitRun { & git @args; if ($LASTEXITCODE -ne 0) { throw "git $($args -join ' ') failed ($LASTEXITCODE)" } }
function GitOut { $o = & git @args; if ($LASTEXITCODE -ne 0) { throw "git $($args -join ' ') failed ($LASTEXITCODE)" }; return $o }

function WriteReport([string]$status, [string]$branch) {
    New-Item -ItemType Directory -Force $ReportDir | Out-Null
    $path = Join-Path $ReportDir "update-$stamp.md"
    $head = @("# Update from upstream, $stamp", '', "Status: **$status**", "Branch: $branch", '')
    Set-Content -Path $path -Value ($head + $script:log) -Encoding ascii
    Write-Host "Report: $path"
}

if ((GitOut status --porcelain)) { throw 'Working tree is not clean. Commit or stash first.' }
$startBranch = (GitOut rev-parse --abbrev-ref HEAD).Trim()
$email = (GitOut config user.email).Trim()
if ($email -ne 'popandepo@gmail.com') { throw "git user.email is $email, expected popandepo@gmail.com" }

Say '## Fetch'
GitRun fetch $UpstreamRemote
$target = if ($UpstreamRef) { $UpstreamRef } else { "$UpstreamRemote/$UpstreamBranch" }
$targetSha = (GitOut rev-parse $target).Trim()
$oursSha = (GitOut rev-parse $OursBranch).Trim()
Say "target: $target = $targetSha"
Say "ours:   $OursBranch = $oursSha"

# Our stack = commits on Master that are not reachable from the new upstream head. Only valid while Master has
# not already absorbed the target; if it has, there is nothing to replay.
$base = (GitOut merge-base $OursBranch $targetSha).Trim()
$stack = @(GitOut rev-list --reverse --no-merges "$targetSha..$OursBranch")
$ahead = [int](GitOut rev-list --count "$OursBranch..$targetSha")
Say "merge-base: $base"
Say "commits in our stack: $($stack.Count)"
Say "upstream commits not yet in Master: $ahead"
if ($ahead -eq 0 -and -not $Force) {
    Say 'Master already contains the upstream head. Nothing to do (use -Force to rehearse anyway).'
    WriteReport 'nothing to do' '-'
    exit 4
}

# Keep our mirror of upstream main current (fast-forward only, never forced).
if (-not $NoPush -and -not $UpstreamRef) {
    Say '## Mirror upstream main to origin/main (fast-forward only)'
    & git push origin "${targetSha}:refs/heads/$UpstreamBranch" 2>&1 | ForEach-Object { Say "  $_" }
    if ($LASTEXITCODE -ne 0) { Say '  mirror push failed (not fast-forward?); continuing, check by hand' }
}

$short = $targetSha.Substring(0, 8)
$branch = "ours/sync-$stamp-$short"
Say "## Replay onto new branch $branch"
GitRun checkout -q -b $branch $targetSha

$replayed = @()
foreach ($c in $stack) {
    $subject = (GitOut log -1 --format=%s $c).Trim()
    & git cherry-pick $c 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        $files = @(GitOut diff --name-only --diff-filter=U)
        Say "CONFLICT replaying $c ($subject). Files:"
        $files | ForEach-Object { Say "  $_" }
        & git cherry-pick --abort 2>&1 | Out-Null
        GitRun checkout -q $startBranch
        Say "Branch $branch was left at upstream head with $($replayed.Count) of $($stack.Count) commits. Delete it by hand when done."
        WriteReport 'conflict' $branch
        exit 2
    }
    $new = (GitOut rev-parse HEAD).Trim()
    $replayed += [pscustomobject]@{ Old = $c; New = $new; Subject = $subject }
    Say "  replayed $($c.Substring(0,8)) -> $($new.Substring(0,8))  $subject"
}

Say '## Did a replayed commit change meaning? (patch-id of old vs new; a difference means hunks moved or merged)'
$meaningChanged = 0
foreach ($r in $replayed) {
    $a = ((& git show $r.Old) | & git patch-id --stable) -split ' ' | Select-Object -First 1
    $b = ((& git show $r.New) | & git patch-id --stable) -split ' ' | Select-Object -First 1
    if ($a -ne $b) {
        $meaningChanged++
        Say "  DIFFERS: $($r.Old.Substring(0,8)) -> $($r.New.Substring(0,8))  $($r.Subject)  (read the range-diff below)"
    }
}
if ($meaningChanged -eq 0) { Say '  all replayed commits have identical patch ids' }
Say '### range-diff summary (= same, ! changed; run git range-diff without -s on a ! line for the hunks)'
if ($stack.Count -gt 0) {
    & git range-diff -s "$base..$OursBranch" "$targetSha..$branch" 2>&1 | ForEach-Object { Say "    $_" }
}

Say '## Upstream changes in files we touch'
$ours = @(GitOut diff --name-only "$targetSha..$branch")
$theirs = @(GitOut diff --name-only "$base..$targetSha")
$both = @($ours | Where-Object { $theirs -contains $_ })
Say "files we change: $($ours.Count); files upstream changed since the old base: $($theirs.Count); both: $($both.Count)"
$both | ForEach-Object { Say "  $_" }

$failed = $false
if (-not $SkipBuild) {
    $env:Path = "$ToolchainDir\go\bin;$ToolchainDir\zig;$ToolchainDir\task;$env:Path"
    Say '## Build'
    if ((GitOut diff --name-only "$base..$targetSha" -- package-lock.json)) { Say '  package-lock.json changed upstream: npm ci'; & npm ci 2>&1 | Select-Object -Last 3 | ForEach-Object { Say "  $_" } }
    # RMRF override: the Taskfile's own clean-up variable is a recursive force delete, which is not allowed here.
    & task build:backend 'RMRF=cmd /c echo skip-rmrf' 2>&1 | Select-Object -Last 4 | ForEach-Object { Say "  $_" }
    if ($LASTEXITCODE -ne 0) { Say '  BUILD FAILED'; $failed = $true }
    # generated files may drift; report, then restore so the branch stays exactly our stack
    $drift = @(GitOut status --porcelain)
    if ($drift.Count -gt 0) { Say '  generated files changed by the build (restored):'; $drift | ForEach-Object { Say "    $_" }; GitRun checkout -- . }

    Say '## Checks'
    $vt = & npx vitest run 2>&1 | Select-Object -Last 6
    $vt | ForEach-Object { Say "  $_" }
    if ($LASTEXITCODE -ne 0) { Say '  VITEST FAILED'; $failed = $true }

    $tscOut = & npx tsc --noEmit 2>&1
    $tscErrors = @($tscOut | Where-Object { $_ -match 'error TS' } | ForEach-Object { ($_ -split '\(')[0] })
    $baselineFile = Join-Path $repo 'tools\ours\tsc-baseline.txt'
    $baseline = if (Test-Path $baselineFile) { [int](Get-Content $baselineFile -TotalCount 1) } else { -1 }
    Say "  tsc errors: $($tscErrors.Count) (baseline $baseline)"
    if ($baseline -ge 0 -and $tscErrors.Count -gt $baseline) { Say '  TSC: MORE ERRORS THAN BASELINE'; $failed = $true }

    & go test ./cmd/wsh/... 2>&1 | Select-Object -Last 4 | ForEach-Object { Say "  $_" }
    if ($LASTEXITCODE -ne 0) { Say '  GO TEST FAILED'; $failed = $true }

    $seams = Join-Path $repo 'tools\ours\Check-Seams.ps1'
    if (Test-Path $seams) {
        Say '## Seams'
        & $seams 2>&1 | ForEach-Object { Say "  $_" }
        if ($LASTEXITCODE -ne 0) { Say '  SEAMS CHECK FAILED'; $failed = $true }
    } else { Say '## Seams: tools\ours\Check-Seams.ps1 not present yet' }
} else { Say '## Build and checks skipped (-SkipBuild)' }

if (-not $NoPush) {
    Say '## Push branch (no force)'
    & git push -u origin $branch 2>&1 | ForEach-Object { Say "  $_" }
    if ($OpenPr) {
        $title = "Sync with upstream $short"
        $body = "Our stack replayed on $target ($targetSha).`n`nReport: see the update review brief (docs/ours/update-review.md). Needs the independent update review before merge.`n`nStatus: " + $(if ($failed) { 'BUILD OR CHECKS FAILED' } else { 'build and checks passed' }) + "`n`n$([char]::ConvertFromUtf32(0x1F916)) Generated with [Claude Code](https://claude.com/claude-code)"
        & gh pr create --repo popandepo/waveterm --base $OursBranch --head $branch --title $title --body $body 2>&1 | ForEach-Object { Say "  $_" }
    }
}

GitRun checkout -q $startBranch
WriteReport $(if ($failed) { 'build or checks failed' } else { 'ok' }) $branch
if ($failed) { exit 3 }
exit 0
