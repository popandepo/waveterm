// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;

namespace WaveShell;

static class Program
{
    [STAThread]
    static int Main(string[] args)
    {
        var o = new Dictionary<string, string>();
        for (int i = 0; i < args.Length; i++)
        {
            if (!args[i].StartsWith("--")) continue;
            var k = args[i].Substring(2);
            o[k] = i + 1 < args.Length && !args[i + 1].StartsWith("--") ? args[++i] : "1";
        }
        if (!o.ContainsKey("data-dir") || !o.ContainsKey("dist"))
        {
            File.WriteAllText(Path.Combine(Path.GetTempPath(), "waveshell-usage.txt"), "WaveShell --data-dir <fresh folder> --dist <dir with bin and frontend> [--show] [--probe-file <out>] [--timeout <sec>]");
            return 2;
        }
        Application.EnableVisualStyles();
        var form = new MainForm(o);
        Application.Run(form);
        return form.ExitCode;
    }
}

sealed class MainForm : Form
{
    readonly Dictionary<string, string> opt;
    readonly string dataDir, dist;
    readonly WaveSrv srv = new();
    readonly WebView2 wv = new() { Dock = DockStyle.Fill };
    readonly StreamWriter log;
    readonly HttpClient http = new();
    JsonObject initOpts;
    bool waveReady;
    public int ExitCode = 1;

    public MainForm(Dictionary<string, string> o)
    {
        opt = o;
        dataDir = Path.GetFullPath(o["data-dir"]);
        dist = Path.GetFullPath(o["dist"]);
        Directory.CreateDirectory(dataDir);
        log = new StreamWriter(Path.Combine(dataDir, "shell.log"), true) { AutoFlush = true };
        Text = "Wave (shell spike)";
        Width = 1200; Height = 800;
        if (!o.ContainsKey("show"))
        {
            StartPosition = FormStartPosition.Manual;
            Location = new Point(-32000, -32000);
            ShowInTaskbar = false;
        }
        Controls.Add(wv);
        Load += async (_, _) => { try { await Boot(); } catch (Exception e) { L("boot failed: " + e); Finish(1); } };
        FormClosed += (_, _) => srv.Stop();
    }

    void L(string s)
    {
        lock (log) log.WriteLine($"{DateTime.Now:HH:mm:ss.fff} {s}");
    }

    void Finish(int code)
    {
        ExitCode = code;
        L("exit " + code);
        BeginInvoke(() => Close());
    }

    async Task<JsonNode> Call(string service, string method, params object[] args)
    {
        var body = new JsonObject { ["service"] = service, ["method"] = method, ["args"] = JsonSerializer.SerializeToNode(args) };
        var req = new HttpRequestMessage(HttpMethod.Post, $"http://{srv.WebEndpoint}/wave/service?service={service}&method={method}")
        { Content = new StringContent(body.ToJsonString(), Encoding.UTF8, "application/json") };
        req.Headers.Add("X-AuthKey", srv.AuthKey);
        var resp = await http.SendAsync(req);
        var txt = await resp.Content.ReadAsStringAsync();
        if (!resp.IsSuccessStatusCode) throw new Exception($"{service}.{method} http {(int)resp.StatusCode}");
        var j = JsonNode.Parse(txt);
        if (j?["error"] != null) throw new Exception($"{service}.{method}: {j["error"]}");
        return j?["data"];
    }

    async Task Boot()
    {
        var t0 = DateTime.Now;
        srv.Log = L;
        srv.Exited += () => { L("wavesrv exited"); BeginInvoke(() => Close()); };
        srv.Start(dist, dataDir);
        if (!await srv.Ready || srv.WebEndpoint == null) throw new Exception("no WAVESRV-ESTART");
        L($"estart ws={srv.WsEndpoint} web={srv.WebEndpoint} version={srv.Version} after {(DateTime.Now - t0).TotalMilliseconds:0}ms");

        var client = await Call("client", "GetClientData");
        var clientId = client["oid"].GetValue<string>();
        var winIds = client["windowids"] as JsonArray;
        string windowId;
        if (winIds == null || winIds.Count == 0)
        {
            var w = await Call("window", "CreateWindow", null, "");
            windowId = w["oid"].GetValue<string>();
            L("created window " + windowId);
        }
        else windowId = winIds[0].GetValue<string>();
        var win = await Call("window", "GetWindow", windowId);
        var ws = await Call("workspace", "GetWorkspace", win["workspaceid"].GetValue<string>());
        var tabId = ws["activetabid"].GetValue<string>();
        initOpts = new JsonObject { ["tabId"] = tabId, ["clientId"] = clientId, ["windowId"] = windowId, ["activate"] = true };
        L($"client={clientId} window={windowId} tab={tabId}");

        var env = await CoreWebView2Environment.CreateAsync(null, Path.Combine(dataDir, "webview2"));
        await wv.EnsureCoreWebView2Async(env);
        var core = wv.CoreWebView2;
        core.Settings.AreDevToolsEnabled = true;
        core.Settings.IsStatusBarEnabled = false;
        core.SetVirtualHostNameToFolderMapping("wave.local", Path.Combine(dist, "frontend"), CoreWebView2HostResourceAccessKind.Allow);

        core.AddWebResourceRequestedFilter($"http://{srv.WebEndpoint}/*", CoreWebView2WebResourceContext.All);
        core.AddWebResourceRequestedFilter($"ws://{srv.WsEndpoint}/*", CoreWebView2WebResourceContext.All);
        core.WebResourceRequested += (_, e) =>
        {
            e.Request.Headers.SetHeader("X-AuthKey", srv.AuthKey);
            if (e.Request.Uri.StartsWith("ws:")) L("authkey injected on ws request");
        };

        var consts = new JsonObject
        {
            ["authKey"] = srv.AuthKey, ["isDev"] = false, ["platform"] = "win32",
            ["userName"] = Environment.UserName, ["hostName"] = Environment.MachineName,
            ["dataDir"] = dataDir, ["configDir"] = Path.Combine(dataDir, "config"),
            ["homeDir"] = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            ["about"] = new JsonObject { ["version"] = srv.Version, ["buildTime"] = 0 },
            ["webviewPreload"] = "", ["updaterStatus"] = "up-to-date", ["updaterChannel"] = "latest",
            ["env"] = new JsonObject { ["WAVE_SERVER_WEB_ENDPOINT"] = srv.WebEndpoint, ["WAVE_SERVER_WS_ENDPOINT"] = srv.WsEndpoint },
        };
        var shim = new StreamReader(typeof(MainForm).Assembly.GetManifestResourceStream("shim.js")).ReadToEnd();
        await core.AddScriptToExecuteOnDocumentCreatedAsync("window.__waveConst=" + consts.ToJsonString() + ";\n" + shim);

        core.WebMessageReceived += (_, e) => OnPageMessage(JsonNode.Parse(e.WebMessageAsJson));
        core.NavigationCompleted += (_, e) => L($"navigation completed ok={e.IsSuccess} status={e.WebErrorStatus}");
        await core.CallDevToolsProtocolMethodAsync("Runtime.enable", "{}");
        core.GetDevToolsProtocolEventReceiver("Runtime.exceptionThrown").DevToolsProtocolEventReceived += (_, e) => L("page exception: " + Trunc(e.ParameterObjectAsJson));
        core.GetDevToolsProtocolEventReceiver("Runtime.consoleAPICalled").DevToolsProtocolEventReceived += (_, e) =>
        {
            var j = JsonNode.Parse(e.ParameterObjectAsJson);
            if (j["type"]?.GetValue<string>() is "error" or "warning") L("page console." + j["type"] + ": " + Trunc(j["args"]?.ToJsonString()));
        };
        core.Navigate("https://wave.local/index.html");

        var timeout = int.Parse(opt.GetValueOrDefault("timeout", "0"));
        if (timeout > 0)
        {
            var timer = new System.Windows.Forms.Timer { Interval = timeout * 1000 };
            timer.Tick += (_, _) => { L("timeout, waveReady=" + waveReady); Finish(waveReady ? 0 : 1); };
            timer.Start();
        }
    }

    static string Trunc(string s) => s == null ? "" : s.Length > 600 ? s.Substring(0, 600) + "..." : s;

    void Post(object msg) => wv.CoreWebView2.PostWebMessageAsJson(JsonSerializer.Serialize(msg));

    void OnPageMessage(JsonNode m)
    {
        var t = m["t"]?.GetValue<string>();
        var ch = m["ch"]?.GetValue<string>();
        var args = m["args"] as JsonArray ?? new JsonArray();
        if (t == "invoke")
        {
            L("invoke " + ch);
            Post(new { t = "res", id = m["id"].GetValue<int>(), val = (object)null });
            return;
        }
        switch (ch)
        {
            case "set-window-init-status":
                var status = args[0]?.GetValue<string>();
                L("init status " + status);
                if (status == "ready")
                {
                    Post(new { t = "ev", ch = "wave-init", args = new[] { initOpts } });
                }
                else if (status == "wave-ready")
                {
                    waveReady = true;
                    OnWaveReady();
                }
                break;
            case "fe-log": L("fe-log " + Trunc(args[0]?.ToString())); break;
            case "set-is-active":
            case "increment-term-commands":
            case "set-keyboard-chord-mode":
            case "register-global-webview-keys":
                break;
            default: L("unhandled send " + ch); break;
        }
    }

    void OnWaveReady()
    {
        L("wave-ready");
        if (opt.ContainsKey("probe-file")) _ = Probe();
    }

    // Types a command into the terminal of this trial window only (CDP input, no OS keystrokes), then reads the file it wrote.
    async Task Probe()
    {
        var core = wv.CoreWebView2;
        var outFile = Path.GetFullPath(opt["probe-file"]);
        if (File.Exists(outFile)) File.Delete(outFile);
        await Task.Delay(4000);
        L("probe: typing into the terminal, out=" + outFile);
        await core.ExecuteScriptAsync("document.querySelector('.xterm-helper-textarea, textarea')?.focus()");
        var cmd = $"echo SHELL-PROBE-OK > \"{outFile}\"; wsh version >> \"{outFile}\"";
        await core.CallDevToolsProtocolMethodAsync("Input.insertText", JsonSerializer.Serialize(new { text = cmd }));
        foreach (var type in new[] { "keyDown", "keyUp" })
            await core.CallDevToolsProtocolMethodAsync("Input.dispatchKeyEvent", JsonSerializer.Serialize(new { type, key = "Enter", code = "Enter", windowsVirtualKeyCode = 13, text = type == "keyDown" ? "\r" : (string)null }));
        for (int i = 0; i < 30; i++)
        {
            await Task.Delay(1000);
            if (File.Exists(outFile) && File.ReadAllText(outFile).Contains("wsh")) break;
        }
        var res = File.Exists(outFile) ? File.ReadAllText(outFile) : "";
        L("probe result: " + res.Replace("\r", " ").Replace("\n", " | "));
        Finish(res.Contains("SHELL-PROBE-OK") ? 0 : 1);
    }
}
