// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Collections.Concurrent;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;

namespace WaveNative.Protocol;

// pkg/util/shellutil/tokenswap.go: the shell gets WAVETERM_SWAPTOKEN = base64(json{token, rpccontext}); the server keeps the entry (env and init script) until
// it is exchanged once.
public sealed class SwapEntry
{
    public required string Token { get; init; }
    public required WaveClaims Context { get; init; }
    public Dictionary<string, string> Env { get; init; } = new();
    public string ScriptText { get; init; } = "";
    public DateTimeOffset Expires { get; init; } = DateTimeOffset.UtcNow.AddMinutes(5);

    public string PackForClient()
    {
        var ctx = new JsonObject { ["routeid"] = Context.RouteId ?? "" };
        if (!string.IsNullOrEmpty(Context.Sock)) ctx["sockname"] = Context.Sock;
        if (Context.ProcRoute) ctx["procroute"] = true;
        if (!string.IsNullOrEmpty(Context.BlockId)) ctx["blockid"] = Context.BlockId;
        if (!string.IsNullOrEmpty(Context.Conn)) ctx["conn"] = Context.Conn;
        var obj = new JsonObject { ["token"] = Token, ["rpccontext"] = ctx };
        return Convert.ToBase64String(Encoding.UTF8.GetBytes(obj.ToJsonString()));
    }
}

public sealed class SwapTokenStore
{
    readonly ConcurrentDictionary<string, SwapEntry> entries = new();

    public void Add(SwapEntry entry)
    {
        Sweep();
        if (!entries.TryAdd(entry.Token, entry)) throw new InvalidOperationException("token already exists");
    }

    // One use: a token that was exchanged is gone.
    public SwapEntry? Take(string token)
    {
        Sweep();
        return entries.TryRemove(token, out var e) ? e : null;
    }

    void Sweep()
    {
        var now = DateTimeOffset.UtcNow;
        foreach (var kv in entries)
            if (kv.Value.Expires < now) entries.TryRemove(kv.Key, out _);
    }
}
