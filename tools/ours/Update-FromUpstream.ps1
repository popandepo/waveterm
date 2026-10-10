# Replays our commit stack on the current upstream head, on a NEW branch, then builds, checks and reports.
#
# Never rewrites Master, never force pushes. The result arrives as a branch (and optionally a pull request
# into Master). ASCII only on purpose: Windows PowerShell 5.1 reads BOM-less files as ANSI.
#
# Exit codes: 0 ok (or ok but build and checks skipped, the report says so), 1 error, 2 replay conflict (files
# listed), 3 build, checks, push or PR creation failed (branch kept), 4 nothing to do.
# Whatever happens after the new branch is created, the script returns to the branch it started on and writes a report.
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
$script:exitCode = 0
$script:status = 'error'
$script:branch = '-'

function Say([string]$m) { Write-Host $m; $script:log.Add($m) }
function GitRun { & git @args; if ($LASTEXITCODE -ne 0) { throw "git $($args -join ' ') failed ($LASTEXITCODE)" } }
function GitOut { $o = & git @args; if ($LASTEXITCODE -ne 0) { throw "git $($args -join ' ') failed ($LASTEXITCODE)" }; return $o }
function Fail([string]$m) { Say "FAILED: $m"; $script:failed = $true }

function WriteReport {
    New-Item -ItemType Directory -Force $ReportDir | Out-Null
    $path = Join-Path $ReportDir "update-$stamp.md"
    $head = @("# Update from upstream, $stamp", '', "Status: **$($script:status)**", "Branch: $($script:branch)", '')
    Set-Content -Path $path -Value ($head + $script:log) -Encoding ascii
    Write-Host "Report: $path"
}

if ((GitOut status --porcelain)) { throw 'Working tree is not clean. Commit or stash first.' }
$startBranch = (GitOut rev-parse --abbrev-ref HEAD).Trim()
$email = (GitOut config user.email).Trim()
if ($email -ne 'popandepo@gmail.com') { throw "git user.email is $email, expected popandepo@gmail.com" }
$script:failed = $false
$script:created = $false

try {
  & {
    Say '## Fetch'
    GitRun fetch $UpstreamRemote
    $target = if ($UpstreamRef) { $UpstreamRef } else { "$UpstreamRemote/$UpstreamBranch" }
    $targetSha = (GitOut rev-parse $target).Trim()
    $oursSha = (GitOut rev-parse $OursBranch).Trim()
    Say "target: $target = $targetSha"
    Say "ours:   $OursBranch = $oursSha"

    # Our stack = commits on Master that are not reachable from the new upstream head.
    $base = (GitOut merge-base $OursBranch $targetSha).Trim()
    $stack = @(GitOut rev-list --reverse --no-merges "$targetSha..$OursBranch")
    $ahead = [int](GitOut rev-list --count "$OursBranch..$targetSha")
    Say "merge-base: $base"
    Say "commits in our stack: $($stack.Count)"
    Say "upstream commits not yet in Master: $ahead"
    if ($ahead -eq 0 -and -not $Force) {
        Say 'Master already contains the upstream head. Nothing to do (use -Force to rehearse anyway).'
        $script:status = 'nothing to do'
        $script:exitCode = 4
        return
    }

    # Keep our mirror of upstream main current (fast-forward only, never forced).
    if (-not $NoPush -and -not $UpstreamRef) {
        Say '## Mirror upstream main to origin/main (fast-forward only)'
        & git push origin "${targetSha}:refs/heads/$UpstreamBranch" 2>&1 | ForEach-Object { Say "  $_" }
        if ($LASTEXITCODE -ne 0) { Fail 'mirror push of upstream main to origin/main (not a fast-forward?)' }
    }

    $short = $targetSha.Substring(0, 8)
    $script:branch = "ours/sync-$stamp-$short"
    $branch = $script:branch
    Say "## Replay onto new branch $branch"
    GitRun checkout -q -b $branch $targetSha
    $script:created = $true

    $replayed = @()
    $skipped = @()
    foreach ($c in $stack) {
        $subject = (GitOut log -1 --format=%s $c).Trim()
        & git cherry-pick $c 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            $files = @(GitOut diff --name-only --diff-filter=U)
            $cpHead = (& git rev-parse -q --verify CHERRY_PICK_HEAD 2>$null)
            $dirty = @(GitOut status --porcelain)
            if ($files.Count -eq 0 -and $cpHead -and $dirty.Count -eq 0) {
                # A cherry-pick in progress with nothing conflicted, staged or modified is the 'now empty' case: upstream
                # already has this change. Anything else that stops a cherry-pick without conflicts (a hook, for example)
                # is an error and falls through to the throw below.
                & git cherry-pick --skip 2>&1 | Out-Null
                if ($LASTEXITCODE -ne 0) { & git cherry-pick --abort 2>&1 | Out-Null; throw "cherry-pick of $c stopped with no conflicting files and could not be skipped" }
                $skipped += $c
                Say "  SKIPPED $($c.Substring(0,8)) (already in upstream, empty after replay)  $subject"
                continue
            }
            if ($files.Count -eq 0) { & git cherry-pick --abort 2>&1 | Out-Null; throw "cherry-pick of $c failed without conflicts and is not empty (a hook or an environment problem); nothing was skipped" }
            Say "CONFLICT replaying $c ($subject). Files:"
            $files | ForEach-Object { Say "  $_" }
            & git cherry-pick --abort 2>&1 | Out-Null
            Say "Branch $branch was left at upstream head with $($replayed.Count) of $($stack.Count) commits. Delete it by hand when done."
            $script:status = 'conflict'
            $script:exitCode = 2
            return
        }
        $new = (GitOut rev-parse HEAD).Trim()
        $replayed += [pscustomobject]@{ Old = $c; New = $new; Subject = $subject }
        Say "  replayed $($c.Substring(0,8)) -> $($new.Substring(0,8))  $subject"
    }
    if ($skipped.Count -gt 0) { Say "  $($skipped.Count) commit(s) skipped because upstream already has them; review that they really are the same change" }

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
    if ($replayed.Count -gt 0) {
        & git range-diff -s "$base..$OursBranch" "$targetSha..$branch" 2>&1 | ForEach-Object { Say "    $_" }
    }

    Say '## Upstream changes in files we touch'
    $ours = @(GitOut diff --name-only "$targetSha..$branch")
    $theirs = @(GitOut diff --name-only "$base..$targetSha")
    $both = @($ours | Where-Object { $theirs -contains $_ })
    Say "files we change: $($ours.Count); files upstream changed since the old base: $($theirs.Count); both: $($both.Count)"
    $both | ForEach-Object { Say "  $_" }

    if (-not $SkipBuild) {
        $env:Path = "$ToolchainDir\go\bin;$ToolchainDir\zig;$ToolchainDir\task;$env:Path"
        Say '## Build'
        if ((GitOut diff --name-only "$base..$targetSha" -- package-lock.json)) {
            Say '  package-lock.json changed upstream: npm ci'
            & npm ci 2>&1 | Select-Object -Last 3 | ForEach-Object { Say "  $_" }
            if ($LASTEXITCODE -ne 0) { Fail 'npm ci' }
        }
        # RMRF override: the Taskfile's own clean-up variable is a recursive force delete, which is not allowed here.
        & task build:backend 'RMRF=cmd /c echo skip-rmrf' 2>&1 | Select-Object -Last 4 | ForEach-Object { Say "  $_" }
        if ($LASTEXITCODE -ne 0) { Fail 'task build:backend' }
        # generated files may drift; report, then restore so the branch stays exactly our stack
        $drift = @(GitOut status --porcelain)
        if ($drift.Count -gt 0) { Say '  generated files changed by the build (restored):'; $drift | ForEach-Object { Say "    $_" }; GitRun checkout -- . }

        Say '## Checks'
        & npx vitest run 2>&1 | Select-Object -Last 6 | ForEach-Object { Say "  $_" }
        if ($LASTEXITCODE -ne 0) {
            Say '  vitest failed, retrying once (timeouts happen on a cold or loaded machine)'
            & npx vitest run 2>&1 | Select-Object -Last 6 | ForEach-Object { Say "  $_" }
            if ($LASTEXITCODE -ne 0) { Fail 'vitest' } else { Say '  vitest passed on retry (flaky first run)' }
        }

        $tscOut = & npx tsc --noEmit 2>&1
        $tscExit = $LASTEXITCODE
        $tscErrors = @($tscOut | Where-Object { $_ -match 'error TS' })
        $baselineFile = Join-Path $repo 'tools\ours\tsc-baseline.txt'
        $known = @(if (Test-Path $baselineFile) { Get-Content $baselineFile | Where-Object { $_ -and -not $_.StartsWith('#') } })
        $current = @($tscErrors | ForEach-Object { [regex]::Replace($_, '^(.*?)\(\d+,\d+\): error (TS\d+): (.*)$', '$1|$2|$3').Trim() })
        # multiset difference: each known diagnostic can excuse one occurrence
        $pool = New-Object System.Collections.Generic.List[string]; $pool.AddRange([string[]]$known)
        $newErrors = @(foreach ($e in $current) { if (-not $pool.Remove($e)) { $e } })
        Say "  tsc: exit $tscExit, $($current.Count) diagnostics, $($known.Count) known, $($newErrors.Count) new, $($pool.Count) known ones gone"
        if ($tscExit -ne 0 -and $current.Count -eq 0) { Fail 'tsc did not run (non-zero exit with no compiler errors)'; $tscOut | Select-Object -Last 5 | ForEach-Object { Say "    $_" } }
        elseif ($newErrors.Count -gt 0) { Fail 'tsc: diagnostics that are not in tools/ours/tsc-baseline.txt'; $newErrors | Select-Object -First 10 | ForEach-Object { Say "    NEW: $_" } }
        elseif ($pool.Count -gt 0) { Say '  tsc: some known diagnostics are gone; lower tools/ours/tsc-baseline.txt in the next PR' }

        & go test ./cmd/wsh/... 2>&1 | Select-Object -Last 4 | ForEach-Object { Say "  $_" }
        if ($LASTEXITCODE -ne 0) { Fail 'go test ./cmd/wsh/...' }

        $seams = Join-Path $repo 'tools\ours\Check-Seams.ps1'
        if (Test-Path $seams) {
            Say '## Seams'
            & $seams -Ref $targetSha -BaseRef $base 2>&1 | ForEach-Object { Say "  $_" }
            if ($LASTEXITCODE -ne 0) { Fail 'Check-Seams.ps1' }
        } else { Say '## Seams: tools\ours\Check-Seams.ps1 not present yet' }
    } else {
        Say '## Build and checks SKIPPED (-SkipBuild): nothing below this line was verified'
    }

    $verdict = if ($script:failed) { 'build or checks failed' } elseif ($SkipBuild) { 'replayed only, build and checks skipped' } else { 'ok' }
    if (-not $NoPush) {
        Say '## Push branch (no force)'
        & git push -u origin $branch 2>&1 | ForEach-Object { Say "  $_" }
        if ($LASTEXITCODE -ne 0) { Fail 'git push of the sync branch' }
        elseif ($OpenPr) {
            $title = "Sync with upstream $short"
            $robot = [char]::ConvertFromUtf32(0x1F916)
            $statusLine = switch ($verdict) { 'ok' { 'build and checks passed' } 'replayed only, build and checks skipped' { 'REPLAYED ONLY: build and checks were skipped' } default { 'BUILD OR CHECKS FAILED' } }
            $body = "Our stack replayed on $target ($targetSha).`n`nSee the update review brief (docs/ours/update-review.md). Needs the independent update review before merge.`n`nStatus: $statusLine`n`n$robot Generated with [Claude Code](https://claude.com/claude-code)"
            & gh pr create --repo popandepo/waveterm --base $OursBranch --head $branch --title $title --body $body 2>&1 | ForEach-Object { Say "  $_" }
            if ($LASTEXITCODE -ne 0) { Fail 'gh pr create' }
        }
    }
    $script:status = if ($script:failed) { 'build, checks, push or PR creation failed' } else { $verdict }
    if ($script:failed) { $script:exitCode = 3 }
  }
}
catch {
    Say "ERROR: $($_.Exception.Message)"
    Say ($_.ScriptStackTrace)
    $script:status = 'error'
    $script:exitCode = 1
}
finally {
    if ($script:created) {
        & git cherry-pick --abort 2>&1 | Out-Null
        & git checkout -q -- . 2>&1 | Out-Null
        & git checkout -q $startBranch 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            $now = (& git rev-parse --abbrev-ref HEAD)
            Say "FAILED: could not return to $startBranch, you are on $now"
            $script:status = 'could not return to the start branch'
            $script:exitCode = 1
        }
    }
    try { WriteReport } catch { Write-Host "could not write the report: $_" }
}
exit $script:exitCode