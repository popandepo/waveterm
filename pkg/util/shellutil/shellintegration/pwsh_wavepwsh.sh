# We source this file with -NoExit -File
$env:PATH = {{.WSHBINDIR_PWSH}} + "{{.PATHSEP}}" + $env:PATH

# Source dynamic script from wsh token
$waveterm_swaptoken_output = wsh token $env:WAVETERM_SWAPTOKEN pwsh 2>$null | Out-String
if ($waveterm_swaptoken_output -and $waveterm_swaptoken_output -ne "") {
    Invoke-Expression $waveterm_swaptoken_output
}
Remove-Variable -Name waveterm_swaptoken_output
Remove-Item Env:WAVETERM_SWAPTOKEN

# Per-block command history (setting term:perblockhistory): PSReadLine's one shared history file makes every new shell show the last
# command typed in any other. Pointing it at this block's own file before the first prompt means the history is read from there (an
# empty one for a new block); if the line editor is already running, its history is replaced with the block's file.
# Only for a line editor that can clear its history (PSReadLine 2.4.5 on pwsh 7.6 is the one this was checked against); an older one that has
# already loaded the shared file would show it anyway while saving to the block's file, so there the history is left alone.
# wave-history-begin
# The entries of a PSReadLine history file: one per line, a command of several lines has a backtick at the end of every line but the last
# (PSReadLine's own format), so those lines are joined again; empty lines are skipped.
function Global:_waveterm_history_entries([string]$Path) {
    $entry = $null
    foreach ($line in [System.IO.File]::ReadAllLines($Path)) {
        if ($line.EndsWith('`')) {
            $part = $line.Substring(0, $line.Length - 1)
            $entry = if ($null -eq $entry) { $part } else { $entry + "`n" + $part }
            continue
        }
        $full = if ($null -eq $entry) { $line } else { $entry + "`n" + $line }
        $entry = $null
        if ($full.Trim().Length -gt 0) { $full }
    }
    if ($null -ne $entry -and $entry.Trim().Length -gt 0) { $entry }
}

# Puts the saved entries into a running line editor WITHOUT saving them again (with incremental saving every added entry would be appended
# to the file once more, and every restart would double it): saving is switched off while they go in and put back afterwards, also on a failure.
function Global:_waveterm_load_history([string]$Path, [scriptblock]$Add, [scriptblock]$GetStyle, [scriptblock]$SetStyle) {
    if (-not (Test-Path -LiteralPath $Path)) { return }
    $entries = @(_waveterm_history_entries $Path)
    if ($entries.Count -eq 0) { return }
    $style = & $GetStyle
    & $SetStyle 'SaveNothing'
    try {
        foreach ($e in $entries) { & $Add $e }
    } finally {
        & $SetStyle $style
    }
}

# Whether this line editor can clear its history (an older one that cannot is left alone, see above).
function Global:_waveterm_can_clear_history([type]$Type) {
    return ($null -ne $Type.GetMethod('ClearHistory'))
}

if ($env:WAVETERM_PWSH_HISTFILE) {
    try {
        Import-Module PSReadLine -ErrorAction Stop
        if (_waveterm_can_clear_history ([Microsoft.PowerShell.PSConsoleReadLine])) {
            Set-PSReadLineOption -HistorySavePath $env:WAVETERM_PWSH_HISTFILE
            # A running line editor has the shared history in memory: replace it. One that has not started reading yet throws here, and then
            # reads the block's file itself on its first prompt (the path is already set).
            try {
                [Microsoft.PowerShell.PSConsoleReadLine]::ClearHistory()
                _waveterm_load_history $env:WAVETERM_PWSH_HISTFILE `
                    { param($c) [Microsoft.PowerShell.PSConsoleReadLine]::AddToHistory($c) } `
                    { (Get-PSReadLineOption).HistorySaveStyle } `
                    { param($s) Set-PSReadLineOption -HistorySaveStyle $s }
            } catch { }
        }
    } catch { }
    Remove-Item Env:WAVETERM_PWSH_HISTFILE
}
# wave-history-end

# Load Wave completions
wsh completion powershell | Out-String | Invoke-Expression

if ($PSVersionTable.PSVersion.Major -lt 7) {
    return  # skip OSC setup entirely
}

if ($PSStyle.FileInfo.Directory -eq "`e[44;1m") {
    $PSStyle.FileInfo.Directory = "`e[34;1m"
}

$Global:_WAVETERM_SI_FIRSTPROMPT = $true

# shell integration
function Global:_waveterm_si_blocked {
    # Check if we're in tmux or screen
    return ($env:TMUX -or $env:STY -or $env:TERM -like "tmux*" -or $env:TERM -like "screen*")
}

function Global:_waveterm_si_osc7 {
    if (_waveterm_si_blocked) { return }
    
    # Percent-encode the raw path as-is (handles UNC, drive letters, etc.)
    $encoded_pwd = [System.Uri]::EscapeDataString($PWD.Path)
    
    # OSC 7 - current directory
    Write-Host -NoNewline "`e]7;file://localhost/$encoded_pwd`a"
}

function Global:_waveterm_si_prompt {
    if (_waveterm_si_blocked) { return }
    
    if ($Global:_WAVETERM_SI_FIRSTPROMPT) {
		# not sending uname
		       $shellversion = $PSVersionTable.PSVersion.ToString()
		       Write-Host -NoNewline "`e]16162;M;{`"shell`":`"pwsh`",`"shellversion`":`"$shellversion`",`"integration`":false}`a"
        $Global:_WAVETERM_SI_FIRSTPROMPT = $false
    }
    
    _waveterm_si_osc7
    # private marker for wavesrv: the prompt is about to be shown (it decides, together with the input that is sent to the shell,
    # whether this shell waits at a prompt; see blockstatus.PromptTracker)
    Write-Host -NoNewline "`e]16162;P`a"
}

# Add the OSC 7 call to the prompt function
if (Test-Path Function:\prompt) {
    $global:_waveterm_original_prompt = $function:prompt
    function Global:prompt {
        _waveterm_si_prompt
        & $global:_waveterm_original_prompt
    }
} else {
    function Global:prompt {
        _waveterm_si_prompt
        "PS $($executionContext.SessionState.Path.CurrentLocation)$('>' * ($nestedPromptLevel + 1)) "
    }
}