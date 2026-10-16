# Self test for Assert-ClaudeConfigDir.ps1. Starts nothing, reads nothing inside any config folder.
. (Join-Path $PSScriptRoot 'Assert-ClaudeConfigDir.ps1')
$fail = 0
function Check($label, $ok) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label" } }
$h = $env:USERPROFILE
Check 'unset is refused' (-not (Test-ClaudeConfigDir ''))
Check '.claude is refused' (-not (Test-ClaudeConfigDir "$h\.claude"))
Check '.claude is refused even as throwaway' (-not (Test-ClaudeConfigDir "$h\.claude" -Throwaway))
Check 'a folder inside .claude is refused' (-not (Test-ClaudeConfigDir "$h\.claude\projects" -Throwaway))
Check '.claude-work is refused even as throwaway' (-not (Test-ClaudeConfigDir "$h\.claude-work" -Throwaway))
Check 'dot dot out of .claude-personal into .claude is refused' (-not (Test-ClaudeConfigDir "$h\.claude-personal\..\.claude"))
Check '.claude-personal is allowed' (Test-ClaudeConfigDir "$h\.claude-personal")
Check '.claude-personal with a trailing slash is allowed' (Test-ClaudeConfigDir "$h\.claude-personal\")
Check '.claude-personalx (a different folder) is not treated as .claude-personal' (-not (Test-ClaudeConfigDir "$h\.claude-personalx"))
Check '.claude-personal2 (account 2) is allowed' (Test-ClaudeConfigDir "$h\.claude-personal2")
Check '.claude-personal3 is refused' (-not (Test-ClaudeConfigDir "$h\.claude-personal3"))
Check '.claude-personal2x is refused' (-not (Test-ClaudeConfigDir "$h\.claude-personal2x"))
Check 'a temp folder is refused without -Throwaway' (-not (Test-ClaudeConfigDir "$env:TEMP\x"))
Check 'a temp folder is allowed with -Throwaway' (Test-ClaudeConfigDir "$env:TEMP\x" -Throwaway)
$threw = $false; try { Assert-ClaudeConfigDir "$h\.claude" } catch { $threw = $true }
Check 'Assert throws for .claude' $threw
if ($fail) { "FAILED: $fail"; exit 1 } else { 'all green' }
