# Self test for the expected-ids check in Check-Seams.ps1: an expected file that cannot be read, or that is empty, must fail the run.
# Each case runs in a child PowerShell process. The test folder is new and uniquely named, and its files are removed one by one.
$check = Join-Path $PSScriptRoot 'Check-Seams.ps1'
$dir = Join-Path ([IO.Path]::GetTempPath()) ("seams-test-" + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $dir
$fail = 0
function Check($label, $ok, $detail) { if ($ok) { "GREEN $label" } else { $script:fail++; "RED   $label : $detail" } }
function Run($expected) { $o = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $check -ExpectedFile $expected 2>&1 | Out-String; return @{ Code = $LASTEXITCODE; Out = $o } }

$unreadable = Join-Path $dir 'a-folder-not-a-file'
$null = New-Item -ItemType Directory -Path $unreadable
$r = Run $unreadable
Check 'an expected file that cannot be read fails the run' ($r.Code -eq 1 -and $r.Out -match 'FATAL: the expected ids could not be read') "$($r.Code) $($r.Out)"

$empty = Join-Path $dir 'empty.txt'
Set-Content $empty '# only a comment'
$r = Run $empty
Check 'an expected file without ids fails the run' ($r.Code -eq 1 -and $r.Out -match 'holds no ids') "$($r.Code) $($r.Out)"

$missing = Join-Path $dir 'nope.txt'
$r = Run $missing
Check 'an expected file that does not exist fails the run' ($r.Code -eq 1 -and $r.Out -match 'file missing') "$($r.Code) $($r.Out)"

$ghost = Join-Path $dir 'ghost.txt'
Set-Content $ghost @('# a valid file naming one id that is not in the seam list', 'tab-flagcolor-const', 'no-such-seam-id')
$r = Run $ghost
Check 'a valid expected file naming an id that is absent from the seam and watch lists fails the run with the missing-id message' ($r.Code -eq 1 -and $r.Out -match 'RED\s+no-such-seam-id : in seams-expected.txt but missing from the seam and watch lists') "$($r.Code) $($r.Out)"
# A row that is really dropped from the script's own lists, while its id stays in the expected file, must be RED. Done on copies
# of the script and the expected file in a new folder; the repository's files are not touched.
$root = (& git -C $PSScriptRoot rev-parse --show-toplevel).Trim()
foreach ($dropped in @('upstream-tab-group', 'upstream-confirm-close-tab', 'stack-menu-hook')) {
    $copy = Join-Path $dir ("copy-" + $dropped)
    $null = New-Item -ItemType Directory -Path $copy
    $lines = @(Get-Content -LiteralPath $check | Where-Object { $_ -notmatch ("^\s*@\('" + [regex]::Escape($dropped) + "',") })
    Check "the copy for $dropped has one row fewer" ($lines.Count -eq @(Get-Content -LiteralPath $check).Count - 1) "$($lines.Count)"
    Set-Content -LiteralPath (Join-Path $copy 'Check-Seams.ps1') $lines
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'seams-expected.txt') -Destination (Join-Path $copy 'seams-expected.txt')
    $o = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $copy 'Check-Seams.ps1') -Root $root 2>&1 | Out-String
    Check "dropping the row $dropped while its id stays in the expected file is RED" ($LASTEXITCODE -eq 1 -and $o -match ("RED\s+" + [regex]::Escape($dropped) + " : in seams-expected.txt but missing")) "$LASTEXITCODE $o"
    Remove-Item -LiteralPath (Join-Path $copy 'Check-Seams.ps1')
    Remove-Item -LiteralPath (Join-Path $copy 'seams-expected.txt')
    Remove-Item -LiteralPath $copy
}
$real = Run (Join-Path $PSScriptRoot 'seams-expected.txt')
Check 'the real expected file passes' ($real.Code -eq 0 -and $real.Out -match 'seams red: 0') "$($real.Code) $($real.Out)"

Remove-Item $empty, $ghost
Remove-Item $unreadable
Remove-Item $dir
if ($fail -gt 0) { exit 1 }