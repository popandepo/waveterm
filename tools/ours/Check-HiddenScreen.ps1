# Live check for the local API: every terminal block in a tab that is not the active one must answer /v1/screen with 200
# and lines. Prints counts only, never screen text. Exit 0 when every hidden terminal block answers 200 with lines, 1 otherwise
# (also when there is no hidden terminal block to look at, since then nothing was checked).
param([string]$ApiDir = (Join-Path $env:LOCALAPPDATA 'WaveFork\profile\data\api'), [switch]$SelfTest)

# The number of screen lines an answer holds: 0 for anything but 200, and for a 200 whose lines are missing, null or empty.
function Get-ScreenLineCount([int]$status, [string]$text) {
    if ($status -ne 200) { return 0 }
    $lines = ($text | ConvertFrom-Json).lines
    if ($null -eq $lines) { return 0 }
    return @($lines).Count
}

if ($SelfTest) {
    $cases = @(
        @('200 with lines', 200, '{"lines":["a","b"]}', 2),
        @('200 with one line', 200, '{"lines":["a"]}', 1),
        @('200 with null lines', 200, '{"lines":null}', 0),
        @('200 with an empty list', 200, '{"lines":[]}', 0),
        @('200 without lines', 200, '{}', 0),
        @('409 not loaded', 409, '{"error":"the terminal view of this block is not loaded"}', 0),
        @('500', 500, '{"error":"internal error"}', 0))
    $failed = 0
    foreach ($c in $cases) {
        $got = Get-ScreenLineCount $c[1] $c[2]
        if ($got -eq $c[3]) { "GREEN $($c[0])" } else { $failed++; "RED   $($c[0]): got $got, want $($c[3])" }
    }
    exit $(if ($failed) { 1 } else { 0 })
}

$token = (Get-Content -Raw (Join-Path $ApiDir 'api-token')).Trim()
$sock = Join-Path $ApiDir 'wave-api.sock'
# The connect callback runs on a pool thread that has no PowerShell runspace, so it has to be compiled code, not a script block.
Add-Type -TypeDefinition @'
using System.Net.Http;
using System.Net.Sockets;
public static class UnixHttp
{
    public static HttpClient Make(string path)
    {
        var handler = new SocketsHttpHandler();
        handler.ConnectCallback = async (ctx, ct) =>
        {
            var s = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
            await s.ConnectAsync(new UnixDomainSocketEndPoint(path), ct);
            return new NetworkStream(s, true);
        };
        return new HttpClient(handler);
    }
}
'@
$http = [UnixHttp]::Make($sock)
$http.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token)
function Call($path, $body) {
    $content = [System.Net.Http.StringContent]::new(($body | ConvertTo-Json -Compress), [System.Text.Encoding]::UTF8, 'application/json')
    $resp = $http.PostAsync("http://wave$path", $content).GetAwaiter().GetResult()
    [pscustomobject]@{ Status = [int]$resp.StatusCode; Text = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult() }
}

$tree = (Call '/v1/list' @{}).Text | ConvertFrom-Json
$hidden = @()
foreach ($ws in $tree.workspaces) {
    foreach ($tab in $ws.tabs) {
        if ($tab.tabid -eq $ws.activetabid) { continue }
        $hidden += $tab.blocks | Where-Object { $_.view -eq 'term' }
    }
}
$ok = 0; $bad = 0
foreach ($b in $hidden) {
    $r = Call '/v1/screen' @{ blockid = $b.blockid }
    $n = Get-ScreenLineCount $r.Status $r.Text
    if ($r.Status -eq 200 -and $n -gt 0) { $ok++ } else { $bad++; "block $($b.blockid): status $($r.Status), $n lines" }
}
"hidden terminal blocks: $($hidden.Count), readable: $ok, not readable: $bad"
if ($hidden.Count -eq 0 -or $bad -gt 0) { exit 1 }
