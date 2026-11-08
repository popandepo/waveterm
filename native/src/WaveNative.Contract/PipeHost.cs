// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.IO.Pipes;
using System.Text;

namespace WaveNative.Contract;

// The stub app's end of the pipe: one request line in, one reply line out, one request per connection (cli-contract.md section 1).
// The peer-process check of the real app (the client must be in the block's Job Object) is not here: the stub has no processes.
public sealed class PipeHost : IDisposable
{
    readonly ContractHost host;
    readonly CancellationTokenSource cts = new();

    public string PipeName { get; }

    public PipeHost(ContractHost host, string? pipeName = null)
    {
        this.host = host;
        PipeName = pipeName ?? "wave-stub-" + Guid.NewGuid().ToString("N")[..12];
    }

    public void Start()
    {
        for (int i = 0; i < 4; i++) _ = AcceptLoop();
    }

    public void Dispose() => cts.Cancel();

    async Task AcceptLoop()
    {
        while (!cts.IsCancellationRequested)
        {
            var server = new NamedPipeServerStream(PipeName, PipeDirection.InOut, NamedPipeServerStream.MaxAllowedServerInstances, PipeTransmissionMode.Byte, PipeOptions.Asynchronous);
            try
            {
                await server.WaitForConnectionAsync(cts.Token);
            }
            catch (OperationCanceledException)
            {
                server.Dispose();
                return;
            }
            await Serve(server);
        }
    }

    async Task Serve(NamedPipeServerStream server)
    {
        using (server)
        {
            try
            {
                using var reader = new StreamReader(server, new UTF8Encoding(false), false, 4096, leaveOpen: true);
                var line = await reader.ReadLineAsync(cts.Token);
                if (line == null) return;
                Reply reply;
                try { reply = host.Handle(Request.Parse(line)); }
                catch (System.Text.Json.JsonException) { reply = Reply.Failure(Exit.Usage, "the request is not valid JSON"); }
                var bytes = new UTF8Encoding(false).GetBytes(reply.ToLine() + "\n");
                await server.WriteAsync(bytes, cts.Token);
                await server.FlushAsync(cts.Token);
            }
            catch (Exception e) when (e is IOException or OperationCanceledException or ObjectDisposedException) { }
        }
    }
}
