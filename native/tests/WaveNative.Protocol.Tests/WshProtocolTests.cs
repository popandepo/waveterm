// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Diagnostics;
using System.Net.Sockets;
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Protocol;
using Xunit;

namespace WaveNative.Protocol.Tests;

// These run the REAL Go wsh binary against the C# server, so a pass means the existing wsh works unchanged. The binary is looked up in WAVENATIVE_WSH, then in
// dist\bin of this checkout and of the main checkout. A missing binary fails the test (it is not skipped): no binary means nothing was verified.
public sealed class WshProtocolTests : IDisposable
{
    const string BlockId = "11111111-2222-3333-4444-555555555555";
    readonly string sock = Path.Combine(Path.GetTempPath(), "wn-" + Guid.NewGuid().ToString("N")[..8] + ".sock");
    readonly WaveJwt jwt = WaveJwt.Generate();
    readonly SwapTokenStore swaps = new();
    readonly WaveRpcServer server;

    public WshProtocolTests()
    {
        server = new WaveRpcServer(sock, jwt, swaps);
        server.Handle("getmeta", (caller, data) =>
        {
            Assert.Equal("block:" + BlockId, data?["oref"]?.GetValue<string>());
            return Task.FromResult<JsonNode?>(new JsonObject { ["view"] = "term", ["controller"] = "shell", ["caller"] = caller.RouteId });
        });
        server.Start();
    }

    public void Dispose() => server.Dispose();

    static string FindWsh()
    {
        var fromEnv = Environment.GetEnvironmentVariable("WAVENATIVE_WSH");
        if (!string.IsNullOrEmpty(fromEnv) && File.Exists(fromEnv)) return fromEnv;
        for (var dir = new DirectoryInfo(AppContext.BaseDirectory); dir != null; dir = dir.Parent)
        {
            var bin = Path.Combine(dir.FullName, "dist", "bin");
            if (!Directory.Exists(bin)) continue;
            var hit = Directory.GetFiles(bin, "wsh-*-windows.x64.exe").FirstOrDefault();
            if (hit != null) return hit;
        }
        throw new FileNotFoundException("no wsh binary: set WAVENATIVE_WSH to a wsh-*-windows.x64.exe");
    }

    // A shell started inside a live Wave carries WAVETERM_* values that point at that install: none may reach the child.
    static (int Exit, string Out, string Err) RunWsh(IDictionary<string, string> env, params string[] args)
    {
        var psi = new ProcessStartInfo(FindWsh())
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };
        foreach (var k in psi.Environment.Keys.Where(k => k.StartsWith("WAVETERM_", StringComparison.OrdinalIgnoreCase)).ToList())
            psi.Environment.Remove(k);
        foreach (var kv in env) psi.Environment[kv.Key] = kv.Value;
        foreach (var a in args) psi.ArgumentList.Add(a);
        using var p = Process.Start(psi)!;
        var so = p.StandardOutput.ReadToEndAsync();
        var se = p.StandardError.ReadToEndAsync();
        Assert.True(p.WaitForExit(20000), "wsh did not exit in 20 s");
        return (p.ExitCode, so.Result, se.Result);
    }

    [Fact]
    public void Jwt_roundtrip_and_tamper()
    {
        var token = jwt.Sign(new WaveClaims { Sock = "x", RouteId = "proc:a", BlockId = "b" });
        var c = jwt.Validate(token);
        Assert.NotNull(c);
        Assert.Equal("proc:a", c!.RouteId);
        Assert.Equal("b", c.BlockId);
        Assert.Null(WaveJwt.Generate().Validate(token));
        var parts = token.Split('.');
        var forgedBody = Convert.ToBase64String(Encoding.UTF8.GetBytes("{\"iss\":\"waveterm\",\"routeid\":\"proc:evil\"}")).TrimEnd('=').Replace('+', '-').Replace('/', '_');
        Assert.Null(jwt.Validate(parts[0] + "." + forgedBody + "." + parts[2]));
        Assert.Null(jwt.Validate("not.a.token"));
    }

    [Fact]
    public void Wsh_getmeta_authenticates_with_jwt_and_gets_the_answer()
    {
        var token = jwt.Sign(new WaveClaims { Sock = sock, RouteId = "proc:test", BlockId = BlockId });
        var r = RunWsh(new Dictionary<string, string> { ["WAVETERM_JWT"] = token, ["WAVETERM_BLOCKID"] = BlockId }, "getmeta", "-b", "block:" + BlockId);
        Assert.True(r.Exit == 0, $"exit {r.Exit}: {r.Err}");
        Assert.Contains("\"view\"", r.Out);
        Assert.Contains("term", r.Out);
        Assert.Contains("proc:test", r.Out);
    }

    [Fact]
    public void Wsh_with_a_jwt_signed_by_another_key_is_refused()
    {
        var forged = WaveJwt.Generate().Sign(new WaveClaims { Sock = sock, RouteId = "proc:test", BlockId = BlockId });
        var r = RunWsh(new Dictionary<string, string> { ["WAVETERM_JWT"] = forged, ["WAVETERM_BLOCKID"] = BlockId }, "getmeta", "-b", "block:" + BlockId);
        Assert.NotEqual(0, r.Exit);
        Assert.Contains("error authenticating", r.Err + r.Out);
        Assert.DoesNotContain("term", r.Out);
    }

    [Fact]
    public void Wsh_token_exchange_returns_env_and_init_script_once()
    {
        var entry = new SwapEntry
        {
            Token = Guid.NewGuid().ToString(),
            Context = new WaveClaims { Sock = sock, RouteId = "proc:shell1", BlockId = BlockId },
            Env = new Dictionary<string, string> { ["WAVETERM_BLOCKID"] = BlockId, ["WAVETERM_VERSION"] = "9.9.9" },
            ScriptText = "# init-script-marker",
        };
        swaps.Add(entry);
        var packed = entry.PackForClient();
        var first = RunWsh(new Dictionary<string, string>(), "token", packed, "pwsh");
        Assert.True(first.Exit == 0, $"exit {first.Exit}: {first.Err}");
        Assert.Contains("$env:WAVETERM_BLOCKID", first.Out);
        Assert.Contains("$env:WAVETERM_VERSION = \"9.9.9\"", first.Out);
        Assert.Contains("# init-script-marker", first.Out);
        var second = RunWsh(new Dictionary<string, string>(), "token", packed, "pwsh");
        Assert.NotEqual(0, second.Exit);
        Assert.Contains("no token entry found", second.Err + second.Out);
    }

    [Fact]
    public async Task A_link_that_has_not_authenticated_cannot_call_commands()
    {
        using var conn = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        await conn.ConnectAsync(new UnixDomainSocketEndPoint(sock));
        using var stream = new NetworkStream(conn);
        var req = new RpcMessage { Command = "getmeta", ReqId = "r1", Data = new JsonObject { ["oref"] = "block:" + BlockId } };
        await stream.WriteAsync(Encoding.UTF8.GetBytes(req.ToLine() + "\n"));
        using var reader = new StreamReader(stream);
        var line = await reader.ReadLineAsync().WaitAsync(TimeSpan.FromSeconds(10));
        var reply = RpcMessage.Parse(line!)!;
        Assert.Equal("r1", reply.ResId);
        Assert.Contains("not authenticated", reply.Error);
        Assert.Null(reply.Data);
    }

    [Fact]
    public async Task A_second_token_for_another_block_on_the_same_link_is_refused()
    {
        using var conn = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        await conn.ConnectAsync(new UnixDomainSocketEndPoint(sock));
        using var stream = new NetworkStream(conn);
        using var reader = new StreamReader(stream);
        async Task<RpcMessage> Auth(string id, string block)
        {
            var token = jwt.Sign(new WaveClaims { Sock = sock, RouteId = "proc:p", BlockId = block });
            var req = new RpcMessage { Command = "authenticate", ReqId = id, Route = RpcMessage.ControlRoute, Data = JsonValue.Create(token) };
            await stream.WriteAsync(Encoding.UTF8.GetBytes(req.ToLine() + "\n"));
            return RpcMessage.Parse((await reader.ReadLineAsync().WaitAsync(TimeSpan.FromSeconds(10)))!)!;
        }
        var ok = await Auth("a1", BlockId);
        Assert.Null(ok.Error);
        var again = await Auth("a2", BlockId);
        Assert.Null(again.Error);
        var other = await Auth("a3", "99999999-0000-0000-0000-000000000000");
        Assert.Contains("already authenticated", other.Error);
    }
}
