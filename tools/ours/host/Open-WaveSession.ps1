# Wave version of MalfestioMaster\tools\Open-SessionTab.ps1: opens a personal Claude session in a NEW BACKGROUND TAB of
# Wave and prints the tab and block ids as JSON. Must run inside a Wave block (wsh needs the block's token), or with -DryRun.
# The tab is not shown and nothing takes keyboard focus unless -Activate is given. The program is started directly
# (no shell, no launcher), so a prompt containing ; & | or quotes reaches the starter unchanged.
#
# A session opened here COMES BACK after Wave restarts (changed 11 Oct 2026):
#   - the block is made with --restart, so Wave starts the same command again at its next start, in the same block;
#   - the command is the resumable starter (a copy at %LOCALAPPDATA%\WaveFork\tools\Start-ResumableSession.ps1, kept outside any working
#     folder): its FIRST start creates the session under -SessionId (a new guid when none is given), every later start resumes it;
#   - the block carries session:id, session:name, session:model, session:effort and session:permissionmode (only those that were given),
#     which is what puts it into the "sessions" index of the state record (<data dir>\state\wave-state.json) that Restore-Sessions.ps1 reads.
# CHANGED 11 Oct 2026 (evening): a prompt of a restartable session now goes through a FILE (-PromptFile under %LOCALAPPDATA%\WaveFork\prompts), and any other
# argument with a single quote is refused. The next real open must be preceded by a -DryRun (three live sessions were opened with the earlier version).
# CHANGED 12 Oct 2026: -Stack <block> opens the session into an existing stack instead of a tab of its own (see the note at the param block).
# -StartSession (the old launcher of the caller) is accepted and ignored: a plain launcher cannot be started again with the same id.
# -NoRestart opens the old way (no restart, no meta).
#
#   .\Open-WaveSession.ps1 -Cwd C:\Projects\Personal\Tether -Name tether-notes -SessionId <guid> `
#       -Model claude-sonnet-5-5 -Effort low -PermissionMode auto -TabColor '#2D7DD2' -Symbol star -Prompt "a; b"
param(
    [Parameter(Mandatory)][string]$Cwd,
    [Parameter(Mandatory)][string]$Name,
    [string]$SessionId,
    [string]$Model,
    [string]$Effort,
    [string]$PermissionMode,
    [switch]$SkipPermissions,
    [string]$Prompt,
    [string]$TabColor,
    [string]$Symbol,
    [switch]$Activate,
    [string[]]$Meta,
    [string]$StartSession,
    [switch]$NoRestart,
    [switch]$DryRun,
    [string]$ConfigDir = $env:CLAUDE_CONFIG_DIR,
    [switch]$ThrowawayConfig,
    [string]$PluginDir = $(if (Test-Path -LiteralPath 'C:\Projects\Personal\MalfestioMaster\tools\mods\usage-file' -PathType Container) { 'C:\Projects\Personal\MalfestioMaster\tools\mods\usage-file' } else { '' }),
    [string]$StarterSource = (Join-Path (Split-Path $PSScriptRoot) 'Start-ResumableSession.ps1'),
    [string]$StarterDir = (Join-Path $env:LOCALAPPDATA 'WaveFork\tools'),
    [string]$PromptDir = (Join-Path $env:LOCALAPPDATA 'WaveFork\prompts'),
    [string]$Stack
)
$ErrorActionPreference = 'Stop'
# -Stack <block id or number>: NO new tab; the session becomes a member of the group (stack) of that block (wsh session start --stack). -Name, -TabColor and
# -Symbol then style the block (strip label, colour, strip symbol), not a tab. Not with -Activate. Needs a wsh that has --stack (PR "session start --stack").
if ($Stack -and $Stack -notmatch '^[A-Za-z0-9:-]{1,64}$') { throw "-Stack must be a block id or number: $Stack" }
if ($Stack -and $Activate) { throw '-Stack cannot be combined with -Activate (there is no new tab to switch to)' }
. (Join-Path $PSScriptRoot 'Assert-ClaudeConfigDir.ps1')
$Guid = '^[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}$'
if ($SessionId -and $SessionId -notmatch $Guid) { throw "SessionId must be a guid: $SessionId" }
if (-not $NoRestart -and -not $SessionId) { $SessionId = [guid]::NewGuid().ToString() }

# The session that this block will start uses the config folder of THIS environment. There is no default: an unset or .claude/.claude-work folder is refused (exit 2).
$ResolvedConfig = $null
if (-not $NoRestart) { $ResolvedConfig = Resolve-AllowedConfigDir $ConfigDir $ThrowawayConfig.IsPresent }
if (-not $NoRestart -and -not $ResolvedConfig) {
    [Console]::Error.WriteLine("Refusing: CLAUDE_CONFIG_DIR '$ConfigDir' is empty, is .claude or .claude-work, or is not under .claude-personal (use -ThrowawayConfig for a throwaway folder).")
    exit 2
}

# the starter: a copy outside any working folder, because Wave starts it again at every start and a working folder can be on another branch
$starter = $null
if (-not $NoRestart) {
    if (-not (Test-Path -LiteralPath $StarterSource)) { throw "the resumable starter was not found: $StarterSource" }
    $text = Get-Content -LiteralPath $StarterSource -Raw
    if ($text -notmatch 'Resolve-AllowedConfigDir') { throw "the starter at $StarterSource has no config folder rule (an old version with a default of .claude): use the starter with the rule" }
    if ($Prompt -and $text -notmatch 'PromptFile') { throw "the starter at $StarterSource cannot read a prompt from a file (an old version): a prompt with a single quote would be cut by Wave's command quoting" }
    if ($text -notmatch 'WAVETERM_WSHBINDIR') { throw "the starter at $StarterSource does not put wsh on the PATH (an old version): check out Master or the starter fix first" }
    $starter = Join-Path $StarterDir 'Start-ResumableSession.ps1'
    $sameCopy = (Test-Path -LiteralPath $starter) -and ((Get-FileHash -LiteralPath $starter).Hash -eq (Get-FileHash -LiteralPath $StarterSource).Hash)
    if (-not $sameCopy -and -not $DryRun) {
        $null = New-Item -ItemType Directory -Force -Path $StarterDir
        Copy-Item -LiteralPath $StarterSource -Destination $starter -Force
    }
}

$argv = if ($NoRestart) {
    if (-not $StartSession) { throw '-StartSession is needed with -NoRestart (the old way)' }
    @('pwsh', '-NoLogo', '-File', $StartSession, '-Name', $Name)
} else {
    @('pwsh', '-NoLogo', '-File', $starter, '-SessionId', $SessionId, '-Name', $Name, '-ConfigDir', $ResolvedConfig) + $(if ($PluginDir) { @('-PluginDir', $PluginDir) } else { @() }) + $(if ($ThrowawayConfig) { @('-ThrowawayConfig') } else { @() })
}
if ($NoRestart -and $SessionId) { $argv += '-SessionId', $SessionId }
if ($Model) { $argv += '-Model', $Model }
if ($Effort) { $argv += '-Effort', $Effort }
if ($SkipPermissions) { $argv += '-SkipPermissions' }
elseif ($PermissionMode) { $argv += '-PermissionMode', $PermissionMode }
# The prompt of a restartable session goes through a FILE: Wave joins a tile's command with Unix-style quoting (a single quote becomes '"'"'), which PowerShell
# reads as several arguments, so a prompt with a single quote was cut and its pieces bound to -Model and -Effort (claude exit 1, no transcript). The file is
# under %LOCALAPPDATA%\WaveFork\prompts (the profile's own folder, not readable by other users), named by the session id, written once and never deleted by the
# starter. The old way (-NoRestart) still passes -Prompt on the command line.
$promptFile = $null
if ($Prompt -and -not $NoRestart) {
    $promptFile = Join-Path $PromptDir ($SessionId + '.txt')
    $argv += '-PromptFile', $promptFile, '-PromptRoot', $PromptDir
} elseif ($Prompt) { $argv += '-Prompt', $Prompt }

# Nothing else on the stored command line may hold a single quote (a name, a folder, a model): Wave's quoting would split it. Refused here, before anything is made.
foreach ($a in $argv) {
    if ($a -match "'") { throw "Wave's command quoting cannot carry a single quote: refused ($($a.Substring(0, [Math]::Min(40, $a.Length))))" }
}

$allMeta = @()
if (-not $NoRestart) {
    $allMeta += "session:id=$SessionId", "session:name=$Name"
    if ($Model) { $allMeta += "session:model=$Model" }
    if ($Effort) { $allMeta += "session:effort=$Effort" }
    if ($PermissionMode -and -not $SkipPermissions) { $allMeta += "session:permissionmode=$PermissionMode" }
}
foreach ($m in $Meta) { $allMeta += $m }

# The arguments travel in a JSON file, not on a command line, so no quoting layer can split or alter them.
# Colour and symbol by session name (what Marius set by hand on 11 Oct 2026). An explicit -TabColor or -Symbol wins; a name that is not in the table gets none.
$LookByName = @{
    'runner-research' = @('#F59E0B', 'flask')
    'voice'           = @('#EC4899', 'microphone')
    'storage'         = @('#06B6D4', 'database')
    'tether'          = @('#3B82F6', 'mobile-screen')
    'tab-janitor'     = @('#F97316', 'broom')
    'medicine-tower'  = @('#EF4444', 'pills')
    'tether-tests'    = @('#A3E635', 'vial')
    'status'          = @('#14B8A6', 'diagram-project')
    'wave'            = @('#6366F1', 'water')
}
if ($Name -and $LookByName.ContainsKey($Name.ToLowerInvariant())) {
    $look = $LookByName[$Name.ToLowerInvariant()]
    if (-not $TabColor) { $TabColor = $look[0] }
    if (-not $Symbol) { $Symbol = $look[1] }
}
$wshArgs = @('session', 'start', '--name', $Name, '--cwd', $Cwd, '--args-file', '<file>')
if ($TabColor) { $wshArgs += '--color', $TabColor }
if ($Symbol) { $wshArgs += '--symbol', $Symbol }
if ($Activate) { $wshArgs += '--activate' }
if ($Stack) { $wshArgs += '--stack', $Stack }
if (-not $NoRestart) { $wshArgs += '--restart' }
foreach ($m in $allMeta) { $wshArgs += '--meta', $m }

if ($DryRun) {
    [pscustomobject]@{ wshargs = $wshArgs; argv = $argv; starter = $starter; sessionid = $SessionId; promptfile = $promptFile } | ConvertTo-Json -Depth 4
    return
}

if ($promptFile) {
    $null = New-Item -ItemType Directory -Force -Path $PromptDir
    [IO.File]::WriteAllText($promptFile, $Prompt, [Text.UTF8Encoding]::new($false))
}
$argsFile = Join-Path ([IO.Path]::GetTempPath()) ("wave-session-" + [guid]::NewGuid().ToString('N') + '.json')
try {
    ConvertTo-Json -InputObject @($argv) -Compress | Set-Content -LiteralPath $argsFile -Encoding utf8
    $wshArgs[$wshArgs.IndexOf('<file>')] = $argsFile
    if ($SessionId) { Write-Host "session id: $SessionId" }
    & wsh @wshArgs
    if ($LASTEXITCODE -ne 0) { throw "wsh session start failed ($LASTEXITCODE)" }
}
finally { Remove-Item -LiteralPath $argsFile -ErrorAction SilentlyContinue }
