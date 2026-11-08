// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.IO.Pipes;
using System.Text;

namespace WaveNative.Contract;

// The stub app's end of the pipe: one request line in, one reply line out, one request per connection (cli-contract.md section 1). Connections are served
// concurrently, a client that sends nothing is dropped after ReadTimeout, a line is limited to MaxLineBytes, and no exception leaves a connection.
// The peer-process check of the real app (the client must be in the block's Job Object) is not here: the stub has no processes.
public sealed class PipeHost : IDisposable
{
    readonly ContractHost host;
    readonly CancellationTokenSource cts = new();

    public string PipeName { get; }
    public TimeSpan ReadTimeout { get; set; } = TimeSpan.FromSeconds(5);
    public int MaxLineBytes { get; set; } = 1024 * 1024;

    public PipeHost(ContractHost host, string? pipeName = null)
    {
        this.host = host;
        PipeName = pipeName ?? "wave-stub-" + Guid.NewGuid().ToString("N")[..12];
    }

    public void Start() => _ = AcceptLoop();

    public void Dispose() => cts.Cancel();

    async Task AcceptLoop()
    {
        while (!cts.IsCancellationRequested)
        {
            NamedPipeServerStream server;
            try
            {
                server = new NamedPipeServerStream(PipeName, PipeDirection.InOut, NamedPipeServerStream.MaxAllowedServerInstances, PipeTransmissionMode.Byte, PipeOptions.Asynchronous);
                await server.WaitForConnectionAsync(cts.Token);
            }
            catch (OperationCanceledException)
            {
                return;
            }
            catch (IOException)
            {
                await Task.Delay(20);
                continue;
            }
            _ = Serve(server);
        }
    }

    async Task Serve(NamedPipeServerStream server)
    {
        using (server)
        {
            try
            {
                using var timeout = CancellationTokenSource.CreateLinkedTokenSource(cts.Token);
                timeout.CancelAfter(ReadTimeout);
                var line = await ReadLine(server, timeout.Token);
                if (line == null) return;
                Reply reply;
                try { reply = host.Handle(Request.Parse(line)); }
                catch (System.Text.Json.JsonException) { reply = Reply.Failure(Exit.Usage, "the request is not valid"); }
                var bytes = new UTF8Encoding(false).GetBytes(reply.ToLine() + "\n");
                await server.WriteAsync(bytes, cts.Token);
                await server.FlushAsync(cts.Token);
                server.WaitForPipeDrain();
            }
            catch (Exception e) when (e is IOException or OperationCanceledException or ObjectDisposedException or InvalidDataException)
            {
                // a dropped, slow or oversized client ends its own connection and nothing else
            }
        }
    }

    async Task<string?> ReadLine(Stream s, CancellationToken ct)
    {
        var buf = new byte[4096];
        var ms = new MemoryStream();
        while (true)
        {
            int n = await s.ReadAsync(buf, ct);
            if (n == 0) return ms.Length == 0 ? null : new UTF8Encoding(false).GetString(ms.GetBuffer(), 0, (int)ms.Length);
            int nl = Array.IndexOf(buf, (byte)'\n', 0, n);
            int take = nl >= 0 ? nl : n;
            if (ms.Length + take > MaxLineBytes) throw new InvalidDataException("line too long");
            ms.Write(buf, 0, take);
            if (nl >= 0) return new UTF8Encoding(false).GetString(ms.GetBuffer(), 0, (int)ms.Length).TrimEnd('\r');
        }
    }
}
