# Checkpoint for "work in Wave without showing it": records the foreground window, the caret/focus window of the foreground
# thread, the active tab of every workspace and the list of visible top-level windows (order, position, minimised) before and
# after a step, and is RED when any of them changed, when a probe failed, or when the result of the step is blank. Prints counts
# and ids only, never screen text.
#   -SelfTest   the pure comparison rules with made-up snapshots (what would make it red). Exit 0 when all cases are right.
#   -Live       run -Step { } between two snapshots against the Wave whose api folder is -ApiDir (default: the current
#               profile's; pass the trial profile's folder for a trial instance), then -Result { } which returns $true when the
#               result is BLANK. Both blocks are required. Exit 0 green, 1 red.
# Known limits: only the state before and after is compared, so a window that appears and goes away again inside the step, a
# taskbar flash, or focus that moves and comes back are not seen; a window list entry changes when its title bar rect changes.
param(
    [switch]$SelfTest,
    [switch]$Live,
    [string]$ApiDir = (Join-Path $env:LOCALAPPDATA 'WaveFork\profile\data\api'),
    [scriptblock]$Step,
    [scriptblock]$Result
)

Add-Type -TypeDefinition @'
using System; using System.Collections.Generic; using System.Runtime.InteropServices; using System.Text;
public static class FgProbe {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L, T, R, B; }
  [StructLayout(LayoutKind.Sequential)] public struct GUITHREADINFO {
    public int cbSize; public int flags; public IntPtr hwndActive, hwndFocus, hwndCapture, hwndMenuOwner, hwndMoveSize, hwndCaret; public RECT rcCaret; }
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] static extern bool GetGUIThreadInfo(uint tid, ref GUITHREADINFO i);
  [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc p, IntPtr l);
  [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] static extern bool IsIconic(IntPtr h);
  [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] static extern int GetWindowTextLength(IntPtr h);
  // "ERR:..." when a probe failed (no foreground window, or its thread info cannot be read): a failed probe is never a value.
  public static string Read() {
    IntPtr fg = GetForegroundWindow();
    if (fg == IntPtr.Zero) return "ERR:no foreground window";
    uint pid; uint tid = GetWindowThreadProcessId(fg, out pid);
    if (tid == 0) return "ERR:no thread for the foreground window";
    GUITHREADINFO i = new GUITHREADINFO(); i.cbSize = Marshal.SizeOf(i);
    if (!GetGUIThreadInfo(tid, ref i)) return "ERR:GetGUIThreadInfo failed";
    return fg.ToInt64() + "|" + i.hwndActive.ToInt64() + "|" + i.hwndFocus.ToInt64() + "|" + i.hwndCaret.ToInt64() + "|" + i.rcCaret.L + "," + i.rcCaret.T + "," + i.rcCaret.R + "," + i.rcCaret.B;
  }
  // Visible top-level windows with a title, front to back: handle, rectangle, minimised.
  public static string Windows() {
    List<string> list = new List<string>();
    EnumWindows((h, l) => {
      if (!IsWindowVisible(h) || GetWindowTextLength(h) == 0) return true;
      RECT r; GetWindowRect(h, out r);
      list.Add(h.ToInt64() + ":" + r.L + "," + r.T + "," + r.R + "," + r.B + ":" + (IsIconic(h) ? "min" : "norm"));
      return true;
    }, IntPtr.Zero);
    return string.Join(";", list);
  }
}
'@

# Reasons the second snapshot is not the first. Empty list = green. A snapshot that holds a failed probe is red by itself.
function Compare-Snapshots($before, $after) {
    $why = @()
    foreach ($s in @(@('before', $before), @('after', $after))) {
        if ("$($s[1].Foreground)" -like 'ERR:*' -or [string]::IsNullOrEmpty("$($s[1].Foreground)")) { $why += "probe failed ($($s[0])): $($s[1].Foreground)" }
        if ([string]::IsNullOrEmpty("$($s[1].Windows)")) { $why += "probe failed ($($s[0])): no window list" }
        if (-not $s[1].ActiveTabs -or $s[1].ActiveTabs.Count -eq 0) { $why += "probe failed ($($s[0])): no workspace listed" }
    }
    if ($why) { return $why }
    if ($before.Foreground -ne $after.Foreground) { $why += "foreground window / focus / caret changed: [$($before.Foreground)] -> [$($after.Foreground)]" }
    if ($before.Windows -ne $after.Windows) {
        $b = $before.Windows -split ';'; $a = $after.Windows -split ';'
        $why += "visible windows changed: $($b.Count) -> $($a.Count) windows, $((Compare-Object $b $a).Count) entries differ (opened, closed, moved, minimised or restored, or the order changed)"
    }
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
    $fg = '100|100|110|0|0,0,0,0'
    $win = '100:0,0,500,500:norm;200:10,10,400,400:norm'
    $base = @{ Foreground = $fg; Windows = $win; ActiveTabs = @{ w1 = 't1'; w2 = 't9' } }
    function Snap($f, $w, $t) { @{ Foreground = $f; Windows = $w; ActiveTabs = $t } }
    $same = @{ w1 = 't1'; w2 = 't9' }
    $cases = @(
        @('nothing changed', $base, (Snap $fg $win $same), 0),
        @('foreground window changed', $base, (Snap '200|200|210|0|0,0,0,0' $win $same), 1),
        @('only the caret window changed', $base, (Snap '100|100|110|55|1,2,3,4' $win $same), 1),
        @('active tab changed', $base, (Snap $fg $win @{ w1 = 't2'; w2 = 't9' }), 1),
        @('a workspace appeared', $base, (Snap $fg $win @{ w1 = 't1'; w2 = 't9'; w3 = 't5' }), 1),
        @('a visible window opened without focus', $base, (Snap $fg ($win + ';300:0,0,100,100:norm') $same), 1),
        @('z-order changed', $base, (Snap $fg '200:10,10,400,400:norm;100:0,0,500,500:norm' $same), 1),
        @('a minimised window was restored', (Snap $fg '100:0,0,500,500:min;200:10,10,400,400:norm' $same), (Snap $fg '100:0,0,500,500:norm;200:10,10,400,400:norm' $same), 1),
        @('foreground probe failed in both snapshots', (Snap 'ERR:GetGUIThreadInfo failed' $win $same), (Snap 'ERR:GetGUIThreadInfo failed' $win $same), 2),
        @('empty foreground', (Snap '' $win $same), (Snap '' $win $same), 2),
        @('no workspaces listed (an error body) in both', (Snap $fg $win @{}), (Snap $fg $win @{}), 2),
        @('no window list in both', (Snap $fg '' $same), (Snap $fg '' $same), 2),
        @('everything changed', $base, (Snap '1|1|1|1|1,1,1,1' '9:0,0,1,1:norm' @{ w1 = 'x'; w2 = 'y' }), 4))
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

if (-not $Live) { Write-Error 'give -SelfTest or -Live'; exit 2 }
if (-not $Step -or -not $Result) { Write-Error '-Live needs both -Step and -Result: without them nothing is done or checked'; exit 2 }

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
# A listing that failed or holds no workspace gives an empty map, which Compare-Snapshots turns into a red probe.
function Get-Snapshot {
    $tabs = @{}
    $r = Call '/v1/list' @{}
    if ($r.Status -eq 200) {
        $tree = $r.Text | ConvertFrom-Json
        foreach ($ws in @($tree.workspaces)) { if ($ws.workspaceid) { $tabs[$ws.workspaceid] = $ws.activetabid } }
    }
    @{ Foreground = [FgProbe]::Read(); Windows = [FgProbe]::Windows(); ActiveTabs = $tabs }
}

$before = Get-Snapshot
& $Step
$after = Get-Snapshot
$why = @(Compare-Snapshots $before $after)
$blank = [bool](& $Result)
foreach ($w in $why) { "RED $w" }
if ($blank) { 'RED the result is blank' }
if (-not $why -and -not $blank) { 'GREEN foreground, caret, visible windows and active tabs unchanged; result not blank' }
exit $(if ($why -or $blank) { 1 } else { 0 })
