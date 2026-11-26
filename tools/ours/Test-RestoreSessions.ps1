# Self test for Restore-Sessions.ps1 against a made-up record, a made-up sessions folder and a stand-in for wsh.
# Starts nothing. Every case runs the script in a child PowerShell process. The test folder is new and uniquely named and its
# files are removed one by one.
$ErrorActionPreference = 'Continue'
$script = Join-Path $PSScriptRoot 'Restore-Sessions.ps1'
$dir = Join-Path ([IO.Path]::GetTempPath()) ("restore-test-" + [guid]::NewGuid().ToString('N'))
$sess = Join-Path $dir 'sessions'
$null = New-Item -ItemType Directory -Path $sess
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }

$idRunning = '11111111-1111-1111-1111-111111111111'   # stack s1, running now (the anchor of its stack)
$idStack1  = '22222222-2222-2222-2222-222222222222'   # stack s1, not running
$idStack2  = '33333333-3333-3333-3333-333333333333'   # stack s1, not running
$idBypass  = '44444444-4444-4444-4444-444444444444'   # recorded in bypass mode, not running
$idAuto    = '55555555-5555-5555-5555-555555555555'   # recorded in auto mode, not running (started "meanwhile" in one case)
$idDead    = '66666666-6666-6666-6666-666666666666'   # a file names it but the process is gone
$idReused  = '77777777-7777-7777-7777-777777777777'   # a file names it, the pid is alive but has another start time
$idBadMode = '88888888-8888-8888-8888-888888888888'   # a mode that is not on the allowed list

function Session($id, $name, $tile, $group, $stack, $mode) {
    $s = [ordered]@{ blockid = "blk-$name"; workspaceid = 'ws1'; workspacename = 'main'; tabid = 'tab1'; tabname = 'T1'; tileposition = $tile; kind = $(if ($stack) { 'stack' } else { 'tile' }); groupposition = $group; active = $false; cwd = 'C:\work'; session = [ordered]@{ id = $id; name = $name; model = 'claude-sonnet-5-5'; effort = 'low' } }
    if ($stack) { $s['stackid'] = $stack }
    if ($mode) { $s.session['permissionmode'] = $mode }
    return $s
}
$record = [ordered]@{ version = 1; sequence = 1; writtenat = 'x'; wave = @{ version = '0' }; workspaces = @(); sessions = @(
    (Session $idRunning 'run' 0 0 's1' 'auto'), (Session $idStack1 'st1' 0 1 's1' 'auto'), (Session $idStack2 'st2' 0 2 's1' $null),
    (Session $idBypass 'byp' 1 0 $null 'bypassPermissions'), (Session $idAuto 'aut' 2 0 $null 'auto'),
    (Session $idDead 'dead' 3 0 $null 'auto'), (Session $idReused 'reuse' 4 0 $null 'auto'), (Session $idBadMode 'badmode' 5 0 $null 'dangerously-skip-permissions'),
    (Session 'not-a-guid' 'bad' 6 0 $null $null)) }
$recordPath = Join-Path $dir 'wave-state.json'
$record | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $recordPath

$me = Get-Process -Id $PID
function WriteLive($file, $id, $procPid, $procStart) {
    @{ pid = $procPid; sessionId = $id; procStart = "$procStart" } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $sess $file)
}
# this test process stands for a live session process: it is alive while the child runs
WriteLive 'running.json' $idRunning $PID $me.StartTime.ToFileTimeUtc()
WriteLive 'dead.json' $idDead 99999999 123
WriteLive 'reused.json' $idReused $PID 1

$stub = Join-Path $dir 'wsh-stub.ps1'
$log = Join-Path $dir 'wsh-calls.txt'
@'
$line = ($args -join ' ')
Add-Content -LiteralPath $env:RESTORE_TEST_LOG -Value $line
if ($args[0] -eq 'session' -and $args[1] -eq 'start') {
    $n = @(Get-Content -LiteralPath $env:RESTORE_TEST_LOG | Where-Object { $_ -like 'session start*' }).Count
    if ($env:RESTORE_TEST_MEANWHILE -and $n -eq 1) {
        $mine = Get-Process -Id $PID
        @{ pid = $PID; sessionId = $env:RESTORE_TEST_MEANWHILE; procStart = "$($mine.StartTime.ToFileTimeUtc())" } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $env:RESTORE_TEST_SESSIONS 'meanwhile.json')
    }
    '{"tabid":"tab-new-' + $n + '","blockid":"blk-new-' + $n + '"}'
}
'@ | Set-Content -LiteralPath $stub

function Run([string[]]$extra, [hashtable]$envVars = @{}) {
    if (Test-Path -LiteralPath $log) { Remove-Item -LiteralPath $log }
    $envVars.GetEnumerator() | ForEach-Object { [Environment]::SetEnvironmentVariable($_.Key, $_.Value) }
    [Environment]::SetEnvironmentVariable('RESTORE_TEST_LOG', $log)
    [Environment]::SetEnvironmentVariable('RESTORE_TEST_SESSIONS', $sess)
    $out = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script -RecordPath $recordPath -SessionsDir $sess -Wsh $stub @extra 2>&1 | Out-String
    $code = $LASTEXITCODE
    $envVars.GetEnumerator() | ForEach-Object { [Environment]::SetEnvironmentVariable($_.Key, $null) }
    $calls = if (Test-Path -LiteralPath $log) { @(Get-Content -LiteralPath $log) } else { @() }
    return @{ Out = $out; Code = $code; Calls = $calls }
}

# 1. the dry run
$r = Run @()
Check 'the dry run exits 0 and says nothing is started' ($r.Code -eq 0 -and $r.Out -match 'PLAN ONLY') "$($r.Code) $($r.Out)"
Check 'the dry run does not call wsh at all' ($r.Calls.Count -eq 0) ($r.Calls -join ' | ')
Check 'a running session is skipped, with its pid' ($r.Out -match "SKIP\s+run $idRunning .*already running \(pid $PID\)") $r.Out
foreach ($id in $idStack1, $idStack2, $idBypass, $idAuto, $idDead, $idReused, $idBadMode) { Check "the dry run plans $id" ($r.Out -match "WOULD\s+\S+ $id") $r.Out }
Check 'a pid file whose process is gone does not count as running' ($r.Out -notmatch "SKIP\s+dead $idDead") $r.Out
Check 'a pid file with another start time (a reused pid) does not count as running' ($r.Out -notmatch "SKIP\s+reuse $idReused") $r.Out
Check 'a session id that is not a guid is skipped' ($r.Out -match 'SKIP\s+bad not-a-guid .*not a guid') $r.Out
Check 'a bypass mode is not passed on and is said so' ($r.Out -match "NOTE\s+byp $idBypass .*bypassPermissions.*not passed on") $r.Out
Check 'a mode that is not on the allowed list is not passed on and is said so' ($r.Out -match "NOTE\s+badmode $idBadMode .*not passed on") $r.Out
Check 'no plan line carries a skip-permissions or bypass flag' ($r.Out -notmatch '-SkipPermissions|--dangerously-skip-permissions|-PermissionMode bypass|-PermissionMode dangerously') $r.Out
Check 'the allowed mode auto is passed on' ($r.Out -match '-PermissionMode auto') $r.Out
Check 'a stack member with a running anchor is planned to join that group' ($r.Out -match 'group add -b <new block> --to blk-run') $r.Out

# 2. a single session
$r = Run @('-SessionId', $idAuto)
Check '-SessionId plans only that one' (([regex]::Matches($r.Out, 'WOULD')).Count -eq 1 -and $r.Out -match "WOULD\s+aut $idAuto") $r.Out

# 3. execute
$r = Run @('-Execute')
$starts = @($r.Calls | Where-Object { $_ -like 'session start*' })
Check 'execute starts exactly the sessions that are not running (7 here)' ($starts.Count -eq 7) "$($starts.Count): $($r.Calls -join ' | ')"
Check 'execute never starts the running session' (-not (($starts -join "`n") -match $idRunning)) ($starts -join ' | ')
Check 'every start is a --restart start with the session id in its meta' (@($starts | Where-Object { $_ -match '--restart' -and $_ -match 'session:id=' }).Count -eq $starts.Count) ($starts -join ' | ')
Check 'no call carries a skip-permissions or bypass flag' (-not (($r.Calls -join "`n") -match 'SkipPermissions|dangerously|bypass')) ($r.Calls -join ' | ')
Check 'every start carries -ConfigDir with a full path in its stored command (Wave itself has no CLAUDE_CONFIG_DIR)' (@($starts | Where-Object { $_ -match '-ConfigDir [A-Za-z]:[\\/]' }).Count -eq $starts.Count) ($starts -join ' | ')
$adds = @($r.Calls | Where-Object { $_ -like 'group add*' })
Check 'both free members of the stack join the running anchor block' ($adds.Count -eq 2 -and @($adds | Where-Object { $_ -match '--to blk-run$' }).Count -eq 2) ($adds -join ' | ')

# 4. started by someone else between the first look and the start
$r = Run @('-Execute') @{ RESTORE_TEST_MEANWHILE = $idAuto }
$starts = @($r.Calls | Where-Object { $_ -like 'session start*' })
Check 'a session that appears running after the first look is skipped at the moment of the start' ($r.Out -match "SKIP\s+aut $idAuto .*started by someone else meanwhile" -and -not (($starts -join "`n") -match $idAuto)) "$($r.Out) | $($starts -join ' | ')"
Remove-Item -LiteralPath (Join-Path $sess 'meanwhile.json') -ErrorAction SilentlyContinue

# 5. when it cannot tell, it does nothing
$gone = Join-Path $dir 'no-such-folder'
if (Test-Path -LiteralPath $log) { Remove-Item -LiteralPath $log }
$out = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script -RecordPath $recordPath -SessionsDir $gone -Wsh $stub -Execute 2>&1 | Out-String
Check 'a sessions folder that is missing stops the script with exit 1 and no wsh call' ($LASTEXITCODE -eq 1 -and $out -match 'cannot read the sessions folder' -and -not (Test-Path -LiteralPath $log)) "$LASTEXITCODE $out"
Set-Content -LiteralPath (Join-Path $sess 'broken.json') '{ not json'
$out = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script -RecordPath $recordPath -SessionsDir $sess -Wsh $stub -Execute 2>&1 | Out-String
Check 'a session file that cannot be read stops the script with exit 1 and no wsh call' ($LASTEXITCODE -eq 1 -and $out -match 'cannot read' -and -not (Test-Path -LiteralPath $log)) "$LASTEXITCODE $out"
Remove-Item -LiteralPath (Join-Path $sess 'broken.json')

# clean up, file by file
foreach ($f in @(Get-ChildItem -LiteralPath $sess -File)) { Remove-Item -LiteralPath $f.FullName }
Remove-Item -LiteralPath $sess
foreach ($f in @(Get-ChildItem -LiteralPath $dir -File)) { Remove-Item -LiteralPath $f.FullName }
Remove-Item -LiteralPath $dir
if ($fail -gt 0) { exit 1 }
