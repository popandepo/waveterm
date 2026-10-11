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
    $other = & $script -SessionId '99999999-2222-3333-4444-555555555555' -ConfigDir $cfg -DryRun
    Check 'another id is not mistaken for it' ($other[0] -match '--session-id 99999999') $other[0]

    # ---- how it is run: a child pwsh, so that this shell keeps its environment ----
    $stub = Join-Path $root 'stub.ps1'
    $rec = Join-Path $root 'record.txt'
    Make-File $stub @'
param()
$lines = @("ARGS: " + ($args -join ' '))
Get-ChildItem env: | Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -in 'NO_COLOR', 'FORCE_COLOR' } | ForEach-Object { $lines += "ENV: $($_.Name)=$($_.Value)" }
$lines += 'PATHLAST: ' + (@($env:PATH -split ';' | Where-Object { $_ })[-1])
$lines += 'PATHCOUNT: ' + (@($env:PATH -split ';' | Where-Object { $_ -ieq $env:RESUMABLE_TEST_WSHDIR }).Count)
$found = Get-Command wsh -ErrorAction SilentlyContinue
$lines += 'WSH: ' + $(if ($found) { $found.Source } else { 'none' })
$lines += 'WSHOUT: ' + $(if ($found) { (& wsh 2>&1 | Out-String).Trim() } else { 'none' })
$lines += 'ARGCOUNT: ' + $args.Count
Set-Content -LiteralPath $env:RESUMABLE_TEST_RECORD $lines
'@
    $child = Join-Path $root 'child.ps1'
    Make-File $child @'
param($Script, $Stub, $Record, $WshDir, $PathMode, $Cfg, $Id, [string]$ExtraPrompt)
$env:CLAUDE_CONFIG_DIR = 'X:\somewhere\else'; $env:CLAUDE_CODE_CHILD_SESSION = '1'; $env:CLAUDECODE = '1'; $env:NO_COLOR = '1'
$env:RESUMABLE_TEST_RECORD = $Record
$env:RESUMABLE_TEST_WSHDIR = $WshDir
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
    'unset' { Remove-Item Env:WAVETERM_WSHBINDIR -ErrorAction SilentlyContinue }
}
$extra = @{}
if ($ExtraPrompt) { $extra.Prompt = $ExtraPrompt }
& $Script -SessionId $Id -ConfigDir $Cfg -Command $Stub -Model m1 @extra
exit $LASTEXITCODE
'@
    $wshDir = Join-Path $root 'wshbin'
    $otherDir = Join-Path $root 'otherbin'
    Make-Dir $wshDir; Make-Dir $otherDir
    Make-File (Join-Path $wshDir 'wsh.cmd') '@echo wsh-from-wave'
    Make-File (Join-Path $otherDir 'wsh.cmd') '@echo wsh-already-there'
    $env:RESUMABLE_TEST_OTHERDIR = $otherDir
    function Run-Child([string]$mode, [string]$cfgDir = $cfg, [string]$prompt = '') {
        Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
        $a = @('-NoProfile', '-File', $child, '-Script', $script, '-Stub', $stub, '-Record', $rec, '-WshDir', $wshDir, '-PathMode', $mode, '-Cfg', $cfgDir, '-Id', $id)
        if ($prompt) { $a += '-ExtraPrompt', $prompt }
        & pwsh @a 2>$null | Out-Null
        $code = $LASTEXITCODE
        $lines = @(); if (Test-Path -LiteralPath $rec) { $lines = @(Get-Content -LiteralPath $rec) }
        return [pscustomobject]@{ Code = $code; Lines = $lines; Text = ($lines -join ' | ') }
    }
    $shellBefore = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
    $r = Run-Child 'set'
    $shellAfter = @('CLAUDE_CONFIG_DIR', 'CLAUDE_CODE_CHILD_SESSION', 'CLAUDECODE', 'NO_COLOR') | ForEach-Object { "$_=" + [Environment]::GetEnvironmentVariable($_) }
    Check 'this shell is left as it was (the launch ran in a child process)' (($shellBefore -join '|') -eq ($shellAfter -join '|')) ''
    Check 'the launched program gets the searched config folder, not the inherited one' ($r.Lines -contains "ENV: CLAUDE_CONFIG_DIR=$cfg") $r.Text
    Check 'the inherited child-session markers are gone' (-not ($r.Text -match 'ENV: (CLAUDE_CODE_CHILD_SESSION|CLAUDECODE)=')) $r.Text
    Check 'NO_COLOR is gone' (-not ($r.Text -match 'ENV: NO_COLOR=')) $r.Text
    Check 'the launched program gets the resume arguments' ($r.Text -match "ARGS: --resume $id") $r.Text
    Check 'the folder of wsh is put AFTER the existing PATH entries' ($r.Lines -contains "PATHLAST: $wshDir") $r.Text
    Check 'wsh can be run from there and the stub prints what wsh printed' ($r.Lines -contains 'WSHOUT: wsh-from-wave') $r.Text
    foreach ($mode in 'missing', 'unset', 'relative', 'rooted-no-drive', 'drive-relative', 'file') {
        $r = Run-Child $mode
        Check "with WAVETERM_WSHBINDIR '$mode' the session starts and the PATH is left alone" (($r.Code -eq 0) -and ($r.Lines -contains 'PATHCOUNT: 0') -and -not ($r.Lines -contains "PATHLAST: $wshDir") -and $r.Lines.Count -gt 0) $r.Text
    }
    foreach ($mode in 'illegal', 'path-unset') {
        $r = Run-Child $mode
        Check "with WAVETERM_WSHBINDIR '$mode' the session still starts" (($r.Code -eq 0) -and ($r.Lines.Count -gt 0)) "exit=$($r.Code) $($r.Text)"
    }
    $r = Run-Child 'already'
    Check 'a folder that is already on the PATH is not added again' ($r.Lines -contains 'PATHCOUNT: 1') $r.Text
    $r = Run-Child 'spelling'
    Check 'the same folder under another spelling (case, dot dot, trailing slash) is not added again' (-not ($r.Lines -contains "PATHLAST: $wshDir")) $r.Text
    $r = Run-Child 'other'
    Check 'a wsh that is already on the PATH wins over the one Wave names' (($r.Lines -contains 'WSHOUT: wsh-already-there') -and ($r.Lines -contains "PATHLAST: $wshDir")) $r.Text

    # ---- the prompt ----
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

    # ---- the session id ----
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & pwsh -NoProfile -File $script -SessionId ($id + "`n") -ConfigDir $cfg -Command $stub 2>$null | Out-Null
    Check 'a session id with a trailing newline is refused' (($LASTEXITCODE -ne 0) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE"
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
    foreach ($c in $cases) {
        Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
        $a = @('-NoProfile', '-File', $script, '-SessionId', $id, '-Command', $stub)
        if ($null -ne $c[1]) { $a += '-ConfigDir', $c[1] }
        if ($c[2]) { $a += '-ThrowawayConfig' }
        $env:RESUMABLE_TEST_RECORD = $rec
        Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
        & pwsh @a 2>$null | Out-Null
        Check ("config folder '" + $c[0] + "' is refused with exit code 2 and nothing is started") (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE record=$(Test-Path -LiteralPath $rec)"
    }
    $env:CLAUDE_CONFIG_DIR = $oldDir
    Remove-Item -LiteralPath $rec -ErrorAction SilentlyContinue
    & pwsh -NoProfile -File $script -SessionId $id -Command $stub 2>$null | Out-Null
    Check 'a CLAUDE_CONFIG_DIR of .claude in the environment is refused with exit code 2' (($LASTEXITCODE -eq 2) -and -not (Test-Path -LiteralPath $rec)) "exit=$LASTEXITCODE"
    Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue

    # ---- what is accepted ----
    $dry = & $script -SessionId $id -ConfigDir $cfg -DryRun
    Check 'a folder under .claude-personal is accepted without the throwaway switch' ($dry[1] -eq "config=$cfg") ($dry -join ' | ')
    $dry = & $script -SessionId $id -ConfigDir $throw -ThrowawayConfig -DryRun
    Check 'a throwaway folder is accepted with the switch' ($dry[1] -eq "config=$throw") ($dry -join ' | ')
    $dry = & $script -SessionId $id -ConfigDir $junc.Replace('to-old', 'cfg') -DryRun
    Check 'a folder name inside .claude-personal that does not exist yet is accepted' ($dry[1] -like '*\cfg') ($dry -join ' | ')
    $dry = & $script -SessionId $id -ConfigDir ($personal + '\cfg\..\cfg') -DryRun
    Check 'the folder handed on is the resolved full path' ($dry[1] -eq "config=$cfg") ($dry -join ' | ')
} finally {
    $env:USERPROFILE = $realProfile
    Remove-Item Env:RESUMABLE_TEST_OTHERDIR -ErrorAction SilentlyContinue
    Remove-Item Env:RESUMABLE_TEST_RECORD -ErrorAction SilentlyContinue
    Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $rec -Force -Confirm:$false -ErrorAction SilentlyContinue
    for ($i = $made.Count - 1; $i -ge 0; $i--) {
        $p = $made[$i]
        try {
            $item = Get-Item -LiteralPath $p -Force -ErrorAction Stop
            if ($item.LinkType -eq 'Junction') { [IO.Directory]::Delete($p) } else { Remove-Item -LiteralPath $p -Force -Confirm:$false -ErrorAction Stop }
        } catch { }
    }
}
"failures: $fail"
exit $(if ($fail -gt 0) { 1 } else { 0 })
