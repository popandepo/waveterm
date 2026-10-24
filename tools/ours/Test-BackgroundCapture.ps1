# Checkpoint for "work in Wave without showing it": records the foreground window, the caret/focus window of the foreground
# thread and the active tab of every workspace before and after a step, and is RED when any of them changed or when the
# result of the step is blank. Prints counts and ids only, never screen text.
#   -SelfTest   the pure comparison rules with made-up snapshots (what would make it red). Exit 0 when all cases are right.
#   -Live       run Step { } between two snapshots against the Wave whose api folder is -ApiDir (default: the one of the
#               current profile; pass the trial profile's api folder for a trial instance). Exit 0 green, 1 red.
param(
    [switch]$SelfTest,
    [switch]$Live,
    [string]$ApiDir = (Join-Path $env:LOCALAPPDATA 'WaveFork\profile\data\api'),
    [scriptblock]$Step,
    [scriptblock]$Result
)

Add-Type -TypeDefinition @'
using System; using System.Runtime.InteropServices;
public static class FgProbe {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L, T, R, B; }
  [StructLayout(LayoutKind.Sequential)] public struct GUITHREADINFO {
    public int cbSize; public int flags; public IntPtr hwndActive, hwndFocus, hwndCapture, hwndMenuOwner, hwndMoveSize, hwndCaret; public RECT rcCaret; }
  [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] static extern bool GetGUIThreadInfo(uint tid, ref GUITHREADINFO i);
  public static string Read() {
    IntPtr fg = GetForegroundWindow(); uint pid; uint tid = GetWindowThreadProcessId(fg, out pid);
    GUITHREADINFO i = new GUITHREADINFO(); i.cbSize = Marshal.SizeOf(i);
    if (!GetGUIThreadInfo(tid, ref i)) return fg.ToInt64() + "|nogui";
    return fg.ToInt64() + "|" + i.hwndActive.ToInt64() + "|" + i.hwndFocus.ToInt64() + "|" + i.hwndCaret.ToInt64() + "|" + i.rcCaret.L + "," + i.rcCaret.T + "," + i.rcCaret.R + "," + i.rcCaret.B;
  }
}
'@

# Reasons the second snapshot is not the first. Empty list = green.
function Compare-Snapshots($before, $after) {
    $why = @()
    if ($before.Foreground -ne $after.Foreground) { $why += "foreground window / focus / caret changed: [$($before.Foreground)] -> [$($after.Foreground)]" }
    foreach ($k in $before.ActiveTabs.Keys) {
        if ($before.ActiveTabs[$k] -ne $after.ActiveTabs[$k]) { $why += "active tab of workspace $k changed: $($before.ActiveTabs[$k]) -> $($after.ActiveTabs[$k])" }
    }
    foreach ($k in $after.ActiveTabs.Keys) {
        if (-not $before.ActiveTabs.ContainsKey($k)) { $why += "workspace $k appeared" }
    }
    return $why
}

# A capture is blank when it has no bytes of image or text, or (for a picture, given as RGBA/RGB samples) one single value.
function Test-BlankResult([int]$byteCount, [int[]]$samples) {
    if ($byteCount -le 0) { return $true }
    if ($samples -and $samples.Count -gt 0 -and ($samples | Select-Object -Unique).Count -eq 1) { return $true }
    return $false
}

if ($SelfTest) {
    $base = @{ Foreground = '100|100|110|0|0,0,0,0'; ActiveTabs = @{ w1 = 't1'; w2 = 't9' } }
    $cases = @(
        @('nothing changed', $base, @{ Foreground = $base.Foreground; ActiveTabs = @{ w1 = 't1'; w2 = 't9' } }, 0),
        @('foreground window changed', $base, @{ Foreground = '200|200|210|0|0,0,0,0'; ActiveTabs = $base.ActiveTabs }, 1),
        @('only the caret window changed', $base, @{ Foreground = '100|100|110|55|1,2,3,4'; ActiveTabs = $base.ActiveTabs }, 1),
        @('active tab changed', $base, @{ Foreground = $base.Foreground; ActiveTabs = @{ w1 = 't2'; w2 = 't9' } }, 1),
        @('a workspace appeared', $base, @{ Foreground = $base.Foreground; ActiveTabs = @{ w1 = 't1'; w2 = 't9'; w3 = 't5' } }, 1),
        @('everything changed', $base, @{ Foreground = '1|1|1|1|1,1,1,1'; ActiveTabs = @{ w1 = 'x'; w2 = 'y' } }, 3))
    $failed = 0
    foreach ($c in $cases) {
        $n = @(Compare-Snapshots $c[1] $c[2]).Count
        if ($n -eq $c[3]) { "GREEN $($c[0])" } else { $failed++; "RED   $($c[0]): $n reasons, want $($c[3])" }
    }
    $blank = @(
        @('zero bytes', 0, @(), $true), @('all-black picture', 5000, @(0, 0, 0, 0), $true), @('all-white picture', 5000, @(255, 255), $true),
        @('picture with two values', 5000, @(0, 255), $false), @('text with bytes, no samples', 40, @(), $false))
    foreach ($c in $blank) {
        $got = Test-BlankResult $c[1] $c[2]
        if ($got -eq $c[3]) { "GREEN $($c[0])" } else { $failed++; "RED   $($c[0]): blank=$got, want $($c[3])" }
    }
    exit $(if ($failed) { 1 } else { 0 })
}

Add-Type -TypeDefinition @'
using System.Net.Http; using System.Net.Sockets;
public static class UnixHttp2 {
  public static HttpClient Make(string path) {
    var handler = new SocketsHttpHandler();
    handler.ConnectCallback = async (ctx, ct) => { var s = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified); await s.ConnectAsync(new UnixDomainSocketEndPoint(path), ct); return new NetworkStream(s, true); };
    return new HttpClient(handler);
  }
}
'@
$token = (Get-Content -Raw (Join-Path $ApiDir 'api-token')).Trim()
$http = [UnixHttp2]::Make((Join-Path $ApiDir 'wave-api.sock'))
$http.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token)
function Call($path, $body) {
    $content = [System.Net.Http.StringContent]::new(($body | ConvertTo-Json -Compress), [System.Text.Encoding]::UTF8, 'application/json')
    $resp = $http.PostAsync("http://wave$path", $content).GetAwaiter().GetResult()
    [pscustomobject]@{ Status = [int]$resp.StatusCode; Text = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult() }
}
function Get-Snapshot {
    $tree = (Call '/v1/list' @{}).Text | ConvertFrom-Json
    $tabs = @{}
    foreach ($ws in $tree.workspaces) { $tabs[$ws.workspaceid] = $ws.activetabid }
    @{ Foreground = [FgProbe]::Read(); ActiveTabs = $tabs }
}

$before = Get-Snapshot
if ($Step) { & $Step }
$after = Get-Snapshot
$why = @(Compare-Snapshots $before $after)
$blank = $false
if ($Result) { $blank = [bool](& $Result) }
foreach ($w in $why) { "RED $w" }
if ($blank) { 'RED the result is blank' }
if (-not $why -and -not $blank) { 'GREEN foreground, caret and active tabs unchanged; result not blank' }
exit $(if ($why -or $blank) { 1 } else { 0 })
