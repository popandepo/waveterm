// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;

namespace WaveNative.Contract;

public sealed partial class ContractHost
{
    // ---- session list and close ----

    JsonArray SessionList(Block caller, JsonObject a)
    {
        bool all = B(a, "all");
        if (all) RequireCap(caller, Caps.CrossWorkspace, "--all");
        var rtn = new JsonArray();
        foreach (var w in workspaces.Values.Where(w => all || w.Id == caller.WorkspaceId))
        {
            for (int i = 0; i < w.Tabs.Count; i++)
            {
                var t = w.Tabs[i];
                var bl = new JsonArray();
                bool running = false;
                foreach (var id in t.BlockIds)
                {
                    var b = blocks[id];
                    var meta = new JsonObject();
                    foreach (var kv in b.Session) meta[kv.Key] = kv.Value;
                    var o = new JsonObject { ["blockid"] = b.Id, ["view"] = b.View };
                    if (b.Title != null) o["title"] = TextSafety.Escape(b.Title, false);
                    o["status"] = b.View == "term" ? b.Status : Status.None;
                    o["exitcode"] = b.ExitCode;
                    o["meta"] = meta;
                    bl.Add(o);
                    running |= b.View == "term" && b.IsRunning;
                }
                var tabObj = new JsonObject
                {
                    ["tabid"] = t.Id, ["name"] = TextSafety.Escape(t.Name, false), ["workspaceid"] = w.Id, ["index"] = i,
                    ["active"] = w.ActiveTabId == t.Id,
                };
                if (t.Color != null) tabObj["color"] = t.Color;
                if (t.Symbol != null) tabObj["symbol"] = t.Symbol;
                tabObj["running"] = running;
                tabObj["blocks"] = bl;
                rtn.Add(tabObj);
            }
        }
        return rtn;
    }

    JsonObject SessionClose(Block caller, JsonObject a)
    {
        var tab = ResolveTab(caller, S(a, "tab"));
        bool force = B(a, "force");
        if (tab.Id == caller.TabId) throw CliError.Refused("this is your own tab: it is not closed from inside");
        var w = workspaces[tab.WorkspaceId];
        if (w.Tabs.Count == 1) throw CliError.Refused("this is the last tab: the window is not closed by a command");
        if (!OwnsTab(caller, tab)) throw CliError.Denied("that tab is not yours: it needs the admin capability");
        var members = tab.BlockIds.Select(id => blocks[id]).ToList();
        foreach (var b in members) RequireOwn(caller, b);
        if (!force)
        {
            var open = members.FirstOrDefault(Unfinished);
            if (open != null) throw CliError.Refused($"block {open.Id} is not known to be finished (status {open.Status}); use --force to close anyway");
        }
        foreach (var b in members) DeleteBlockInternal(b);
        int idx = w.Tabs.IndexOf(tab);
        w.Tabs.Remove(tab);
        tabs.Remove(tab.Id);
        badges.Remove(tab.Id);
        if (w.ActiveTabId == tab.Id) w.ActiveTabId = w.Tabs[Math.Max(0, Math.Min(idx - 1, w.Tabs.Count - 1))].Id;
        return new JsonObject { ["closed"] = tab.Id, ["blocks"] = members.Count };
    }

    void DeleteBlockInternal(Block b)
    {
        if (tabs.TryGetValue(b.TabId, out var tab)) RemoveFromTiles(tab, b);
        tokens.Remove(b.Token);
        blocks.Remove(b.Id);
        badges.Remove(b.Id);
    }

    // ---- tabs and workspace ----

    JsonObject TabNew(Block caller, JsonObject a)
    {
        var name = S(a, "name") is string n ? Validators.Name(n) : "";
        bool activate = B(a, "activate");
        if (activate) RequireCap(caller, Caps.Focus, "--activate");
        var t = MakeTab(workspaces[caller.WorkspaceId], name, caller.Id);
        if (activate) workspaces[t.WorkspaceId].ActiveTabId = t.Id;
        return new JsonObject { ["tabid"] = t.Id, ["status"] = "ok" };
    }

    JsonObject TabRename(Block caller, JsonObject a)
    {
        var t = ResolveTab(caller, S(a, "tab"));
        var name = Validators.Name(S(a, "name") ?? throw CliError.Usage("a name is required"));
        if (name.Length == 0) throw CliError.Usage("name is empty");
        if (!OwnsTab(caller, t)) throw CliError.Denied("that tab is not yours: it needs the admin capability");
        if (workspaces[t.WorkspaceId].Tabs.Any(x => x != t && string.Equals(x.Name, name, StringComparison.OrdinalIgnoreCase))) throw CliError.Refused($"a tab named \"{TextSafety.Escape(name, false)}\" already exists");
        t.Name = name;
        return new JsonObject { ["tabid"] = t.Id, ["name"] = TextSafety.Escape(t.Name, false) };
    }

    JsonObject TabMove(Block caller, JsonObject a)
    {
        var t = ResolveTab(caller, S(a, "tab"));
        var w = workspaces[t.WorkspaceId];
        int index = I(a, "index") ?? throw CliError.Usage("--index is required");
        if (index < 0 || index >= w.Tabs.Count) throw CliError.Usage($"--index must be between 0 and {w.Tabs.Count - 1}");
        if (!OwnsTab(caller, t)) throw CliError.Denied("that tab is not yours: it needs the admin capability");
        w.Tabs.Remove(t);
        w.Tabs.Insert(index, t);
        return new JsonObject { ["tabid"] = t.Id, ["index"] = index };
    }

    JsonObject TabStyle(Block caller, JsonObject a)
    {
        var t = ResolveTab(caller, S(a, "tab") ?? "tab");
        bool clear = B(a, "clear");
        var color = S(a, "color"); var accent = S(a, "accent"); var symbol = S(a, "symbol");
        if (clear && (color != null || accent != null || symbol != null)) throw CliError.Usage("--clear cannot be combined with --color, --symbol or --accent");
        if (!clear && color == null && accent == null && symbol == null) throw CliError.Usage("nothing to set: give --color, --symbol, --accent or --clear");
        string? vc = color != null ? Validators.Color(color) : null;
        string? va = accent != null ? Validators.Color(accent, "accent") : null;
        string? vs = symbol != null ? Validators.Symbol(symbol) : null;
        if (!OwnsTab(caller, t)) throw CliError.Denied("that tab is not yours: it needs the admin capability");
        if (clear) { t.Color = null; t.Accent = null; t.Symbol = null; }
        if (vc != null) t.Color = vc;
        if (va != null) t.Accent = va;
        if (vs != null) t.Symbol = vs;
        var o = new JsonObject { ["tabid"] = t.Id };
        if (t.Color != null) o["color"] = t.Color;
        if (t.Accent != null) o["accent"] = t.Accent;
        if (t.Symbol != null) o["symbol"] = t.Symbol;
        return o;
    }

    JsonObject TabSwitch(Block caller, JsonObject a)
    {
        RequireCap(caller, Caps.Focus, "tab switch");
        var t = ResolveTab(caller, S(a, "tab"));
        workspaces[t.WorkspaceId].ActiveTabId = t.Id;
        return new JsonObject { ["active"] = t.Id };
    }

    JsonObject WorkspaceStyle(Block caller, JsonObject a)
    {
        bool clear = B(a, "clear");
        var accent = S(a, "accent");
        if (clear && accent != null) throw CliError.Usage("--clear cannot be combined with --accent");
        if (!clear && accent == null) throw CliError.Usage("nothing to set: give --accent or --clear");
        var va = accent != null ? Validators.Color(accent, "accent") : null;
        RequireCap(caller, Caps.Admin, "workspace style");
        var w = workspaces[caller.WorkspaceId];
        w.Accent = clear ? null : va;
        var o = new JsonObject { ["workspaceid"] = w.Id };
        if (w.Accent != null) o["accent"] = w.Accent;
        return o;
    }
}
