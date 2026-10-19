# Live check for the local API: every terminal block in a tab that is not the active one must answer /v1/screen with 200
# and lines. Prints counts only, never screen text. Exit 0 when every hidden terminal block answers 200 with lines, 1 otherwise
# (also when there is no hidden terminal block to look at, since then nothing was checked).
param([string]$ApiDir = (Join-Path $env:LOCALAPPDATA 'WaveFork\profile\data\api'))

$token = (Get-Content -Raw (Join-Path $ApiDir 'api-token')).Trim()
$sock = Join-Path $ApiDir 'wave-api.sock'
$handler = [System.Net.Http.SocketsHttpHandler]::new()
$handler.ConnectCallback = {
    param($ctx, $ct)
    $s = [System.Net.Sockets.Socket]::new([System.Net.Sockets.AddressFamily]::Unix, [System.Net.Sockets.SocketType]::Stream, [System.Net.Sockets.ProtocolType]::Unspecified)
    $s.Connect([System.Net.Sockets.UnixDomainSocketEndPoint]::new($sock))
    [System.Threading.Tasks.ValueTask[System.IO.Stream]]::new([System.Net.Sockets.NetworkStream]::new($s, $true))
}
$http = [System.Net.Http.HttpClient]::new($handler)
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
    $n = if ($r.Status -eq 200) { @(($r.Text | ConvertFrom-Json).lines).Count } else { 0 }
    if ($r.Status -eq 200 -and $n -gt 0) { $ok++ } else { $bad++; "block $($b.blockid): status $($r.Status), $n lines" }
}
"hidden terminal blocks: $($hidden.Count), readable: $ok, not readable: $bad"
if ($hidden.Count -eq 0 -or $bad -gt 0) { exit 1 }
