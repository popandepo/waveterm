# Dot-source this, then call Assert-ClaudeConfigDir (or Test-ClaudeConfigDir). The same rule as Start-ResumableSession.ps1 (the block below is copied from it):
# a personal session may only use a Claude config folder under %USERPROFILE%.claude-personal, or an explicitly named throwaway folder, never .claude or
# .claude-work, after links, 8.3 names and ".." are resolved. An unset value is refused: Claude would fall back to the off-limits %USERPROFILE%.claude.
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
    # account 1 and account 2 (R-20261014-008, 14 Oct 2026): exactly these two folders and what is below them; .claude-personal3 or
    # .claude-personal2x are neither, since Test-Under needs the whole folder name
    foreach ($ok in '.claude-personal', '.claude-personal2') { if (Test-Under $final "$homeFinal\$ok") { return $final } }
    if ($throwaway) { return $final }
    return $null
}

function Test-ClaudeConfigDir([string]$Dir, [switch]$Throwaway) { return [bool](Resolve-AllowedConfigDir $Dir $Throwaway.IsPresent) }
function Assert-ClaudeConfigDir([string]$Dir, [switch]$Throwaway) {
    $r = Resolve-AllowedConfigDir $Dir $Throwaway.IsPresent
    if (-not $r) { throw "Refusing to start Claude: config folder '$Dir' is not fully qualified, is at or below .claude or .claude-work, or is not under .claude-personal (and not an explicit throwaway)." }
    return $r
}
