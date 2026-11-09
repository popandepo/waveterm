# Self test of Open-WaveSession.ps1 with -DryRun: nothing is started, no wave.exe is called, nothing is copied.
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'Open-WaveSession.ps1'
$src = 'C:\Projects\Personal\wave\waveterm\tools\ours\Start-ResumableSession.ps1'
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }
$cwd = $env:TEMP
$st = Join-Path $env:TEMP ('new-starter-' + [guid]::NewGuid().ToString('N') + '.ps1'); Set-Content $st '# fake starter with the markers: WAVE_TOKEN Resolve-AllowedConfigDir PromptFile'
# a made-up profile: nothing in this test names or opens a folder of the real profile
$realProfile = $env:USERPROFILE
$fakeProfile = Join-Path $env:TEMP ('openwave-test-profile-' + [guid]::NewGuid().ToString('N'))
$fakeDirs = @($fakeProfile, (Join-Path $fakeProfile '.claude-personal'), (Join-Path $fakeProfile '.claude-personal2'), (Join-Path $fakeProfile '.claude'), (Join-Path $fakeProfile '.claude-work'), (Join-Path $env:TEMP 'throwaway'))
foreach ($d in $fakeDirs) { $null = New-Item -ItemType Directory -Path $d -Force }
$env:USERPROFILE = $fakeProfile
$env:CLAUDE_CONFIG_DIR = Join-Path $env:USERPROFILE '.claude-personal'
$id = '11111111-2222-3333-4444-555555555555'

$j = & $script -Cwd $cwd -Name tether-notes -SessionId $id -Model claude-sonnet-5-5 -Effort low -PermissionMode auto -Prompt 'a; b' -TabColor '#2D7DD2' -StarterSource $st -DryRun | ConvertFrom-Json
$waveArgs = @($j.waveargs)
$argv = @($j.argv)
# The real starter of this tree, not a fake: the marker check in the opener is text matching only, so this at least proves the two files agree.
$realThrew = $false
try { $null = & $script -Cwd $cwd -Name tether-notes -SessionId $id -Prompt 'a' -DryRun } catch { $realThrew = $true }
Check 'the real starter of this tree passes the marker check of the opener' (-not $realThrew) ''
Check 'a session is opened with --restart' ($waveArgs -contains '--restart') ($waveArgs -join ' ')
$metaValues = @(); for ($i = 0; $i -lt $waveArgs.Count - 1; $i++) { if ($waveArgs[$i] -eq '--meta') { $metaValues += $waveArgs[$i + 1] } }
foreach ($want in "session:id=$id", 'session:name=tether-notes', 'session:model=claude-sonnet-5-5', 'session:effort=low', 'session:permissionmode=auto') {
    Check "meta $want" ($metaValues -contains $want) ($metaValues -join ' | ')
}
Check 'no other meta key' ($metaValues.Count -eq 5) ($metaValues -join ' | ')
Check 'the command is the resumable starter, by a stable path outside the repo' ($argv[2] -eq '-File' -and $argv[3] -like '*\WaveFork\tools\Start-ResumableSession.ps1') ($argv -join ' ')
Check 'the command has -SessionId with the id and never --session-id' (($argv -join ' ') -match "-SessionId $id" -and ($argv -join ' ') -notmatch '--session-id') ($argv -join ' ')
# since 11 Oct the prompt of a restartable session goes through a file (Wave's command quoting cut it); the command line names the file, never the text
$pf = $argv[[array]::IndexOf($argv, '-PromptFile') + 1]
Check 'the prompt travels through a file named on the command line, not as text' (($argv -contains '-PromptFile') -and -not ($argv -contains 'a; b') -and $pf -like '*.txt') ($argv -join ' | ')
Check 'no inline code form that the restart check refuses' (-not ($argv -contains '-Command') -and -not ($argv -contains '-c')) ($argv -join ' ')

$k = & $script -Cwd $cwd -Name n2 -StarterSource $st -DryRun | ConvertFrom-Json
Check 'without a session id a new guid is made and used' ($k.sessionid -match '^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$' -and (@($k.argv) -contains $k.sessionid)) ($k | ConvertTo-Json -Compress)
$m = @($k.waveargs); $mv = @(); for ($i = 0; $i -lt $m.Count - 1; $i++) { if ($m[$i] -eq '--meta') { $mv += $m[$i + 1] } }
Check 'only the given facts become meta (name and id here)' (($mv -contains "session:id=$($k.sessionid)") -and ($mv -contains 'session:name=n2') -and $mv.Count -eq 2) ($mv -join ' | ')

$s = & $script -Cwd $cwd -Name n3 -SessionId $id -SkipPermissions -PermissionMode auto -StarterSource $st -DryRun | ConvertFrom-Json
Check 'skip-permissions is passed on and no permissionmode meta is written for it' ((@($s.argv) -contains '-SkipPermissions') -and (-not (@($s.waveargs) -contains 'session:permissionmode=auto'))) (($s | ConvertTo-Json -Compress))


$threw = $false; try { & $script -Cwd $cwd -Name n5 -SessionId 'nope' -StarterSource $st -DryRun | Out-Null } catch { $threw = $true }
Check 'a session id that is not a guid is refused' $threw ''
$threw = $false; try { & $script -Cwd $cwd -Name n6 -StarterSource 'C:\nonexistent\x.ps1' -DryRun | Out-Null } catch { $threw = $true }
Check 'a missing starter stops it' $threw ''
$old2 = Join-Path $env:TEMP ('old-starter-' + [guid]::NewGuid().ToString('N') + '.ps1'); Set-Content $old2 '# an old starter with WAVETERM_JWT and no gate'
$threw = $false; try { & $script -Cwd $cwd -Name n7 -StarterSource $old2 -DryRun | Out-Null } catch { $threw = $true }
Remove-Item $old2 -ErrorAction SilentlyContinue
Check 'an old starter (old WAVETERM_ names, no WAVE_TOKEN) stops it' $threw ''
# the config folder rule: refused with exit code 2, and handed to the starter
$cfg = (@($j.argv) -join ' ')
Check 'the config folder of this environment is handed to the starter' ($cfg -match [regex]::Escape('-ConfigDir ' + $env:CLAUDE_CONFIG_DIR)) $cfg
$ErrorActionPreference = 'Continue'
foreach ($case in @(@('unset', ''), @('old', (Join-Path $env:USERPROFILE '.claude')), @('work', (Join-Path $env:USERPROFILE '.claude-work')), @('oldsub', (Join-Path $env:USERPROFILE '.claude\projects')))) {
    $envScript = if ($case[1]) { "`$env:CLAUDE_CONFIG_DIR = '" + $case[1] + "'" } else { 'Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue' }
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$envScript; & '$script' -Cwd '$cwd' -Name x -StarterSource '$st' -DryRun; exit `$LASTEXITCODE" 2>$null | Out-Null
    Check ("config folder case '" + $case[0] + "' is refused with exit code 2") ($LASTEXITCODE -eq 2) "exit=$LASTEXITCODE"
}
$ErrorActionPreference = 'Stop'
$thr = & $script -Cwd $cwd -Name x -StarterSource $st -ConfigDir (Join-Path $env:TEMP 'throwaway') -ThrowawayConfig -DryRun | ConvertFrom-Json
Check 'a throwaway folder needs -ThrowawayConfig and is then passed on with it' ((@($thr.argv) -contains '-ThrowawayConfig') -and (@($thr.argv) -contains (Join-Path $env:TEMP 'throwaway'))) ($thr | ConvertTo-Json -Compress)
# the usage mod follows the account (17 Oct 2026: account-2 sessions got the account-1 mod and wrote account 2's figures into account 1's file)
function PluginOf($r) { $a = @($r.argv); $i = [array]::IndexOf($a, '-PluginDir'); if ($i -ge 0) { $a[$i + 1] } else { '' } }
$u1 = & $script -Cwd $cwd -Name u1 -StarterSource $st -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal') -DryRun | ConvertFrom-Json
Check 'account 1 gets the usage-file mod' ((PluginOf $u1) -like '*\mods\usage-file') (PluginOf $u1)
$u2 = & $script -Cwd $cwd -Name u2 -StarterSource $st -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal2') -DryRun | ConvertFrom-Json
Check 'account 2 gets the usage-file-2 mod' ((PluginOf $u2) -like '*\mods\usage-file-2') (PluginOf $u2)
Check 'a throwaway folder gets no usage mod' (-not (PluginOf $thr)) (PluginOf $thr)
$ux = & $script -Cwd $cwd -Name ux -StarterSource $st -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal2') -PluginDir $env:TEMP -DryRun | ConvertFrom-Json
Check 'a plugin folder given by the caller is kept' ((PluginOf $ux) -eq $env:TEMP) (PluginOf $ux)
# tool-rows and -ClickRows (31 Oct 2026): the mod folder is passed on when it exists; the fullscreen switch only when asked for
function ArgOf($r, [string]$flag) { $a = @($r.argv); $i = [array]::IndexOf($a, $flag); if ($i -ge 0) { if ($i + 1 -lt $a.Count) { $a[$i + 1] } else { '' } } else { $null } }
$tr = & $script -Cwd $cwd -Name tr -StarterSource $st -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal') -ToolRowsDir $env:TEMP -DryRun | ConvertFrom-Json
Check 'an existing -ToolRowsDir is passed on to the starter' ((ArgOf $tr '-ToolRowsDir') -eq $env:TEMP) ($tr | ConvertTo-Json -Compress)
Check 'without -ClickRows the starter is not told to go fullscreen' (@($tr.argv) -notcontains '-ClickRows') ($tr | ConvertTo-Json -Compress)
$missing = Join-Path $env:TEMP 'no-such-tool-rows-folder'
$errFile = Join-Path $env:TEMP ('tool-rows-stderr-' + [guid]::NewGuid().ToString('N') + '.txt')
# a child process, so that what the script writes to the process's stderr ([Console]::Error) is caught
$tn = & pwsh -NoProfile -File $script -Cwd $cwd -Name tn -StarterSource $st -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal') -ToolRowsDir $missing -DryRun 2>$errFile | ConvertFrom-Json
Check 'a -ToolRowsDir that does not exist is left out' ($null -eq (ArgOf $tn '-ToolRowsDir')) ($tn | ConvertTo-Json -Compress)
$errText = if (Test-Path -LiteralPath $errFile) { Get-Content -LiteralPath $errFile -Raw } else { '' }
Remove-Item -LiteralPath $errFile -ErrorAction SilentlyContinue
Check 'and it says so: one line naming the folder and that tool-rows is not loaded' (($errText -split "`n" | Where-Object { $_ -match [regex]::Escape($missing) -and $_ -match 'tool-rows is not loaded' }).Count -eq 1) $errText
$tc = & $script -Cwd $cwd -Name tc -StarterSource $st -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal') -ClickRows -DryRun | ConvertFrom-Json
Check '-ClickRows is passed on to the starter' (@($tc.argv) -contains '-ClickRows') ($tc | ConvertTo-Json -Compress)
Remove-Item $st -ErrorAction SilentlyContinue
$env:USERPROFILE = $realProfile
Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
[array]::Reverse($fakeDirs)
foreach ($d in $fakeDirs) { if ($d -eq (Join-Path $env:TEMP 'throwaway')) { continue }; try { [IO.Directory]::Delete($d) } catch { } }
"failures: $fail"
exit $fail
