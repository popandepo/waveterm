#Requires -Version 7
# Self test for Start-ResumableSession.ps1. Starts nothing but a stub that stands in for claude. Run with pwsh (PowerShell 7).
# The whole test runs under a made-up profile folder (USERPROFILE is set to it for this process and every child): no path in this test is a folder of the
# real profile, and nothing here opens, lists or creates anything in the real .claude or .claude-work. The made-up profile has a name with spaces, so that
# it also has an 8.3 short name.
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'Start-ResumableSession.ps1'
$id = '11111111-2222-3333-4444-555555555555'
$root = Join-Path ([IO.Path]::GetTempPath()) ("resumable-test-" + [guid]::NewGuid().ToString('N'))
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }
$made = New-Object System.Collections.Generic.List[string]
function Make-Dir([string]$p) { $null = New-Item -ItemType Directory -Path $p -Force; $made.Add($p) }
function Make-File([string]$p, [string]$text) { Set-Content -LiteralPath $p -Value $text; $made.Add($p) }

$realProfile = $env:USERPROFILE
$profileDir = Join-Path $root 'Long Profile Name'
$personal = Join-Path $profileDir '.claude-personal'
$cfg = Join-Path $personal 'cfg'
$oldDir = Join-Path $profileDir '.claude'
$workDir = Join-Path $profileDir '.claude-work'
$throw = Join-Path $root 'throwaway'
foreach ($d in $root, $profileDir, $personal, $cfg, $oldDir, $workDir, $throw, (Join-Path $cfg 'projects'), (Join-Path $cfg 'projects\some-folder')) { Make-Dir $d }
$env:USERPROFILE = $profileDir
$realLocal = $env:LOCALAPPDATA
$env:LOCALAPPDATA = Join-Path $root 'localappdata'
Make-Dir $env:LOCALAPPDATA
$logDir = Join-Path $root 'logs'
Make-Dir $logDir
$swapLog = Join-Path $logDir 'starter.log'
$made.Add($swapLog)
# the manager is whoever the manager file names; Wave counts as started long ago unless a gate check says otherwise
$managerTestId = 'aaaaaaaa-2222-3333-4444-555555555555'
$managerFile = Join-Path $root 'manager.json'
Make-File $managerFile ('{"sessionId":"' + $managerTestId + '"}')
$env:WAVEFORK_MANAGER_FILE = $managerFile
$env:WAVE_STARTED_AT = '2020-01-01T00:00:00'
$env:RESUMABLE_TEST_LOG = $swapLog
$env:RESUMABLE_TEST_LOGROOT = $root
# The result must be the same inside a Wave block as outside one: the WAVE_* values of a block would leak into every child below.
foreach ($v in @(Get-ChildItem env: | Where-Object { $_.Name -like 'WAVE_*' -or $_.Name -eq 'TERM_PROGRAM' })) { Remove-Item -LiteralPath ('Env:' + $v.Name) }
try {
    # ---- the choice between create and resume ----
    $firstOut = & $script -SessionId $id -ConfigDir $cfg -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
    $first = $firstOut[0]
    Check 'the folder that is searched is the folder Claude is told to use' ($firstOut[1] -eq "config=$cfg") ($firstOut -join ' | ')
    Check 'first start creates the session under the id' ($first -match "--session-id $id" -and $first -notmatch '--resume') $first
    Check 'first start carries model, effort, mode, name, prompt' ($first -match '--model m1' -and $first -match '--effort low' -and $first -match '--permission-mode auto' -and $first -match '--remote-control n1' -and $first -match 'hello') $first
    $asFolder = Join-Path $cfg "projects\some-folder\$id.jsonl"
    Make-Dir $asFolder
    $f = & $script -SessionId $id -ConfigDir $cfg -DryRun
    Check 'a FOLDER named <id>.jsonl is not a transcript' ($f[0] -match "--session-id $id") $f[0]
    [IO.Directory]::Delete($asFolder); $made.Remove($asFolder) | Out-Null
    Make-File $asFolder '{}'
    $later = & $script -SessionId $id -ConfigDir $cfg -Model m1 -Effort low -PermissionMode auto -Name n1 -Prompt 'hello' -DryRun
    Check 'a later start resumes the same id' ($later[0] -match "--resume $id" -and $later[0] -notmatch '--session-id') $later[0]
    Check 'a resumed start does not repeat the first prompt' ($later[0] -notmatch 'hello') $later[0]
    Check 'model and effort are kept on resume' ($later[0] -match '--model m1' -and $later[0] -match '--effort low') $later[0]
    # The peer name other sessions message it by is set by --name; --remote-control only names the Remote Control session
    # (18 Oct 2026: research tiles opened as projects-research-night showed in ListAgents as ashfallprotocol-3c).
    Check 'the session gets its name with --name, on the first start and on resume' (($first -match '--name n1(\s|$)') -and ($later[0] -match '--name n1(\s|$)')) "$first || $($later[0])"
    $other = & $script -SessionId '99999999-2222-3333-4444-555555555555' -ConfigDir $cfg -DryRun
    Check 'another id is not mistaken for it' ($other[0] -match '--session-id 99999999') $other[0]
    # the manager session alone compacts at 300k (R-20261016-007)
    $mgr = & $script -SessionId $managerTestId -ConfigDir $cfg -DryRun
    Check 'the manager session gets the 300k compact window' ($mgr[3] -eq 'compact=300000') ($mgr -join ' | ')
    Check 'any other session gets no compact window' ($other[3] -eq 'compact=' -and $later[3] -eq 'compact=') (($other -join ' | ') + ' // ' + ($later -join ' | '))

    # ---- the start gate ----
    $now = (Get-Date).ToString('s'); $env:WAVE_STARTED_AT = $now
    $gateDir = Join-Path $root 'gate'; Make-Dir $gateDir
    $g = { param($sid, $n) (& $script -SessionId $sid -ConfigDir $cfg -Name $n -GateDir $gateDir -DryRun)[-1] }
    Check 'a restored session is held just after Wave started' ((& $g $id 'n1') -eq 'gate=hold') (& $g $id 'n1')
    Check 'the manager is never held' ((& $g $managerTestId 'whatever') -eq 'gate=go') ''
    Check 'manager-duty is never held' ((& $g $id 'manager-duty') -eq 'gate=go') ''
    Check 'an unreadable manager file holds nobody' (((& $script -SessionId $id -ConfigDir $cfg -Name n1 -GateDir $gateDir -ManagerFile (Join-Path $root 'no-manager.json') -DryRun)[-1]) -eq 'gate=go') ''
    foreach ($odd in '.', '..', '_all') { Check "a session named $odd is not held" ((& $g $id $odd) -eq 'gate=go') '' }
    # a real held start, up to the point of starting claude: the command does not exist, so the starter ends with exit 3 right after the hold
    $gateClaude = Join-Path $root 'no-such-claude.exe'
    $t0 = Get-Date
    $h = & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -Name n1 -GateDir $gateDir -GateGiveUpSeconds 2 -Command $gateClaude -CommandWaitSeconds 1 -LogPath $swapLog -LogRoot $root 2>$null
    $took = ((Get-Date) - $t0).TotalSeconds
    Check 'a held start nobody releases waits, then goes on to start claude by itself' ($LASTEXITCODE -eq 3 -and $took -ge 2 -and ($h -join ' ') -match 'Held by the start gate' -and ([string](Get-Content -LiteralPath $swapLog -Raw)) -match ' start=gave-up') "$LASTEXITCODE $took $h"
    $relFile = Join-Path $gateDir 'n4'
    $job = Start-ThreadJob -ArgumentList $relFile -ScriptBlock { param($f) Start-Sleep -Seconds 3; Set-Content -LiteralPath $f -Value 'x' }
    $t0 = Get-Date
    $h = & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -Name n4 -GateDir $gateDir -GateGiveUpSeconds 60 -Command $gateClaude -CommandWaitSeconds 1 -LogPath $swapLog -LogRoot $root 2>$null
    $took = ((Get-Date) - $t0).TotalSeconds; $null = $job | Wait-Job | Remove-Job
    Check 'a release during the hold starts claude at once' ($LASTEXITCODE -eq 3 -and $took -ge 3 -and $took -lt 40 -and ([string](Get-Content -LiteralPath $swapLog -Raw)) -match ' start=released') "$LASTEXITCODE $took"
    Make-Dir (Join-Path $gateDir 'n5'); (Get-Item (Join-Path $gateDir 'n5')).LastWriteTime = (Get-Date).AddMinutes(1)
    Check 'a folder with the name of the session is not a release' ((& $g $id 'n5') -eq 'gate=hold') ''
    Make-File (Join-Path $gateDir 'other') 'x'; (Get-Item (Join-Path $gateDir 'other')).LastWriteTime = (Get-Date).AddMinutes(1)
    Check 'a release of another session does not release this one' ((& $g $id 'n1') -eq 'gate=hold') ''
    Make-File (Join-Path $gateDir 'n1') 'x'; (Get-Item (Join-Path $gateDir 'n1')).LastWriteTime = (Get-Date).AddDays(-1)
    Check 'a release from before this Wave start does not count' ((& $g $id 'n1') -eq 'gate=hold') ''
    (Get-Item (Join-Path $gateDir 'n1')).LastWriteTime = (Get-Date).AddMinutes(1)
    Check 'a release after this Wave start lets it go' ((& $g $id 'n1') -eq 'gate=go') ''
    Make-File (Join-Path $gateDir '_all') 'x'; (Get-Item (Join-Path $gateDir '_all')).LastWriteTime = (Get-Date).AddMinutes(1)
    Check 'the release of all lets any session go' ((& $g $id 'n2') -eq 'gate=go') ''
    $env:WAVE_STARTED_AT = (Get-Date).AddMinutes(-10).ToString('s')
    Check 'a block opened later is not held' ((& $g $id 'n3') -eq 'gate=go') ''
    $env:WAVE_STARTED_AT = 'not a date'
    Check 'an unreadable start time lets it start' ((& $g $id 'n3') -eq 'gate=go') ''
    $env:WAVE_STARTED_AT = ''
    Check 'no start time at all (the app did not set WAVE_STARTED_AT) lets it start' ((& $g $id 'n3') -eq 'gate=go') ''
    Check 'the gate asks the app, not a process: the starter names no wavesrv' (-not ((Get-Content -LiteralPath $script -Raw) -match 'wavesrv')) ''
    $env:WAVE_STARTED_AT = '2020-01-01T00:00:00'

    # ---- how it is run: a child pwsh, so that this shell keeps its environment ----
    $stub = Join-Path $root 'stub.ps1'
    $rec = Join-Path $root 'record.txt'
    Make-File $stub @'
param()
$lines = @("ARGS: " + ($args -join ' '))
Get-ChildItem env: | Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -like 'STATUS_*' -or $_.Name -in 'NO_COLOR', 'FORCE_COLOR', 'TERM_PROGRAM', 'EVIL_VAR' } | ForEach-Object { $lines += "ENV: $($_.Name)=$($_.Value)" }
$lines += 'ARGCOUNT: ' + $args.Count
Set-Content -LiteralPath $env:RESUMABLE_TEST_RECORD $lines
'@
    $child = Join-Path $root 'child.ps1'
    Make-File $child @'
param($Script, $Stub, $Record, $Cfg, $Id, [string]$ExtraPrompt, [string]$ExtraName, [string]$ExtraPromptFile)
$env:CLAUDE_CONFIG_DIR = 'X:\somewhere\else'; $env:CLAUDE_CODE_CHILD_SESSION = '1'; $env:CLAUDECODE = '1'; $env:NO_COLOR = '1'; $env:CLAUDE_CODE_AUTO_COMPACT_WINDOW = '999'
$env:RESUMABLE_TEST_RECORD = $Record
$env:STATUS_SESSION = 'stale-1234'; $env:STATUS_TAB = 'stale'
$extra = @{}
if ($env:RESUMABLE_TEST_LOG) { $extra.LogPath = $env:RESUMABLE_TEST_LOG }
if ($env:RESUMABLE_TEST_LOGROOT) { $extra.LogRoot = $env:RESUMABLE_TEST_LOGROOT }
if ($env:RESUMABLE_TEST_ACCOUNTLOG) { $extra.AccountDir = $env:RESUMABLE_TEST_ACCOUNTLOG }
if ($env:RESUMABLE_TEST_ACCOUNTAT) { $extra.AccountAt = $env:RESUMABLE_TEST_ACCOUNTAT }
if ($env:RESUMABLE_TEST_NOLOCALAPPDATA) { Remove-Item Env:LOCALAPPDATA -ErrorAction SilentlyContinue }
if ($ExtraPrompt) { $extra.Prompt = $ExtraPrompt }
if ($ExtraPromptFile) { $extra.PromptFile = $ExtraPromptFile }
if ($env:RESUMABLE_TEST_PROMPTROOT) { $extra.PromptRoot = $env:RESUMABLE_TEST_PROMPTROOT }
if ($ExtraName) { $extra.Name = $ExtraName }
if ($env:RESUMABLE_TEST_CLICKROWS) { $extra.ClickRows = $true }
if ($env:RESUMABLE_TEST_TOOLROWS) { $extra.ToolRowsDir = $env:RESUMABLE_TEST_TOOLROWS }
if ($env:RESUMABLE_TEST_TOOLROWS_ALLOWED) { $extra.ToolRowsAllowed = $env:RESUMABLE_TEST_TOOLROWS_ALLOWED }
& $Script -SessionId $Id -ConfigDir $Cfg -Command $Stub -Model m1 @extra
exit $LASTEXITCODE
'@
    function Run-Child([string]$cfgDir = $cfg, [string]$prompt = '', [string]$name = '', [string]$promptFile = '', [string]$runId = $id) {
        Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
        $a = @('-NoProfile', '-File', $child, '-Script', $script, '-Stub', $stub, '-Record', $rec, '-Cfg', $cfgDir, '-Id', $runId)
        if ($prompt) { $a += '-ExtraPrompt', $prompt }
        if ($name) { $a += '-ExtraName', $name }
        if ($promptFile) { $a += '-ExtraPromptFile', $promptFile }
        $errFile = Join-Path $root 'child-error.txt'
        & pwsh @a 2>$errFile | Out-Null
        $code = $LASTEXITCODE
        $errText = if (Test-Path -LiteralPath $errFile) { (Get-Content -LiteralPath $errFile -Raw) } else { '' }
        Remove-Item -LiteralPath $errFile -ErrorAction SilentlyContinue
        $lines = @(); if (Test-Path -LiteralPath $rec) { $lines = @(Get-Content -LiteralPath $rec) }
        return [pscustomobject]@{ Code = $code; Lines = $lines; Text = (($lines -join ' | ') + $(if ($errText) { ' || stderr: ' + $errText.Substring(0, [Math]::Min(900, $errText.Length)) } else { '' })) }
    }
    $shellBefore = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
    $r = Run-Child 
    $shellAfter = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
    Check 'this shell is left as it was (the launch ran in a child process)' (($shellBefore -join '|') -eq ($shellAfter -join '|')) ''
    Check 'the launched program gets the searched config folder, not the inherited one' ($r.Lines -contains "ENV: CLAUDE_CONFIG_DIR=$cfg") $r.Text
    Check 'the inherited child-session markers are gone' (-not ($r.Text -match 'ENV: (CLAUDE_CODE_CHILD_SESSION|CLAUDECODE)=')) $r.Text
    Check 'NO_COLOR is gone' (-not ($r.Text -match 'ENV: NO_COLOR=')) $r.Text
    Check 'an inherited compact window is gone for any other session' (-not ($r.Text -match 'ENV: CLAUDE_CODE_AUTO_COMPACT_WINDOW=')) $r.Text
    $rm = Run-Child -runId $managerTestId
    Check 'the manager session is started with the 300k compact window (R-20261016-007)' ($rm.Lines -contains 'ENV: CLAUDE_CODE_AUTO_COMPACT_WINDOW=300000') $rm.Text
    # -ClickRows (31 Oct 2026): fullscreen only when asked for; the tool-rows folder is judged like any plugin folder
    Check 'without -ClickRows the launched program gets neither CLAUDE_CODE_NO_FLICKER nor the full-repaint variable' (-not ($r.Text -match 'ENV: CLAUDE_CODE_NO_FLICKER=') -and -not ($r.Text -match 'ENV: CLAUDE_CODE_ALT_SCREEN_FULL_REPAINT=')) $r.Text
    $env:RESUMABLE_TEST_CLICKROWS = '1'; $rc = Run-Child ; Remove-Item Env:RESUMABLE_TEST_CLICKROWS
    Check 'with -ClickRows the launched program gets CLAUDE_CODE_NO_FLICKER=1 and the full-repaint fix' ($rc.Lines -contains 'ENV: CLAUDE_CODE_NO_FLICKER=1' -and $rc.Lines -contains 'ENV: CLAUDE_CODE_ALT_SCREEN_FULL_REPAINT=1') $rc.Text
    $trDir = Join-Path $root 'tool-rows-folder'; Make-Dir $trDir
    # only the one trusted folder is accepted, compared after links are resolved (the seam -ToolRowsAllowed stands for the real MalfestioMaster folder)
    $env:RESUMABLE_TEST_TOOLROWS_ALLOWED = $trDir
    $env:RESUMABLE_TEST_TOOLROWS = $trDir; $rt = Run-Child ; Remove-Item Env:RESUMABLE_TEST_TOOLROWS
    Check 'the trusted -ToolRowsDir is started as a --plugin-dir' ($rt.Code -eq 0 -and $rt.Text -match '--plugin-dir [^|]*tool-rows-folder') $rt.Text
    $sibling = Join-Path $root 'tool-rows-sibling'; Make-Dir $sibling
    $env:RESUMABLE_TEST_TOOLROWS = $sibling; $rs = Run-Child ; Remove-Item Env:RESUMABLE_TEST_TOOLROWS
    Check 'a sibling folder is refused as -ToolRowsDir (exit 2, nothing started)' ($rs.Code -eq 2 -and -not ($rs.Text -match 'ARGS:')) $rs.Text
    $junction = Join-Path $root 'tool-rows-junction'
    New-Item -ItemType Junction -Path $junction -Target $sibling | Out-Null
    $env:RESUMABLE_TEST_TOOLROWS = $junction; $rj = Run-Child ; Remove-Item Env:RESUMABLE_TEST_TOOLROWS
    Check 'a junction to another folder is refused as -ToolRowsDir (exit 2, nothing started)' ($rj.Code -eq 2 -and -not ($rj.Text -match 'ARGS:')) $rj.Text
    [IO.Directory]::Delete($junction)
    $junction2 = Join-Path $root 'tool-rows-junction-to-trusted'
    New-Item -ItemType Junction -Path $junction2 -Target $trDir | Out-Null
    $env:RESUMABLE_TEST_TOOLROWS = $junction2; $rj2 = Run-Child ; Remove-Item Env:RESUMABLE_TEST_TOOLROWS
    Check 'a junction to the trusted folder resolves to it and is accepted' ($rj2.Code -eq 0 -and $rj2.Text -match '--plugin-dir') $rj2.Text
    [IO.Directory]::Delete($junction2)
    $env:RESUMABLE_TEST_TOOLROWS = 'relative-tool-rows'; $rb = Run-Child ; Remove-Item Env:RESUMABLE_TEST_TOOLROWS
    Check 'a -ToolRowsDir that is not a fully qualified existing folder refuses the start (exit 2)' ($rb.Code -eq 2) $rb.Text
    Remove-Item Env:RESUMABLE_TEST_TOOLROWS_ALLOWED -ErrorAction SilentlyContinue
    Check 'the launched program gets the resume arguments' ($r.Text -match "ARGS: --resume $id") $r.Text

    # ---- the status page's keys ----
    $r = Run-Child 
    Check 'without -Name no STATUS_SESSION or STATUS_TAB reaches the program, and the inherited ones are removed' (-not ($r.Text -match 'ENV: STATUS_')) $r.Text
    $r1 = Run-Child $cfg '' 'n1'
    $r2 = Run-Child $cfg '' 'n1'
    $s1 = ($r1.Lines | Where-Object { $_ -like 'ENV: STATUS_SESSION=*' }) -replace '^ENV: STATUS_SESSION=', ''
    $s2 = ($r2.Lines | Where-Object { $_ -like 'ENV: STATUS_SESSION=*' }) -replace '^ENV: STATUS_SESSION=', ''
    Check 'with -Name STATUS_SESSION is <name>-<8 hex> and STATUS_TAB is the name' (($s1 -match '^n1-[0-9a-f]{8}$') -and ($r1.Lines -contains 'ENV: STATUS_TAB=n1')) $r1.Text
    Check 'every start gets a new STATUS_SESSION' (($s1 -match '^n1-') -and ($s2 -match '^n1-') -and ($s1 -ne $s2)) "$s1 $s2"
    $dryName = & $script -SessionId $id -ConfigDir $cfg -Name n1 -DryRun
    Check 'the dry run prints the status key and tab' ($dryName[2] -match '^status=n1-[0-9a-f]{8} tab=n1$') ($dryName -join ' | ')

    # ---- plugin folders ----
    $plug = Join-Path $root 'plugin one'; Make-Dir $plug
    $d = & $script -SessionId $id -ConfigDir $cfg -PluginDir $plug -Model m1 -DryRun
    Check 'a plugin folder is passed as --plugin-dir, ahead of the other arguments, as its resolved path' ($d[0].StartsWith("--plugin-dir $plug ")) $d[0]
    $d2 = & $script -SessionId $id -ConfigDir $cfg -PluginDir $plug, $throw -DryRun
    Check 'two plugin folders are both passed' (([regex]::Matches($d2[0], '--plugin-dir')).Count -eq 2) $d2[0]
    $plugFile = Join-Path $root 'plugin-file.txt'; Make-File $plugFile 'x'
    $juncP = Join-Path $root 'plugin-junction'; $null = New-Item -ItemType Junction -Path $juncP -Target $oldDir; $made.Add($juncP)
    foreach ($pc in @(@('relative', 'plugin one'), @('a folder that does not exist', ($root + '\nope')), @('a file', $plugFile), @('.claude', $oldDir), @('below .claude-work', $workDir), @('a junction that leads to .claude', $juncP))) {
        & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -PluginDir $pc[1] -DryRun 2>$null | Out-Null
        Check ("plugin folder '" + $pc[0] + "' is refused with exit code 2") ($LASTEXITCODE -eq 2) "exit=$LASTEXITCODE"
    }
    foreach ($pp2 in @($oldDir, (Join-Path $workDir 'x'), ($personal + '\..\.claude'))) {
        $perr = Join-Path $root 'plugin-error.txt'
        & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -PluginDir $pp2 -DryRun 2>$perr | Out-Null
        $ptext = Get-Content -LiteralPath $perr -Raw
        Remove-Item -LiteralPath $perr -ErrorAction SilentlyContinue
        Check ("plugin folder '" + $pp2.Replace($root, '<root>') + "' is refused by its plain spelling, before anything is opened") (($LASTEXITCODE -eq 2) -and ($ptext -match 'plain spelling \(nothing was opened\)')) "exit=$LASTEXITCODE $ptext"
    }

    # ---- follow-up: tilde paths, a missing LOCALAPPDATA, listed names cleared, the plugin folder gaps ----
    $tildeCfg = Join-Path $profileDir '.claude~1'
    foreach ($tc in @(@('a config folder with a tilde', @('-ConfigDir', ($personal + '\~x'))), @('a config folder with a tilde below a throwaway switch', @('-ConfigDir', ($throw + '\a~1'), '-ThrowawayConfig')))) {
        Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
        $env:RESUMABLE_TEST_RECORD = $rec
        $terr = Join-Path $root 'tilde-error.txt'
        & pwsh -NoProfile -File $script -SessionId $id -Command $stub @($tc[1]) 2>$terr | Out-Null
        $ttext = Get-Content -LiteralPath $terr -Raw
        Remove-Item -LiteralPath $terr -ErrorAction SilentlyContinue
        Check ($tc[0] + ' is refused by its text alone, with exit 2 and nothing is started') (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec) -and ($ttext -match 'plain spelling \(nothing was opened\)')) "exit=$LASTEXITCODE $ttext"
    }
    foreach ($tp in @(@('a plugin folder with a tilde', ($plug + '~1')), @('a drive-relative plugin folder', 'C:plugin'))) {
        & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -PluginDir $tp[1] -DryRun 2>$null | Out-Null
        Check ($tp[0] + ' is refused with exit code 2') ($LASTEXITCODE -eq 2) "exit=$LASTEXITCODE"
    }
    $juncWork = Join-Path $root 'plugin-to-work'
    $null = New-Item -ItemType Junction -Path $juncWork -Target $workDir; $made.Add($juncWork)
    & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -PluginDir $juncWork -DryRun 2>$null | Out-Null
    Check 'a plugin folder that is a junction into .claude-work is refused after the link is resolved' ($LASTEXITCODE -eq 2) "exit=$LASTEXITCODE"
    $dotted = ($plug + '\..\plugin one')
    $dd = & $script -SessionId $id -ConfigDir $cfg -PluginDir $dotted -DryRun
    Check 'a plugin folder is handed on as its resolved full path, not as it was written' ($dd[0].StartsWith("--plugin-dir $plug ")) $dd[0]

    # ---- the prompt ----
    foreach ($lead in '-x', ' -x', "`t-x", "  `t -x") {
        $rd = Run-Child $cfg ($lead + ' starts with a dash')
        Check ("a prompt that starts with '" + $lead.Replace("`t", '<tab>') + "' is refused with exit 2 and nothing is started") (($rd.Code -eq 2) -and $rd.Lines.Count -eq 0) "exit=$($rd.Code) $($rd.Text)"
    }
    $r = Run-Child $cfg '-x starts with a dash'
    Check 'a prompt that starts with a dash is refused with exit 2 and nothing is started' (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    $quoted = 'say "hi" and a\"b c'
    $first = Join-Path $cfg 'projects\some-folder'
    Remove-Item -LiteralPath $asFolder; $made.Remove($asFolder) | Out-Null
    $r = Run-Child $cfg $quoted
    Check 'a prompt with quotes and a backslash-quote arrives as ONE argument' (($r.Lines -contains 'ARGCOUNT: 7') -or ($r.Text -match 'ARGCOUNT: \d+' -and ($r.Lines | Where-Object { $_ -like 'ARGS:*' }) -like "*$quoted") ) $r.Text
    Make-File $asFolder '{}'
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Stub $stub -Record $rec -Cfg $cfg -Id $id 2>$null | Out-Null
    Check 'a real launch under Windows PowerShell is refused with exit 2 and nothing is started' (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE"

    # ---- the prompt from a file (-PromptFile): any text arrives as ONE argument, byte for byte; refusals are exit 2 with nothing started ----
    $promptRoot = Join-Path $env:LOCALAPPDATA 'WaveFork\prompts'
    Make-Dir (Join-Path $env:LOCALAPPDATA 'WaveFork'); Make-Dir $promptRoot
    $env:RESUMABLE_TEST_PROMPTROOT = $promptRoot
    function Write-PromptBytes([string]$name, [byte[]]$bytes) {
        $p = Join-Path $promptRoot $name
        [IO.File]::WriteAllBytes($p, $bytes); $made.Add($p)
        return $p
    }
    $aring = [string][char]0x00E5
    $texts = @(
        "it's a 'quoted' prompt",
        'say "hi" and a\"b c',
        'semicolon; and & ampersand | pipe',
        'dollar $HOME and `backtick` and 100%',
        'ends with a full stop.',
        ('unicode ' + $aring + ' text'),
        'trailing backslash \',
        "tab`there"
    )
    $n = 0
    $baseCount = $null
    foreach ($text in $texts) {
        $n++
        $pf = Write-PromptBytes ("p$n.txt") ([Text.UTF8Encoding]::new($false).GetBytes($text + "`n"))
        # a fresh id each time: a prompt is only handed on at a session's first start, and $id has a transcript by now
        $r = Run-Child $cfg '' '' $pf -runId ([guid]::NewGuid().ToString())
        $argLine = ($r.Lines | Where-Object { $_ -like 'ARGS:*' }) -join "`n"
        $count = ($r.Lines | Where-Object { $_ -like 'ARGCOUNT:*' })
        if ($null -eq $baseCount) { $baseCount = $count }
        Check ("a prompt file with '" + $text.Substring(0, [Math]::Min(18, $text.Length)).Replace("`n", '<nl>') + "' arrives as one argument, byte for byte") (($r.Code -eq 0) -and $argLine.EndsWith($text) -and ($count -eq $baseCount)) "exit=$($r.Code) count=$count base=$baseCount $($r.Text)"
        Check 'the prompt file is still there afterwards (the starter deletes nothing)' (Test-Path -LiteralPath $pf) ''
    }
    $crlf = Write-PromptBytes 'crlf.txt' ([Text.UTF8Encoding]::new($true).GetPreamble() + [Text.UTF8Encoding]::new($false).GetBytes("with bom and crlf`r`n"))
    $r = Run-Child $cfg '' '' $crlf -runId ([guid]::NewGuid().ToString())
    Check 'a byte order mark and one trailing line break are dropped, nothing else' ((($r.Lines | Where-Object { $_ -like 'ARGS:*' }) -join '').EndsWith(' with bom and crlf') -and -not (($r.Lines | Where-Object { $_ -like 'ARGS:*' }) -join '').Contains([string][char]0xFEFF)) $r.Text
    # refusals
    $okFile = Join-Path $promptRoot 'p1.txt'
    $bad = [ordered]@{
        'a dash start'            = (Write-PromptBytes 'dash.txt' ([Text.Encoding]::UTF8.GetBytes('-x starts with a dash')))
        'invalid UTF-8'           = (Write-PromptBytes 'bad-utf8.txt' ([byte[]](0x68, 0x69, 0xFF, 0xFE, 0x41)))
        'above 64 KB'             = (Write-PromptBytes 'big.txt' ([byte[]]((1..70000) | ForEach-Object { 97 })))
        'a missing file'          = (Join-Path $promptRoot 'does-not-exist.txt')
        'a relative path'         = 'p1.txt'
        'a path with a tilde'     = ($promptRoot + '\p1~1.txt')
        'a file outside the root' = (Write-PromptBytes '..\..\..\outside-prompt.txt' ([Text.Encoding]::UTF8.GetBytes('outside')))
    }
    foreach ($k in $bad.Keys) {
        $r = Run-Child $cfg '' '' $bad[$k]
        Check ("a prompt file that is " + $k + " is refused with exit 2 and nothing is started") (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    }
    $claudePrompt = Join-Path $oldDir 'prompt.txt'
    Make-File $claudePrompt 'below .claude'
    $r = Run-Child $cfg '' '' $claudePrompt
    Check 'a prompt file below .claude is refused with exit 2 and nothing is started' (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    $promptJunc = Join-Path $promptRoot 'to-old'
    $null = New-Item -ItemType Junction -Path $promptJunc -Target $oldDir; $made.Add($promptJunc)
    $r = Run-Child $cfg '' '' (Join-Path $promptJunc 'prompt.txt')
    Check 'a prompt file reached through a junction into .claude is refused after the link is resolved' (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    $r = Run-Child $cfg 'also a prompt' '' $okFile
    Check 'giving -Prompt and -PromptFile together is refused with exit 2 and nothing is started' (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    Remove-Item Env:RESUMABLE_TEST_PROMPTROOT -ErrorAction SilentlyContinue

    # ---- the session id ----
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & pwsh -NoProfile -File $script -SessionId ($id + "`n") -ConfigDir $cfg -Command $stub 2>$null | Out-Null
    Check 'a session id with a trailing newline is refused' (($LASTEXITCODE -ne 0) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE"
    $threw = $false; try { & $script -SessionId ($id + "`n") -ConfigDir $cfg -DryRun | Out-Null } catch { $threw = $true }
    Check 'a session id with a trailing newline is refused (in process, so that the newline arrives)' $threw ''
    $threw = $false; try { & $script -SessionId 'not-a-guid' -ConfigDir $cfg -DryRun } catch { $threw = $true }
    Check 'a session id that is not a guid is refused' $threw ''

    # ---- the config folder rule: refused with exit 2 and nothing started ----
    $junc = Join-Path $personal 'to-old'
    $null = New-Item -ItemType Junction -Path $junc -Target $oldDir; $made.Add($junc)
    $juncThrow = Join-Path $throw 'to-work'
    $null = New-Item -ItemType Junction -Path $juncThrow -Target $workDir; $made.Add($juncThrow)
    $short = (New-Object -ComObject Scripting.FileSystemObject).GetFolder($profileDir).ShortPath
    $evil = Join-Path $profileDir '.claude-personal-evil'; Make-Dir $evil
    $fileCfg = Join-Path $personal 'a-file'; Make-File $fileCfg 'x'
    $cases = @(
        @('unset', $null, $false), @('empty', '', $false), @('blank', '   ', $false),
        @('.claude', $oldDir, $false), @('.claude with the switch', $oldDir, $true),
        @('below .claude', (Join-Path $oldDir 'projects'), $true),
        @('.CLAUDE in capitals', $oldDir.ToUpper(), $true),
        @('.claude-work', $workDir, $true), @('below .claude-work', (Join-Path $workDir 'x'), $false),
        @('dot dot out of .claude-personal into .claude', "$personal\..\.claude", $false),
        @('forward slashes into .claude', ($oldDir -replace '\\', '/'), $true),
        @('8.3 name of the profile into .claude', "$short\.claude", $true),
        @('a junction under .claude-personal that leads to .claude', $junc, $false),
        @('a junction under a throwaway folder that leads to .claude-work', $juncThrow, $true),
        @('UNC form of .claude (localhost admin share)', ('\\localhost\' + ($oldDir.Substring(0, 1)) + '$' + $oldDir.Substring(2)), $true),
        @('extended length form of .claude', ('\\?\' + $oldDir), $true),
        @('relative folder', 'cfg', $true), @('drive relative folder', 'C:cfg', $true),
        @('a folder next to .claude-personal with the same start', $evil, $false),
        @('a throwaway folder without the switch', $throw, $false)
    )
    $plainCases = @('.claude', '.claude with the switch', 'below .claude', '.CLAUDE in capitals', '.claude-work', 'below .claude-work', 'dot dot out of .claude-personal into .claude', 'forward slashes into .claude', '8.3 name of the profile into .claude')
    foreach ($c in $cases) {
        Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
        $a = @('-NoProfile', '-File', $script, '-SessionId', $id, '-Command', $stub)
        if ($null -ne $c[1]) { $a += '-ConfigDir', $c[1] }
        if ($c[2]) { $a += '-ThrowawayConfig' }
        $env:RESUMABLE_TEST_RECORD = $rec
        Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
        $errPath = Join-Path $root 'refuse-error.txt'
        & pwsh @a 2>$errPath | Out-Null
        $refusalText = if (Test-Path -LiteralPath $errPath) { Get-Content -LiteralPath $errPath -Raw } else { '' }
        Remove-Item -LiteralPath $errPath -ErrorAction SilentlyContinue
        $wantPlain = $plainCases -contains $c[0]
        $gotPlain = $refusalText -match 'by its plain spelling \(nothing was opened\)'
        if ($wantPlain) { Check ("config folder '" + $c[0] + "' is refused by its plain spelling, before anything is opened") $gotPlain $refusalText }
        else { Check ("config folder '" + $c[0] + "' needs the resolver (it is not refused by its plain spelling)") (-not $gotPlain) $refusalText }
        Check ("config folder '" + $c[0] + "' is refused with exit code 2 and nothing is started") (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE record=$(Test-Path -LiteralPath $rec)"
    }
    $env:CLAUDE_CONFIG_DIR = $oldDir
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & pwsh -NoProfile -File $script -SessionId $id -Command $stub 2>$null | Out-Null
    Check 'a CLAUDE_CONFIG_DIR of .claude in the environment is refused with exit code 2' (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE"
    Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue

    # ---- the deny list is case-insensitive even where nothing on disk gives the real spelling (a profile that has no .claude folder) ----
    $bareProfile = Join-Path $root 'bare profile'
    Make-Dir $bareProfile; Make-Dir (Join-Path $bareProfile '.claude-personal')
    $env:USERPROFILE = $bareProfile
    foreach ($spelling in '.CLAUDE', '.Claude-Work', '.claude') {
        Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
        & pwsh -NoProfile -File $script -SessionId $id -ConfigDir ($bareProfile + '\' + $spelling + '\x') -ThrowawayConfig -Command $stub 2>$null | Out-Null
        Check ("a folder that does not exist yet below '$spelling' is refused with exit code 2") (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE"
    }
    $env:USERPROFILE = $profileDir

    # ---- what is accepted ----
    $dry = & $script -SessionId $id -ConfigDir $cfg -DryRun
    Check 'a folder under .claude-personal is accepted without the throwaway switch' ($dry[1] -eq "config=$cfg") ($dry -join ' | ')
    # account 2 (R-20261014-008): .claude-personal2 is the second personal folder; a look-alike is not
    $acct2 = Join-Path $profileDir '.claude-personal2'; Make-Dir $acct2
    # account 2 keeps no CLAUDE.md or settings.json of its own: both are copied from account 1 at every start (a stale copy once missed a hook)
    Make-File (Join-Path $personal 'CLAUDE.md') 'rules one'
    Make-File (Join-Path $personal 'settings.json') '{"hooks":"one"}'
    $dry = @(& pwsh -NoProfile -File $script -SessionId $id -ConfigDir $acct2 -DryRun 2>&1)
    Check 'account 2: .claude-personal2 is accepted without the throwaway switch' ($dry.Count -ge 2 -and $dry[1] -eq "config=$acct2") ($dry -join ' | ')
    $look = Join-Path $profileDir '.claude-personal2x'; Make-Dir $look
    & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $look -DryRun 2>$null | Out-Null
    Check '.claude-personal2x is refused with exit code 2' ($LASTEXITCODE -eq 2) "exit=$LASTEXITCODE"
    # a missing file reads as '<missing>' so a check that fails says so (a read of a missing file would stop the whole test run)
    function Text-Of([string]$p) { if (Test-Path -LiteralPath $p -PathType Leaf) { return (Get-Content -LiteralPath $p -Raw).Trim() } else { return '<missing>' } }
    $dry = @(& pwsh -NoProfile -File $script -SessionId $id -ConfigDir $acct2 -DryRun 2>&1)
    Check 'account 2 copy: a dry run names both files and writes neither' ($dry.Count -ge 5 -and $dry[4] -eq 'copy=CLAUDE.md,settings.json' -and -not (Test-Path -LiteralPath (Join-Path $acct2 'CLAUDE.md')) -and -not (Test-Path -LiteralPath (Join-Path $acct2 'settings.json'))) ($dry -join ' | ')
    $r = Run-Child $acct2
    Check 'account 2 copy: a start copies CLAUDE.md and settings.json from account 1 and the session starts' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and (Text-Of (Join-Path $acct2 'CLAUDE.md')) -eq 'rules one' -and (Text-Of (Join-Path $acct2 'settings.json')) -eq '{"hooks":"one"}') $r.Text
    $made.Add((Join-Path $acct2 'CLAUDE.md')); $made.Add((Join-Path $acct2 'settings.json'))
    Set-Content -LiteralPath (Join-Path $personal 'CLAUDE.md') -Value 'rules two'
    Set-Content -LiteralPath (Join-Path $personal 'settings.json') -Value '{"hooks":"two"}'
    $r = Run-Child $acct2
    Check 'account 2 copy: a later start replaces an older copy' ($r.Code -eq 0 -and (Text-Of (Join-Path $acct2 'CLAUDE.md')) -eq 'rules two' -and (Text-Of (Join-Path $acct2 'settings.json')) -eq '{"hooks":"two"}') $r.Text
    # the start itself leaves the proof in the starter log: the first 12 hex of each file's SHA-256 after the copy, equal to the source's
    function Hash12([string]$p) { (Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash.Substring(0, 12).ToLowerInvariant() }
    $want = 'account2copy=ok set=CLAUDE.md:' + (Hash12 (Join-Path $personal 'CLAUDE.md')) + ',settings.json:' + (Hash12 (Join-Path $personal 'settings.json'))
    $lastLog = if (Test-Path -LiteralPath $swapLog) { [string](Get-Content -LiteralPath $swapLog -Tail 1) } else { '<no log>' }
    Check 'account 2 copy: the start logs account2copy=ok with the hashes of what it copied, equal to account 1''s files' ($lastLog.EndsWith($want)) "last log line: $lastLog // wanted the end: $want"
    $before = if (Test-Path -LiteralPath $swapLog) { @(Get-Content -LiteralPath $swapLog).Count } else { 0 }
    $null = & $script -SessionId $id -ConfigDir $acct2 -DryRun 2>&1
    $after = if (Test-Path -LiteralPath $swapLog) { @(Get-Content -LiteralPath $swapLog).Count } else { 0 }
    Check 'account 2 copy: a dry run logs nothing' ($before -eq $after) "log lines $before -> $after"
    $dry = & $script -SessionId $id -ConfigDir $cfg -DryRun
    Check 'account 2 copy: a start on account 1 copies nothing' ($dry[4] -eq 'copy=') ($dry -join ' | ')
    # every line a start adds to the log is looked at, not only the last
    # (the starter log keeps only its last 200 lines, so a count before and after cannot tell what one start added: each run gets a fresh log file instead)
    function Lines-Added-By([scriptblock]$run) {
        $saved = $env:RESUMABLE_TEST_LOG
        $fresh = Join-Path $logDir ('fresh-' + [guid]::NewGuid().ToString('N').Substring(0, 8) + '.log')
        $made.Add($fresh)
        $env:RESUMABLE_TEST_LOG = $fresh
        try { $null = & $run } finally { $env:RESUMABLE_TEST_LOG = $saved }
        if (Test-Path -LiteralPath $fresh) { return @(Get-Content -LiteralPath $fresh) } else { return @() }
    }
    $added = Lines-Added-By { Run-Child $cfg }
    Check 'account 2 copy: a start on account 1 adds no account2copy line' (@($added | Where-Object { $_ -match 'account2copy' }).Count -eq 0) ($added -join ' // ')
    $added = Lines-Added-By { Run-Child $acct2 }
    $mine = @($added | Where-Object { $_ -match 'account2copy' })
    Check 'account 2 copy: a start on account 2 adds exactly one account2copy line, ok, with both hashes' ($mine.Count -eq 1 -and $mine[0].EndsWith($want)) ($added -join ' // ')
    # a copy that cannot finish: the destination is held open by another program (the share mode none), so Copy-Item cannot write it
    $holdFile = Join-Path $acct2 'CLAUDE.md'
    $hold = [IO.File]::Open($holdFile, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    try { $added = Lines-Added-By { Run-Child $acct2 } } finally { $hold.Dispose() }
    $mine = @($added | Where-Object { $_ -match 'account2copy' })
    Check 'account 2 copy: one file that cannot be copied is ONE line, partial, naming the failed file and the hash of the other' ($mine.Count -eq 1 -and $mine[0] -match ' account2copy=partial set=CLAUDE\.md:failed,settings\.json:' + (Hash12 (Join-Path $personal 'settings.json')) + '$') ($added -join ' // ')
    $holdA = [IO.File]::Open((Join-Path $acct2 'CLAUDE.md'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    $holdB = [IO.File]::Open((Join-Path $acct2 'settings.json'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    try { $added = Lines-Added-By { Run-Child $acct2 } } finally { $holdA.Dispose(); $holdB.Dispose() }
    $mine = @($added | Where-Object { $_ -match 'account2copy' })
    Check 'account 2 copy: no file copied is ONE line, failed, naming both' ($mine.Count -eq 1 -and $mine[0] -match ' account2copy=failed set=CLAUDE\.md:failed,settings\.json:failed$') ($added -join ' // ')
    # a destination that can be read but denies writes: the old file is still there, and the start must not log it as copied
    Set-Content -LiteralPath (Join-Path $personal 'CLAUDE.md') -Value 'rules denied'
    $denyFile = Join-Path $acct2 'CLAUDE.md'
    $denyAcl = Get-Acl -LiteralPath $denyFile
    $denyRule = [Security.AccessControl.FileSystemAccessRule]::new([Security.Principal.WindowsIdentity]::GetCurrent().Name, 'WriteData,AppendData,WriteAttributes,WriteExtendedAttributes', 'Deny')
    $denyAcl.AddAccessRule($denyRule); Set-Acl -LiteralPath $denyFile -AclObject $denyAcl
    try { $added = Lines-Added-By { Run-Child $acct2 } } finally { $undo = Get-Acl -LiteralPath $denyFile; $null = $undo.RemoveAccessRule($denyRule); Set-Acl -LiteralPath $denyFile -AclObject $undo }
    $mine = @($added | Where-Object { $_ -match 'account2copy' })
    Check 'account 2 copy: a destination that denies writes is logged as failed, never ok, and keeps its old content' ($mine.Count -eq 1 -and $mine[0] -match ' account2copy=partial set=CLAUDE\.md:failed,settings\.json:' -and (Text-Of $denyFile) -eq 'rules two') ($added -join ' // ') + " // content: $(Text-Of $denyFile)"
    $below = Join-Path $acct2 'cfg'; Make-Dir $below
    $r = Run-Child $below
    Check 'account 2 copy: a folder below the account 2 folder is not the account folder and copies nothing' ($r.Code -eq 0 -and -not (Test-Path -LiteralPath (Join-Path $below 'CLAUDE.md')) -and -not (Test-Path -LiteralPath (Join-Path $below 'settings.json'))) $r.Text
    $savedSettings = Join-Path $personal 'settings.json'
    Remove-Item -LiteralPath $savedSettings
    Set-Content -LiteralPath (Join-Path $personal 'CLAUDE.md') -Value 'rules three'
    $r = Run-Child $acct2
    Check 'account 2 copy: a missing source is skipped with a warning, the other file is copied, the session starts' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and $r.Text -match 'settings.json was not copied' -and (Text-Of (Join-Path $acct2 'CLAUDE.md')) -eq 'rules three' -and (Text-Of (Join-Path $acct2 'settings.json')) -eq '{"hooks":"two"}') $r.Text
    Make-Dir $savedSettings
    $r = Run-Child $acct2
    Check 'account 2 copy: a folder in place of a source file is skipped, the session starts' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and $r.Text -match 'settings.json was not copied') $r.Text
    [IO.Directory]::Delete($savedSettings); $made.Remove($savedSettings) | Out-Null
    Make-File $savedSettings '{"hooks":"four"}'
    # a destination that is a link is never written through: its target stays as it was, the other file is still copied, the session starts
    $decoy2 = Join-Path $root 'a2-decoy'; Make-Dir $decoy2
    $dstLink = Join-Path $acct2 'settings.json'
    if (Test-Path -LiteralPath $dstLink) { Remove-Item -LiteralPath $dstLink }
    $null = New-Item -ItemType Junction -Path $dstLink -Target $decoy2; $made.Add($dstLink)
    $r = Run-Child $acct2
    Check 'account 2 copy: a destination that is a link is not written through, the other file is copied, the session starts' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and @(Get-ChildItem -LiteralPath $decoy2 -Force).Count -eq 0 -and $r.Text -match 'settings.json was not copied' -and (Text-Of (Join-Path $acct2 'CLAUDE.md')) -eq 'rules three') "decoy has $(@(Get-ChildItem -LiteralPath $decoy2 -Force).Count) entries; $($r.Text)"
    [IO.Directory]::Delete($dstLink); $made.Remove($dstLink) | Out-Null
    # account 1's folder as a link: its files are somewhere else, so nothing is read from it
    $elsewhere = Join-Path $root 'a1-elsewhere'; Make-Dir $elsewhere; Make-File (Join-Path $elsewhere 'CLAUDE.md') 'outside rules'; Make-File (Join-Path $elsewhere 'settings.json') '{"outside":true}'
    $prof2 = Join-Path $root 'Profile Two'; Make-Dir $prof2
    $p2a1 = Join-Path $prof2 '.claude-personal'; $p2a2 = Join-Path $prof2 '.claude-personal2'; Make-Dir $p2a2
    $null = New-Item -ItemType Junction -Path $p2a1 -Target $elsewhere; $made.Add($p2a1)
    $env:USERPROFILE = $prof2
    $dry = @(& pwsh -NoProfile -File $script -SessionId $id -ConfigDir $p2a2 -DryRun 2>&1)
    $env:USERPROFILE = $profileDir
    Check 'account 2 copy: account 1 folder that is a link gives no copy and a warning' ($dry -contains 'copy=' -and ($dry -join ' | ') -match 'not a real folder' -and -not (Test-Path -LiteralPath (Join-Path $p2a2 'CLAUDE.md'))) ($dry -join ' | ')
    [IO.Directory]::Delete($p2a1); $made.Remove($p2a1) | Out-Null
    $dry = & $script -SessionId $id -ConfigDir $throw -ThrowawayConfig -DryRun
    Check 'a throwaway folder is accepted with the switch' ($dry[1] -eq "config=$throw") ($dry -join ' | ')
    $dry = & $script -SessionId $id -ConfigDir $junc.Replace('to-old', 'cfg') -DryRun
    Check 'a folder name inside .claude-personal that does not exist yet is accepted' ($dry[1] -like '*\cfg') ($dry -join ' | ')
    $dry = & $script -SessionId $id -ConfigDir ($personal + '\cfg\..\cfg') -DryRun
    Check 'the folder handed on is the resolved full path' ($dry[1] -eq "config=$cfg") ($dry -join ' | ')
    # ---- the session-to-account record (so the budget scan can split tokens per account): one small file per real start, written failure-safe ----
    # <AccountDir>\<session id>-<at as yyyyMMddHHmmssfff>-<account>.json holding {at, session, account}: no shared file, no lock, no append, no trim.
    $acctDir = Join-Path $logDir 'session-accounts'
    $personal2 = Join-Path $profileDir '.claude-personal2'
    $cfg2 = Join-Path $personal2 'cfg'
    Make-Dir $personal2; Make-Dir $cfg2; Make-Dir (Join-Path $cfg2 'projects')
    $env:RESUMABLE_TEST_ACCOUNTLOG = $acctDir
    function Acct-Files([string]$dir = $acctDir) { if (Test-Path -LiteralPath $dir -PathType Container) { return @(Get-ChildItem -LiteralPath $dir -File -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -like '*.json' }) } else { return @() } }
    function Acct-Rows([string]$dir = $acctDir) { return @(Acct-Files $dir | ForEach-Object { try { Get-Content -LiteralPath $_.FullName -Raw | ConvertFrom-Json -DateKind String } catch { $null } }) }
    $r = Run-Child $cfg
    $af = @(Acct-Files); $row = if ($af.Count) { Get-Content -LiteralPath $af[0].FullName -Raw | ConvertFrom-Json -DateKind String } else { $null }
    Check 'account record: a start on account 1 writes ONE file named <session>-<yyyyMMddHHmmssfff>-1-<32 hex>.json holding at, session, account and nothing else' ($af.Count -eq 1 -and $af[0].Name -match ('^' + [regex]::Escape($id) + '-\d{17}-1-[0-9a-f]{32}\.json$') -and $row.session -eq $id -and $row.account -eq 1 -and $row.at -match '^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$' -and (@($row.PSObject.Properties.Name) -join ',') -eq 'at,session,account') "files=$(($af | ForEach-Object { $_.Name }) -join ', ') $($r.Text)"
    Check 'account record: the session itself started (the stub ran, exit 0) and no temporary file is left' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and @(Get-ChildItem -LiteralPath $acctDir -Filter '.session-account-*.tmp' -Force).Count -eq 0) "exit=$($r.Code) $($r.Text)"
    Start-Sleep -Milliseconds 30
    $r = Run-Child $cfg2
    $rows = @(Acct-Rows)
    Check 'account record: the same session started again on account 2 leaves BOTH records (nothing is overwritten)' (@(Acct-Files).Count -eq 2 -and @($rows | Where-Object { $_.account -eq 1 }).Count -eq 1 -and @($rows | Where-Object { $_.account -eq 2 }).Count -eq 1 -and @($rows | Where-Object { $_.session -eq $id }).Count -eq 2) "files=$(@(Acct-Files).Count) rows=$(($rows | ForEach-Object { $_.account }) -join ',')"
    $dry = & $script -SessionId $id -ConfigDir $cfg -DryRun -AccountDir $acctDir
    Check 'account record: a dry run writes nothing' (@(Acct-Files).Count -eq 2) "files=$(@(Acct-Files).Count)"
    # many starts at the same moment (every reboot: Wave restarts all --restart blocks together): every record must be there, nothing corrupt, every session started
    $parDir = Join-Path $logDir 'session-accounts-parallel'
    $parStub = Join-Path $root 'par-stub.ps1'; Make-File $parStub 'exit 0'
    $parIds = @(1..30 | ForEach-Object { [guid]::NewGuid().ToString() })
    $procs = @()
    foreach ($round in 0..4) {
        foreach ($k in 0..5) {
            $sid = $parIds[$round * 6 + $k]
            $procs += Start-Process -FilePath pwsh -ArgumentList @('-NoProfile', '-File', ('"' + $script + '"'), '-SessionId', $sid, '-ConfigDir', ('"' + $cfg + '"'), '-Command', ('"' + $parStub + '"'), '-AccountDir', ('"' + $parDir + '"'), '-LogRoot', ('"' + $root + '"'), '-LogPath', ('"' + (Join-Path $logDir 'par-starter.log') + '"')) -PassThru -WindowStyle Hidden
        }
        foreach ($pr in $procs[($round * 6)..($round * 6 + 5)]) { $null = $pr.WaitForExit(120000) }
    }
    $parRows = @(Acct-Rows $parDir)
    $parSessions = @($parRows | Where-Object { $_ } | ForEach-Object { $_.session })
    Check 'account record, parallel starts: 6 starts at once, five rounds: every start started (exit 0)' (@($procs | Where-Object { $_.ExitCode -ne 0 }).Count -eq 0) "exit codes: $(($procs | ForEach-Object { $_.ExitCode }) -join ',')"
    Check 'account record, parallel starts: all 30 records are there, none lost, none corrupt' (@(Acct-Files $parDir).Count -eq 30 -and @($parRows | Where-Object { -not $_ }).Count -eq 0 -and @($parIds | Where-Object { $_ -notin $parSessions }).Count -eq 0) "files=$(@(Acct-Files $parDir).Count) of 30, missing=$(@($parIds | Where-Object { $_ -notin $parSessions }).Count), corrupt=$(@($parRows | Where-Object { -not $_ }).Count)"
    Check 'account record, parallel starts: no temporary file is left in the folder' (@(Get-ChildItem -LiteralPath $parDir -Filter '.session-account-*.tmp' -Force).Count -eq 0) ''
    # the same session started twice in the same instant on the same account is not an error and not a loss of the first
    # failure-safe: a file where the folder should be, a read-only folder, a path below .claude, a folder outside the log root, a link in place of the folder
    $acctAsFile = Join-Path $logDir 'acct-as-file'; Make-File $acctAsFile 'a file, not a folder'
    $env:RESUMABLE_TEST_ACCOUNTLOG = $acctAsFile
    $r = Run-Child $cfg
    Check 'account record: a file in place of the folder does not stop the start and is left as it was' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and ((Get-Content -LiteralPath $acctAsFile -Raw).Trim() -eq 'a file, not a folder')) "exit=$($r.Code) $($r.Text)"
    $env:RESUMABLE_TEST_ACCOUNTLOG = Join-Path $oldDir 'logs\session-accounts'
    $r = Run-Child $cfg
    Check 'account record: a folder below .claude is refused (nothing created there), the session starts' ((-not (Test-Path -LiteralPath (Join-Path $oldDir 'logs'))) -and $r.Code -eq 0 -and $r.Lines.Count -gt 0) "exit=$($r.Code)"
    $outsideDir = Join-Path $env:TEMP ('acct-outside-' + [guid]::NewGuid().ToString('N') + '\session-accounts')
    $env:RESUMABLE_TEST_ACCOUNTLOG = $outsideDir
    $r = Run-Child $cfg
    Check 'account record: a folder outside the log root is refused, nothing is created, the session starts' ((-not (Test-Path -LiteralPath (Split-Path -Parent $outsideDir))) -and $r.Code -eq 0 -and $r.Lines.Count -gt 0) "exit=$($r.Code)"
    # a link in place of the folder: its target is never written
    $decoy = Join-Path $profileDir 'acct-decoy-target'; Make-Dir $decoy
    $linkDir = Join-Path $logDir 'acct-link'
    $null = New-Item -ItemType Junction -Path $linkDir -Target $decoy; $made.Add($linkDir)
    $env:RESUMABLE_TEST_ACCOUNTLOG = $linkDir
    $r = Run-Child $cfg
    Check 'account record: a folder that is a link is not written into: the target stays empty, the session starts' (@(Get-ChildItem -LiteralPath $decoy -Force).Count -eq 0 -and $r.Code -eq 0 -and $r.Lines.Count -gt 0) "target has $(@(Get-ChildItem -LiteralPath $decoy -Force).Count) entries, exit=$($r.Code)"
    $oldLinkDir = Join-Path $logDir 'acct-link-into-old'
    $null = New-Item -ItemType Junction -Path $oldLinkDir -Target $oldDir; $made.Add($oldLinkDir)
    $env:RESUMABLE_TEST_ACCOUNTLOG = Join-Path $oldLinkDir 'session-accounts'
    $r = Run-Child $cfg
    Check 'account record: a junction below the log root that leads into .claude gets nothing and no new folder' ((-not (Test-Path -LiteralPath (Join-Path $oldDir 'session-accounts'))) -and $r.Code -eq 0 -and $r.Lines.Count -gt 0) "exit=$($r.Code)"
    # two starts of the SAME session on the SAME account in the SAME millisecond must both be recorded (a short random suffix in the name): -AccountAt fixes the time for the test
    $sameDir = Join-Path $logDir 'session-accounts-same-ms'
    $sameStub = Join-Path $root 'same-stub.ps1'; Make-File $sameStub 'exit 0'
    $sameId = [guid]::NewGuid().ToString()
    $sameProcs = @(1..6 | ForEach-Object {
        Start-Process -FilePath pwsh -ArgumentList @('-NoProfile', '-File', ('"' + $script + '"'), '-SessionId', $sameId, '-ConfigDir', ('"' + $cfg + '"'), '-Command', ('"' + $sameStub + '"'), '-AccountDir', ('"' + $sameDir + '"'), '-AccountAt', '2026-10-18T12:00:00.123Z', '-LogRoot', ('"' + $root + '"'), '-LogPath', ('"' + (Join-Path $logDir 'same-starter.log') + '"')) -PassThru -WindowStyle Hidden })
    foreach ($pr in $sameProcs) { $null = $pr.WaitForExit(120000) }
    $sameFiles = @(Acct-Files $sameDir)
    Check 'account record: 6 starts of ONE session on ONE account in the same millisecond keep all 6 records (names differ by a random 32-hex suffix), every start exits 0' ($sameFiles.Count -eq 6 -and @($sameFiles | Where-Object { $_.Name -notmatch ('^' + [regex]::Escape($sameId) + '-20261018120000123-1-[0-9a-f]{32}\.json$') }).Count -eq 0 -and @($sameProcs | Where-Object { $_.ExitCode -ne 0 }).Count -eq 0) "files=$($sameFiles.Count): $(($sameFiles | ForEach-Object { $_.Name }) -join ', ') exits=$(($sameProcs | ForEach-Object { $_.ExitCode }) -join ',')"
    # a record that cannot be written never fails or stops a start (the scope is ACCIDENTAL faults: parallel starts, a link set up by mistake, a folder in the wrong place)
    $env:RESUMABLE_TEST_ACCOUNTLOG = $acctAsFile
    $r = Run-Child $cfg
    Check 'account record: a record that cannot be written never fails or stops a start (exit 0, the stub ran with the resume arguments)' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and ($r.Text -match '--(resume|session-id)')) "exit=$($r.Code) $($r.Text)"
    # a refused or failed record is NOTED in starter.log (a fixed code, never a path) and the start still runs
    $env:RESUMABLE_TEST_LOG = $swapLog
    $noteCases = @(
        @('a file in place of the folder', $acctAsFile, 'folder-is-file-or-link'),
        @('a link in place of the folder', $linkDir, 'folder-is-file-or-link'),
        @('a folder written plainly below .claude', (Join-Path $oldDir 'logs\session-accounts'), 'folder-forbidden'),
        @('a junction below the log root that leads into .claude', (Join-Path $oldLinkDir 'session-accounts'), 'folder-under-claude'),
        @('a folder outside the log root', (Join-Path $env:TEMP ('acct-outside-' + [guid]::NewGuid().ToString('N') + '\session-accounts')), 'folder-outside-log-root'))
    foreach ($nc in $noteCases) {
        Make-File $swapLog ''
        $env:RESUMABLE_TEST_ACCOUNTLOG = $nc[1]
        $r = Run-Child $cfg
        $noted = @(Get-Content -LiteralPath $swapLog | Where-Object { $_ -match ' account-record=' })
        Check ("account record: " + $nc[0] + " is refused, the refusal is noted in starter.log as account-record=" + $nc[2] + " (no path in the line), and the start runs") ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and $noted.Count -eq 1 -and $noted[0] -match (' account-record=' + [regex]::Escape($nc[2]) + '$')) "exit=$($r.Code) noted=$($noted -join ' | ') $($r.Text)"
    }
    $env:RESUMABLE_TEST_ACCOUNTLOG = $acctDir
    Make-File $swapLog ''
    $r = Run-Child $cfg
    Check 'account record: a recorded start leaves no account-record line in starter.log' ($r.Code -eq 0 -and @(Get-Content -LiteralPath $swapLog | Where-Object { $_ -match 'account-record=' }).Count -eq 0) "exit=$($r.Code)"
    # the name carries the FULL 32-hex guid as its suffix
    $fullNames = @(Acct-Files | ForEach-Object { $_.Name })
    Check 'account record: the name ends with a full 32-hex guid (<session>-<at>-<account>-<32 hex>.json)' ($fullNames.Count -ge 1 -and @($fullNames | Where-Object { $_ -notmatch '-\d{17}-[12]-[0-9a-f]{32}\.json$' }).Count -eq 0) ($fullNames -join ', ')
    $env:RESUMABLE_TEST_ACCOUNTLOG = $acctDir

    # ---- claude not found (20 Oct 2026 16:35: an npm update of claude-code rewrote its shims in the same seconds as a start; the block died with
    # "The term 'claude' is not recognized" and starter.log said nothing) ----
    Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
    $env:RESUMABLE_TEST_RECORD = $rec
    $missing = Join-Path $root 'late\claude-stub.ps1'
    $missErr = Join-Path $root 'missing-error.txt'
    Make-File $swapLog ''
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    $t0 = Get-Date
    & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -Command $missing -CommandWaitSeconds 2 -LogPath $swapLog -LogRoot $root 2>$missErr | Out-Null
    $code = $LASTEXITCODE; $took = ((Get-Date) - $t0).TotalSeconds
    $errText = if (Test-Path -LiteralPath $missErr) { Get-Content -LiteralPath $missErr -Raw } else { '' }
    $logText = [string](Get-Content -LiteralPath $swapLog -Raw)
    Check 'claude that never appears: waits the given seconds, exits 3, says so in the block, logs start=claude-missing' ($code -eq 3 -and $took -ge 2 -and $errText -match 'not found' -and $logText -match ' start=claude-missing' -and -not (Test-Path -LiteralPath $rec)) "exit=$code took=$took err=$errText log=$logText"
    # the command appears while the starter waits: it starts, and the wait is logged
    Make-File $swapLog ''
    Make-Dir (Split-Path -Parent $missing)
    # the copy waits for the starter's signal that a lookup has FAILED, so a slow start of the child cannot let the stub appear before the first look
    $signal = Join-Path $root 'lookup-signal.txt'
    $job = Start-ThreadJob -ArgumentList $stub, $missing, $signal -ScriptBlock {
        param($from, $to, $sig)
        $until = (Get-Date).AddSeconds(60)
        while (-not (Test-Path -LiteralPath $sig) -and (Get-Date) -lt $until) { Start-Sleep -Milliseconds 100 }
        # copied beside it under another name, then renamed into place: the starter never sees a half-written stub
        $part = $to + '.part'; Copy-Item -LiteralPath $from -Destination $part; [IO.File]::Move($part, $to)
    }
    & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -Command $missing -CommandWaitSeconds 20 -LookupSignal $signal -LogPath $swapLog -LogRoot $root 2>$missErr | Out-Null
    $code = $LASTEXITCODE
    $null = $job | Wait-Job | Remove-Job
    $made.Add($missing); if (Test-Path -LiteralPath $signal) { $made.Add($signal) }
    $logText = [string](Get-Content -LiteralPath $swapLog -Raw)
    $ran = (Test-Path -LiteralPath $rec) -and ((Get-Content -LiteralPath $rec -Raw) -match '--(resume|session-id)')
    Check 'claude that appears during the wait: the session starts and starter.log says start=claude-found-after:<n>s' ($code -eq 0 -and $ran -and $logText -match ' start=claude-found-after:\d+s') "exit=$code ran=$ran log=$logText"
    # found at once: no start line at all (the log stays quiet on the normal path)
    Make-File $swapLog ''
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -Command $missing -LogPath $swapLog -LogRoot $root 2>$missErr | Out-Null
    $ranNow = (Test-Path -LiteralPath $rec) -and ((Get-Content -LiteralPath $rec -Raw) -match '--(resume|session-id)')
    Check 'claude found at once: it is started (a fresh record) and starter.log has no start= line' ($LASTEXITCODE -eq 0 -and $ranNow -and -not ([string](Get-Content -LiteralPath $swapLog -Raw) -match ' start=')) "ran=$ranNow log=$([string](Get-Content -LiteralPath $swapLog -Raw))"
    # a function of that name is not a file: it is never taken for claude (Clear-Host is a function in every pwsh)
    Make-File $swapLog ''
    & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -Command 'Clear-Host' -CommandWaitSeconds 1 -LogPath $swapLog -LogRoot $root 2>$missErr | Out-Null
    Check 'a function named as the command is not taken for claude: exit 3 and start=claude-missing' ($LASTEXITCODE -eq 3 -and ([string](Get-Content -LiteralPath $swapLog -Raw) -match ' start=claude-missing')) "exit=$LASTEXITCODE log=$([string](Get-Content -LiteralPath $swapLog -Raw))"
    Remove-Item -LiteralPath $missErr -ErrorAction SilentlyContinue
} finally {
    $env:USERPROFILE = $realProfile
    $env:LOCALAPPDATA = $realLocal
    Remove-Item Env:RESUMABLE_TEST_LOG, Env:RESUMABLE_TEST_LOGROOT -ErrorAction SilentlyContinue
    Remove-Item Env:RESUMABLE_TEST_RECORD -ErrorAction SilentlyContinue
    Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $rec -Force -Confirm:$false -ErrorAction SilentlyContinue
    for ($i = $made.Count - 1; $i -ge 0; $i--) {
        $p = $made[$i]
        try {
            $item = Get-Item -LiteralPath $p -Force -ErrorAction Stop
            if ($item.PSIsContainer) { [IO.Directory]::Delete($p) } else { [IO.File]::Delete($p) }
        } catch { }
    }
}
"failures: $fail"

exit $(if ($fail -gt 0) { 1 } else { 0 })
