# Runs INSIDE a Wave terminal block that was made with `wsh session start --restart --meta session:id=<guid> ...`.
# Starts claude with a clean environment (the launching session's CLAUDE* variables, NO_COLOR and the like are removed), plus one decision that a plain launcher does not
# make: with a fixed session id, the FIRST start creates the session under that id (--session-id) and every later start
# (Wave was restarted) resumes it (--resume), so the same conversation comes back in the same block.
# -DryRun prints the claude arguments instead of starting it (used by the self test).
param(
    [Parameter(Mandatory)][string]$SessionId,
    [string]$Name,
    [string]$Model,
    [string]$Effort,
    [string]$PermissionMode,
    [switch]$SkipPermissions,
    [string]$Prompt,
    [string]$ConfigDir = $(if ($env:CLAUDE_CONFIG_DIR) { $env:CLAUDE_CONFIG_DIR } else { Join-Path $env:USERPROFILE '.claude' }),
    [string]$Command = 'claude',
    [switch]$DryRun
)
$ErrorActionPreference = 'Stop'
if ($SessionId -notmatch '^[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}$') { throw "SessionId must be a guid: $SessionId" }

function Test-SessionExists([string]$root, [string]$id) {
    $projects = Join-Path $root 'projects'
    if (-not (Test-Path -LiteralPath $projects)) { return $false }
    return [bool](Get-ChildItem -LiteralPath $projects -Directory -ErrorAction SilentlyContinue |
        Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName "$id.jsonl") } | Select-Object -First 1)
}

function Get-ClaudeArgs {
    $a = @()
    if ($SkipPermissions) { $a += '--dangerously-skip-permissions' } elseif ($PermissionMode) { $a += '--permission-mode', $PermissionMode }
    if (Test-SessionExists $ConfigDir $SessionId) { $a += '--resume', $SessionId } else { $a += '--session-id', $SessionId }
    if ($Name) { $a += '--remote-control', $Name }
    if ($Model) { $a += '--model', $Model }
    if ($Effort) { $a += '--effort', $Effort }
    if ($Prompt -and -not (Test-SessionExists $ConfigDir $SessionId)) { $a += $Prompt }
    return $a
}

if ($DryRun) { (Get-ClaudeArgs) -join ' '; "config=$ConfigDir"; return }

Get-ChildItem env: |
    Where-Object { $_.Name -like 'CLAUDE*' -or $_.Name -in 'NO_COLOR', 'FORCE_COLOR', 'GIT_EDITOR',
        'GIT_ASKPASS', 'GIT_TERMINAL_PROMPT', 'NoDefaultCurrentDirectoryInExePath' } |
    ForEach-Object { Remove-Item "env:$($_.Name)" -ErrorAction SilentlyContinue }
# The folder that was searched for the transcript is the folder Claude must use, or the next start would look elsewhere.
$env:CLAUDE_CONFIG_DIR = $ConfigDir
if ($Name) { $Host.UI.RawUI.WindowTitle = $Name }
# Wave puts the folder of wsh on the PATH of the shells it starts (wavepwsh.ps1), not of a program that it starts directly, as this one is. A Claude session
# in a tile that cannot run wsh cannot use the rest of Wave, so the folder Wave names (WAVETERM_WSHBINDIR) is put on the PATH here, once.
$wshDir = $env:WAVETERM_WSHBINDIR
if ($wshDir -and (Test-Path -LiteralPath $wshDir -PathType Container)) {
    $entries = @($env:PATH -split [IO.Path]::PathSeparator | Where-Object { $_ })
    if (-not ($entries | Where-Object { $_.TrimEnd('\', '/') -ieq $wshDir.TrimEnd('\', '/') })) {
        $env:PATH = $wshDir + [IO.Path]::PathSeparator + $env:PATH
    }
}
$claudeArgs = @(Get-ClaudeArgs)
& $Command @claudeArgs
