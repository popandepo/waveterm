// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Diagnostics;
using System.Text.RegularExpressions;

namespace WaveShell;

public sealed class WaveSrv
{
    static readonly Regex EstartRe = new(@"ws:([a-z0-9.:]+) web:([a-z0-9.:]+) version:([a-z0-9.-]+) buildtime:(\d+)");
    readonly TaskCompletionSource<bool> ready = new();
    Process proc;
    public string WsEndpoint, WebEndpoint, Version;
    public string AuthKey = Guid.NewGuid().ToString();
    public Action<string> Log = _ => { };
    public event Action<string> Event;
    public event Action Exited;

    public Task<bool> Ready => ready.Task;

    public void Start(string distDir, string dataDir)
    {
        var exe = Path.Combine(distDir, "bin", "wavesrv.x64.exe");
        var psi = new ProcessStartInfo(exe)
        {
            WorkingDirectory = dataDir,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };
        // A shell started inside a live Wave carries WAVETERM_* and WAVE_* values that point at that install: none may reach the trial server.
        foreach (var k in psi.Environment.Keys.Where(k => k.StartsWith("WAVETERM_", StringComparison.OrdinalIgnoreCase) || k.StartsWith("WAVE_", StringComparison.OrdinalIgnoreCase)).ToList())
            psi.Environment.Remove(k);
        psi.Environment["WAVETERM_AUTH_KEY"] = AuthKey;
        psi.Environment["WAVETERM_DATA_HOME"] = dataDir;
        psi.Environment["WAVETERM_CONFIG_HOME"] = Path.Combine(dataDir, "config");
        psi.Environment["WAVETERM_APP_PATH"] = distDir;
        psi.Environment["WAVETERM_RESOURCES_PATH"] = distDir;
        psi.Environment["WAVETERM_ELECTRONEXECPATH"] = "";
        Directory.CreateDirectory(Path.Combine(dataDir, "config"));
        proc = new Process { StartInfo = psi, EnableRaisingEvents = true };
        proc.OutputDataReceived += (_, e) => { if (e.Data != null) Log("out: " + e.Data); };
        proc.ErrorDataReceived += (_, e) =>
        {
            if (e.Data == null) return;
            if (e.Data.Contains("WAVESRV-ESTART"))
            {
                var m = EstartRe.Match(e.Data);
                if (!m.Success) { ready.TrySetResult(false); return; }
                WsEndpoint = m.Groups[1].Value;
                WebEndpoint = m.Groups[2].Value;
                Version = m.Groups[3].Value;
                ready.TrySetResult(true);
                return;
            }
            if (e.Data.StartsWith("WAVESRV-EVENT:")) { Event?.Invoke(e.Data.Substring("WAVESRV-EVENT:".Length)); return; }
            Log("err: " + e.Data);
        };
        proc.Exited += (_, _) => { ready.TrySetResult(false); Exited?.Invoke(); };
        proc.Start();
        proc.BeginOutputReadLine();
        proc.BeginErrorReadLine();
    }

    public void Stop()
    {
        try { proc?.StandardInput.Close(); } catch { }
        if (proc != null && !proc.WaitForExit(3000)) { try { proc.Kill(); } catch { } }
    }
}
