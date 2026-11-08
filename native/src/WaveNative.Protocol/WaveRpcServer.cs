// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Collections.Concurrent;
using System.Net.Sockets;
using System.Text;
using System.Text.Json.Nodes;

namespace WaveNative.Protocol;

public sealed record RpcCaller(string RouteId, string? BlockId, string PeerInfo);

public delegate Task<JsonNode?> RpcHandler(RpcCaller caller, JsonNode? data);

public sealed class WaveRpcServerOptions
{
    public int MaxLineBytes { get; init; } = 16 * 1024 * 1024;
    public int MaxConnections { get; init; } = 256;
    public TimeSpan AuthTimeout { get; init; } = TimeSpan.FromSeconds(30);
}

// The domain-socket side of wavesrv (pkg/wshutil/wshutil.go handleDomainSocketClient plus the $control commands of wshrouter_controlimpl.go): one JSON message
// per line, a link is untrusted until it authenticates with a JWT or a swap token, and only then may it call commands. Not built: route forwarding, router
// links, streaming (cont) and cancel; a request for another route than the local one is refused.
public sealed class WaveRpcServer : IDisposable
{
    public const string LocalRoute = "wavesrv";

    readonly string socketPath;
    readonly WaveJwt jwt;
    readonly SwapTokenStore swaps;
    readonly WaveRpcServerOptions options;
    readonly ConcurrentDictionary<string, RpcHandler> handlers = new();
    readonly CancellationTokenSource cts = new();
    Socket? listener;
    SemaphoreSlim? slots;
    bool bound;

    public WaveRpcServer(string socketPath, WaveJwt jwt, SwapTokenStore swaps, WaveRpcServerOptions? options = null)
    {
        this.socketPath = socketPath;
        this.jwt = jwt;
        this.swaps = swaps;
        this.options = options ?? new WaveRpcServerOptions();
    }

    public void Handle(string command, RpcHandler handler) => handlers[command] = handler;

    public void Start()
    {
        if (File.Exists(socketPath))
        {
            if (IsLive(socketPath)) throw new IOException($"socket {socketPath} is in use by another process");
            File.Delete(socketPath);
        }
        slots = new SemaphoreSlim(options.MaxConnections, options.MaxConnections);
        listener = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        listener.Bind(new UnixDomainSocketEndPoint(socketPath));
        bound = true;
        listener.Listen(128);
        _ = AcceptLoop();
    }

    public void Dispose()
    {
        cts.Cancel();
        listener?.Dispose();
        if (bound && File.Exists(socketPath)) File.Delete(socketPath);
    }

    static bool IsLive(string path)
    {
        using var probe = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        try
        {
            probe.Connect(new UnixDomainSocketEndPoint(path));
            return true;
        }
        catch (SocketException)
        {
            return false;
        }
    }

    async Task AcceptLoop()
    {
        while (!cts.IsCancellationRequested)
        {
            Socket conn;
            try
            {
                conn = await listener!.AcceptAsync(cts.Token);
            }
            catch (Exception e) when (e is OperationCanceledException or ObjectDisposedException)
            {
                return;
            }
            catch (SocketException)
            {
                await Task.Delay(50);
                continue;
            }
            if (!slots!.Wait(0))
            {
                conn.Dispose();
                continue;
            }
            _ = Serve(conn);
        }
    }

    async Task Serve(Socket conn)
    {
        try
        {
            using var stream = new NetworkStream(conn, ownsSocket: true);
            using var linkCts = CancellationTokenSource.CreateLinkedTokenSource(cts.Token);
            linkCts.CancelAfter(options.AuthTimeout);
            var link = new Link();
            var writeLock = new SemaphoreSlim(1, 1);
            async Task Send(RpcMessage m)
            {
                await writeLock.WaitAsync();
                try { await stream.WriteAsync(Encoding.UTF8.GetBytes(m.ToLine() + "\n")); }
                finally { writeLock.Release(); }
            }
            var lines = new LineReader(stream, options.MaxLineBytes);
            while (await lines.ReadLineAsync(linkCts.Token) is { } line)
            {
                if (line.Length == 0) continue;
                RpcMessage? msg;
                try { msg = RpcMessage.Parse(line); }
                catch (System.Text.Json.JsonException) { continue; }
                if (msg == null || !msg.IsRequest) continue;
                var isControl = msg.Route == RpcMessage.ControlRoute || msg.Route == "$control:root";
                if (isControl)
                {
                    var reply = await Dispatch(link, msg);
                    if (link.RouteId != null) linkCts.CancelAfter(Timeout.InfiniteTimeSpan);
                    if (!string.IsNullOrEmpty(msg.ReqId)) await Send(reply);
                    continue;
                }
                _ = Task.Run(async () =>
                {
                    var reply = await Dispatch(link, msg);
                    if (string.IsNullOrEmpty(msg.ReqId)) return;
                    try { await Send(reply); }
                    catch (Exception e) when (e is IOException or ObjectDisposedException or SocketException) { }
                });
            }
        }
        catch (Exception e) when (e is IOException or OperationCanceledException or SocketException or ObjectDisposedException or InvalidDataException) { }
        finally
        {
            slots!.Release();
        }
    }

    sealed class Link
    {
        public volatile string? RouteId;
        public string? BlockId;
        public string PeerInfo = "";
    }

    async Task<RpcMessage> Dispatch(Link link, RpcMessage msg)
    {
        var reqId = msg.ReqId ?? "";
        try
        {
            if (msg.Route == RpcMessage.ControlRoute || msg.Route == "$control:root")
                return RpcMessage.Response(reqId, Control(link, msg));
            if (link.RouteId == null) return RpcMessage.ErrorResponse(reqId, "link is not authenticated");
            if (!string.IsNullOrEmpty(msg.Route) && msg.Route != LocalRoute)
                return RpcMessage.ErrorResponse(reqId, $"route {msg.Route} not found");
            if (!handlers.TryGetValue(msg.Command ?? "", out var h))
                return RpcMessage.ErrorResponse(reqId, $"command {msg.Command} not implemented");
            var data = await h(new RpcCaller(link.RouteId, link.BlockId, link.PeerInfo), msg.Data);
            return RpcMessage.Response(reqId, data);
        }
        catch (RpcException e)
        {
            return RpcMessage.ErrorResponse(reqId, e.Message);
        }
        catch (Exception e)
        {
            return RpcMessage.ErrorResponse(reqId, $"internal error in {msg.Command}: {e.GetType().Name}");
        }
    }

    static string StringData(RpcMessage msg) =>
        msg.Data is JsonValue v && v.TryGetValue<string>(out var s) ? s : throw new RpcException($"{msg.Command}: data must be a string");

    static JsonObject ObjectData(RpcMessage msg) =>
        msg.Data as JsonObject ?? throw new RpcException($"{msg.Command}: data must be an object");

    JsonNode? Control(Link link, RpcMessage msg)
    {
        switch (msg.Command)
        {
            case "authenticate":
                {
                    var claims = jwt.Validate(StringData(msg)) ?? throw new RpcException("error validating token: invalid jwt");
                    if (claims.Router) throw new RpcException("router tokens are not supported on this link");
                    var routeId = claims.GenerateRouteId();
                    if (routeId == "") throw new RpcException("invalid context, must have a routeid");
                    Bind(link, routeId, claims.BlockId);
                    return new JsonObject { ["routeid"] = routeId };
                }
            case "authenticatetoken":
                {
                    var token = ObjectData(msg)["token"] is JsonValue tv && tv.TryGetValue<string>(out var t) ? t : "";
                    if (token == "") throw new RpcException("no token in authenticatetoken message");
                    var entry = swaps.Take(token) ?? throw new RpcException("no token entry found");
                    if (entry.Context.Router) throw new RpcException("cannot auth router via token");
                    var routeId = entry.Context.GenerateRouteId();
                    if (routeId == "") throw new RpcException("no routeid");
                    Bind(link, routeId, entry.Context.BlockId);
                    var ctx = new JsonObject { ["routeid"] = routeId };
                    if (!string.IsNullOrEmpty(entry.Context.Sock)) ctx["sockname"] = entry.Context.Sock;
                    if (entry.Context.ProcRoute) ctx["procroute"] = true;
                    if (!string.IsNullOrEmpty(entry.Context.BlockId)) ctx["blockid"] = entry.Context.BlockId;
                    if (!string.IsNullOrEmpty(entry.Context.Conn)) ctx["conn"] = entry.Context.Conn;
                    var rtn = new JsonObject { ["routeid"] = routeId, ["rpccontext"] = ctx };
                    if (entry.Env.Count > 0)
                    {
                        var env = new JsonObject();
                        foreach (var kv in entry.Env) env[kv.Key] = kv.Value;
                        rtn["env"] = env;
                    }
                    if (entry.ScriptText != "") rtn["initscripttext"] = entry.ScriptText;
                    return rtn;
                }
            case "setpeerinfo":
                if (link.RouteId == null) throw new RpcException("link is not authenticated");
                link.PeerInfo = StringData(msg);
                return null;
            default:
                if (link.RouteId == null) throw new RpcException("link is not authenticated");
                throw new RpcException($"control command {msg.Command} not implemented");
        }
    }

    // A link authenticates once: a second token on the same link must name the same route and block (wshrouter_controlimpl.go bindLeafIdentity). A
    // procroute token has no fixed route: the second token would mint another id, so it is refused like any other identity change.
    static void Bind(Link link, string routeId, string? blockId)
    {
        if (link.RouteId != null && (link.RouteId != routeId || link.BlockId != blockId))
            throw new RpcException("this link is already authenticated as another route or block");
        link.BlockId = blockId;
        link.RouteId = routeId;
    }
}

public sealed class RpcException : Exception
{
    public RpcException(string message) : base(message) { }
}

// Reads '\n' terminated lines from a stream and refuses a line longer than the limit (InvalidDataException), so a client cannot make the server buffer without end.
sealed class LineReader
{
    readonly Stream stream;
    readonly int max;
    readonly byte[] buf = new byte[65536];
    int pos, len;
    readonly MemoryStream line = new();

    public LineReader(Stream stream, int maxLineBytes)
    {
        this.stream = stream;
        max = maxLineBytes;
    }

    public async Task<string?> ReadLineAsync(CancellationToken ct)
    {
        while (true)
        {
            if (pos >= len)
            {
                len = await stream.ReadAsync(buf, ct);
                pos = 0;
                if (len == 0) return null;
            }
            var nl = Array.IndexOf(buf, (byte)'\n', pos, len - pos);
            var take = (nl < 0 ? len : nl) - pos;
            if (line.Length + take > max) throw new InvalidDataException("line too long");
            line.Write(buf, pos, take);
            pos += take;
            if (nl < 0) continue;
            pos++;
            var s = Encoding.UTF8.GetString(line.GetBuffer(), 0, (int)line.Length);
            line.SetLength(0);
            return s.TrimEnd('\r');
        }
    }
}
