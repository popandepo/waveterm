# Puts Claude sessions back after Wave was restarted, from the live state record that Wave writes
# (<data dir>\state\wave-state.json, see docs/ours/what-wave-can-do.md section 24).
#
# By default it only PRINTS what it would do. Add -Execute to act. It never starts a session that is already running: a session
# counts as running when a file in -SessionsDir names its id and the process in that file is alive with the same start time. If
# that folder cannot be read the script stops (it cannot prove that a session is not running). It never uses a bypass or
# skip-permissions mode: only the permission modes in $AllowedModes are passed on, anything else is dropped and the session
# starts in its default mode.
#
# Each session that is not running is started with `wsh session start --restart` in its own background tab, running
# Start-ResumableSession.ps1 (first start creates the session under its id, every later start resumes it). A session that was
# a member of a stack is added to the group of another member of that stack afterwards (`wsh group add`), when one is known.
# Must run inside a Wave block when -Execute is used (wsh needs the block's token); the dry run needs nothing.
#
#   .\Restore-Sessions.ps1                       print the plan
#   .\Restore-Sessions.ps1 -Execute              do it
#   .\Restore-Sessions.ps1 -SessionId <guid>     only this session
param(
    [string]$RecordPath = (Join-Path $env:LOCALAPPDATA 'WaveFork\profile\data\state\wave-state.json'),
    [string]$SessionsDir = (Join-Path $env:USERPROFILE '.claude-personal\sessions'),
    [string]$Starter = (Join-Path $PSScriptRoot 'Start-ResumableSession.ps1'),
    [string]$Wsh = 'wsh',
    [string]$Shell = 'pwsh',
    # Default: the usage-file mod of the account -ConfigDir belongs to (Get-UsageModDir in host\Assert-ClaudeConfigDir.ps1).
    [string]$PluginDir,
    [string]$ConfigDir = (Join-Path $env:USERPROFILE '.claude-personal'),
    [string[]]$SessionId = @(),
    [switch]$Execute
)
$ErrorActionPreference = 'Stop'
$AllowedModes = @('auto', 'default', 'acceptEdits', 'plan')
$GuidPattern = '^[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}$'

function Fail([string]$text, [int]$code = 1) { [Console]::Error.WriteLine($text); exit $code }

function Get-RunningSessions([string]$dir) {
    if (-not (Test-Path -LiteralPath $dir -PathType Container)) { Fail "cannot read the sessions folder '$dir': stopping, because I cannot tell which sessions are running" }
    $running = @{}
    foreach ($f in @(Get-ChildItem -LiteralPath $dir -Filter '*.json' -File)) {
        try { $j = Get-Content -LiteralPath $f.FullName -Raw | ConvertFrom-Json } catch { Fail "cannot read '$($f.FullName)': stopping" }
        if (-not $j.sessionId -or -not $j.pid) { continue }
        $p = Get-Process -Id ([int]$j.pid) -ErrorAction SilentlyContinue
        if (-not $p) { continue }
        # a reused pid has another start time; without a start time in the file the pid alone counts (the safe side)
        if ($j.procStart) {
            $start = $null
            try { $start = $p.StartTime.ToFileTimeUtc() } catch { }
            if ($start -ne $null -and "$start" -ne "$($j.procStart)") { continue }
        }
        $running[("$($j.sessionId)").ToLower()] = [int]$j.pid
    }
    return $running
}

if (-not (Test-Path -LiteralPath $RecordPath)) { Fail "no state record at $RecordPath" }
$rec = Get-Content -LiteralPath $RecordPath -Raw | ConvertFrom-Json
if ($rec.version -ne 1) { Fail "unknown record version $($rec.version)" }
if (-not (Test-Path -LiteralPath $Starter)) { Fail "starter not found: $Starter" }
# Wave's own environment never has CLAUDE_CONFIG_DIR (the launcher removes it), so every restored session carries its config folder in its stored command.
if ($ConfigDir -notmatch '^[A-Za-z]:[\\/]') { Fail "-ConfigDir must be a full path: '$ConfigDir'" }
if (-not $PSBoundParameters.ContainsKey('PluginDir')) {
    . (Join-Path $PSScriptRoot 'host\Assert-ClaudeConfigDir.ps1')
    $resolved = Get-FinalPath $ConfigDir
    $PluginDir = if ($resolved) { Get-UsageModDir $resolved } else { '' }
}

$running = Get-RunningSessions $SessionsDir
$sessions = @($rec.sessions | Where-Object { $_ -and $_.session -and $_.session.id })
if ($SessionId.Count -gt 0) {
    $want = @($SessionId | ForEach-Object { $_.ToLower() })
    $sessions = @($sessions | Where-Object { $want -contains ("$($_.session.id)").ToLower() })
}
$sessions = @($sessions | Sort-Object { $_.workspacename }, { [int]$_.tileposition }, { [int]$_.groupposition })

# the block that stands for a stack: any member whose session is running now (its block still exists), else the first one started here
$anchor = @{}
foreach ($s in $sessions) {
    if ($s.stackid -and $running.ContainsKey(("$($s.session.id)").ToLower()) -and -not $anchor.ContainsKey($s.stackid)) { $anchor[$s.stackid] = $s.blockid }
}

$mode = if ($Execute) { 'EXECUTE' } else { 'PLAN ONLY (nothing is started; add -Execute to act)' }
"restore: $mode, $($sessions.Count) session(s) in the record, $($running.Count) running now"
$started = 0; $skipped = 0
foreach ($s in $sessions) {
    $id = "$($s.session.id)"
    $label = "$($s.session.name) $id (workspace '$($s.workspacename)', tab '$($s.tabname)', tile $([int]$s.tileposition + 1)$(if ($s.stackid) { ", group position $([int]$s.groupposition + 1)" }))"
    if ($id -notmatch $GuidPattern) { "SKIP    $label : the id is not a guid"; $skipped++; continue }
    if ($running.ContainsKey($id.ToLower())) { "SKIP    $label : already running (pid $($running[$id.ToLower()]))"; $skipped++; continue }
    $pm = "$($s.session.permissionmode)"
    $pmText = ''
    if ($pm) {
        if ($AllowedModes -contains $pm) { $pmText = $pm } else { "NOTE    $label : permission mode '$pm' is not passed on, the session starts in its default mode" }
    }
    $starterArgs = @('-NoLogo', '-File', $Starter, '-SessionId', $id, '-ConfigDir', $ConfigDir)
    if ($PluginDir) { $starterArgs += '-PluginDir', $PluginDir }
    if ($s.session.name) { $starterArgs += '-Name', "$($s.session.name)" }
    if ($s.session.model) { $starterArgs += '-Model', "$($s.session.model)" }
    if ($s.session.effort) { $starterArgs += '-Effort', "$($s.session.effort)" }
    if ($pmText) { $starterArgs += '-PermissionMode', $pmText }
    $wshArgs = @('session', 'start', '--restart', '--name', "$($s.session.name)", '--meta', "session:id=$id")
    foreach ($k in 'name', 'model', 'effort') { if ($s.session.$k) { $wshArgs += '--meta', "session:$k=$($s.session.$k)" } }
    if ($s.cwd) { $wshArgs += '--cwd', "$($s.cwd)" }
    $wshArgs += '--', $Shell
    $wshArgs += $starterArgs
    $join = if ($s.stackid -and $anchor.ContainsKey($s.stackid)) { $anchor[$s.stackid] } else { $null }
    if (-not $Execute) {
        "WOULD   $label"
        "          $Wsh $($wshArgs -join ' ')"
        if ($join) { "          then: $Wsh group add -b <new block> --to $join" } elseif ($s.stackid) { "          then: it becomes the anchor of its stack for the members after it" }
        continue
    }
    # check again right before the start: another process may have started it since the first look
    $now = Get-RunningSessions $SessionsDir
    if ($now.ContainsKey($id.ToLower())) { "SKIP    $label : started by someone else meanwhile (pid $($now[$id.ToLower()]))"; $skipped++; continue }
    $global:LASTEXITCODE = 0
    $out = & $Wsh @wshArgs
    if ($LASTEXITCODE -ne 0) { Fail "wsh session start failed for $label (exit $LASTEXITCODE)" }
    $info = ($out | Out-String | ConvertFrom-Json)
    "STARTED $label : tab $($info.tabid) block $($info.blockid)"
    $started++
    if ($s.stackid) {
        if ($join) {
            $global:LASTEXITCODE = 0
            & $Wsh group add -b $info.blockid --to $join | Out-Null
            if ($LASTEXITCODE -ne 0) { "NOTE    $label : could not join the group of $join (exit $LASTEXITCODE); it stays in its own tab" }
        } else { $anchor[$s.stackid] = $info.blockid }
    }
}
"restore: started $started, skipped $skipped"
