// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Net.Sockets;
using System.Text;
using System.Text.Json.Nodes;

namespace WaveNative.Protocol;

public sealed record RpcCaller(string RouteId, string? BlockId, string PeerInfo);

public delegate Task<JsonNode?> RpcHandler(RpcCaller caller, JsonNode? data);

// The domain-socket side of wavesrv (pkg/wshutil/wshutil.go handleDomainSocketClient plus the $control commands of wshrouter_controlimpl.go): one JSON message
// per line, a link is untrusted until it authenticates with a JWT or a swap token, and only then may it call commands.
public sealed class WaveRpcServer : IDisposable
{
    readonly string socketPath;
    readonly WaveJwt jwt;
    readonly SwapTokenStore swaps;
    readonly Dictionary<string, RpcHandler> handlers = new();
    readonly CancellationTokenSource cts = new();
    Socket? listener;

    public WaveRpcServer(string socketPath, WaveJwt jwt, SwapTokenStore swaps)
    {
        this.socketPath = socketPath;
        this.jwt = jwt;
        this.swaps = swaps;
    }

    public void Handle(string command, RpcHandler handler) => handlers[command] = handler;

    public void Start()
    {
        if (File.Exists(socketPath)) File.Delete(socketPath);
        listener = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        listener.Bind(new UnixDomainSocketEndPoint(socketPath));
        listener.Listen(32);
        _ = AcceptLoop();
    }

    public void Dispose()
    {
        cts.Cancel();
        listener?.Dispose();
        if (File.Exists(socketPath)) File.Delete(socketPath);
    }

    async Task AcceptLoop()
    {
        while (!cts.IsCancellationRequested)
        {
            Socket conn;
            try { conn = await listener!.AcceptAsync(cts.Token); }
            catch { return; }
            _ = Serve(conn);
        }
    }

    async Task Serve(Socket conn)
    {
        using var stream = new NetworkStream(conn, ownsSocket: true);
        using var reader = new StreamReader(stream, new UTF8Encoding(false));
        var link = new Link();
        var writeLock = new SemaphoreSlim(1, 1);
        async Task Send(RpcMessage m)
        {
            await writeLock.WaitAsync();
            try
            {
                var bytes = Encoding.UTF8.GetBytes(m.ToLine() + "\n");
                await stream.WriteAsync(bytes);
            }
            finally { writeLock.Release(); }
        }
        try
        {
            while (await reader.ReadLineAsync(cts.Token) is { } line)
            {
                if (line.Length == 0) continue;
                RpcMessage? msg;
                try { msg = RpcMessage.Parse(line); }
                catch (System.Text.Json.JsonException) { continue; }
                if (msg == null || !msg.IsRequest) continue;
                var reply = await Dispatch(link, msg);
                if (reply != null && !string.IsNullOrEmpty(msg.ReqId)) await Send(reply);
            }
        }
        catch (Exception e) when (e is IOException or OperationCanceledException or SocketException) { }
    }

    sealed class Link
    {
        public string? RouteId;
        public string? BlockId;
        public string PeerInfo = "";
    }

    async Task<RpcMessage?> Dispatch(Link link, RpcMessage msg)
    {
        var reqId = msg.ReqId ?? "";
        try
        {
            if (msg.Route == RpcMessage.ControlRoute || msg.Route == "$control:root")
                return RpcMessage.Response(reqId, await Control(link, msg));
            if (link.RouteId == null) return RpcMessage.ErrorResponse(reqId, "link is not authenticated");
            if (!handlers.TryGetValue(msg.Command ?? "", out var h))
                return RpcMessage.ErrorResponse(reqId, $"command {msg.Command} not implemented");
            var data = await h(new RpcCaller(link.RouteId, link.BlockId, link.PeerInfo), msg.Data);
            return RpcMessage.Response(reqId, data);
        }
        catch (RpcException e)
        {
            return RpcMessage.ErrorResponse(reqId, e.Message);
        }
    }

    Task<JsonNode?> Control(Link link, RpcMessage msg)
    {
        switch (msg.Command)
        {
            case "authenticate":
                {
                    var claims = jwt.Validate(msg.Data?.GetValue<string>() ?? "")
                        ?? throw new RpcException("error validating token: invalid jwt");
                    if (claims.Router) throw new RpcException("router tokens are not supported on this link");
                    var routeId = claims.GenerateRouteId();
                    if (routeId == "") throw new RpcException("invalid context, must have a routeid");
                    Bind(link, routeId, claims.BlockId);
                    return Task.FromResult<JsonNode?>(new JsonObject { ["routeid"] = routeId });
                }
            case "authenticatetoken":
                {
                    var token = msg.Data?["token"]?.GetValue<string>() ?? "";
                    if (token == "") throw new RpcException("no token in authenticatetoken message");
                    var entry = swaps.Take(token) ?? throw new RpcException("no token entry found");
                    var routeId = entry.Context.GenerateRouteId();
                    if (routeId == "") throw new RpcException("no routeid");
                    Bind(link, routeId, entry.Context.BlockId);
                    var ctx = new JsonObject { ["routeid"] = routeId };
                    if (!string.IsNullOrEmpty(entry.Context.BlockId)) ctx["blockid"] = entry.Context.BlockId;
                    var rtn = new JsonObject { ["routeid"] = routeId, ["rpccontext"] = ctx };
                    if (entry.Env.Count > 0)
                    {
                        var env = new JsonObject();
                        foreach (var kv in entry.Env) env[kv.Key] = kv.Value;
                        rtn["env"] = env;
                    }
                    if (entry.ScriptText != "") rtn["initscripttext"] = entry.ScriptText;
                    return Task.FromResult<JsonNode?>(rtn);
                }
            case "setpeerinfo":
                if (link.RouteId == null) throw new RpcException("link is not authenticated");
                link.PeerInfo = msg.Data?.GetValue<string>() ?? "";
                return Task.FromResult<JsonNode?>(null);
            default:
                throw new RpcException($"control command {msg.Command} not implemented");
        }
    }

    // A link authenticates once: a second token on the same link must name the same route and block (wshrouter_controlimpl.go bindLeafIdentity).
    static void Bind(Link link, string routeId, string? blockId)
    {
        if (link.RouteId != null && (link.RouteId != routeId || link.BlockId != blockId))
            throw new RpcException("this link is already authenticated as another route or block");
        link.RouteId = routeId;
        link.BlockId = blockId;
    }
}

public sealed class RpcException : Exception
{
    public RpcException(string message) : base(message) { }
}
