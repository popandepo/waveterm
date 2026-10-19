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
    [string]$PromptFile,
    [string]$PromptRoot = $(if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'WaveFork\prompts' } else { '' }),
    [string]$ConfigDir = $env:CLAUDE_CONFIG_DIR,
    [switch]$ThrowawayConfig,
    [string[]]$PluginDir = @(),
    [string]$Command = 'claude',
    [ValidateRange(1, 30)][int]$SwapTimeoutSeconds = 15,
    [string]$WshRoot = $(if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'WaveFork\profile\data\bin' } else { '' }),
    [string]$LogPath = $(if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'WaveFork\logs\starter.log' } else { '' }),
    [string]$LogRoot = $(if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'WaveFork' } else { '' }),
    [string]$AccountDir = $(if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'WaveFork\logs\session-accounts' } else { '' }),
    [string]$AccountAt = '',   # test seam: a fixed UTC time for the record (an unparsable value is ignored)
    [string[]]$TrustRoot = @('C:\Projects\Personal'),
    [ValidateRange(5, 300)][int]$TrustWaitSeconds = 90,
    [string[]]$TrustExclude = @('C:\Projects\Personal\_wt', 'C:\Projects\Personal\_archive'),
    [string]$TrustExcludeFile = 'C:\Projects\Personal\MalfestioMaster\tools\external-repos.json',
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
    # A "~" anywhere in the text could be an 8.3 short name: GetFullPath asks the file system for the long name of every part of such a path, so a path below
    # .claude would be looked at by name inside that folder before it is refused. Such a path is refused on its text alone, before GetFullPath.
    if ($path.Contains('~')) { return $true }
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
    # account 1 and account 2 (R-20261014-008): exactly these two folders and what is below them
    foreach ($ok in '.claude-personal', '.claude-personal2') { if (Test-Under $final "$homeFinal\$ok") { return $final } }
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
# The key of the status page for this start: <name>-<8 hex>, new at every start (a restart gives a new one). Set after the environment is cleaned.
$statusSession = $null
if ($Name) { $statusSession = $Name + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8) }

# The folder that wsh.exe is run from (and put on the PATH): fully qualified, not plainly at or below .claude or .claude-work (checked before anything is
# opened), existing, and with its final path at or below $WshRoot and not at or below .claude or .claude-work. Returns the final path, or $null.
function Resolve-WshFolder([string]$dir) {
    if ([string]::IsNullOrWhiteSpace($dir) -or $dir -notmatch '^[A-Za-z]:[\\/]') { return $null }
    if (Test-PlainlyForbidden $dir) { return $null }
    if (-not (Test-Path -LiteralPath $dir -PathType Container)) { return $null }
    $final = Get-FinalPath $dir
    $homeFinal = Get-FinalPath $env:USERPROFILE
    $rootFinal = if ($WshRoot -match '^[A-Za-z]:[\\/]' -and -not (Test-PlainlyForbidden $WshRoot)) { Get-FinalPath $WshRoot } else { $null }
    if (-not $final -or -not $homeFinal -or -not $rootFinal) { return $null }
    foreach ($bad in '.claude', '.claude-work') { if (Test-Under $final "$homeFinal\$bad") { return $null } }
    if (-not (Test-Under $final $rootFinal)) { return $null }
    return $final
}

# ---- the rest -----------------------------------------------------------------------------------------------------------------------------------------------
if ($SessionId -notmatch '^[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}\z') { throw "SessionId must be a guid: $SessionId" }
# -PromptFile: the first prompt is read from a file instead of the command line. Wave joins a tile's command with Unix-style quoting (a single quote becomes
# '"'"'), which PowerShell reads as several arguments, so a prompt with a quote would be cut. A file carries any text. The file is only READ (never written,
# never deleted by this script). It must be a plain file (not a link) of at most 64 KB, fully qualified, not plainly at or below .claude or .claude-work, and
# its folder's FINAL path must be at or below -PromptRoot (default %LOCALAPPDATA%\WaveFork\prompts) and not at or below .claude or .claude-work. UTF-8 only;
# one trailing line break is dropped, nothing else is changed. A refusal is exit code 2 before anything starts.
function Read-PromptFile([string]$file) {
    if ([string]::IsNullOrWhiteSpace($file) -or $file -notmatch '^[A-Za-z]:[\\/]') { return $null }
    if ([string]::IsNullOrWhiteSpace($PromptRoot) -or $PromptRoot -notmatch '^[A-Za-z]:[\\/]') { return $null }
    if ((Test-PlainlyForbidden $file) -or (Test-PlainlyForbidden $PromptRoot)) { return $null }
    try {
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { return $null }
        $item = Get-Item -LiteralPath $file -Force
        if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { return $null }
        if ($item.Length -gt 65536) { return $null }
        $dirFinal = Get-FinalPath (Split-Path -Parent $item.FullName)
        $rootFinal = Get-FinalPath $PromptRoot
        $homeFinal = Get-FinalPath $env:USERPROFILE
        if (-not $dirFinal -or -not $rootFinal -or -not $homeFinal -or -not (Test-Under $dirFinal $rootFinal)) { return $null }
        foreach ($bad in '.claude', '.claude-work') { if (Test-Under $dirFinal "$homeFinal\$bad") { return $null } }
        $text = [Text.UTF8Encoding]::new($false, $true).GetString([IO.File]::ReadAllBytes($item.FullName))
        if ($text.Length -gt 0 -and $text[0] -eq [char]0xFEFF) { $text = $text.Substring(1) }
        if ($text.EndsWith("`r`n")) { $text = $text.Substring(0, $text.Length - 2) } elseif ($text.EndsWith("`n")) { $text = $text.Substring(0, $text.Length - 1) }
        return $text
    } catch { return $null }
}
if ($PromptFile) {
    if ($Prompt) {
        [Console]::Error.WriteLine('Refusing to start Claude: give -Prompt or -PromptFile, not both.')
        exit 2
    }
    $fromFile = Read-PromptFile $PromptFile
    if ($null -eq $fromFile) {
        [Console]::Error.WriteLine('Refusing to start Claude: the prompt file is not a plain UTF-8 file of at most 64 KB below the prompt folder (nothing was started).')
        exit 2
    }
    $Prompt = $fromFile
}
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
    # --name is the name other sessions message it by (the session registry's "name"); --remote-control only names the
    # Remote Control session (18 Oct 2026: tiles opened as projects-research-night showed in ListAgents as ashfallprotocol-3c).
    if ($Name) { $a += '--name', $Name, '--remote-control', $Name }
    if ($Model) { $a += '--model', $Model }
    if ($Effort) { $a += '--effort', $Effort }
    if ($Prompt -and -not $exists) { $a += $Prompt }
    return $a
}

# The manager session (and only it) compacts at 300k tokens instead of the default (R-20261016-007; the same line as
# MalfestioMaster tools\Start-Session.ps1). It is set after the CLAUDE* clean-up below, which would otherwise remove it.
$compactWindow = if ($SessionId -eq '3c641601-9a7a-44be-a93c-e5a89419429b') { '300000' } else { '' }
# Account 2 has its own login and nothing else of its own: CLAUDE.md and settings.json are copies of account 1's, refreshed at every start the way the
# claude-personal2 command does it (a file link needs admin here). A session started without the refresh ran on an old copy and missed a hook.
# Only when the folder IS the account 2 folder, and only plain files whose own final place is below account 1's folder (never a link, so nothing outside it
# is ever read). A file that is missing or not plain is skipped with a warning on stderr; the session still starts.
function Get-Account2CopyPlan {
    $homeFinal = Get-FinalPath $env:USERPROFILE
    if (-not $homeFinal -or $ConfigDir -ine "$homeFinal\.claude-personal2") { return @() }
    $from = "$homeFinal\.claude-personal"
    # account 1's folder must be a real folder in its place: if it is a link, its files are somewhere else (possibly .claude or .claude-work)
    if ((Get-FinalPath $from) -ine $from) {
        [Console]::Error.WriteLine("Account 2 config: nothing was copied, $from is not a real folder in its place (a link?).")
        return @()
    }
    $plan = @()
    foreach ($name in 'CLAUDE.md', 'settings.json') {
        $src = Join-Path $from $name
        $dst = Join-Path $ConfigDir $name
        $item = Get-Item -LiteralPath $src -Force -ErrorAction SilentlyContinue
        if (-not $item -or $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            [Console]::Error.WriteLine("Account 2 config: $name was not copied, $src is missing or not a plain file.")
            continue
        }
        # Copy-Item -Force writes through a link, so a destination that is a link (or a folder) would put the file wherever it points
        $existing = Get-Item -LiteralPath $dst -Force -ErrorAction SilentlyContinue
        if ($existing -and ($existing.PSIsContainer -or ($existing.Attributes -band [IO.FileAttributes]::ReparsePoint))) {
            [Console]::Error.WriteLine("Account 2 config: $name was not copied, $dst is a link or a folder.")
            continue
        }
        $plan += [pscustomobject]@{ Name = $name; From = $src; To = $dst }
    }
    return $plan
}
$account2Copy = @(Get-Account2CopyPlan)
if ($DryRun) { (Get-ClaudeArgs) -join ' '; "config=$ConfigDir"; "status=$statusSession tab=$Name"; "compact=$compactWindow"; "copy=$(($account2Copy | ForEach-Object { $_.Name }) -join ',')"; return }

Get-ChildItem env: |
    Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -in 'STATUS_SESSION', 'STATUS_TAB', 'NO_COLOR', 'FORCE_COLOR', 'GIT_EDITOR',
        'GIT_ASKPASS', 'GIT_TERMINAL_PROMPT', 'NoDefaultCurrentDirectoryInExePath' } |
    ForEach-Object { Remove-Item "env:$($_.Name)" -ErrorAction SilentlyContinue }
# The folder that was searched for the transcript is the folder Claude must use, or the next start would look elsewhere.
$env:CLAUDE_CONFIG_DIR = $ConfigDir
if ($compactWindow) { $env:CLAUDE_CODE_AUTO_COMPACT_WINDOW = $compactWindow }
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
    $wshFull = Resolve-WshFolder $wshDir
    if ($wshFull) {
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
#  - wsh is the wsh.exe in the folder Wave names (WAVETERM_WSHBINDIR), run ONCE with "token <token> pwsh". That folder is the only one a program is ever run
#    from, so it gets the same gates as the config and plugin folders: fully qualified; a plain-spelling refusal of .claude and .claude-work before any file
#    system call; then its FINAL path must be at or below -WshRoot (default %LOCALAPPDATA%\WaveFork\profile\data\bin: what Wave sets, its data folder plus
#    "bin"; a test names its own root) and never at or below .claude or .claude-work. The same folder is checked before it goes on the PATH.
#  - The token is the PACKED form Wave puts in the environment (base64 of JSON with the real token and the socket name, a few hundred characters): base64
#    alphabet only, 8 to 8192 characters, no leading dash, and it must decode to JSON with a token and a rpccontext.sockname (nothing of it is printed);
#    anything else is no token and wsh is not called. It goes in as ONE argument list element, never joined into a string.
#  - The outcome is ONE code from a closed list (ok, no-token, token-shape, wsh-folder, wsh-missing, start-failed, timeout, too-large, no-lines, exit-code:<n>),
#    never wsh's own words. It is on the Note line and in one line of a small log (-LogPath, default %LOCALAPPDATA%\WaveFork\logs\starter.log, 200 lines).
#  - One deadline (-SwapTimeoutSeconds, default 15) covers the whole exchange: the read of the output and the exit. At the deadline wsh is killed and the
#    session goes on. The output is read in pieces and refused above 256 KB; a line above 16 KB is ignored.
#  - What it prints is PARSED, never run: only lines of the exact form  $env:NAME = "value"  with NAME on the fixed list below are taken; every other line is
#    ignored (a command, a subexpression, an assignment to PATH or to another name, a line with more after the value). Inside the value Wave escapes a double
#    quote, a backtick and a dollar sign with a backtick; that is all that is undone. A value that holds a newline, a control character or any other
#    backtick sequence is refused. A non-zero exit means nothing is taken.
#  - The token never reaches the screen or an error message. It is on the command line of the wsh call while that runs (a few milliseconds; another process of
#    the same user can list it then, as it can read this process's environment all along); it is single use, and it is removed from this process's environment
#    afterwards (an empty one too), so claude and its shells never see a spent token.
#  - A failed, slow or refused exchange never stops the session: it starts without wsh access and one plain line says so. When an exchange was tried and did
#    not succeed, any WAVETERM_JWT, BLOCKID, TABID and WORKSPACEID that were inherited are removed too, so that they are present only after a success.
$SwapVariables = @('WAVETERM_JWT', 'WAVETERM_BLOCKID', 'WAVETERM_TABID', 'WAVETERM_WORKSPACEID', 'WAVETERM_CLIENTID', 'WAVETERM_CONN', 'WAVETERM_VERSION', 'WAVETERM', 'TERM_PROGRAM')
$SwapMaxOutputChars = 262144
$SwapMaxLineChars = 16384

function ConvertFrom-WshAssignment([string]$line) {
    if ($line.Length -gt $SwapMaxLineChars) { return $null }
    $m = [regex]::Match($line, '^\$env:([A-Z][A-Z0-9_]*) = "((?:[^"`$]|`["`$`])*)"$')
    if (-not $m.Success -or $SwapVariables -cnotcontains $m.Groups[1].Value) { return $null }
    $value = $m.Groups[2].Value.Replace('``', [string][char]1).Replace('`"', '"').Replace('`$', '$').Replace([string][char]1, '`')
    if ($value -match '[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]') { return $null }
    return @{ Name = $m.Groups[1].Value; Value = $value }
}

$SwapCodes = @('ok', 'no-token', 'token-shape', 'wsh-folder', 'wsh-missing', 'start-failed', 'timeout', 'too-large', 'no-lines', 'no-access')
$script:SwapNames = @()

# The packed token that Wave puts in WAVETERM_SWAPTOKEN: base64 (standard alphabet) of a JSON object {"token": <uuid>, "rpccontext": {...}}
# (shellutil TokenSwapEntry.PackForClient). It is judged by its form and by what it decodes to, and nothing of it is ever printed or logged: exactly the
# two keys, the token a lower-case canonical uuid, and rpccontext only the fields of wshrpc.RpcContext with their types, sockname not empty and routeid
# present (it has no omitempty, so Wave always writes it, often empty).
$SwapContextTypes = @{ sockname = [string]; routeid = [string]; procroute = [bool]; blockid = [string]; conn = [string]; isrouter = [bool] }
function Test-PackedSwapToken([string]$token) {
    if ($token -cnotmatch '^[A-Za-z0-9_][A-Za-z0-9_+/=-]{7,8191}\z') { return $false }
    try {
        $json = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($token)) | ConvertFrom-Json -NoEnumerate -ErrorAction Stop
        if ($json -isnot [System.Management.Automation.PSCustomObject]) { return $false }
        $top = @($json.PSObject.Properties.Name)
        if ($top.Count -ne 2 -or $top -cnotcontains 'token' -or $top -cnotcontains 'rpccontext') { return $false }
        if ($json.token -isnot [string] -or $json.token -cnotmatch '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\z') { return $false }
        $ctx = $json.rpccontext
        if ($ctx -isnot [System.Management.Automation.PSCustomObject]) { return $false }
        foreach ($prop in $ctx.PSObject.Properties) {
            # field names are matched exactly (a hashtable lookup ignores case, so BLOCKID would pass as blockid)
            if (@($SwapContextTypes.Keys) -cnotcontains $prop.Name -or $prop.Value -isnot $SwapContextTypes[$prop.Name]) { return $false }
        }
        $names = @($ctx.PSObject.Properties.Name)
        return [bool](($names -ccontains 'sockname') -and $ctx.sockname -and ($names -ccontains 'routeid'))
    } catch { return $false }
}

# Returns ONE code from the closed list in $SwapCodes, or 'exit-code:<n>'. Never wsh's own words (its error text can hold a path, a socket name or the token).
function Invoke-WaveTokenSwap([int]$timeoutSeconds) {
    $token = $env:WAVETERM_SWAPTOKEN
    if (-not $token) { return 'no-token' }
    if (-not (Test-PackedSwapToken $token)) { return 'token-shape' }
    $binDir = Resolve-WshFolder $env:WAVETERM_WSHBINDIR
    if (-not $binDir) { return 'wsh-folder' }
    $wsh = Join-Path $binDir 'wsh.exe'
    if (-not (Test-Path -LiteralPath $wsh -PathType Leaf)) { return 'wsh-missing' }
    $proc = $null
    try {
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $wsh
        foreach ($a in 'token', $token, 'pwsh') { $psi.ArgumentList.Add($a) }
        $psi.UseShellExecute = $false
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.CreateNoWindow = $true
        $clock = [System.Diagnostics.Stopwatch]::StartNew()
        $deadlineMs = $timeoutSeconds * 1000
        try { $proc = [System.Diagnostics.Process]::Start($psi) } catch { return 'start-failed' }
        $null = $proc.StandardError.ReadToEndAsync()
        # one deadline for the read and the exit: the output is read in pieces (a wsh whose child keeps the pipe open, or one that never ends, ends at the
        # deadline), and above the size limit it is refused
        $text = New-Object System.Text.StringBuilder
        $buffer = New-Object 'char[]' 4096
        $reader = $proc.StandardOutput
        while ($true) {
            $left = $deadlineMs - $clock.ElapsedMilliseconds
            if ($left -le 0) { try { $proc.Kill($true) } catch { }; return 'timeout' }
            $read = $reader.ReadAsync($buffer, 0, $buffer.Length)
            if (-not $read.Wait([int]$left)) { try { $proc.Kill($true) } catch { }; return 'timeout' }
            if ($read.Result -le 0) { break }
            [void]$text.Append($buffer, 0, $read.Result)
            if ($text.Length -gt $SwapMaxOutputChars) { try { $proc.Kill($true) } catch { }; return 'too-large' }
        }
        $left = $deadlineMs - $clock.ElapsedMilliseconds
        if ($left -le 0 -or -not $proc.WaitForExit([int]$left)) { try { $proc.Kill($true) } catch { }; return 'timeout' }
        if ($proc.ExitCode -ne 0) { return ('exit-code:' + [int]$proc.ExitCode) }
        $found = @()
        foreach ($line in ($text.ToString() -split "`r?`n")) {
            $parsed = ConvertFrom-WshAssignment $line
            if ($parsed) { $found += $parsed }
        }
        if ($found.Count -eq 0) { return 'no-lines' }
        # wsh in this session needs WAVETERM_JWT; without it the exchange gave no access, so it is no success and nothing is set
        if (-not ($found | Where-Object { $_.Name -ceq 'WAVETERM_JWT' -and $_.Value })) { return 'no-access' }
        # every listed name is cleared first: a name that wsh did not print must not keep whatever was inherited
        foreach ($n in $SwapVariables) { Remove-Item "Env:$n" -ErrorAction SilentlyContinue }
        foreach ($parsed in $found) { Set-Item -Path "env:$($parsed.Name)" -Value $parsed.Value }
        $script:SwapNames = @($found | ForEach-Object { $_.Name })
        return 'ok'
    } catch {
        return 'start-failed'
    } finally {
        if ($proc) { $proc.Dispose() }
    }
}

# One line "<time> <session name> swap=<code>" (on success also the NAMES of the variables set, never a value) in a small log, capped at 200 lines by
# rewriting. Only fixed codes and names that passed the parser go in. A log that cannot be written is ignored.
function Write-SwapLog([string]$code, [string[]]$names, [string]$label = 'swap') {
    try {
        if (-not $LogPath -or $DryRun) { return }
        if ($LogPath -notmatch '^[A-Za-z]:[\\/]' -or (Test-PlainlyForbidden $LogPath)) { return }
        $who = $(if ($Name) { $Name } else { $SessionId }) -replace '[^A-Za-z0-9_.-]', '_'
        if ($who.Length -gt 40) { $who = $who.Substring(0, 40) }
        $line = (Get-Date -Format 'yyyy-MM-dd HH:mm:ss') + ' ' + $who + ' ' + $label + '=' + $code
        if ($names -and $names.Count -gt 0) { $line += ' set=' + ($names -join ',') }
        $dir = Split-Path -Parent $LogPath
        # the log's FINAL folder (the deepest existing ancestor resolved, as for every other folder) must be at or below -LogRoot and never at or below .claude or
        # .claude-work: judged BEFORE any folder is made or the file is opened. A log file that is itself a link is not written.
        if ($LogRoot -notmatch '^[A-Za-z]:[\\/]' -or (Test-PlainlyForbidden $LogRoot)) { return }
        $finalDir = Get-FinalPath $dir
        $rootFinal = Get-FinalPath $LogRoot
        $homeFinal = Get-FinalPath $env:USERPROFILE
        if (-not $finalDir -or -not $rootFinal -or -not $homeFinal -or -not (Test-Under $finalDir $rootFinal)) { return }
        foreach ($bad in '.claude', '.claude-work') { if (Test-Under $finalDir "$homeFinal\$bad") { return } }
        if ((Test-Path -LiteralPath $LogPath) -and ((Get-Item -LiteralPath $LogPath -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { return }
        if (-not (Test-Path -LiteralPath $dir -PathType Container)) { $null = New-Item -ItemType Directory -Path $finalDir -Force }
        # The existing log is only READ (its tail). The new content goes into a new file beside it (random name, exclusive create) and then replaces the
        # directory entry, so an existing log that is a HARD link to some other file is never written to: the other name keeps its content.
        # Remaining window, in plain words: between the checks above and this write a program running as the same user could swap the folder for a link in
        # that instant; nothing in the starter can close that (it needs such a program to be running already).
        $old = @(); if (Test-Path -LiteralPath $LogPath -PathType Leaf) { $old = @(Get-Content -LiteralPath $LogPath -Tail 199) }
        $tmp = Join-Path $finalDir ('.starter-log-' + [guid]::NewGuid().ToString('N').Substring(0, 12) + '.tmp')
        try {
            $stream = [IO.File]::Open($tmp, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
            try { $bytes = [Text.UTF8Encoding]::new($false).GetBytes((((@($old) + $line) -join [Environment]::NewLine) + [Environment]::NewLine)); $stream.Write($bytes, 0, $bytes.Length) } finally { $stream.Dispose() }
            [IO.File]::Move($tmp, (Join-Path $finalDir (Split-Path -Leaf $LogPath)), $true)
        } catch {
            try { if (Test-Path -LiteralPath $tmp) { [IO.File]::Delete($tmp) } } catch { }
        }
    } catch { }
}

if (Test-Path -LiteralPath Env:WAVETERM_SWAPTOKEN) {
    $swapCode = 'start-failed'
    try { $swapCode = Invoke-WaveTokenSwap $SwapTimeoutSeconds } catch { }
    Remove-Item Env:WAVETERM_SWAPTOKEN -ErrorAction SilentlyContinue
    Write-SwapLog $swapCode $script:SwapNames
    if ($swapCode -ne 'ok') {
        # a failed exchange leaves nothing that looks like access: whatever was inherited is removed
        foreach ($n in 'WAVETERM_JWT', 'WAVETERM_BLOCKID', 'WAVETERM_TABID', 'WAVETERM_WORKSPACEID') { Remove-Item "Env:$n" -ErrorAction SilentlyContinue }
        [Console]::Error.WriteLine("Note: the Wave token could not be exchanged (swap=$swapCode), so this session starts without wsh access. Restarting the block gives a new token.")
    }
}
# ---- account 2: the copy of account 1's CLAUDE.md and settings.json (planned above) ---------------------------------------------------------------------------
# ONE line in the starter log per start that tried to copy: account2copy=<ok|partial|failed> set=<file>:<first 12 hex of its SHA-256 after the copy, or "failed">,...
# so that a start can be shown to have copied (and what), not only to have found equal files. ok: every planned file was copied; failed: none was; partial: some
# were (the set says which). The session starts either way.
if ($account2Copy.Count -gt 0) {
    $copyResults = @(); $copiedCount = 0
    foreach ($c in $account2Copy) {
        try {
            Copy-Item -LiteralPath $c.From -Destination $c.To -Force -ErrorAction Stop
            # a copy that raised nothing but left the old file (a destination that denies writes) must not be logged as done
            $copiedHash = (Get-FileHash -LiteralPath $c.To -Algorithm SHA256).Hash
            if ($copiedHash -ne (Get-FileHash -LiteralPath $c.From -Algorithm SHA256).Hash) { throw 'the file at the destination is not the source' }
            $copyResults += $c.Name + ':' + $copiedHash.Substring(0, 12).ToLowerInvariant()
            $copiedCount++
        } catch {
            [Console]::Error.WriteLine("Account 2 config: $($c.Name) was not copied: $($_.Exception.Message)")
            $copyResults += $c.Name + ':failed'
        }
    }
    $copyCode = if ($copiedCount -eq $account2Copy.Count) { 'ok' } elseif ($copiedCount -eq 0) { 'failed' } else { 'partial' }
    Write-SwapLog $copyCode $copyResults 'account2copy'
}
# ---- the "trust this folder" question ---------------------------------------------------------------------------------------------------------------------
# Claude asks once per folder whether to trust it, and the default answer is "No, exit". A session opened in a background tile then sits on that question
# (or ends) and nobody sees it (17 Oct 2026, the Ashfall session). The manager has his standing yes for his own project folders, so this block answers it
# itself: a watcher reads this block's own screen through wsh (no focus, no keys typed into the window) and, when the question is up for a folder under
# -TrustRoot, moves to "Yes, I trust this folder" and confirms. Anything else (another folder, no wsh access, the question never shows) is left alone; the
# watcher stops after -TrustWaitSeconds.
# Only the last lines of the screen are read (the scrollback of a resumed session can quote the question), and only when the dialog itself is up: the
# "Accessing workspace", "No, exit" and the folder (white space removed, a long path wraps) within those lines. Before Enter the dialog is read again and the pointer must be on
# the Yes row. Not trusted by this: -TrustExclude (checkouts of other people's branches, the archive) and every folder in -TrustExcludeFile (outside clones).
# Any error in this part leaves the session to start as before: it never stops the start.
$wshExe = $null; $trustOk = $false
try {
    $cwdFinal = Get-FinalPath (Get-Location).Path
    foreach ($r in $TrustRoot) {
        if ([string]::IsNullOrWhiteSpace($r) -or $r -notmatch '^[A-Za-z]:[\\/].+' -or (Test-PlainlyForbidden $r)) { continue }
        $rf = Get-FinalPath $r; if ($cwdFinal -and $rf -and (Test-Under $cwdFinal $rf)) { $trustOk = $true }
    }
    $excluded = @($TrustExclude)
    if ($TrustExcludeFile -and (Test-Path -LiteralPath $TrustExcludeFile -PathType Leaf)) {
        $excluded += @((Get-Content -LiteralPath $TrustExcludeFile -Raw | ConvertFrom-Json).repos | ForEach-Object { $_.path })
    }
    foreach ($x in $excluded) { if ($x) { $xf = Get-FinalPath $x; if ($cwdFinal -and $xf -and (Test-Under $cwdFinal $xf)) { $trustOk = $false } } }
    if ($trustOk -and -not $DryRun -and $env:WAVETERM_BLOCKID -and $env:WAVETERM_JWT) {
        $wshHome = if ($wshFull) { $wshFull } else { Resolve-WshFolder $WshRoot }
        if ($wshHome) { $wshExe = Join-Path $wshHome 'wsh.exe' }
    }
    if ($trustOk -and $wshExe -and (Test-Path -LiteralPath $wshExe -PathType Leaf)) {
        $null = Start-ThreadJob -ArgumentList $wshExe, $env:WAVETERM_BLOCKID, $cwdFinal, $TrustWaitSeconds -ScriptBlock {
            param($wsh, $block, $folder, $wait)
            $want = ($folder -replace '\s', '').ToLowerInvariant()
            function Read-Dialog {
                $tail = @(& $wsh screen -b $block --last 25 2>$null)
                $flat = (($tail -join '') -replace '\s', '').ToLowerInvariant()
                $up = ($tail -match 'Accessing workspace').Count -gt 0 -and ($tail -match '^\W*(\d\.\s*)?No, exit\s*$').Count -gt 0 -and $flat.Contains($want)
                $onYes = ($tail -match '^\s*[^\s\w]\s*(\d\.\s*)?Yes, I trust this folder\s*$').Count -gt 0
                [pscustomobject]@{ Up = $up; OnYes = $onYes }
            }
            $deadline = (Get-Date).AddSeconds($wait)
            while ((Get-Date) -lt $deadline) {
                Start-Sleep -Seconds 2
                $d = Read-Dialog
                if (-not $d.Up) { continue }
                if (-not $d.OnYes) { $null = & $wsh send -b $block 'key:Down' 2>$null; Start-Sleep -Milliseconds 500; $d = Read-Dialog }
                if ($d.Up -and $d.OnYes) { $null = & $wsh send -b $block 'key:Enter' 2>$null }
                return
            }
        }
    }
} catch { }
# ---- which account a session runs on -------------------------------------------------------------------------------------------------------------------------
# Nothing else records it (a session file has no account, both accounts' transcripts sit in one folder), and the budget scan needs it to split tokens per account.
# ONE SMALL FILE PER START: <AccountDir>\<session id>-<at as yyyyMMddHHmmssfff>-<account>-<6 hex>.json (the suffix, a full 32-hex guid, keeps two starts of one session in one millisecond apart) holding {"at","session","account"}. There is no shared file, so no lock, no
# append, no trim and no lost update (Wave restarts all --restart blocks at the same moment after a reboot: a shared file was rewritten by each and the later start lost the earlier
# record). The reader takes every file of a session, sorted by "at", so a session moved between accounts keeps both records. Written to a fresh temporary file in the same
# folder (CreateNew) and then moved into place, so a reader never sees a half-written record. Only the two personal config folders count (.claude-personal = 1,
# .claude-personal2 = 2); a throwaway config or a dry run writes nothing. The folder is judged like the starter log: its FINAL path must be at or below -LogRoot and never at or
# below .claude or .claude-work, and a folder that is itself a link is never written into. Any error is ignored: this step can never stop the session.
# What is promised: every start is recorded unless a guard refuses the folder (a file, a link, outside the log root, under .claude); a refused or failed record never stops the start, and the refusal
# is noted in starter.log as "account-record=<code>" (a fixed word, never a path). Scope: ACCIDENTAL faults (parallel starts, a link set up by mistake, a folder in the wrong place). A hostile swap of the folder between check and write, and a stalled disk, are out of scope.
# Pruning (records older than 30 days, and .session-account-*.tmp files older than a day) is a step of the manager's daily chores, not of the starter.
function Write-SessionAccount {
    try {
        if (-not $AccountDir -or $DryRun) { return }
        if ($SessionId -notmatch '^[0-9A-Fa-f-]{8,64}$') { return }
        $homeFinal = Get-FinalPath $env:USERPROFILE
        if (-not $homeFinal) { return }
        $account = $null
        if (Test-Under $ConfigDir "$homeFinal\.claude-personal2") { $account = 2 } elseif (Test-Under $ConfigDir "$homeFinal\.claude-personal") { $account = 1 }
        if (-not $account) { return }
        if ($AccountDir -notmatch '^[A-Za-z]:[\\/]' -or (Test-PlainlyForbidden $AccountDir)) { Write-SwapLog 'folder-forbidden' @() 'account-record'; return }
        if ($LogRoot -notmatch '^[A-Za-z]:[\\/]' -or (Test-PlainlyForbidden $LogRoot)) { Write-SwapLog 'logroot-refused' @() 'account-record'; return }
        # the folder itself must not be a link (its final path would hide where the files really go)
        if (Test-Path -LiteralPath $AccountDir) {
            $item = Get-Item -LiteralPath $AccountDir -Force
            if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { Write-SwapLog 'folder-is-file-or-link' @() 'account-record'; return }
        }
        $finalDir = Get-FinalPath $AccountDir
        $rootFinal = Get-FinalPath $LogRoot
        if (-not $finalDir -or -not $rootFinal -or -not (Test-Under $finalDir $rootFinal)) { Write-SwapLog 'folder-outside-log-root' @() 'account-record'; return }
        foreach ($bad in '.claude', '.claude-work') { if (Test-Under $finalDir "$homeFinal\$bad") { Write-SwapLog 'folder-under-claude' @() 'account-record'; return } }
        if (-not (Test-Path -LiteralPath $AccountDir -PathType Container)) { $null = New-Item -ItemType Directory -Path $finalDir -Force }
        $at = (Get-Date).ToUniversalTime()
        if ($AccountAt) { $fixed = [datetime]::MinValue; if ([datetime]::TryParse($AccountAt, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::AdjustToUniversal -bor [Globalization.DateTimeStyles]::AssumeUniversal, [ref]$fixed)) { $at = $fixed } }
        $sid = $SessionId.ToLowerInvariant()
        $json = ([ordered]@{ at = $at.ToString('yyyy-MM-ddTHH:mm:ssZ'); session = $sid; account = $account } | ConvertTo-Json -Compress)
        $name = '{0}-{1}-{2}-{3}.json' -f $sid, $at.ToString('yyyyMMddHHmmssfff'), $account, [guid]::NewGuid().ToString('N')
        $tmp = Join-Path $finalDir ('.session-account-' + [guid]::NewGuid().ToString('N').Substring(0, 12) + '.tmp')
        try {
            $stream = [IO.File]::Open($tmp, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
            try { $bytes = [Text.UTF8Encoding]::new($false).GetBytes($json + [Environment]::NewLine); $stream.Write($bytes, 0, $bytes.Length) } finally { $stream.Dispose() }
            [IO.File]::Move($tmp, (Join-Path $finalDir $name), $false)
        } catch {
            try { if (Test-Path -LiteralPath $tmp) { [IO.File]::Delete($tmp) } } catch { }
            Write-SwapLog 'write-failed' @() 'account-record'
        }
    } catch { }
}
Write-SessionAccount
$claudeArgs = @(Get-ClaudeArgs)
& $Command @claudeArgs
