// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Diagnostics;
using System.Text.Json.Nodes;
using WaveNative.Contract;

namespace WaveNative.Contract.Tests;

// One stub app with a real named pipe, a workspace with three tabs made by the app (Home, Managers, Projects), a manager block in Home (focus, admin, grant,
// restart) and two ordinary sessions S1 and S2 in Projects (no capabilities, no creator). Everything the tests need goes through Call (in process) or Cli
// (the real wave.exe client over the pipe).
public sealed class World : IDisposable
{
    public ContractHost Host { get; }
    public PipeHost Pipe { get; }
    public Workspace Ws { get; }
    public Tab Home { get; }
    public Tab Managers { get; }
    public Tab Projects { get; }
    public Block Manager { get; }
    public Block S1 { get; }
    public Block S2 { get; }
    public string DataDir { get; }
    public string Cwd { get; }

    public World()
    {
        DataDir = Path.Combine(Path.GetTempPath(), "wave-contract-" + Guid.NewGuid().ToString("N")[..10]);
        Cwd = Path.Combine(DataDir, "work");
        Directory.CreateDirectory(Cwd);
        Host = new ContractHost(DataDir);
        Ws = Host.CreateWorkspace();
        Home = Host.CreateTabAsApp(Ws, "Home");
        Managers = Host.CreateTabAsApp(Ws, "Managers");
        Projects = Host.CreateTabAsApp(Ws, "Projects");
        Manager = Host.StartBlockAsApp(Home, new[] { "pwsh", "-NoLogo", "-File", "manager.ps1" }, Cwd, new[] { Caps.Focus, Caps.Admin, Caps.Grant }, restart: true, title: "manager");
        S1 = Host.StartBlockAsApp(Projects, new[] { "pwsh", "-NoLogo", "-File", "s1.ps1" }, Cwd, title: "s1");
        S2 = Host.StartBlockAsApp(Projects, new[] { "pwsh", "-NoLogo", "-File", "s2.ps1" }, Cwd, title: "s2");
        Pipe = new PipeHost(Host);
        Pipe.Start();
    }

    public void Dispose()
    {
        Pipe.Dispose();
        // Named files first, then the folders from the bottom up: no recursive delete.
        try
        {
            foreach (var f in Directory.EnumerateFiles(DataDir, "*", SearchOption.AllDirectories)) File.Delete(f);
            foreach (var dir in Directory.EnumerateDirectories(DataDir, "*", SearchOption.AllDirectories).OrderByDescending(x => x.Length)) Directory.Delete(dir, false);
            Directory.Delete(DataDir, false);
        }
        catch (IOException) { }
    }

    public static JsonObject Args(params (string Key, object? Value)[] kv)
    {
        var o = new JsonObject();
        foreach (var (k, v) in kv)
        {
            o[k] = v switch
            {
                null => null,
                string s => s,
                bool b => b,
                int i => i,
                string[] arr => new JsonArray(arr.Select(x => (JsonNode?)x).ToArray()),
                _ => throw new ArgumentException("unsupported arg type " + v.GetType()),
            };
        }
        return o;
    }

    public Reply Call(Block caller, string verb, JsonObject? args = null) =>
        Host.Handle(new Request { Verb = verb, Args = args ?? new JsonObject(), Token = caller.Token });

    public Reply Call(Block caller, string verb, params (string Key, object? Value)[] kv) => Call(caller, verb, Args(kv));

    public Reply CallNoToken(string verb, JsonObject? args = null) => Host.Handle(new Request { Verb = verb, Args = args ?? new JsonObject() });

    // Runs the real wave.exe client (as `dotnet wave.dll`) against the pipe with the caller's token.
    public (int Exit, string Stdout, string Stderr) Cli(Block? caller, params string[] args) => CliWithEnv(caller, null, null, args);

    public (int Exit, string Stdout, string Stderr) CliWithEnv(Block? caller, string? pipeOverride, string? stdin, params string[] args)
    {
        var psi = new ProcessStartInfo("dotnet")
        {
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            RedirectStandardInput = stdin != null,
            StandardInputEncoding = stdin != null ? new System.Text.UTF8Encoding(false) : null,
            UseShellExecute = false,
            CreateNoWindow = true,
        };
        psi.ArgumentList.Add(Path.Combine(AppContext.BaseDirectory, "wave.dll"));
        foreach (var a in args) psi.ArgumentList.Add(a);
        foreach (var k in psi.Environment.Keys.Where(k => k.StartsWith("WAVE", StringComparison.OrdinalIgnoreCase)).ToList()) psi.Environment.Remove(k);
        if (caller != null) psi.Environment["WAVE_TOKEN"] = caller.Token;
        psi.Environment["WAVE_PIPE"] = pipeOverride ?? Pipe.PipeName;
        using var p = Process.Start(psi)!;
        if (stdin != null)
        {
            p.StandardInput.Write(stdin);
            p.StandardInput.Close();
        }
        var so = p.StandardOutput.ReadToEndAsync();
        var se = p.StandardError.ReadToEndAsync();
        if (!p.WaitForExit(30000)) { p.Kill(); throw new TimeoutException("wave.dll did not exit in 30 s"); }
        return (p.ExitCode, so.Result, se.Result);
    }

    public static JsonNode Out(Reply r) => r.Out ?? throw new InvalidOperationException("no stdout JSON, exit " + r.Exit + " err " + r.Err?.ToJsonString());

    public static string ErrMessage(Reply r) => r.Err?["error"]?["message"]?.GetValue<string>() ?? "";
}
