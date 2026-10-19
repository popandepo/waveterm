# Live check for the local API: every terminal block in a tab that is not the active one must answer /v1/screen with 200
# and lines. Prints counts only, never screen text. Exit 0 when every hidden terminal block answers 200 with lines, 1 otherwise
# (also when there is no hidden terminal block to look at, since then nothing was checked).
param([string]$ApiDir = (Join-Path $env:LOCALAPPDATA 'WaveFork\profile\data\api'))

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
    $lines = if ($r.Status -eq 200) { ($r.Text | ConvertFrom-Json).lines } else { $null }
    $n = if ($null -eq $lines) { 0 } else { @($lines).Count }
    if ($r.Status -eq 200 -and $n -gt 0) { $ok++ } else { $bad++; "block $($b.blockid): status $($r.Status), $n lines" }
}
"hidden terminal blocks: $($hidden.Count), readable: $ok, not readable: $bad"
if ($hidden.Count -eq 0 -or $bad -gt 0) { exit 1 }
