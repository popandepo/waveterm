// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;
using WaveNative.Contract;

namespace WaveNative.StubApp;

// A runnable stub app for trying scripts against the contract: pipe host plus a world shaped like the owner's layout, world.json written for the scripts.
// It exits when stdin closes, when the file `stop` appears in the data folder, or after --exit-after seconds. No real program is started.
static class Program
{
    static int Main(string[] args)
    {
        string dataDir = Path.Combine(Path.GetTempPath(), "wave-stubapp-" + Guid.NewGuid().ToString("N")[..8]);
        int exitAfter = 0;
        for (int i = 0; i < args.Length; i++)
        {
            if (args[i] == "--data-dir" && i + 1 < args.Length) dataDir = Path.GetFullPath(args[++i]);
            else if (args[i] == "--exit-after" && i + 1 < args.Length) exitAfter = int.Parse(args[++i]);
        }
        Directory.CreateDirectory(dataDir);
        var cwd = Path.Combine(dataDir, "work");
        Directory.CreateDirectory(cwd);

        var host = new ContractHost(dataDir);
        var ws = host.CreateWorkspace();
        var home = host.CreateTabAsApp(ws, "Home");
        var managers = host.CreateTabAsApp(ws, "Managers");
        var tether = host.CreateTabAsApp(ws, "Tether");
        var projects = host.CreateTabAsApp(ws, "Projects");

        var manager = host.StartBlockAsApp(home, new[] { "pwsh", "-NoLogo", "-File", "manager.ps1" }, cwd, new[] { Caps.Focus, Caps.Admin, Caps.Grant }, restart: true, title: "manager");
        manager.Session["session:name"] = "manager";
        manager.Session["session:id"] = Guid.NewGuid().ToString();
        var homeTile = host.StartWebBlockAsApp(home, "http://127.0.0.1:47474/home?t=entrytoken0123456789abcdef01234567");
        homeTile.Title = "Tether";
        homeTile.Page["#line"] = "12 modules";
        homeTile.Page[""] = "Tether home 12 modules";
        var managersFirst = host.StartBlockAsApp(managers, new[] { "pwsh", "-NoLogo" }, cwd, title: "managers-first");
        var tetherFirst = host.StartBlockAsApp(tether, new[] { "pwsh", "-NoLogo" }, cwd, title: "tether-first");
        var projectsFirst = host.StartBlockAsApp(projects, new[] { "pwsh", "-NoLogo" }, cwd, title: "projects-first");
        var s1 = host.StartBlockAsApp(projects, new[] { "pwsh", "-NoLogo", "-File", "s1.ps1" }, cwd, title: "s1");
        s1.Session["session:name"] = "s1";
        s1.Screen.AddRange(new[] { "Accessing workspace:", "  C:\\work", "", "Yes, I trust this folder", "No, exit" });
        var s2 = host.StartBlockAsApp(projects, new[] { "pwsh", "-NoLogo", "-File", "s2.ps1" }, cwd, title: "s2");
        s2.Session["session:name"] = "s2";

        var pipe = new PipeHost(host);
        pipe.Start();

        JsonObject Who(Block b) => new() { ["id"] = b.Id, ["token"] = b.Token, ["tab"] = b.TabId, ["workspace"] = b.WorkspaceId };
        var world = new JsonObject
        {
            ["pipe"] = pipe.PipeName,
            ["dataDir"] = dataDir,
            ["cwd"] = cwd,
            ["workspace"] = ws.Id,
            ["tabs"] = new JsonObject { ["Home"] = home.Id, ["Managers"] = managers.Id, ["Tether"] = tether.Id, ["Projects"] = projects.Id },
            ["blocks"] = new JsonObject
            {
                ["manager"] = Who(manager), ["homeTile"] = Who(homeTile), ["managersFirst"] = Who(managersFirst), ["tetherFirst"] = Who(tetherFirst),
                ["projectsFirst"] = Who(projectsFirst), ["s1"] = Who(s1), ["s2"] = Who(s2),
            },
        };
        File.WriteAllText(Path.Combine(dataDir, "world.json"), world.ToJsonString(new System.Text.Json.JsonSerializerOptions { WriteIndented = true }));
        Console.Out.WriteLine(Path.Combine(dataDir, "world.json"));
        Console.Out.Flush();

        var started = DateTime.UtcNow;
        var stop = Path.Combine(dataDir, "stop");
        var stdinClosed = Task.Run(() => { while (Console.In.ReadLine() != null) { } });
        while (!stdinClosed.IsCompleted && !File.Exists(stop) && (exitAfter == 0 || (DateTime.UtcNow - started).TotalSeconds < exitAfter))
            Thread.Sleep(200);
        pipe.Dispose();
        return 0;
    }
}
