// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Net.Sockets;
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Protocol;
using Xunit;

namespace WaveNative.Protocol.Tests;

// Raw-link tests: what the server does with input a real client should never send, and with ones it may. No Go binary is involved here.
public sealed class HardeningTests : IDisposable
{
    const string BlockId = "11111111-2222-3333-4444-555555555555";
    readonly string sock = Path.Combine(Path.GetTempPath(), "wh-" + Guid.NewGuid().ToString("N")[..8] + ".sock");
    readonly WaveJwt jwt = WaveJwt.Generate();
    readonly SwapTokenStore swaps = new();
    readonly WaveRpcServer server;

    public HardeningTests()
    {
        server = new WaveRpcServer(sock, jwt, swaps, new WaveRpcServerOptions { MaxLineBytes = 4096, AuthTimeout = TimeSpan.FromSeconds(2) });
        server.Handle("echo", (caller, data) => Task.FromResult<JsonNode?>(new JsonObject { ["route"] = caller.RouteId, ["block"] = caller.BlockId, ["peer"] = caller.PeerInfo }));
        server.Handle("boom", (_, _) => throw new InvalidOperationException("handler failed"));
        server.Handle("slow", async (_, _) => { await Task.Delay(1500); return JsonValue.Create("slow-done"); });
        server.Start();
    }

    public void Dispose() => server.Dispose();

    sealed class RawLink : IDisposable
    {
        readonly Socket conn = new(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        readonly NetworkStream stream;
        readonly StreamReader reader;

        public RawLink(string path)
        {
            conn.Connect(new UnixDomainSocketEndPoint(path));
            stream = new NetworkStream(conn);
            reader = new StreamReader(stream);
        }

        public void SendRaw(string text) => stream.Write(Encoding.UTF8.GetBytes(text));

        public void Send(RpcMessage m) => SendRaw(m.ToLine() + "\n");

        public RpcMessage? Read(int seconds = 10)
        {
            var line = reader.ReadLineAsync().WaitAsync(TimeSpan.FromSeconds(seconds)).GetAwaiter().GetResult();
            return line == null ? null : RpcMessage.Parse(line);
        }

        public RpcMessage Call(string command, JsonNode? data, string? route = null, string id = "r")
        {
            Send(new RpcMessage { Command = command, ReqId = id, Route = route, Data = data });
            return Read()!;
        }

        public void Dispose() { reader.Dispose(); conn.Dispose(); }
    }

    string Token(WaveClaims c) => jwt.Sign(c);

    RpcMessage Auth(RawLink l, WaveClaims c) => l.Call("authenticate", JsonValue.Create(Token(c)), RpcMessage.ControlRoute);

    [Fact]
    public void Expired_wrong_alg_and_wrong_issuer_tokens_are_refused()
    {
        var claims = new WaveClaims { RouteId = "proc:a" };
        Assert.Null(jwt.Validate(jwt.Sign(claims, TimeSpan.FromSeconds(-5))));
        var good = new JsonObject { ["iss"] = "waveterm", ["routeid"] = "proc:a" };
        Assert.NotNull(jwt.Validate(jwt.SignClaims(good)));
        Assert.Null(jwt.Validate(jwt.SignClaims(good, alg: "none")));
        Assert.Null(jwt.Validate(jwt.SignClaims(good, alg: "HS256")));
        Assert.Null(jwt.Validate(jwt.SignClaims(new JsonObject { ["iss"] = "someone-else", ["routeid"] = "proc:a" })));
        Assert.Null(jwt.Validate(jwt.SignClaims(new JsonObject { ["iss"] = "waveterm", ["routeid"] = "proc:a", ["exp"] = 1 })));
        Assert.Null(jwt.Validate("a.b.c"));
        Assert.Null(jwt.Validate(""));
    }

    [Fact]
    public void A_token_without_a_routeid_is_refused_but_a_procroute_token_gets_a_proc_id()
    {
        using var l1 = new RawLink(sock);
        var none = Auth(l1, new WaveClaims { BlockId = BlockId });
        Assert.Contains("must have a routeid", none.Error);
        using var l2 = new RawLink(sock);
        var proc = Auth(l2, new WaveClaims { ProcRoute = true, BlockId = BlockId });
        Assert.Null(proc.Error);
        Assert.StartsWith("proc:", proc.Data!["routeid"]!.GetValue<string>());
    }

    [Fact]
    public void A_router_token_is_refused_on_a_leaf_link()
    {
        using var l = new RawLink(sock);
        var r = Auth(l, new WaveClaims { Router = true });
        Assert.Contains("router tokens", r.Error);
    }

    [Fact]
    public void Control_commands_and_the_root_control_route_need_authentication_where_they_should()
    {
        using var l = new RawLink(sock);
        Assert.Contains("not authenticated", l.Call("setpeerinfo", JsonValue.Create("x"), RpcMessage.ControlRoute).Error);
        Assert.Contains("not authenticated", l.Call("echo", null, "$control:root").Error);
        Assert.Contains("not authenticated", l.Call("echo", null, RpcMessage.ControlRoute).Error);
        Assert.Contains("not authenticated", l.Call("echo", null).Error);
        Assert.Contains("not authenticated", l.Call("echo", null, "wavesrv").Error);
    }

    [Fact]
    public void Bad_data_gets_an_error_reply_and_the_link_stays_usable()
    {
        using var l = new RawLink(sock);
        Assert.Contains("must be a string", l.Call("authenticate", new JsonObject(), RpcMessage.ControlRoute, "a").Error);
        Assert.Contains("must be an object", l.Call("authenticatetoken", JsonValue.Create("abc"), RpcMessage.ControlRoute, "b").Error);
        Assert.Contains("no token", l.Call("authenticatetoken", new JsonObject(), RpcMessage.ControlRoute, "c").Error);
        l.SendRaw("this is not json\n");
        l.SendRaw("\n");
        var ok = Auth(l, new WaveClaims { RouteId = "proc:ok", BlockId = BlockId });
        Assert.Null(ok.Error);
    }

    [Fact]
    public void A_handler_that_throws_gives_an_error_reply_and_later_commands_still_work()
    {
        using var l = new RawLink(sock);
        Auth(l, new WaveClaims { RouteId = "proc:ok", BlockId = BlockId });
        var bad = l.Call("boom", null, null, "x1");
        Assert.Equal("x1", bad.ResId);
        Assert.Contains("internal error in boom", bad.Error);
        Assert.DoesNotContain("handler failed", bad.Error);
        var good = l.Call("echo", null, null, "x2");
        Assert.Equal("proc:ok", good.Data!["route"]!.GetValue<string>());
    }

    [Fact]
    public void A_command_for_another_route_is_refused_and_the_local_route_name_works()
    {
        using var l = new RawLink(sock);
        Auth(l, new WaveClaims { RouteId = "proc:ok" });
        Assert.Contains("route tab:x not found", l.Call("echo", null, "tab:x").Error);
        Assert.Null(l.Call("echo", null, "wavesrv").Error);
    }

    [Fact]
    public void Setpeerinfo_after_authentication_is_seen_by_handlers()
    {
        using var l = new RawLink(sock);
        Auth(l, new WaveClaims { RouteId = "proc:ok", BlockId = BlockId });
        Assert.Null(l.Call("setpeerinfo", JsonValue.Create("domain:block:" + BlockId), RpcMessage.ControlRoute).Error);
        Assert.Equal("domain:block:" + BlockId, l.Call("echo", null).Data!["peer"]!.GetValue<string>());
    }

    [Fact]
    public void After_a_refused_second_identity_the_first_one_still_holds()
    {
        using var l = new RawLink(sock);
        Auth(l, new WaveClaims { RouteId = "proc:ok", BlockId = BlockId });
        Assert.Contains("already authenticated", Auth(l, new WaveClaims { RouteId = "proc:other", BlockId = BlockId }).Error);
        Assert.Contains("already authenticated", Auth(l, new WaveClaims { RouteId = "proc:ok", BlockId = "other-block" }).Error);
        var e = l.Call("echo", null);
        Assert.Equal("proc:ok", e.Data!["route"]!.GetValue<string>());
        Assert.Equal(BlockId, e.Data["block"]!.GetValue<string>());
    }

    [Fact]
    public void A_swap_token_after_a_jwt_on_the_same_link_must_be_the_same_identity()
    {
        var entry = new SwapEntry { Token = Guid.NewGuid().ToString(), Context = new WaveClaims { RouteId = "proc:swap", BlockId = BlockId } };
        swaps.Add(entry);
        using var l = new RawLink(sock);
        Auth(l, new WaveClaims { RouteId = "proc:ok", BlockId = BlockId });
        var r = l.Call("authenticatetoken", new JsonObject { ["token"] = entry.Token }, RpcMessage.ControlRoute);
        Assert.Contains("already authenticated", r.Error);
    }

    [Fact]
    public void A_router_swap_entry_cannot_be_exchanged()
    {
        var entry = new SwapEntry { Token = Guid.NewGuid().ToString(), Context = new WaveClaims { Router = true, RouteId = "x" } };
        swaps.Add(entry);
        using var l = new RawLink(sock);
        Assert.Contains("cannot auth router", l.Call("authenticatetoken", new JsonObject { ["token"] = entry.Token }, RpcMessage.ControlRoute).Error);
    }

    [Fact]
    public void An_expired_swap_token_is_gone_and_an_empty_one_cannot_be_added()
    {
        var entry = new SwapEntry { Token = Guid.NewGuid().ToString(), Context = new WaveClaims { RouteId = "proc:e" }, Expires = DateTimeOffset.UtcNow.AddSeconds(-1) };
        swaps.Add(entry);
        Assert.Null(swaps.Take(entry.Token));
        Assert.Throws<InvalidOperationException>(() => swaps.Add(new SwapEntry { Token = "", Context = new WaveClaims { RouteId = "x" } }));
    }

    [Fact]
    public void A_line_longer_than_the_limit_closes_that_link_and_the_server_keeps_serving()
    {
        using (var l = new RawLink(sock))
        {
            l.SendRaw(new string('a', 10000));
            // shorter than the 2 s auth timeout, so only the line limit can close the link in time
            Assert.Null(l.Read(1));
        }
        using var other = new RawLink(sock);
        Assert.Null(Auth(other, new WaveClaims { RouteId = "proc:ok" }).Error);
    }

    [Fact]
    public void A_link_that_never_authenticates_is_closed_after_the_timeout()
    {
        using var l = new RawLink(sock);
        Assert.Null(l.Read(8));
    }

    [Fact]
    public void A_slow_command_does_not_block_other_commands_on_the_same_link()
    {
        using var l = new RawLink(sock);
        Auth(l, new WaveClaims { RouteId = "proc:ok" });
        l.Send(new RpcMessage { Command = "slow", ReqId = "s1" });
        l.Send(new RpcMessage { Command = "echo", ReqId = "e1" });
        var first = l.Read()!;
        Assert.Equal("e1", first.ResId);
        var second = l.Read()!;
        Assert.Equal("s1", second.ResId);
    }

    [Fact]
    public void Many_links_at_once_all_authenticate()
    {
        var results = Enumerable.Range(0, 40).Select(i => Task.Run(() =>
        {
            using var l = new RawLink(sock);
            return Auth(l, new WaveClaims { RouteId = "proc:" + i }).Data!["routeid"]!.GetValue<string>();
        })).ToArray();
        Task.WaitAll(results, TimeSpan.FromSeconds(30));
        Assert.Equal(Enumerable.Range(0, 40).Select(i => "proc:" + i).OrderBy(x => x), results.Select(r => r.Result).OrderBy(x => x));
    }

    [Fact]
    public void The_socket_file_goes_with_Dispose_and_a_live_socket_is_not_taken_over()
    {
        var path = Path.Combine(Path.GetTempPath(), "wd-" + Guid.NewGuid().ToString("N")[..8] + ".sock");
        var a = new WaveRpcServer(path, jwt, swaps);
        a.Start();
        Assert.True(File.Exists(path));
        var b = new WaveRpcServer(path, jwt, swaps);
        Assert.Throws<IOException>(() => b.Start());
        Assert.True(File.Exists(path));
        a.Dispose();
        Assert.False(File.Exists(path));
        b.Dispose();
    }
}
