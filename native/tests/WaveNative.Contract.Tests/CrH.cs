// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// Contract section 11 item 5 (manager config entry, 3.5) and the rules of 4.1, 4.5, 4.10, 4.12, 4.13 not covered elsewhere.
static class CrH
{
    public static readonly string Abs = Path.GetTempPath().TrimEnd('\\', '/');

    public static JsonObject A(string[]? argv, params (string Key, object? Value)[] kv)
    {
        var o = World.Args(kv);
        if (argv != null) o["argv"] = new JsonArray(argv.Select(x => (JsonNode?)x).ToArray());
        return o;
    }

    public static string[] Argv(string joined) => joined.Split('|');

    public static (int Blocks, int Tabs, int Tiles) Snap(World w) =>
        (w.Host.AllBlocks().Count, w.Ws.Tabs.Count, w.Ws.Tabs.Sum(t => t.Tiles.Count));

    public static string Kind(Reply r) => r.Err!["error"]!["kind"]!.GetValue<string>();

    public static string Str(Reply r, string key) => World.Out(r)[key]!.GetValue<string>();

    public static JsonObject Cfg(Action<JsonObject>? mut = null)
    {
        var o = new JsonObject { ["cwd"] = Abs, ["argv"] = new JsonArray("pwsh", "-File", "m.ps1") };
        mut?.Invoke(o);
        return o;
    }

    public static ManagerConfig Parse(JsonObject o, Func<string, bool>? exists = null) => ManagerConfig.Parse(o, exists ?? (_ => true));

    public static string Rep(string s, int n) => string.Concat(Enumerable.Repeat(s, n));
}
