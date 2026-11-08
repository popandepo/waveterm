// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;

namespace WaveNative.Contract;

public static class Caps
{
    public const string Focus = "focus";
    public const string Admin = "admin";
    public const string CrossWorkspace = "crossworkspace";
    public const string Grant = "grant";

    public static readonly string[] All = { Focus, Admin, CrossWorkspace, Grant };

    // What `--cap` may name: grant is never passed on (cli-contract.md 3.4).
    public static readonly string[] Passable = { Focus, Admin, CrossWorkspace };
}

public static class Status
{
    public const string None = "none";
    public const string Init = "init";
    public const string Running = "running";
    public const string Done = "done";
}

public sealed class Workspace
{
    public string Id { get; init; } = Guid.NewGuid().ToString();
    public string? Accent { get; set; }
    public List<Tab> Tabs { get; } = new();
    public string? ActiveTabId { get; set; }
}

public sealed class Tab
{
    public string Id { get; init; } = Guid.NewGuid().ToString();
    public required string WorkspaceId { get; init; }
    public string Name { get; set; } = "";
    public string? Color { get; set; }
    public string? Accent { get; set; }
    public string? Symbol { get; set; }
    public string? CreatedBy { get; init; }
    public List<Tile> Tiles { get; } = new();

    public IEnumerable<string> BlockIds => Tiles.SelectMany(t => t.Members);
}

// A tile is one block, or a stack (one shown member out of several).
public sealed class Tile
{
    public string Id { get; init; } = Guid.NewGuid().ToString();
    public bool IsStack { get; set; }
    public List<string> Members { get; } = new();
    public string ActiveId { get; set; } = "";
}

public sealed class Block
{
    public string Id { get; init; } = Guid.NewGuid().ToString();
    public required string TabId { get; set; }
    public required string WorkspaceId { get; init; }
    public string View { get; init; } = "term";
    public string? CreatedBy { get; set; }
    public HashSet<string> Caps { get; } = new();
    public string Token { get; set; } = "";

    public List<string> Argv { get; init; } = new();
    public string Cwd { get; init; } = "";
    public Dictionary<string, string> Env { get; init; } = new();
    public bool Restart { get; init; }
    public string Status { get; set; } = WaveNative.Contract.Status.Running;
    public int ExitCode { get; set; }

    public string? Title { get; set; }
    public string? Icon { get; set; }
    public string? IconColor { get; set; }
    public string? BorderColor { get; set; }
    public string? Url { get; set; }
    public string? Partition { get; set; }
    public Dictionary<string, string> Session { get; } = new();
    public string? AttentionLevel { get; set; }
    public string? AttentionReason { get; set; }

    // Test hooks: what a terminal shows and what a page contains (selector "" is the whole page).
    public List<string> Screen { get; } = new();
    public Dictionary<string, string> Page { get; } = new();
    public List<byte[]> Input { get; } = new();

    public bool IsRunning => Status is WaveNative.Contract.Status.Init or WaveNative.Contract.Status.Running;
}

public sealed class BadgeState
{
    public long Seq { get; init; }
    public double Priority { get; init; }
    public string Icon { get; init; } = "";
    public string? Color { get; init; }
}

// The `manager` entry of the app's config file (cli-contract.md v0.3, section 3.5): the one way a block gets `grant` and the
// rights it hands on. No CLI verb edits this file.
public sealed class ManagerConfig
{
    public string Name { get; set; } = "manager";
    public string Tab { get; set; } = "Home";
    public string Cwd { get; set; } = "";
    public List<string> Argv { get; set; } = new();
    public Dictionary<string, string> Env { get; set; } = new();
    public List<string> Caps { get; set; } = new() { WaveNative.Contract.Caps.Focus, WaveNative.Contract.Caps.Admin, WaveNative.Contract.Caps.Grant };
    public Dictionary<string, string> Session { get; set; } = new();
    public List<string> Tabs { get; set; } = new();

    public static ManagerConfig Parse(JsonObject manager, Func<string, bool> directoryExists)
    {
        var allowedKeys = new HashSet<string> { "name", "tab", "cwd", "argv", "env", "caps", "session", "tabs" };
        foreach (var kv in manager)
            if (!allowedKeys.Contains(kv.Key)) throw new ConfigException($"manager: unknown key \"{kv.Key}\"");
        var c = new ManagerConfig();
        try
        {
            if (manager.ContainsKey("name")) c.Name = Validators.Name(Str(manager["name"], "name"));
            if (manager.ContainsKey("tab")) c.Tab = Validators.Name(Str(manager["tab"], "tab"), "tab");
            if (c.Name.Length == 0 || c.Tab.Length == 0) throw CliError.Usage("name and tab cannot be empty");
            c.Cwd = Str(manager["cwd"], "cwd");
            Validators.Cwd(c.Cwd, directoryExists);
        }
        catch (CliError e) { throw new ConfigException("manager: " + e.Message); }
        if (manager["argv"] is not JsonArray a || a.Count == 0) throw new ConfigException("manager: argv must be a non-empty array of strings");
        foreach (var x in a) c.Argv.Add(Str(x, "argv"));
        if (string.IsNullOrEmpty(c.Argv[0])) throw new ConfigException("manager: argv[0] is empty");
        if (manager.ContainsKey("env"))
        {
            if (manager["env"] is not JsonObject envObj) throw new ConfigException("manager: env must be an object of strings");
            foreach (var kv in envObj)
            {
                var value = Str(kv.Value, "env");
                try { Validators.EnvKey(kv.Key); } catch (CliError e) { throw new ConfigException("manager: " + e.Message); }
                c.Env[kv.Key] = value;
            }
        }
        if (manager.ContainsKey("caps"))
        {
            if (manager["caps"] is not JsonArray caps) throw new ConfigException("manager: caps must be an array of strings");
            c.Caps = caps.Select(x => Str(x, "caps")).ToList();
            foreach (var cap in c.Caps)
                if (!WaveNative.Contract.Caps.All.Contains(cap)) throw new ConfigException($"manager: unknown capability \"{cap}\"");
            if (c.Caps.Distinct().Count() != c.Caps.Count) throw new ConfigException("manager: a capability is listed twice");
        }
        if (manager.ContainsKey("tabs"))
        {
            if (manager["tabs"] is not JsonArray tabsArr) throw new ConfigException("manager: tabs must be an array of strings");
            try
            {
                foreach (var x in tabsArr)
                {
                    var name = Validators.Name(Str(x, "tabs"), "tab");
                    if (name.Length == 0) throw CliError.Usage("a tab name cannot be empty");
                    if (c.Tabs.Any(t => string.Equals(t, name, StringComparison.OrdinalIgnoreCase))) throw CliError.Usage($"tab \"{name}\" is listed twice");
                    c.Tabs.Add(name);
                }
            }
            catch (CliError e) { throw new ConfigException("manager: " + e.Message); }
        }
        if (manager.ContainsKey("session"))
        {
            if (manager["session"] is not JsonObject s) throw new ConfigException("manager: session must be an object of strings");
            foreach (var kv in s)
                if (!System.Text.RegularExpressions.Regex.IsMatch(kv.Key, @"\A[A-Za-z0-9._-]+\z"))
                    throw new ConfigException("manager: session key \"" + TextSafety.Escape(kv.Key, false) + "\" must match [A-Za-z0-9._-]+");
            try { c.Session = Validators.SessionMetaPairs(s.Select(kv => "session:" + kv.Key + "=" + Str(kv.Value, "session"))); }
            catch (CliError ex) { throw new ConfigException("manager: " + ex.Message); }
        }
        return c;
    }

    static string Str(JsonNode? n, string what) =>
        n is JsonValue v && v.TryGetValue<string>(out var s) ? s : throw new ConfigException($"manager: {what} must be a string");
}

public sealed class ConfigException : Exception
{
    public ConfigException(string message) : base(message) { }
}
