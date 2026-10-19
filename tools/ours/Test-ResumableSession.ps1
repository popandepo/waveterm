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
$env:RESUMABLE_TEST_LOG = $swapLog
$env:RESUMABLE_TEST_LOGROOT = $root
# a token made the way Wave makes it (shellutil PackSwapToken): base64 of JSON {"token": uuid, "rpccontext": {...}}, here with bytes that give + / and =
$packedJson = '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"C:\\Users\\x\\AppData\\Local\\Temp\\wave-1234\\wave.sock","routeid":"","procroute":true,"blockid":"22222222-aaaa-bbbb-cccc-333333333333","conn":">>>???>>>???x"}}'
$packedToken = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($packedJson))
$env:RESUMABLE_TEST_PACKED = $packedToken
$env:RESUMABLE_TEST_WSHROOT = $root
# The result must be the same inside a Wave tile as outside one: the WAVETERM_* values of a tile (a real JWT and block id) and its PATH entry for the real wsh
# would leak into every child below and turn six checks red (seen on 12 Oct 2026, the first run from a tile). Each scenario sets what it needs itself.
foreach ($v in @(Get-ChildItem env: | Where-Object { $_.Name -like 'WAVETERM*' -or $_.Name -eq 'TERM_PROGRAM' })) { Remove-Item -LiteralPath ('Env:' + $v.Name) }
function Test-HasWsh([string]$dir) { try { (Test-Path -LiteralPath (Join-Path $dir 'wsh.exe')) -or (Test-Path -LiteralPath (Join-Path $dir 'wsh.cmd')) } catch { $false } }
$env:PATH = (@($env:PATH -split ';' | Where-Object { $_ -and -not (Test-HasWsh $_) }) -join ';')
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
    $mgr = & $script -SessionId '3c641601-9a7a-44be-a93c-e5a89419429b' -ConfigDir $cfg -DryRun
    Check 'the manager session gets the 300k compact window' ($mgr[3] -eq 'compact=300000') ($mgr -join ' | ')
    Check 'any other session gets no compact window' ($other[3] -eq 'compact=' -and $later[3] -eq 'compact=') (($other -join ' | ') + ' // ' + ($later -join ' | '))

    # ---- how it is run: a child pwsh, so that this shell keeps its environment ----
    $stub = Join-Path $root 'stub.ps1'
    $rec = Join-Path $root 'record.txt'
    Make-File $stub @'
param()
$lines = @("ARGS: " + ($args -join ' '))
Get-ChildItem env: | Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -like 'STATUS_*' -or $_.Name -like 'WAVETERM*' -or $_.Name -in 'NO_COLOR', 'FORCE_COLOR', 'TERM_PROGRAM', 'EVIL_VAR' } | ForEach-Object { $lines += "ENV: $($_.Name)=$($_.Value)" }
$lines += 'PATHLAST: ' + (@($env:PATH -split ';' | Where-Object { $_ })[-1])
$lines += 'PATHCOUNT: ' + (@($env:PATH -split ';' | Where-Object { $_ -ieq $env:RESUMABLE_TEST_WSHDIR }).Count)
$found = Get-Command wsh -ErrorAction SilentlyContinue
$lines += 'WSH: ' + $(if ($found) { $found.Source } else { 'none' })
$lines += 'WSHOUT: ' + $(if ($found -and -not $env:RESUMABLE_TEST_NOWSH) { (& wsh 2>&1 | Out-String).Trim() } else { 'none' })
$lines += 'ARGCOUNT: ' + $args.Count
Set-Content -LiteralPath $env:RESUMABLE_TEST_RECORD $lines
'@
    $child = Join-Path $root 'child.ps1'
    Make-File $child @'
param($Script, $Stub, $Record, $WshDir, $PathMode, $Cfg, $Id, [string]$ExtraPrompt, [string]$ExtraName, [string]$SwapDir, [string]$ExtraPromptFile)
$env:CLAUDE_CONFIG_DIR = 'X:\somewhere\else'; $env:CLAUDE_CODE_CHILD_SESSION = '1'; $env:CLAUDECODE = '1'; $env:NO_COLOR = '1'; $env:CLAUDE_CODE_AUTO_COMPACT_WINDOW = '999'
$env:RESUMABLE_TEST_RECORD = $Record
$env:RESUMABLE_TEST_WSHDIR = $WshDir
$env:STATUS_SESSION = 'stale-1234'; $env:STATUS_TAB = 'stale'
$sep = ';'
switch ($PathMode) {
    'set' { $env:WAVETERM_WSHBINDIR = $WshDir }
    'missing' { $env:WAVETERM_WSHBINDIR = $WshDir + '-nope' }
    'already' { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = $WshDir + $sep + $env:PATH }
    'spelling' { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = $WshDir.ToUpper() + '\..\' + (Split-Path $WshDir -Leaf) + '\' + $sep + $env:PATH }
    'relative' { Set-Location -LiteralPath (Split-Path $WshDir -Parent); [Environment]::CurrentDirectory = (Split-Path $WshDir -Parent); $env:WAVETERM_WSHBINDIR = 'wshbin' }
    'rooted-no-drive' { $env:WAVETERM_WSHBINDIR = '\' + (Split-Path $WshDir -NoQualifier).TrimStart('\') }
    'drive-relative' { $env:WAVETERM_WSHBINDIR = (Split-Path $WshDir -Qualifier) + 'wshbin' }
    'illegal' { $env:WAVETERM_WSHBINDIR = 'C:\no"such<>folder' }
    'file' { $env:WAVETERM_WSHBINDIR = $WshDir + '\wsh.cmd' }
    'path-unset' { $env:WAVETERM_WSHBINDIR = $WshDir; [Environment]::SetEnvironmentVariable('PATH', $null) }
    'other' { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = $env:RESUMABLE_TEST_OTHERDIR + $sep + $env:PATH }
    'path-huge' { $env:WAVETERM_WSHBINDIR = $WshDir; $env:PATH = ('x' + $sep) * 16380; $env:RESUMABLE_TEST_NOWSH = '1' }
    'unset' { Remove-Item Env:WAVETERM_WSHBINDIR -ErrorAction SilentlyContinue }
    'swap' { $env:WAVETERM_SWAPTOKEN = $(if ($env:RESUMABLE_TEST_SWAPTOKEN) { $env:RESUMABLE_TEST_SWAPTOKEN } else { $env:RESUMABLE_TEST_PACKED }); $env:WAVETERM_WSHBINDIR = $SwapDir }
    'swap-nowsh' { $env:WAVETERM_SWAPTOKEN = $env:RESUMABLE_TEST_PACKED; Remove-Item Env:WAVETERM_WSHBINDIR -ErrorAction SilentlyContinue }
    'swap-nodir' { $env:WAVETERM_SWAPTOKEN = $env:RESUMABLE_TEST_PACKED; $env:WAVETERM_WSHBINDIR = $WshDir }
}
$extra = @{}
if ($env:RESUMABLE_TEST_WSHROOT -and -not $env:RESUMABLE_TEST_NOROOT) { $extra.WshRoot = $env:RESUMABLE_TEST_WSHROOT }
if ($env:RESUMABLE_TEST_LOG) { $extra.LogPath = $env:RESUMABLE_TEST_LOG }
if ($env:RESUMABLE_TEST_LOGROOT) { $extra.LogRoot = $env:RESUMABLE_TEST_LOGROOT }
if ($env:RESUMABLE_TEST_ACCOUNTLOG) { $extra.AccountDir = $env:RESUMABLE_TEST_ACCOUNTLOG }
if ($env:RESUMABLE_TEST_ACCOUNTAT) { $extra.AccountAt = $env:RESUMABLE_TEST_ACCOUNTAT }
if ($env:RESUMABLE_TEST_NOLOCALAPPDATA) { Remove-Item Env:LOCALAPPDATA -ErrorAction SilentlyContinue }
if ($env:RESUMABLE_TEST_INHERIT) { $env:WAVETERM_JWT = 'inherited-jwt'; $env:WAVETERM_BLOCKID = 'inherited-block'; $env:WAVETERM_TABID = 'inherited-tab'; $env:WAVETERM_WORKSPACEID = 'inherited-ws' }
if ($env:RESUMABLE_TEST_DECOYDIR) { $env:PATH = $env:RESUMABLE_TEST_DECOYDIR + $sep + $env:PATH }
if ($ExtraPrompt) { $extra.Prompt = $ExtraPrompt }
if ($ExtraPromptFile) { $extra.PromptFile = $ExtraPromptFile }
if ($env:RESUMABLE_TEST_PROMPTROOT) { $extra.PromptRoot = $env:RESUMABLE_TEST_PROMPTROOT }
if ($ExtraName) { $extra.Name = $ExtraName }
if ($env:RESUMABLE_TEST_SWAPTIMEOUT) { $extra.SwapTimeoutSeconds = [int]$env:RESUMABLE_TEST_SWAPTIMEOUT }
& $Script -SessionId $Id -ConfigDir $Cfg -Command $Stub -Model m1 @extra
exit $LASTEXITCODE
'@
    $wshDir = Join-Path $root 'wshbin'
    $otherDir = Join-Path $root 'otherbin'
    Make-Dir $wshDir; Make-Dir $otherDir
    Make-File (Join-Path $wshDir 'wsh.cmd') '@echo wsh-from-wave'
    Make-File (Join-Path $otherDir 'wsh.cmd') '@echo wsh-already-there'
    $env:RESUMABLE_TEST_OTHERDIR = $otherDir
    function Run-Child([string]$mode, [string]$cfgDir = $cfg, [string]$prompt = '', [string]$name = '', [string]$swapDir = '', [string]$promptFile = '', [string]$runId = $id) {
        Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
        $a = @('-NoProfile', '-File', $child, '-Script', $script, '-Stub', $stub, '-Record', $rec, '-WshDir', $wshDir, '-PathMode', $mode, '-Cfg', $cfgDir, '-Id', $runId)
        if ($prompt) { $a += '-ExtraPrompt', $prompt }
        if ($name) { $a += '-ExtraName', $name }
        if ($swapDir) { $a += '-SwapDir', $swapDir }
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
    $r = Run-Child 'set'
    $shellAfter = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
    Check 'this shell is left as it was (the launch ran in a child process)' (($shellBefore -join '|') -eq ($shellAfter -join '|')) ''
    Check 'the launched program gets the searched config folder, not the inherited one' ($r.Lines -contains "ENV: CLAUDE_CONFIG_DIR=$cfg") $r.Text
    Check 'the inherited child-session markers are gone' (-not ($r.Text -match 'ENV: (CLAUDE_CODE_CHILD_SESSION|CLAUDECODE)=')) $r.Text
    Check 'NO_COLOR is gone' (-not ($r.Text -match 'ENV: NO_COLOR=')) $r.Text
    Check 'an inherited compact window is gone for any other session' (-not ($r.Text -match 'ENV: CLAUDE_CODE_AUTO_COMPACT_WINDOW=')) $r.Text
    $rm = Run-Child 'set' -runId '3c641601-9a7a-44be-a93c-e5a89419429b'
    Check 'the manager session is started with the 300k compact window (R-20261016-007)' ($rm.Lines -contains 'ENV: CLAUDE_CODE_AUTO_COMPACT_WINDOW=300000') $rm.Text
    Check 'the launched program gets the resume arguments' ($r.Text -match "ARGS: --resume $id") $r.Text
    Check 'the folder of wsh is put AFTER the existing PATH entries' ($r.Lines -contains "PATHLAST: $wshDir") $r.Text
    Check 'wsh can be run from there and the stub prints what wsh printed' ($r.Lines -contains 'WSHOUT: wsh-from-wave') $r.Text
    foreach ($mode in 'missing', 'unset', 'relative', 'rooted-no-drive', 'drive-relative', 'file') {
        $r = Run-Child $mode
        Check "with WAVETERM_WSHBINDIR '$mode' the session starts and the PATH is left alone" (($r.Code -eq 0) -and ($r.Lines -contains 'PATHCOUNT: 0') -and -not ($r.Lines -contains "PATHLAST: $wshDir") -and -not ($r.Lines -contains "PATHLAST: $wshDir-nope") -and $r.Lines.Count -gt 0) $r.Text
    }
    foreach ($mode in 'illegal', 'path-unset', 'path-huge') {
        $r = Run-Child $mode
        Check "with WAVETERM_WSHBINDIR '$mode' the session still starts" (($r.Code -eq 0) -and ($r.Lines.Count -gt 0)) "exit=$($r.Code) $($r.Text)"
    }
    $r = Run-Child 'already'
    Check 'a folder that is already on the PATH is not added again' ($r.Lines -contains 'PATHCOUNT: 1') $r.Text
    $r = Run-Child 'spelling'
    Check 'the same folder under another spelling (case, dot dot, trailing slash) is not added again' (-not ($r.Lines -contains "PATHLAST: $wshDir")) $r.Text
    $r = Run-Child 'other'
    Check 'a wsh that is already on the PATH wins over the one Wave names' (($r.Lines -contains 'WSHOUT: wsh-already-there') -and ($r.Lines -contains "PATHLAST: $wshDir")) $r.Text

    # ---- the status page's keys ----
    $r = Run-Child 'set'
    Check 'without -Name no STATUS_SESSION or STATUS_TAB reaches the program, and the inherited ones are removed' (-not ($r.Text -match 'ENV: STATUS_')) $r.Text
    $r1 = Run-Child 'set' $cfg '' 'n1'
    $r2 = Run-Child 'set' $cfg '' 'n1'
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

    # ---- the token swap: what wsh prints is parsed, never run ----
    $swapDir = Join-Path $root 'swapbin'
    Make-Dir $swapDir
    $swapSrc = Join-Path $root 'swapstub.cs'
    Make-File $swapSrc @'
using System;
using System.IO;
using System.Threading;
class P {
    static int Main(string[] a) {
        string argsFile = Environment.GetEnvironmentVariable("SWAP_ARGS");
        string self = System.Diagnostics.Process.GetCurrentProcess().MainModule.FileName;
        if (File.Exists(Path.Combine(Path.GetDirectoryName(self), "decoy.marker"))) argsFile = Environment.GetEnvironmentVariable("RESUMABLE_TEST_DECOYARGS");
        if (argsFile != null) File.WriteAllText(argsFile, string.Join(" ", a));
        if (Environment.GetEnvironmentVariable("SWAP_HANG") == "1") Thread.Sleep(20000);
        if (Environment.GetEnvironmentVariable("SWAP_ECHO_ERR") == "1") { Console.Error.WriteLine("wsh says: " + string.Join(" ", a)); return 1; }
        if (Environment.GetEnvironmentVariable("SWAP_FAIL") == "1") return 1;
        string o = Environment.GetEnvironmentVariable("SWAP_OUTPUT");
        if (Environment.GetEnvironmentVariable("SWAP_FLOOD") == "1") { for (int i = 0; i < 400; i++) Console.Out.WriteLine("$env:EVIL = \"" + new string('x', 1000) + "\""); Console.Out.WriteLine("$env:WAVETERM_BLOCKID = \"block-flood\""); return 0; }
        if (o != null) Console.Out.Write(File.ReadAllText(o));
        if (Environment.GetEnvironmentVariable("SWAP_CHILD_PIPE") == "1") { Console.Out.Flush(); var si = new System.Diagnostics.ProcessStartInfo("ping", "-n 15 127.0.0.1"); si.UseShellExecute = false; System.Diagnostics.Process.Start(si); return 0; }
        if (Environment.GetEnvironmentVariable("SWAP_FAIL_AFTER") == "1") return 1;
        return 0;
    }
}
'@
    $swapExe = Join-Path $swapDir 'wsh.exe'
    & powershell.exe -NoProfile -Command "Add-Type -TypeDefinition (Get-Content -LiteralPath '$swapSrc' -Raw) -OutputAssembly '$swapExe' -OutputType ConsoleApplication" 2>$null | Out-Null
    $made.Add($swapExe)
    Check 'the stub wsh.exe was built (Windows PowerShell compiles it)' (Test-Path -LiteralPath $swapExe) 'no wsh.exe'
    $swapOut = Join-Path $root 'swap-output.txt'
    $swapArgs = Join-Path $root 'swap-args.txt'
    $bt = [string][char]96
    $swapLines = @(
        ('$env:WAVETERM_JWT = "jwt' + $bt + '"quoted' + $bt + '$value' + $bt + $bt + 'end"'),
        '$env:WAVETERM_BLOCKID = "block-1"',
        '$env:WAVETERM_TABID = "tab-1"',
        '$env:WAVETERM_CLIENTID = "client-1"',
        '$env:EVIL_VAR = "x"',
        '$env:PATH = "C:\evil"',
        '$env:path = "C:\evil2"',
        '$env:WAVETERM_WORKSPACEID = "ws-1" ; $env:EVIL_VAR = "y"',
        ('$env:WAVETERM_VERSION = "' + $bt + 'nnewline"'),
        ('$env:WAVETERM_CONN = "' + $bt + 'x"'),
        '$env:WAVETERM = $(Remove-Item C:\important)',
        'Remove-Item C:\important',
        'calc.exe',
        '& calc.exe',
        '$env:TERM_PROGRAM = "waveterm"'
    )
    Make-File $swapOut ([string]::Join([string][char]10, $swapLines))
    $env:SWAP_OUTPUT = $swapOut; $env:SWAP_ARGS = $swapArgs; $env:RESUMABLE_TEST_NOWSH = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    $jwt = ($r.Lines | Where-Object { $_ -like 'ENV: WAVETERM_JWT=*' }) -replace '^ENV: WAVETERM_JWT=', ''
    Check 'the token is exchanged before claude starts: the variables on the list reach the program' (($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-1') -and ($r.Lines -contains 'ENV: WAVETERM_TABID=tab-1') -and ($r.Lines -contains 'ENV: WAVETERM_CLIENTID=client-1') -and ($r.Lines -contains 'ENV: TERM_PROGRAM=waveterm')) $r.Text
    Check 'a value with an escaped quote, dollar sign and backtick arrives as Wave meant it' ($jwt -eq ('jwt"quoted$value' + $bt + 'end')) "got [$jwt]"
    Check 'a name that is not on the list, a PATH assignment (either case), a line with more after the value, a subexpression and plain commands have no effect' (-not ($r.Text -match 'EVIL_VAR') -and -not ($r.Text -match 'WAVETERM_WORKSPACEID') -and -not ($r.Text -match 'WAVETERM=') -and ($r.Lines -contains 'ENV: TERM_PROGRAM=waveterm') -and ($r.Lines | Where-Object { $_ -like 'PATHLAST:*' }) -notcontains 'PATHLAST: C:\evil') $r.Text
    Check 'a value with an escaped newline or another backtick sequence is refused' (-not ($r.Text -match 'WAVETERM_VERSION') -and -not ($r.Text -match 'WAVETERM_CONN')) $r.Text
    Check 'the swap token is gone from the environment of the program and never printed' (-not ($r.Text -match 'WAVETERM_SWAPTOKEN') -and -not $r.Text.Contains($packedToken) -and -not $r.Text.Contains($uuid)) $r.Text
    Check 'wsh was called once with exactly: token <the token> pwsh' ((Get-Content -LiteralPath $swapArgs -Raw).Trim() -eq ('token ' + $packedToken + ' pwsh')) ((Get-Content -LiteralPath $swapArgs -Raw))
    Check 'an injected command line did not run (the file it names is still there)' (Test-Path -LiteralPath $swapOut) ''
    # a token that is not a plain token never reaches wsh (a space, a quote, a leading dash, too short, too long, a semicolon)
    foreach ($badToken in @(@('tok 123 extra', 'tok"123', "tok'123", ('-' + $packedToken), ($packedToken + ';calc'), ($packedToken + [string][char]10), ([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('[{"token":"x","rpccontext":{"sockname":"s"}}]'))), ([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('{"token":"x","rpccontext":{"sockname":"s"},"pad":"' + ('p' * 7000) + '"}'))), 'short', ('a' * 80), ('a' * 8200), [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('hello this is not json at all')), [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('{"token":"x","rpccontext":{}}')), '11111111-aaaa-bbbb-cccc-222222222222') + @(foreach ($j in @(
            '{"token":"x","rpccontext":{"sockname":"s","routeid":""}}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"s"}}',
            '{"token":"11111111-AAAA-bbbb-cccc-222222222222","rpccontext":{"sockname":"s","routeid":""}}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"s","routeid":"","procroute":"yes"}}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"s","routeid":0}}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"s","routeid":"","evil":"x"}}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"s","routeid":""},"extra":1}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":"s"}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"s","routeid":"","BLOCKID":"x"}}',
            '{"token":"11111111-aaaa-bbbb-cccc-222222222222","rpccontext":{"sockname":"s","routeid":"","ProcRoute":true}}'
        )) { [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($j)) }))) {
        Remove-Item -LiteralPath $swapArgs -ErrorAction SilentlyContinue
        $env:RESUMABLE_TEST_SWAPTOKEN = $badToken
        $r = Run-Child 'swap' $cfg '' '' $swapDir
        Check ("a token '" + $badToken.Substring(0, [Math]::Min(12, $badToken.Length)) + "' is no token: wsh is not called, the session starts, one plain line says so") ((-not (Test-Path -LiteralPath $swapArgs)) -and ($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and ($r.Text -match 'could not be exchanged') -and -not ($r.Text -match 'WAVETERM_BLOCKID')) "exit=$($r.Code) args=$(Test-Path -LiteralPath $swapArgs) $($r.Text)"
        Check "the same token is logged as swap=token-shape and the Note carries the code" (((Get-Content -LiteralPath $swapLog -Tail 1) -match " swap=token-shape$") -and ($r.Text -match "\(swap=token-shape\)")) (Get-Content -LiteralPath $swapLog -Tail 1)
    }
    Remove-Item Env:RESUMABLE_TEST_SWAPTOKEN -ErrorAction SilentlyContinue
    # ---- the real format: a token packed by Wave's own code (tools/ours/packswaptoken, TokenSwapEntry.PackForClient), and the real wsh ----
    $go = if ($env:RESUMABLE_TEST_GO) { $env:RESUMABLE_TEST_GO } else { (Get-Command go -ErrorAction SilentlyContinue).Source }
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    $script:realFormatRan = $false
    if (-not $go) {
        "SKIP  the real-format checks: no go (set RESUMABLE_TEST_GO to go.exe)"
    } else {
        $script:realFormatRan = $true
        $fakeSock = Join-Path $root 'no-such-wave.sock'
        Push-Location $repoRoot
        try {
            $realToken = (& $go run ./tools/ours/packswaptoken $fakeSock 2>$null | Out-String).Trim()
            $realWsh = Join-Path $root 'realwsh\wsh.exe'
            & $go build -o $realWsh ./cmd/wsh 2>$null | Out-Null
            $made.Add($realWsh)
        } finally { Pop-Location }
        Check 'a token packed by Wave itself was made' ($realToken.Length -gt 40) "[$realToken]"
        Remove-Item -LiteralPath $swapArgs -ErrorAction SilentlyContinue
        $env:RESUMABLE_TEST_SWAPTOKEN = $realToken
        $r = Run-Child 'swap' $cfg '' '' $swapDir
        Check 'a token packed by Wave itself passes the shape check and reaches wsh' ((Test-Path -LiteralPath $swapArgs) -and ((Get-Content -LiteralPath $swapArgs -Raw).Trim() -eq ('token ' + $realToken + ' pwsh'))) "args=$(Test-Path -LiteralPath $swapArgs) $(Get-Content -LiteralPath $swapLog -Tail 1)"
        Remove-Item Env:RESUMABLE_TEST_SWAPTOKEN -ErrorAction SilentlyContinue
        Check 'the real wsh was built' (Test-Path -LiteralPath $realWsh) 'no real wsh'
        if (Test-Path -LiteralPath $realWsh) {
            $errOut = Join-Path $root 'realwsh-err.txt'
            $p = Start-Process -FilePath $realWsh -ArgumentList @('token', $realToken, 'pwsh') -NoNewWindow -Wait -PassThru -RedirectStandardError $errOut -RedirectStandardOutput (Join-Path $root 'realwsh-out.txt')
            $made.Add($errOut); $made.Add((Join-Path $root 'realwsh-out.txt'))
            $wshErr = Get-Content -LiteralPath $errOut -Raw
            Check 'the real wsh unpacks the token and fails only on the missing socket (no server here)' (($p.ExitCode -ne 0) -and -not ($wshErr -match 'unpacking token|no rpccontext|no sockname') -and ($wshErr -match 'domain socket')) "exit=$($p.ExitCode) $wshErr"
            # through the starter with the real wsh: the call is made and its failure is a closed code; the success path needs a running Wave
            $realDir = Split-Path -Parent $realWsh
            $env:RESUMABLE_TEST_SWAPTOKEN = $realToken
            $r = Run-Child 'swap' $cfg '' '' $realDir
            Remove-Item Env:RESUMABLE_TEST_SWAPTOKEN -ErrorAction SilentlyContinue
            Check 'the starter with the real wsh and no server: swap=exit-code:1, the session starts, nothing is set' (((Get-Content -LiteralPath $swapLog -Tail 1) -match ' swap=exit-code:1$') -and ($r.Code -eq 0) -and -not ($r.Text -match 'ENV: WAVETERM_JWT')) "$(Get-Content -LiteralPath $swapLog -Tail 1) $($r.Text)"
        }
    }
    # wsh prints good lines and then exits with an error: nothing is taken
    $env:SWAP_FAIL_AFTER = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a wsh that exits with an error is not believed, even if it printed assignments' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Text -match 'WAVETERM_BLOCKID') -and ($r.Text -match 'could not be exchanged')) "exit=$($r.Code) $($r.Text)"
    Remove-Item Env:SWAP_FAIL_AFTER -ErrorAction SilentlyContinue
    # a value with a control character (tab, bell) is refused
    $ctl = Join-Path $root 'swap-control.txt'
    Make-File $ctl ([string]::Join([string][char]10, @(('$env:WAVETERM_CLIENTID = "a' + [char]9 + 'b"'), ('$env:WAVETERM_TABID = "x' + [char]7 + 'y"'), '$env:WAVETERM_BLOCKID = "block-ok"', '$env:WAVETERM_JWT = "jwt-ok"')))
    $env:SWAP_OUTPUT = $ctl
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a value with a control character is refused, the line next to it is still taken' (-not ($r.Text -match 'WAVETERM_CLIENTID') -and -not ($r.Text -match 'WAVETERM_TABID') -and ($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-ok')) $r.Text
    # no WAVETERM_JWT among the lines taken: wsh access is not there, so it is no success and nothing is set
    foreach ($case in @(
            @('only TERM_PROGRAM', @('$env:TERM_PROGRAM = "waveterm"')),
            @('a block id and no JWT', @('$env:WAVETERM_BLOCKID = "block-nojwt"', '$env:TERM_PROGRAM = "waveterm"')),
            @('a JWT that was refused', @(('$env:WAVETERM_JWT = "a' + [char]9 + 'b"'), '$env:WAVETERM_BLOCKID = "block-nojwt"'))
        )) {
        $noJwt = Join-Path $root 'swap-nojwt.txt'
        Make-File $noJwt ([string]::Join([string][char]10, $case[1]))
        $env:SWAP_OUTPUT = $noJwt
        $r = Run-Child 'swap' $cfg '' '' $swapDir
        Check ("wsh output with " + $case[0] + " is swap=no-access: nothing is set and the session starts") (((Get-Content -LiteralPath $swapLog -Tail 1) -match ' swap=no-access$') -and ($r.Text -match '\(swap=no-access\)') -and -not ($r.Text -match 'block-nojwt') -and -not ($r.Text -match 'ENV: TERM_PROGRAM') -and ($r.Code -eq 0) -and $r.Lines.Count -gt 0) "$(Get-Content -LiteralPath $swapLog -Tail 1) $($r.Text)"
    }
    $env:SWAP_OUTPUT = $swapOut
    $env:SWAP_FAIL = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a failing wsh does not stop the session, the token is removed, nothing is set, one plain line says so' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Text -match 'WAVETERM_SWAPTOKEN') -and -not ($r.Text -match 'WAVETERM_BLOCKID') -and ($r.Text -match 'could not be exchanged') -and -not $r.Text.Contains($packedToken)) "exit=$($r.Code) $($r.Text)"
    Remove-Item Env:SWAP_FAIL -ErrorAction SilentlyContinue
    $env:SWAP_HANG = '1'; $env:RESUMABLE_TEST_SWAPTIMEOUT = '1'
    $t0 = Get-Date
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    $took = ((Get-Date) - $t0).TotalSeconds
    Check 'a wsh that hangs is stopped after the time limit and the session starts without wsh access' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and $took -lt 15 -and -not ($r.Text -match 'WAVETERM_BLOCKID') -and ($r.Text -match 'could not be exchanged')) "exit=$($r.Code) took=$took $($r.Text)"
    Remove-Item Env:SWAP_HANG, Env:RESUMABLE_TEST_SWAPTIMEOUT -ErrorAction SilentlyContinue
    $r = Run-Child 'swap-nowsh' $cfg '' '' $swapDir
    Check 'a token and no wsh folder does not stop the session, and the token is removed' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Text -match 'WAVETERM_SWAPTOKEN') -and ($r.Text -match 'could not be exchanged')) "exit=$($r.Code) $($r.Text)"
    $r = Run-Child 'swap-nodir' $cfg '' '' $swapDir
    Check 'a token and a wsh folder without a wsh.exe does not stop the session' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Text -match 'WAVETERM_SWAPTOKEN')) "exit=$($r.Code) $($r.Text)"
    $uuidOfToken = '11111111-aaaa-bbbb-cccc-222222222222'
    $ast = [System.Management.Automation.Language.Parser]::ParseFile($script, [ref]$null, [ref]$null)
    $timeoutParam = $ast.ParamBlock.Parameters | Where-Object { $_.Name.VariablePath.UserPath -eq 'SwapTimeoutSeconds' }
    Check 'the default time limit of the exchange is 15 seconds' ($timeoutParam.DefaultValue.Extent.Text -eq '15') $timeoutParam.DefaultValue.Extent.Text
    # ---- the outcome is one code from a closed list: on the Note line and in the log, never wsh's words or any token ----
    function Last-Log { if (Test-Path -LiteralPath $swapLog) { return [string](Get-Content -LiteralPath $swapLog -Tail 1) } else { return '' } }
    $r = Run-Child 'swap' $cfg '' 'logname' $swapDir
    Check 'a good exchange logs swap=ok with the NAMES of the variables set and no value' (((Last-Log) -match ' logname swap=ok set=') -and ((Last-Log) -match 'WAVETERM_BLOCKID') -and -not ((Last-Log) -cmatch 'block-1|jwt|quoted')) (Last-Log)
    Check 'a log line starts with a date and time' ((Last-Log) -match '^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} logname ') (Last-Log)
    $before = @(Get-Content -LiteralPath $swapLog).Count
    $r = Run-Child 'set'
    Check 'no swap token at all writes no log line and prints no Note' ((@(Get-Content -LiteralPath $swapLog).Count -eq $before) -and -not ($r.Text -match 'could not be exchanged')) "$(Last-Log) $($r.Text)"
    $r = Run-Child 'swap-nowsh' $cfg '' '' $swapDir
    Check 'a token and no wsh folder is swap=wsh-folder' (((Last-Log) -match ' swap=wsh-folder$') -and ($r.Text -match '\(swap=wsh-folder\)')) "$(Last-Log) $($r.Text)"
    $r = Run-Child 'swap-nodir' $cfg '' '' $swapDir
    Check 'a wsh folder without a wsh.exe is swap=wsh-missing' (((Last-Log) -match ' swap=wsh-missing$') -and ($r.Text -match '\(swap=wsh-missing\)')) "$(Last-Log) $($r.Text)"
    $badBin = Join-Path $root 'badbin'
    Make-Dir $badBin
    Make-File (Join-Path $badBin 'wsh.exe') 'this is not a program'
    $r = Run-Child 'swap' $cfg '' '' $badBin
    Check 'a wsh.exe that cannot be started is swap=start-failed and the session starts' (((Last-Log) -match ' swap=start-failed$') -and ($r.Code -eq 0) -and $r.Lines.Count -gt 0) "exit=$($r.Code) $(Last-Log) $($r.Text)"
    $env:SWAP_FAIL = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a wsh that exits with 1 is swap=exit-code:1' (((Last-Log) -match ' swap=exit-code:1$') -and ($r.Text -match '\(swap=exit-code:1\)')) "$(Last-Log) $($r.Text)"
    Remove-Item Env:SWAP_FAIL -ErrorAction SilentlyContinue
    $env:SWAP_HANG = '1'; $env:RESUMABLE_TEST_SWAPTIMEOUT = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a wsh that hangs is swap=timeout' (((Last-Log) -match ' swap=timeout$') -and ($r.Text -match '\(swap=timeout\)')) "$(Last-Log) $($r.Text)"
    Remove-Item Env:SWAP_HANG, Env:RESUMABLE_TEST_SWAPTIMEOUT -ErrorAction SilentlyContinue
    $env:SWAP_FLOOD = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'an output above the limit is swap=too-large' (((Last-Log) -match ' swap=too-large$') -and ($r.Text -match '\(swap=too-large\)')) "$(Last-Log) $($r.Text.Substring(0, [Math]::Min(300, $r.Text.Length)))"
    Remove-Item Env:SWAP_FLOOD -ErrorAction SilentlyContinue
    $noLines = Join-Path $root 'swap-nolines.txt'
    Make-File $noLines "just some text`n`$env:EVIL = `"x`""
    $env:SWAP_OUTPUT = $noLines
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'an output with no usable line is swap=no-lines' (((Last-Log) -match ' swap=no-lines$') -and ($r.Text -match '\(swap=no-lines\)') -and -not ($r.Text -match 'WAVETERM_BLOCKID')) "$(Last-Log) $($r.Text)"
    $env:SWAP_OUTPUT = $swapOut
    # a wsh that echoes its arguments (the token) on stderr: the words are not in the log and not on the screen
    $env:SWAP_ECHO_ERR = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    $logAll = Get-Content -LiteralPath $swapLog -Raw
    Check 'a wsh that echoes the token on stderr leaves no token and none of its words in the log or on the screen' (((Last-Log) -match ' swap=exit-code:1$') -and -not $logAll.Contains($packedToken) -and -not $r.Text.Contains($packedToken) -and -not $logAll.Contains('wsh says') -and -not $r.Text.Contains('wsh says') -and -not $logAll.Contains($uuidOfToken)) "$(Last-Log) $($r.Text)"
    Remove-Item Env:SWAP_ECHO_ERR -ErrorAction SilentlyContinue
    Check 'the Note says that restarting the block gives a new token' ($r.Text -match 'Restarting the block gives a new token') $r.Text
    # the log is capped at 200 lines by rewriting
    Make-File $swapLog ([string]::Join([string][char]10, @(1..250 | ForEach-Object { "old line $_" })))
    $r = Run-Child 'swap' $cfg '' 'capname' $swapDir
    $logLines = @(Get-Content -LiteralPath $swapLog)
    Check 'the log keeps 200 lines at most, the newest last, the oldest dropped' (($logLines.Count -eq 200) -and ($logLines[-1] -match ' capname swap=ok') -and ($logLines[0] -eq 'old line 52')) "count=$($logLines.Count) first=$($logLines[0]) last=$($logLines[-1])"
    $r = Run-Child 'swap' $cfg '' 'a b;c"d' $swapDir
    Check 'a session name is cleaned to letters, digits, dot, dash and underscore in the log' ((Last-Log) -match ' a_b_c_d swap=ok') (Last-Log)
    # the log path is judged like every other path: under .claude it is not written
    $env:RESUMABLE_TEST_LOG = Join-Path $oldDir 'logs\starter.log'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a log path below .claude is refused: nothing is written there, the session still starts' ((-not (Test-Path -LiteralPath (Join-Path $oldDir 'logs'))) -and ($r.Code -eq 0) -and $r.Lines.Count -gt 0) "exit=$($r.Code)"
    $env:RESUMABLE_TEST_LOG = $swapLog
    # the log's FINAL folder is judged: a junction under the allowed root that leads into .claude gets no log and no new folder; a folder outside the root gets none
    $logJunc = Join-Path $root 'log-to-old'
    $null = New-Item -ItemType Junction -Path $logJunc -Target $oldDir; $made.Add($logJunc)
    foreach ($lc in @(@('a junction into .claude (existing folder)', ($logJunc + '\x.log'), (Join-Path $oldDir 'x.log')), @('a junction into .claude (new folder below it)', ($logJunc + '\newdir\x.log'), (Join-Path $oldDir 'newdir')), @('a folder outside the log root', (Join-Path $env:TEMP ('log-outside-' + [guid]::NewGuid().ToString('N') + '\x.log')), $null))) {
        $env:RESUMABLE_TEST_LOG = $lc[1]
        $r = Run-Child 'swap' $cfg '' '' $swapDir
        $made_there = if ($lc[2]) { Test-Path -LiteralPath $lc[2] } else { Test-Path -LiteralPath (Split-Path -Parent $lc[1]) }
        Check ("a log path in " + $lc[0] + " is refused: nothing is created or written, the session starts") ((-not $made_there) -and ($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and ($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-1')) "exit=$($r.Code) created=$made_there"
    }
    # a log that cannot be written never stops the start: a folder where the file should be, a read-only file
    $logAsDir = Join-Path $logDir 'dir-as-log'
    Make-Dir $logAsDir
    $env:RESUMABLE_TEST_LOG = $logAsDir
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a folder in place of the log file does not stop the start' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and ($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-1')) "exit=$($r.Code) $($r.Text)"
    $roLog = Join-Path $logDir 'readonly.log'
    Make-File $roLog 'kept'
    Set-ItemProperty -LiteralPath $roLog -Name IsReadOnly -Value $true
    $env:RESUMABLE_TEST_LOG = $roLog
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    $roKept = ((Get-Content -LiteralPath $roLog -Raw).Trim() -eq 'kept')
    Set-ItemProperty -LiteralPath $roLog -Name IsReadOnly -Value $false
    Check 'a read-only log file does not stop the start and is left as it was' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and $roKept -and ($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-1')) "exit=$($r.Code) kept=$roKept $($r.Text)"
    $env:RESUMABLE_TEST_LOG = $swapLog
    # an existing log that is a HARD link to another file is never written into: the other name keeps its content, the log name gets a new file
    $decoy = Join-Path $profileDir 'decoy-target.txt'
    Make-File $decoy 'decoy content'
    $hardLog = Join-Path $logDir 'hardlinked.log'
    $null = New-Item -ItemType HardLink -Path $hardLog -Target $decoy; $made.Add($hardLog)
    $env:RESUMABLE_TEST_LOG = $hardLog
    $r = Run-Child 'swap' $cfg '' 'hardname' $swapDir
    Check 'a log that is a hard link to another file leaves that file unchanged' ((((Get-Content -LiteralPath $decoy -Raw).Trim()) -eq 'decoy content') -and ($r.Code -eq 0) -and $r.Lines.Count -gt 0) "decoy=[$((Get-Content -LiteralPath $decoy -Raw).Trim())] exit=$($r.Code)"
    Check 'the log name itself got the new line (a new file took its place)' ((Get-Content -LiteralPath $hardLog -Raw) -match ' hardname swap=ok') (Get-Content -LiteralPath $hardLog -Raw)
    Check 'no temporary file is left beside the log' (@(Get-ChildItem -LiteralPath $logDir -Filter '.starter-log-*.tmp' -Force).Count -eq 0) ''
    # -LogRoot at a fake .claude: nothing is written or created there (the plain gate comes before the final path of the root is asked)
    $env:RESUMABLE_TEST_LOG = (Join-Path $logDir 'rooted-elsewhere.log')
    $env:RESUMABLE_TEST_LOGROOT = $oldDir
    $oldCount = @(Get-ChildItem -LiteralPath $oldDir -Force).Count
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a -LogRoot at .claude gives no log, nothing is created there, the session starts' ((-not (Test-Path -LiteralPath $env:RESUMABLE_TEST_LOG)) -and (@(Get-ChildItem -LiteralPath $oldDir -Force).Count -eq $oldCount) -and ($r.Code -eq 0) -and ($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-1')) "exit=$($r.Code)"
    $env:RESUMABLE_TEST_LOGROOT = $root
    $env:RESUMABLE_TEST_LOG = $swapLog
    # the order is pinned in the text: the plain gate is asked about -LogRoot and -WshRoot before their final path is (a file system call)
    $ast2 = [System.Management.Automation.Language.Parser]::ParseFile($script, [ref]$null, [ref]$null)
    foreach ($pair in @(@('Write-SwapLog', 'LogRoot'), @('Resolve-WshFolder', 'WshRoot'))) {
        $fn = $ast2.Find({ param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $pair[0] }, $true)
        $text = $fn.Body.Extent.Text
        $gate = $text.IndexOf('Test-PlainlyForbidden $' + $pair[1])
        $final = $text.IndexOf('Get-FinalPath $' + $pair[1])
        Check ("in " + $pair[0] + " the plain gate for -" + $pair[1] + " comes before Get-FinalPath of it") (($gate -ge 0) -and ($final -gt $gate)) "gate=$gate final=$final"
    }
$env:RESUMABLE_TEST_LOGROOT = $root
    # ---- round 2 of the review of the swap: where wsh is run from, one deadline, no inherited access, sizes ----
    # a folder under .claude, one outside the allowed root, a junction under the root that leads to .claude: wsh.exe in them is never run
    $evilBin = Join-Path $oldDir 'bin'
    Make-Dir $evilBin
    Copy-Item -LiteralPath $swapExe -Destination (Join-Path $evilBin 'wsh.exe'); $made.Add((Join-Path $evilBin 'wsh.exe'))
    $outsideBin = Join-Path $env:TEMP ('swap-outside-' + [guid]::NewGuid().ToString('N'))
    Make-Dir $outsideBin
    Copy-Item -LiteralPath $swapExe -Destination (Join-Path $outsideBin 'wsh.exe'); $made.Add((Join-Path $outsideBin 'wsh.exe'))
    $juncBin = Join-Path $root 'bin-link'
    $null = New-Item -ItemType Junction -Path $juncBin -Target $evilBin; $made.Add($juncBin)
    foreach ($case in @(@('under .claude', $evilBin), @('outside the allowed root', $outsideBin), @('a junction under the root that leads into .claude', $juncBin))) {
        Remove-Item -LiteralPath $swapArgs -ErrorAction SilentlyContinue
        $r = Run-Child 'swap' $cfg '' '' $case[1]
        Check ("wsh.exe in a folder " + $case[0] + " is never run (the stub's argument file is not written), the PATH does not get it, the session starts") ((-not (Test-Path -LiteralPath $swapArgs)) -and ($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Lines -contains ('PATHLAST: ' + $case[1])) -and ($r.Text -match 'could not be exchanged')) "exit=$($r.Code) $($r.Text)"
    }
    # the named folder's wsh.exe runs, not one that comes earlier on the PATH
    $decoyDir = Join-Path $root 'decoybin'
    Make-Dir $decoyDir
    Copy-Item -LiteralPath $swapExe -Destination (Join-Path $decoyDir 'wsh.exe'); $made.Add((Join-Path $decoyDir 'wsh.exe'))
    Make-File (Join-Path $decoyDir 'decoy.marker') 'x'
    $decoyArgs = Join-Path $root 'decoy-args.txt'
    $env:RESUMABLE_TEST_DECOYDIR = $decoyDir; $env:RESUMABLE_TEST_DECOYARGS = $decoyArgs
    Remove-Item -LiteralPath $swapArgs -ErrorAction SilentlyContinue
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check "the wsh.exe of the named folder is the one that runs, not a decoy earlier on the PATH" ((Test-Path -LiteralPath $swapArgs) -and -not (Test-Path -LiteralPath $decoyArgs)) "named ran=$(Test-Path -LiteralPath $swapArgs) decoy ran=$(Test-Path -LiteralPath $decoyArgs)"
    Remove-Item Env:RESUMABLE_TEST_DECOYDIR, Env:RESUMABLE_TEST_DECOYARGS -ErrorAction SilentlyContinue
    # one deadline for the read and the exit: a wsh whose child keeps the pipe open, and one that floods
    $env:RESUMABLE_TEST_SWAPTIMEOUT = '1'
    $env:SWAP_CHILD_PIPE = '1'
    $t0 = Get-Date
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    $took = ((Get-Date) - $t0).TotalSeconds
    Check 'a wsh that exits but leaves a child holding the pipe is cut at the deadline, the session starts without wsh access' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and $took -lt 9 -and -not ($r.Text -match 'WAVETERM_BLOCKID') -and ($r.Text -match 'could not be exchanged')) "exit=$($r.Code) took=$took $($r.Text)"
    Remove-Item Env:SWAP_CHILD_PIPE -ErrorAction SilentlyContinue
    $env:SWAP_FLOOD = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'an output above 256 KB is refused as a whole (nothing from it is taken)' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Text -match 'WAVETERM_BLOCKID') -and ($r.Text -match 'could not be exchanged')) "exit=$($r.Code) $($r.Text.Substring(0, [Math]::Min(300, $r.Text.Length)))"
    Remove-Item Env:SWAP_FLOOD -ErrorAction SilentlyContinue
    $starterSrc = Get-Content -LiteralPath $script -Raw
    $fnStart = $starterSrc.IndexOf('function Resolve-WshFolder')
    $fnBody = $starterSrc.Substring($fnStart, $starterSrc.IndexOf('# ---- the rest', $fnStart) - $fnStart)
    Check 'the folder of wsh is refused by its plain spelling BEFORE any file system call (order pinned in the source: the equivalent final-path result cannot show it)' (($fnBody.IndexOf('Test-PlainlyForbidden') -ge 0) -and ($fnBody.IndexOf('Test-PlainlyForbidden') -lt $fnBody.IndexOf('Test-Path')) -and ($fnBody.IndexOf('Test-Path') -lt $fnBody.IndexOf('Get-FinalPath'))) ''
    Remove-Item Env:RESUMABLE_TEST_SWAPTIMEOUT -ErrorAction SilentlyContinue
    # a line above 16 KB is ignored, the lines next to it are taken
    $bigFile = Join-Path $root 'swap-big.txt'
    Make-File $bigFile ([string]::Join([string][char]10, @(('$env:WAVETERM_CLIENTID = "' + ('x' * 20000) + '"'), '$env:WAVETERM_BLOCKID = "block-big"', '$env:WAVETERM_JWT = "jwt-big"')))
    $env:SWAP_OUTPUT = $bigFile
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'a line with a value above 16 KB is ignored, the next line is still taken' (-not ($r.Text -match 'WAVETERM_CLIENTID') -and ($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-big')) $r.Text.Substring(0, [Math]::Min(300, $r.Text.Length))
    $env:SWAP_OUTPUT = $swapOut
    # inherited access: removed when an exchange was tried and failed, replaced on success, left alone when there was no token at all
    $env:RESUMABLE_TEST_INHERIT = '1'
    $env:SWAP_FAIL = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'after a failed exchange the inherited WAVETERM_JWT, BLOCKID, TABID and WORKSPACEID are removed' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Text -match 'inherited-') -and -not ($r.Text -match 'WAVETERM_JWT')) $r.Text
    Remove-Item Env:SWAP_FAIL -ErrorAction SilentlyContinue
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'after a successful exchange the values are the ones wsh gave, not the inherited ones' (($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-1') -and -not ($r.Text -match 'inherited-block')) $r.Text
    $r = Run-Child 'set'
    Check 'with no swap token at all an inherited variable is left alone (a shell already exchanged it)' ($r.Lines -contains 'ENV: WAVETERM_BLOCKID=inherited-block') $r.Text
    Remove-Item Env:RESUMABLE_TEST_INHERIT -ErrorAction SilentlyContinue

    $r = Run-Child 'set'
    Check 'without a swap token wsh is not called and nothing is added' (-not ($r.Text -match 'WAVETERM_JWT') -and -not ($r.Text -match 'could not be exchanged')) $r.Text
    Remove-Item Env:SWAP_OUTPUT, Env:SWAP_ARGS, Env:RESUMABLE_TEST_NOWSH -ErrorAction SilentlyContinue
    $starterText = Get-Content -LiteralPath $script -Raw
    Check 'the token goes to wsh as an argument list element, never joined into an argument string (a second layer behind the shape check)' (($starterText -match 'ArgumentList.Add') -and ($starterText -notmatch '$psi.Argumentss*=')) ''
    $stray = & pwsh -NoProfile -File $script -SessionId $id -ConfigDir $cfg -DryRun stray-positional-piece 2>&1
    Check 'a stray positional argument is an error, not a model or an effort' (($LASTEXITCODE -ne 0) -and ($stray -join ' ') -match 'positional') ($stray -join ' ')

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
    # a name that wsh did not print does not keep what was inherited, after a success
    $env:RESUMABLE_TEST_INHERIT = '1'
    $env:SWAP_OUTPUT = $swapOut; $env:SWAP_ARGS = $swapArgs; $env:RESUMABLE_TEST_NOWSH = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'after a success a listed name that wsh did not print is cleared, not left inherited' (($r.Lines -contains 'ENV: WAVETERM_BLOCKID=block-1') -and -not ($r.Text -match 'inherited-ws') -and -not ($r.Text -match 'inherited-')) $r.Text
    Remove-Item Env:RESUMABLE_TEST_INHERIT -ErrorAction SilentlyContinue
    # LOCALAPPDATA unset and no -WshRoot: no exchange, the session starts (the default of the parameter must not end the starter)
    $env:RESUMABLE_TEST_NOROOT = '1'; $env:RESUMABLE_TEST_NOLOCALAPPDATA = '1'
    $r = Run-Child 'swap' $cfg '' '' $swapDir
    Check 'with LOCALAPPDATA unset and no -WshRoot there is no exchange and the session still starts' (($r.Code -eq 0) -and $r.Lines.Count -gt 0 -and -not ($r.Text -match 'WAVETERM_BLOCKID') -and ($r.Text -match 'could not be exchanged')) "exit=$($r.Code) $($r.Text)"
    Remove-Item Env:RESUMABLE_TEST_NOROOT, Env:RESUMABLE_TEST_NOLOCALAPPDATA, Env:SWAP_OUTPUT, Env:SWAP_ARGS, Env:RESUMABLE_TEST_NOWSH -ErrorAction SilentlyContinue

    # ---- the prompt ----
    foreach ($lead in '-x', ' -x', "`t-x", "  `t -x") {
        $rd = Run-Child 'set' $cfg ($lead + ' starts with a dash')
        Check ("a prompt that starts with '" + $lead.Replace("`t", '<tab>') + "' is refused with exit 2 and nothing is started") (($rd.Code -eq 2) -and $rd.Lines.Count -eq 0) "exit=$($rd.Code) $($rd.Text)"
    }
    $r = Run-Child 'set' $cfg '-x starts with a dash'
    Check 'a prompt that starts with a dash is refused with exit 2 and nothing is started' (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    $quoted = 'say "hi" and a\"b c'
    $first = Join-Path $cfg 'projects\some-folder'
    Remove-Item -LiteralPath $asFolder; $made.Remove($asFolder) | Out-Null
    $r = Run-Child 'set' $cfg $quoted
    Check 'a prompt with quotes and a backslash-quote arrives as ONE argument' (($r.Lines -contains 'ARGCOUNT: 7') -or ($r.Text -match 'ARGCOUNT: \d+' -and ($r.Lines | Where-Object { $_ -like 'ARGS:*' }) -like "*$quoted") ) $r.Text
    Make-File $asFolder '{}'
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $child -Script $script -Stub $stub -Record $rec -WshDir $wshDir -PathMode set -Cfg $cfg -Id $id 2>$null | Out-Null
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
        $r = Run-Child 'set' $cfg '' '' '' $pf -runId ([guid]::NewGuid().ToString())
        $argLine = ($r.Lines | Where-Object { $_ -like 'ARGS:*' }) -join "`n"
        $count = ($r.Lines | Where-Object { $_ -like 'ARGCOUNT:*' })
        if ($null -eq $baseCount) { $baseCount = $count }
        Check ("a prompt file with '" + $text.Substring(0, [Math]::Min(18, $text.Length)).Replace("`n", '<nl>') + "' arrives as one argument, byte for byte") (($r.Code -eq 0) -and $argLine.EndsWith($text) -and ($count -eq $baseCount)) "exit=$($r.Code) count=$count base=$baseCount $($r.Text)"
        Check 'the prompt file is still there afterwards (the starter deletes nothing)' (Test-Path -LiteralPath $pf) ''
    }
    $crlf = Write-PromptBytes 'crlf.txt' ([Text.UTF8Encoding]::new($true).GetPreamble() + [Text.UTF8Encoding]::new($false).GetBytes("with bom and crlf`r`n"))
    $r = Run-Child 'set' $cfg '' '' '' $crlf -runId ([guid]::NewGuid().ToString())
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
        $r = Run-Child 'set' $cfg '' '' '' $bad[$k]
        Check ("a prompt file that is " + $k + " is refused with exit 2 and nothing is started") (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    }
    $claudePrompt = Join-Path $oldDir 'prompt.txt'
    Make-File $claudePrompt 'below .claude'
    $r = Run-Child 'set' $cfg '' '' '' $claudePrompt
    Check 'a prompt file below .claude is refused with exit 2 and nothing is started' (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    $promptJunc = Join-Path $promptRoot 'to-old'
    $null = New-Item -ItemType Junction -Path $promptJunc -Target $oldDir; $made.Add($promptJunc)
    $r = Run-Child 'set' $cfg '' '' '' (Join-Path $promptJunc 'prompt.txt')
    Check 'a prompt file reached through a junction into .claude is refused after the link is resolved' (($r.Code -eq 2) -and $r.Lines.Count -eq 0) "exit=$($r.Code) $($r.Text)"
    $r = Run-Child 'set' $cfg 'also a prompt' '' '' $okFile
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
    $r = Run-Child 'unset' $acct2
    Check 'account 2 copy: a start copies CLAUDE.md and settings.json from account 1 and the session starts' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and (Text-Of (Join-Path $acct2 'CLAUDE.md')) -eq 'rules one' -and (Text-Of (Join-Path $acct2 'settings.json')) -eq '{"hooks":"one"}') $r.Text
    $made.Add((Join-Path $acct2 'CLAUDE.md')); $made.Add((Join-Path $acct2 'settings.json'))
    Set-Content -LiteralPath (Join-Path $personal 'CLAUDE.md') -Value 'rules two'
    Set-Content -LiteralPath (Join-Path $personal 'settings.json') -Value '{"hooks":"two"}'
    $r = Run-Child 'unset' $acct2
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
    $beforeA1 = @(Get-Content -LiteralPath $swapLog).Count
    $null = Run-Child 'unset' $cfg
    $a1Lines = @(Get-Content -LiteralPath $swapLog)
    Check 'account 2 copy: a start on account 1 logs no account2copy line' ($a1Lines.Count -eq $beforeA1 -or -not ($a1Lines[-1] -match 'account2copy')) ($a1Lines[-1])
    $below = Join-Path $acct2 'cfg'; Make-Dir $below
    $r = Run-Child 'unset' $below
    Check 'account 2 copy: a folder below the account 2 folder is not the account folder and copies nothing' ($r.Code -eq 0 -and -not (Test-Path -LiteralPath (Join-Path $below 'CLAUDE.md')) -and -not (Test-Path -LiteralPath (Join-Path $below 'settings.json'))) $r.Text
    $savedSettings = Join-Path $personal 'settings.json'
    Remove-Item -LiteralPath $savedSettings
    Set-Content -LiteralPath (Join-Path $personal 'CLAUDE.md') -Value 'rules three'
    $r = Run-Child 'unset' $acct2
    Check 'account 2 copy: a missing source is skipped with a warning, the other file is copied, the session starts' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and $r.Text -match 'settings.json was not copied' -and (Text-Of (Join-Path $acct2 'CLAUDE.md')) -eq 'rules three' -and (Text-Of (Join-Path $acct2 'settings.json')) -eq '{"hooks":"two"}') $r.Text
    Make-Dir $savedSettings
    $r = Run-Child 'unset' $acct2
    Check 'account 2 copy: a folder in place of a source file is skipped, the session starts' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and $r.Text -match 'settings.json was not copied') $r.Text
    [IO.Directory]::Delete($savedSettings); $made.Remove($savedSettings) | Out-Null
    Make-File $savedSettings '{"hooks":"four"}'
    # a destination that is a link is never written through: its target stays as it was, the other file is still copied, the session starts
    $decoy2 = Join-Path $root 'a2-decoy'; Make-Dir $decoy2
    $dstLink = Join-Path $acct2 'settings.json'
    if (Test-Path -LiteralPath $dstLink) { Remove-Item -LiteralPath $dstLink }
    $null = New-Item -ItemType Junction -Path $dstLink -Target $decoy2; $made.Add($dstLink)
    $r = Run-Child 'unset' $acct2
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
    $r = Run-Child 'unset' $cfg
    $af = @(Acct-Files); $row = if ($af.Count) { Get-Content -LiteralPath $af[0].FullName -Raw | ConvertFrom-Json -DateKind String } else { $null }
    Check 'account record: a start on account 1 writes ONE file named <session>-<yyyyMMddHHmmssfff>-1-<32 hex>.json holding at, session, account and nothing else' ($af.Count -eq 1 -and $af[0].Name -match ('^' + [regex]::Escape($id) + '-\d{17}-1-[0-9a-f]{32}\.json$') -and $row.session -eq $id -and $row.account -eq 1 -and $row.at -match '^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$' -and (@($row.PSObject.Properties.Name) -join ',') -eq 'at,session,account') "files=$(($af | ForEach-Object { $_.Name }) -join ', ') $($r.Text)"
    Check 'account record: the session itself started (the stub ran, exit 0) and no temporary file is left' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and @(Get-ChildItem -LiteralPath $acctDir -Filter '.session-account-*.tmp' -Force).Count -eq 0) "exit=$($r.Code) $($r.Text)"
    Start-Sleep -Milliseconds 30
    $r = Run-Child 'unset' $cfg2
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
    $r = Run-Child 'unset' $cfg
    Check 'account record: a file in place of the folder does not stop the start and is left as it was' ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and ((Get-Content -LiteralPath $acctAsFile -Raw).Trim() -eq 'a file, not a folder')) "exit=$($r.Code) $($r.Text)"
    $env:RESUMABLE_TEST_ACCOUNTLOG = Join-Path $oldDir 'logs\session-accounts'
    $r = Run-Child 'unset' $cfg
    Check 'account record: a folder below .claude is refused (nothing created there), the session starts' ((-not (Test-Path -LiteralPath (Join-Path $oldDir 'logs'))) -and $r.Code -eq 0 -and $r.Lines.Count -gt 0) "exit=$($r.Code)"
    $outsideDir = Join-Path $env:TEMP ('acct-outside-' + [guid]::NewGuid().ToString('N') + '\session-accounts')
    $env:RESUMABLE_TEST_ACCOUNTLOG = $outsideDir
    $r = Run-Child 'unset' $cfg
    Check 'account record: a folder outside the log root is refused, nothing is created, the session starts' ((-not (Test-Path -LiteralPath (Split-Path -Parent $outsideDir))) -and $r.Code -eq 0 -and $r.Lines.Count -gt 0) "exit=$($r.Code)"
    # a link in place of the folder: its target is never written
    $decoy = Join-Path $profileDir 'acct-decoy-target'; Make-Dir $decoy
    $linkDir = Join-Path $logDir 'acct-link'
    $null = New-Item -ItemType Junction -Path $linkDir -Target $decoy; $made.Add($linkDir)
    $env:RESUMABLE_TEST_ACCOUNTLOG = $linkDir
    $r = Run-Child 'unset' $cfg
    Check 'account record: a folder that is a link is not written into: the target stays empty, the session starts' (@(Get-ChildItem -LiteralPath $decoy -Force).Count -eq 0 -and $r.Code -eq 0 -and $r.Lines.Count -gt 0) "target has $(@(Get-ChildItem -LiteralPath $decoy -Force).Count) entries, exit=$($r.Code)"
    $oldLinkDir = Join-Path $logDir 'acct-link-into-old'
    $null = New-Item -ItemType Junction -Path $oldLinkDir -Target $oldDir; $made.Add($oldLinkDir)
    $env:RESUMABLE_TEST_ACCOUNTLOG = Join-Path $oldLinkDir 'session-accounts'
    $r = Run-Child 'unset' $cfg
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
    $r = Run-Child 'unset' $cfg
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
        $r = Run-Child 'unset' $cfg
        $noted = @(Get-Content -LiteralPath $swapLog | Where-Object { $_ -match ' account-record=' })
        Check ("account record: " + $nc[0] + " is refused, the refusal is noted in starter.log as account-record=" + $nc[2] + " (no path in the line), and the start runs") ($r.Code -eq 0 -and $r.Lines.Count -gt 0 -and $noted.Count -eq 1 -and $noted[0] -match (' account-record=' + [regex]::Escape($nc[2]) + '$')) "exit=$($r.Code) noted=$($noted -join ' | ') $($r.Text)"
    }
    $env:RESUMABLE_TEST_ACCOUNTLOG = $acctDir
    Make-File $swapLog ''
    $r = Run-Child 'unset' $cfg
    Check 'account record: a recorded start leaves no account-record line in starter.log' ($r.Code -eq 0 -and @(Get-Content -LiteralPath $swapLog | Where-Object { $_ -match 'account-record=' }).Count -eq 0) "exit=$($r.Code)"
    # the name carries the FULL 32-hex guid as its suffix
    $fullNames = @(Acct-Files | ForEach-Object { $_.Name })
    Check 'account record: the name ends with a full 32-hex guid (<session>-<at>-<account>-<32 hex>.json)' ($fullNames.Count -ge 1 -and @($fullNames | Where-Object { $_ -notmatch '-\d{17}-[12]-[0-9a-f]{32}\.json$' }).Count -eq 0) ($fullNames -join ', ')
    $env:RESUMABLE_TEST_ACCOUNTLOG = $acctDir} finally {
    $env:USERPROFILE = $realProfile
    $env:LOCALAPPDATA = $realLocal
    Remove-Item Env:RESUMABLE_TEST_LOG, Env:RESUMABLE_TEST_LOGROOT, Env:RESUMABLE_TEST_PACKED -ErrorAction SilentlyContinue
    Remove-Item Env:RESUMABLE_TEST_OTHERDIR -ErrorAction SilentlyContinue
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
# without go the real packer and the real wsh did not run: the run is not called green then, whatever the count
if ($fail -eq 0 -and -not $script:realFormatRan) { "PARTIAL: the real packer and the real wsh were not run (set RESUMABLE_TEST_GO to go.exe)" } elseif ($fail -eq 0) { "GREEN: all checks ran" }
exit $(if ($fail -gt 0) { 1 } else { 0 })
