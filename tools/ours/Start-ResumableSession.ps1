# Runs INSIDE a Wave terminal block that was made with `wsh session start --restart --meta session:id=<guid> ...`.
# Starts claude with a clean environment (the launching session's CLAUDE* variables, NO_COLOR and the like are removed), plus one decision that a plain launcher does not
# make: with a fixed session id, the FIRST start creates the session under that id (--session-id) and every later start
# (Wave was restarted) resumes it (--resume), so the same conversation comes back in the same block.
# -DryRun prints the claude arguments instead of starting it (used by the self test).
# Runs under PowerShell 7 (pwsh): a prompt with quotes or backslashes reaches claude as one argument only there. -DryRun also runs under Windows PowerShell.
[CmdletBinding(PositionalBinding = $false)]
param(
    [Parameter(Mandatory)][string]$SessionId,
    [string]$Name,
    [string]$Model,
    [string]$Effort,
    [string]$PermissionMode,
    [switch]$SkipPermissions,
    [string]$Prompt,
    [string]$ConfigDir = $env:CLAUDE_CONFIG_DIR,
    [switch]$ThrowawayConfig,
    [string[]]$PluginDir = @(),
    [string]$Command = 'claude',
    [ValidateRange(1, 30)][int]$SwapTimeoutSeconds = 5,
    [switch]$DryRun
)
$ErrorActionPreference = 'Stop'

# ---- the config folder rule -------------------------------------------------------------------------------------------------------------------------------
# There is NO default config folder. A missing CLAUDE_CONFIG_DIR would make Claude fall back to %USERPROFILE%\.claude, which belongs to the work side of this
# machine and must never be read or written by a personal session. The folder must be fully qualified (drive and backslash or slash: no relative path, no
# "C:name", no UNC or \\?\ form) and is resolved to its FINAL path (junctions, symbolic links, 8.3 names, "..") before it is judged, with and without
# -ThrowawayConfig. Refused: anything at or below <profile>\.claude or <profile>\.claude-work (compared case-insensitively). Allowed: at or below
# <profile>\.claude-personal, or any other folder when -ThrowawayConfig is given (a test). Refusal is exit code 2, before anything starts.
# The folder handed on to Claude is the resolved full path.
if (-not ('ResumableConfigGuard.Win' -as [type])) {
    Add-Type -Namespace ResumableConfigGuard -Name Win -MemberDefinition @'
[DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
public static extern Microsoft.Win32.SafeHandles.SafeFileHandle CreateFile(string name, uint access, uint share, IntPtr security, uint disposition, uint flags, IntPtr template);
[DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
public static extern uint GetFinalPathNameByHandle(Microsoft.Win32.SafeHandles.SafeFileHandle handle, System.Text.StringBuilder path, uint length, uint flags);
'@
}

function Get-FinalExistingFolder([string]$full) {
    $handle = [ResumableConfigGuard.Win]::CreateFile($full, 0, 7, [IntPtr]::Zero, 3, 0x02000000, [IntPtr]::Zero)
    if ($handle.IsInvalid) { return $null }
    try {
        $sb = New-Object System.Text.StringBuilder 2048
        $n = [ResumableConfigGuard.Win]::GetFinalPathNameByHandle($handle, $sb, 2048, 0)
        if ($n -eq 0 -or $n -gt 2048) { return $null }
        $final = $sb.ToString()
        if ($final.StartsWith('\\?\UNC\')) { return '\\' + $final.Substring(8) }
        if ($final.StartsWith('\\?\')) { return $final.Substring(4) }
        return $final
    } finally { $handle.Dispose() }
}

# The final path of a folder that may not exist yet: the deepest existing ancestor is resolved, the rest is appended.
function Get-FinalPath([string]$path) {
    $full = [IO.Path]::GetFullPath($path).TrimEnd('\', '/')
    $rest = @()
    $cur = $full
    while ($cur) {
        if (Test-Path -LiteralPath $cur -PathType Container) {
            $final = Get-FinalExistingFolder $cur
            if (-not $final) { return $null }
            $final = $final.TrimEnd('\', '/')
            if ($rest.Count -gt 0) { [array]::Reverse($rest); $final = $final + '\' + ($rest -join '\') }
            return $final
        }
        $parent = [IO.Path]::GetDirectoryName($cur)
        if (-not $parent -or $parent -eq $cur) { return $null }
        $rest += [IO.Path]::GetFileName($cur)
        $cur = $parent
    }
    return $null
}

function Test-Under([string]$path, [string]$root) {
    return ($path -ieq $root) -or $path.StartsWith($root + '\', [StringComparison]::OrdinalIgnoreCase)
}

# The plain-spelling gate: after "..", slashes and capitals are read as text (no file system call, nothing is opened), a path at or below
# <profile>\.claude or <profile>\.claude-work is refused at once. Only what passes this gate is looked at on disk (the final path resolution below).
function Test-PlainlyForbidden([string]$path) {
    if ([string]::IsNullOrWhiteSpace($env:USERPROFILE) -or $env:USERPROFILE -notmatch '^[A-Za-z]:[\\/]') { return $false }
    $textFull = [IO.Path]::GetFullPath($path).TrimEnd('\', '/')
    $textHome = [IO.Path]::GetFullPath($env:USERPROFILE).TrimEnd('\', '/')
    foreach ($bad in '.claude', '.claude-work') { if (Test-Under $textFull "$textHome\$bad") { return $true } }
    return $false
}
$script:ConfigRefusedByPlainSpelling = $false

# Returns the resolved full path of the folder when it may be used, otherwise $null.
function Resolve-AllowedConfigDir([string]$dir, [bool]$throwaway) {
    if ([string]::IsNullOrWhiteSpace($dir)) { return $null }
    if ($dir -notmatch '^[A-Za-z]:[\\/]') { return $null }
    if ([string]::IsNullOrWhiteSpace($env:USERPROFILE) -or $env:USERPROFILE -notmatch '^[A-Za-z]:[\\/]') { return $null }
    if (Test-PlainlyForbidden $dir) { $script:ConfigRefusedByPlainSpelling = $true; return $null }
    $final = Get-FinalPath $dir
    $homeFinal = Get-FinalPath $env:USERPROFILE
    if (-not $final -or -not $homeFinal) { return $null }
    foreach ($bad in '.claude', '.claude-work') { if (Test-Under $final "$homeFinal\$bad") { return $null } }
    if (Test-Under $final "$homeFinal\.claude-personal") { return $final }
    if ($throwaway) { return $final }
    return $null
}

$resolvedConfig = Resolve-AllowedConfigDir $ConfigDir $ThrowawayConfig.IsPresent
if (-not $resolvedConfig -and $script:ConfigRefusedByPlainSpelling) {
    [Console]::Error.WriteLine("Refusing to start Claude: config folder '$ConfigDir' is at or below .claude or .claude-work by its plain spelling (nothing was opened).")
    exit 2
}
if (-not $resolvedConfig) {
    [Console]::Error.WriteLine("Refusing to start Claude: config folder '$ConfigDir' is empty, not fully qualified, at or below .claude or .claude-work (after links are resolved), or not under .claude-personal. Set CLAUDE_CONFIG_DIR.")
    exit 2
}
$ConfigDir = $resolvedConfig

# Plugin folders (--plugin-dir): each must be fully qualified, an existing folder, and (after links are resolved) never at or below .claude or .claude-work.
# What is handed on is the resolved path. Refusal is exit code 2, before anything starts.
$pluginPaths = @()
foreach ($pd in @($PluginDir | Where-Object { $_ })) {
    $okPlugin = $null
    if ($pd -match '^[A-Za-z]:[\\/]' -and (Test-PlainlyForbidden $pd)) {
        [Console]::Error.WriteLine("Refusing to start Claude: plugin folder '$pd' is at or below .claude or .claude-work by its plain spelling (nothing was opened).")
        exit 2
    }
    if ($pd -match '^[A-Za-z]:[\\/]' -and (Test-Path -LiteralPath $pd -PathType Container)) {
        $pf = Get-FinalPath $pd
        $hf = Get-FinalPath $env:USERPROFILE
        if ($pf -and $hf -and -not (Test-Under $pf "$hf\.claude") -and -not (Test-Under $pf "$hf\.claude-work")) { $okPlugin = $pf }
    }
    if (-not $okPlugin) {
        [Console]::Error.WriteLine("Refusing to start Claude: plugin folder '$pd' is not a fully qualified existing folder, or is at or below .claude or .claude-work.")
        exit 2
    }
    $pluginPaths += $okPlugin
}
# The key of the status page for this start: <name>-<4 hex>, new at every start (a restart gives a new one). Set after the environment is cleaned.
$statusSession = $null
if ($Name) { $statusSession = $Name + '-' + [guid]::NewGuid().ToString('N').Substring(0, 4) }

# ---- the rest -----------------------------------------------------------------------------------------------------------------------------------------------
if ($SessionId -notmatch '^[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}\z') { throw "SessionId must be a guid: $SessionId" }
# A prompt that starts with a dash would be read by claude as an option, whatever it was meant to say.
if ($Prompt -and $Prompt.TrimStart().StartsWith('-')) {
    [Console]::Error.WriteLine('Refusing to start Claude: the prompt starts with a dash and would be read as an option. Start it with a word.')
    exit 2
}
if (-not $DryRun -and $PSVersionTable.PSVersion.Major -lt 7) {
    [Console]::Error.WriteLine('Refusing to start Claude under Windows PowerShell: a prompt with quotes or backslashes does not arrive as one argument there. Run this with pwsh (PowerShell 7).')
    exit 2
}

# A transcript is a FILE named <id>.jsonl: a folder of that name does not count.
function Test-SessionExists([string]$root, [string]$id) {
    $projects = Join-Path $root 'projects'
    if (-not (Test-Path -LiteralPath $projects -PathType Container)) { return $false }
    return [bool](Get-ChildItem -LiteralPath $projects -Directory -ErrorAction SilentlyContinue |
        Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName "$id.jsonl") -PathType Leaf } | Select-Object -First 1)
}

function Get-ClaudeArgs {
    $a = @()
    foreach ($pp in $pluginPaths) { $a += '--plugin-dir', $pp }
    if ($SkipPermissions) { $a += '--dangerously-skip-permissions' } elseif ($PermissionMode) { $a += '--permission-mode', $PermissionMode }
    $exists = Test-SessionExists $ConfigDir $SessionId
    if ($exists) { $a += '--resume', $SessionId } else { $a += '--session-id', $SessionId }
    if ($Name) { $a += '--remote-control', $Name }
    if ($Model) { $a += '--model', $Model }
    if ($Effort) { $a += '--effort', $Effort }
    if ($Prompt -and -not $exists) { $a += $Prompt }
    return $a
}

if ($DryRun) { (Get-ClaudeArgs) -join ' '; "config=$ConfigDir"; "status=$statusSession tab=$Name"; return }

Get-ChildItem env: |
    Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -in 'STATUS_SESSION', 'STATUS_TAB', 'NO_COLOR', 'FORCE_COLOR', 'GIT_EDITOR',
        'GIT_ASKPASS', 'GIT_TERMINAL_PROMPT', 'NoDefaultCurrentDirectoryInExePath' } |
    ForEach-Object { Remove-Item "env:$($_.Name)" -ErrorAction SilentlyContinue }
# The folder that was searched for the transcript is the folder Claude must use, or the next start would look elsewhere.
$env:CLAUDE_CONFIG_DIR = $ConfigDir
# The status page's hook writes nothing unless STATUS_SESSION is set; a key inherited from the session that opened this block would be the wrong one.
if ($Name) { $env:STATUS_SESSION = $statusSession; $env:STATUS_TAB = $Name }
if ($Name) { try { $Host.UI.RawUI.WindowTitle = $Name } catch { } }

# Wave puts the folder of wsh on the PATH of the shells it starts (wavepwsh.ps1), not of a program that it starts directly, as this one is. A Claude session
# in a tile that cannot run wsh cannot use the rest of Wave, so the folder Wave names (WAVETERM_WSHBINDIR) is put on the PATH here, once. It goes AFTER the
# entries that are already there (a tool of the same name that is already there wins), only when it is a fully qualified path to an existing folder, and is not
# added when the same folder is already there under another spelling (compared as full paths, case-insensitive, trailing separators ignored). This step can
# never stop the session: any error leaves the PATH as it was.
function Get-FullDir([string]$d) { try { return [IO.Path]::GetFullPath($d).TrimEnd('\', '/') } catch { return $null } }
try {
    $wshDir = $env:WAVETERM_WSHBINDIR
    if ($wshDir -and $wshDir -match '^[A-Za-z]:[\\/]' -and (Test-Path -LiteralPath $wshDir -PathType Container)) {
        $wshFull = Get-FullDir $wshDir
        $current = [string]$env:PATH
        $entries = @($current -split [IO.Path]::PathSeparator | Where-Object { $_ })
        if ($wshFull -and -not ($entries | Where-Object { (Get-FullDir $_) -ieq $wshFull })) {
            $env:PATH = $current.TrimEnd([IO.Path]::PathSeparator) + [IO.Path]::PathSeparator + $wshFull
        }
    }
} catch { }
# Wave starts this program directly, so it gets WAVETERM_SWAPTOKEN (one use, valid 5 minutes after the block started) and none of the variables that wsh
# needs (WAVETERM_JWT, BLOCKID, TABID, WORKSPACEID ...). The shells that Wave starts exchange it themselves (wavepwsh.ps1); a program started directly never
# does, and a session that was resumed after a restart sits idle past the 5 minutes. So it is exchanged HERE, before claude starts, and claude and every tool
# shell below it inherit the variables.
#  - wsh is the wsh.exe in the folder Wave names (WAVETERM_WSHBINDIR, an existing fully qualified folder), run ONCE with "token <token> pwsh" and a time limit.
#  - What it prints is PARSED, never run: only lines of the exact form  $env:NAME = "value"  with NAME on the fixed list below are taken; every other line is
#    ignored (a command, a subexpression, an assignment to PATH or to another name, a line with more after the value). Inside the value Wave escapes a double
#    quote, a backtick and a dollar sign with a backtick; that is all that is undone. A value that holds a newline, a control character or any other
#    backtick sequence is refused.
#  - The token never reaches the screen or an error message. It is on the command line of the wsh call while that runs (a few milliseconds; another process of
#    the same user can list it then, as it can read this process's environment all along); it is single use, and it is removed from this process's environment
#    afterwards, so claude and its shells never see a spent token.
#  - A failed, slow or refused exchange never stops the session: it starts without wsh access and one plain line says so.
$SwapVariables = @('WAVETERM_JWT', 'WAVETERM_BLOCKID', 'WAVETERM_TABID', 'WAVETERM_WORKSPACEID', 'WAVETERM_CLIENTID', 'WAVETERM_CONN', 'WAVETERM_VERSION', 'WAVETERM', 'TERM_PROGRAM')

function ConvertFrom-WshAssignment([string]$line) {
    $m = [regex]::Match($line, '^\$env:([A-Z][A-Z0-9_]*) = "((?:[^"`$]|`["`$`])*)"$')
    if (-not $m.Success -or $SwapVariables -cnotcontains $m.Groups[1].Value) { return $null }
    $value = $m.Groups[2].Value.Replace('``', [string][char]1).Replace('`"', '"').Replace('`$', '$').Replace([string][char]1, '`')
    if ($value -match '[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]') { return $null }
    return @{ Name = $m.Groups[1].Value; Value = $value }
}

function Invoke-WaveTokenSwap([int]$timeoutSeconds) {
    $token = $env:WAVETERM_SWAPTOKEN
    if (-not $token) { return $true }
    # Wave mints a UUID (36 characters: hex digits and dashes). Anything that is not a plain token (a space, a quote, a leading dash, a very long text) is no
    # token: wsh is not called, so a token can never become extra arguments of the call.
    if ($token -cnotmatch '^[A-Za-z0-9_][A-Za-z0-9_-]{7,63}$') { return $false }
    $binDir = $env:WAVETERM_WSHBINDIR
    if (-not $binDir -or $binDir -notmatch '^[A-Za-z]:[\\/]' -or -not (Test-Path -LiteralPath $binDir -PathType Container)) { return $false }
    $wsh = Join-Path $binDir 'wsh.exe'
    if (-not (Test-Path -LiteralPath $wsh -PathType Leaf)) { return $false }
    $proc = $null
    try {
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $wsh
        foreach ($a in 'token', $token, 'pwsh') { $psi.ArgumentList.Add($a) }
        $psi.UseShellExecute = $false
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.CreateNoWindow = $true
        $proc = [System.Diagnostics.Process]::Start($psi)
        $outTask = $proc.StandardOutput.ReadToEndAsync()
        $null = $proc.StandardError.ReadToEndAsync()
        if (-not $proc.WaitForExit($timeoutSeconds * 1000)) {
            try { $proc.Kill() } catch { }
            return $false
        }
        if ($proc.ExitCode -ne 0) { return $false }
        $taken = 0
        foreach ($line in ($outTask.Result -split "`r?`n")) {
            $parsed = ConvertFrom-WshAssignment $line
            if ($parsed) { Set-Item -Path "env:$($parsed.Name)" -Value $parsed.Value; $taken++ }
        }
        return ($taken -gt 0)
    } catch {
        return $false
    } finally {
        if ($proc) { $proc.Dispose() }
    }
}

if ($env:WAVETERM_SWAPTOKEN) {
    $swapped = $false
    try { $swapped = Invoke-WaveTokenSwap $SwapTimeoutSeconds } catch { }
    Remove-Item Env:WAVETERM_SWAPTOKEN -ErrorAction SilentlyContinue
    if (-not $swapped) { [Console]::Error.WriteLine('Note: the Wave token could not be exchanged, so this session starts without wsh access (restart the block to get it).') }
}
$claudeArgs = @(Get-ClaudeArgs)
& $Command @claudeArgs
