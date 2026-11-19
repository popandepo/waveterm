# Self test of Open-WaveSession.ps1 with -DryRun: nothing is started, no wsh is called, nothing is copied.
$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'Open-WaveSession.ps1'
$src = 'C:\Projects\Personal\wave\waveterm\tools\ours\Start-ResumableSession.ps1'
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }
$cwd = $env:TEMP
$st = Join-Path $env:TEMP ('new-starter-' + [guid]::NewGuid().ToString('N') + '.ps1'); Set-Content $st '# fake starter with the markers: WAVETERM_WSHBINDIR Resolve-AllowedConfigDir PromptFile'
# a made-up profile: nothing in this test names or opens a folder of the real profile
$realProfile = $env:USERPROFILE
$fakeProfile = Join-Path $env:TEMP ('openwave-test-profile-' + [guid]::NewGuid().ToString('N'))
$fakeDirs = @($fakeProfile, (Join-Path $fakeProfile '.claude-personal'), (Join-Path $fakeProfile '.claude-personal2'), (Join-Path $fakeProfile '.claude'), (Join-Path $fakeProfile '.claude-work'), (Join-Path $env:TEMP 'throwaway'))
foreach ($d in $fakeDirs) { $null = New-Item -ItemType Directory -Path $d -Force }
$env:USERPROFILE = $fakeProfile
$env:CLAUDE_CONFIG_DIR = Join-Path $env:USERPROFILE '.claude-personal'
$id = '11111111-2222-3333-4444-555555555555'
# a made-up MalfestioMaster: Session-Plugins.ps1 (the one on origin/Master), manager.json naming one session, and the mod folders the list names
$mm = Join-Path $env:TEMP ('openwave-test-mm-' + [guid]::NewGuid().ToString('N'))
$mmDirs = @($mm, "$mm\tools", "$mm\tools\mods", "$mm\tools\mods\usage-file", "$mm\tools\mods\usage-file-2", "$mm\tools\mods\tool-rows", "$mm\tools\mods\run-suggest", "$mm\mods", "$mm\mods\context-feed", "$mm\mods\context-feed\hooks")
foreach ($d in $mmDirs) { $null = New-Item -ItemType Directory -Path $d -Force }
$spSrc = (& git -C 'C:\Projects\Personal\MalfestioMaster' show 'origin/Master:tools/Session-Plugins.ps1') -join "`n"
Set-Content -LiteralPath "$mm\tools\Session-Plugins.ps1" -Value $spSrc
$mgrId = '99999999-8888-7777-6666-555555555555'
Set-Content -LiteralPath "$mm\tools\manager.json" -Value ('{"sessionId":"' + $mgrId + '","name":"mgr-test"}')
Set-Content -LiteralPath "$mm\mods\context-feed\hooks\hooks.json" -Value '{}'
$sp = "$mm\tools\Session-Plugins.ps1"
$PSDefaultParameterValues['*:SessionPluginsScript'] = $sp

$j = & $script -Cwd $cwd -Name tether-notes -SessionId $id -Model claude-sonnet-5-5 -Effort low -PermissionMode auto -Prompt 'a; b' -Items '20261115-M-6804a3f2 20261101-C-1f64d43a' -TabColor '#2D7DD2' -StarterSource $st -DryRun | ConvertFrom-Json
$wsh = @($j.wshargs); $argv = @($j.argv)
Check 'a session is opened with --restart' ($wsh -contains '--restart') ($wsh -join ' ')
$metaValues = @(); for ($i = 0; $i -lt $wsh.Count - 1; $i++) { if ($wsh[$i] -eq '--meta') { $metaValues += $wsh[$i + 1] } }
foreach ($want in "session:id=$id", 'session:name=tether-notes', 'session:model=claude-sonnet-5-5', 'session:effort=low', 'session:permissionmode=auto') {
    Check "meta $want" ($metaValues -contains $want) ($metaValues -join ' | ')
}
Check 'no other meta key' ($metaValues.Count -eq 5) ($metaValues -join ' | ')
Check 'the command is the resumable starter, by a stable path outside the repo' ($argv[2] -eq '-File' -and $argv[3] -like '*\WaveFork\tools\Start-ResumableSession.ps1') ($argv -join ' ')
Check 'the command has -SessionId with the id and never --session-id' (($argv -join ' ') -match "-SessionId $id" -and ($argv -join ' ') -notmatch '--session-id') ($argv -join ' ')
# since 11 Oct the prompt of a restartable session goes through a file (Wave's command quoting cut it); the command line names the file, never the text
$pf = $argv[[array]::IndexOf($argv, '-PromptFile') + 1]
Check 'the prompt travels through a file named on the command line, not as text' (($argv -contains '-PromptFile') -and -not ($argv -contains 'a; b') -and $pf -like '*.txt') ($argv -join ' | ')
Check 'the items travel in the command as one comma-joined argument, so a restart hands them again' ($argv[[array]::IndexOf($argv, '-Items') + 1] -eq '20261115-M-6804a3f2,20261101-C-1f64d43a') ($argv -join ' | ')
Check 'no inline code form that the restart check refuses' (-not ($argv -contains '-Command') -and -not ($argv -contains '-c')) ($argv -join ' ')

$k = & $script -Cwd $cwd -Name n2 -StarterSource $st -DryRun | ConvertFrom-Json
Check 'without a session id a new guid is made and used' ($k.sessionid -match '^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$' -and (@($k.argv) -contains $k.sessionid)) ($k | ConvertTo-Json -Compress)
$m = @($k.wshargs); $mv = @(); for ($i = 0; $i -lt $m.Count - 1; $i++) { if ($m[$i] -eq '--meta') { $mv += $m[$i + 1] } }
Check 'only the given facts become meta (name and id here)' (($mv -contains "session:id=$($k.sessionid)") -and ($mv -contains 'session:name=n2') -and $mv.Count -eq 2) ($mv -join ' | ')

$s = & $script -Cwd $cwd -Name n3 -SessionId $id -SkipPermissions -PermissionMode auto -StarterSource $st -DryRun | ConvertFrom-Json
Check 'skip-permissions is passed on and no permissionmode meta is written for it' ((@($s.argv) -contains '-SkipPermissions') -and (-not (@($s.wshargs) -contains 'session:permissionmode=auto'))) (($s | ConvertTo-Json -Compress))

$old = & $script -Cwd $cwd -Name n4 -NoRestart -StartSession 'C:\x\Start-Session.ps1' -StarterSource $st -DryRun | ConvertFrom-Json
Check '-NoRestart is the old way: no restart, no meta, the given launcher' ((-not (@($old.wshargs) -contains '--restart')) -and (-not (@($old.wshargs) -contains '--meta')) -and (@($old.argv) -contains 'C:\x\Start-Session.ps1')) ($old | ConvertTo-Json -Compress)

$threw = $false; try { & $script -Cwd $cwd -Name n5 -SessionId 'nope' -StarterSource $st -DryRun | Out-Null } catch { $threw = $true }
Check 'a session id that is not a guid is refused' $threw ''
$threw = $false; try { & $script -Cwd $cwd -Name n6 -StarterSource 'C:\nonexistent\x.ps1' -DryRun | Out-Null } catch { $threw = $true }
Check 'a missing starter stops it' $threw ''
$old2 = Join-Path $env:TEMP ('old-starter-' + [guid]::NewGuid().ToString('N') + '.ps1'); Set-Content $old2 '# an old starter without the wsh path'
$threw = $false; try { & $script -Cwd $cwd -Name n7 -StarterSource $old2 -DryRun | Out-Null } catch { $threw = $true }
Remove-Item $old2 -ErrorAction SilentlyContinue
Check 'an old starter (no wsh on the PATH) stops it' $threw ''
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
# the plugin list is Get-SessionPluginDirs' (Session-Plugins.ps1, 19 Nov 2026: the manager came back through Wave without context-feed at 18 Nov 23:00):
# every folder travels in ONE comma-joined -PluginDir (a stored restart command goes through pwsh -File), the starter makes one --plugin-dir of each.
function PluginOf($r) { $a = @($r.argv); $i = [array]::IndexOf($a, '-PluginDir'); if ($i -ge 0) { $a[$i + 1] } else { '' } }
function ListOf($r) { $p = PluginOf $r; if ($p) { @($p -split ',') } else { @() } }
function ArgOf($r, [string]$flag) { $a = @($r.argv); $i = [array]::IndexOf($a, $flag); if ($i -ge 0) { if ($i + 1 -lt $a.Count) { $a[$i + 1] } else { '' } } else { $null } }
$cfg1 = Join-Path $env:USERPROFILE '.claude-personal'
$cfg2 = Join-Path $env:USERPROFILE '.claude-personal2'
$base1 = @("$mm\tools\mods\usage-file", "$mm\tools\mods\tool-rows", "$mm\tools\mods\run-suggest")
$feed = "$mm\mods\context-feed"
$mn = & $script -Cwd $cwd -Name mgr-test -StarterSource $st -ConfigDir $cfg1 -SessionPluginsScript $sp -DryRun | ConvertFrom-Json
Check 'the manager (by name) gets usage-file, tool-rows, run-suggest and context-feed, each folder in the one -PluginDir list' (((ListOf $mn) -join '|') -eq (($base1 + $feed) -join '|')) (PluginOf $mn)
$mi = & $script -Cwd $cwd -Name other-name -SessionId $mgrId -StarterSource $st -ConfigDir $cfg1 -SessionPluginsScript $sp -DryRun | ConvertFrom-Json
Check 'the manager by session id gets context-feed too' ((ListOf $mi) -contains $feed) (PluginOf $mi)
$u1 = & $script -Cwd $cwd -Name u1 -StarterSource $st -ConfigDir $cfg1 -SessionPluginsScript $sp -DryRun | ConvertFrom-Json
Check 'a non-manager gets usage-file, tool-rows and run-suggest, and no context-feed' (((ListOf $u1) -join '|') -eq ($base1 -join '|')) (PluginOf $u1)
$u2 = & $script -Cwd $cwd -Name u2 -StarterSource $st -ConfigDir $cfg2 -SessionPluginsScript $sp -DryRun | ConvertFrom-Json
Check 'account 2 gets the usage-file-2 mod in place of usage-file' ((ListOf $u2)[0] -eq "$mm\tools\mods\usage-file-2" -and (ListOf $u2) -notcontains "$mm\tools\mods\usage-file") (PluginOf $u2)
$thr = & $script -Cwd $cwd -Name x -StarterSource $st -ConfigDir (Join-Path $env:TEMP 'throwaway') -ThrowawayConfig -SessionPluginsScript $sp -DryRun | ConvertFrom-Json
Check 'a throwaway folder gets no plugin folders' (-not (PluginOf $thr)) (PluginOf $thr)
Check 'a throwaway folder gets no plugin folders when it is the manager either' ((& $script -Cwd $cwd -Name mgr-test -StarterSource $st -ConfigDir (Join-Path $env:TEMP 'throwaway') -ThrowawayConfig -SessionPluginsScript $sp -DryRun | ConvertFrom-Json | ForEach-Object { PluginOf $_ }) -eq '') 'plugins on a throwaway manager'
$ux = & $script -Cwd $cwd -Name ux -StarterSource $st -ConfigDir $cfg2 -PluginDir $env:TEMP -SessionPluginsScript $sp -DryRun | ConvertFrom-Json
Check 'a plugin folder given by the caller is kept, and replaces the list' ((PluginOf $ux) -eq $env:TEMP) (PluginOf $ux)
Check 'the starter is not given -ToolRowsDir any more (tool-rows is in the list)' ($null -eq (ArgOf $u1 '-ToolRowsDir')) ($u1 | ConvertTo-Json -Compress)
$errFile = Join-Path $env:TEMP ('plugins-stderr-' + [guid]::NewGuid().ToString('N') + '.txt')
$null = & pwsh -NoProfile -File $script -Cwd $cwd -Name nf -StarterSource $st -ConfigDir $cfg1 -SessionPluginsScript (Join-Path $mm 'tools\no-such.ps1') -DryRun 2>$errFile
$errText = if (Test-Path -LiteralPath $errFile) { Get-Content -LiteralPath $errFile -Raw } else { '' }
Remove-Item -LiteralPath $errFile -ErrorAction SilentlyContinue
Check 'a missing Session-Plugins.ps1 stops the open (nonzero exit), it does not start with a shorter list' (($LASTEXITCODE -ne 0) -and ($errText -match 'plugin list script was not found')) "exit=$LASTEXITCODE $errText"
$tc = & $script -Cwd $cwd -Name tc -StarterSource $st -ConfigDir (Join-Path $env:USERPROFILE '.claude-personal') -ClickRows -DryRun | ConvertFrom-Json
Check '-ClickRows is passed on to the starter' (@($tc.argv) -contains '-ClickRows') ($tc | ConvertTo-Json -Compress)
Remove-Item $st -ErrorAction SilentlyContinue
foreach ($f in @("$mm\mods\context-feed\hooks\hooks.json", "$mm\tools\Session-Plugins.ps1", "$mm\tools\manager.json")) { Remove-Item -LiteralPath $f -ErrorAction SilentlyContinue }
[array]::Reverse($mmDirs)
foreach ($d in $mmDirs) { try { [IO.Directory]::Delete($d) } catch { } }
$env:USERPROFILE = $realProfile
Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
[array]::Reverse($fakeDirs)
foreach ($d in $fakeDirs) { if ($d -eq (Join-Path $env:TEMP 'throwaway')) { continue }; try { [IO.Directory]::Delete($d) } catch { } }
"failures: $fail"
exit $fail
