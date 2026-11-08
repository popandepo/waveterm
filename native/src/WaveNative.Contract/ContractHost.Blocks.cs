// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;

namespace WaveNative.Contract;

public sealed partial class ContractHost
{
    // The readable keys of cli-contract.md section 6 for one block. cmd and cmd:args only for the caller's own, created or (admin) blocks; cmd:env never;
    // url redacted unless fullUrl.
    JsonObject BlockMeta(Block caller, Block b, bool fullUrl)
    {
        var m = new JsonObject { ["view"] = b.View };
        if (b.View == "term")
        {
            if (Owns(caller, b))
            {
                m["cmd"] = b.Argv.FirstOrDefault() ?? "";
                m["cmd:args"] = new JsonArray(b.Argv.Skip(1).Select(x => (JsonNode?)x).ToArray());
                m["cmd:cwd"] = b.Cwd;
            }
            m["cmd:runonstart"] = true;
        }
        else
        {
            if (b.Url != null) m["url"] = fullUrl ? b.Url : Validators.RedactUrl(b.Url);
            if (b.Partition != null) m["web:partition"] = b.Partition;
        }
        if (b.Title != null) m["frame:title"] = TextSafety.Escape(b.Title, false);
        if (b.Icon != null) m["frame:icon"] = b.Icon;
        if (b.IconColor != null) m["icon:color"] = b.IconColor;
        if (b.BorderColor != null) m["frame:bordercolor"] = b.BorderColor;
        foreach (var kv in b.Session) m[kv.Key] = kv.Value;
        if (b.AttentionLevel != null) m["attention:level"] = b.AttentionLevel;
        if (b.AttentionReason != null) m["attention:reason"] = TextSafety.Escape(b.AttentionReason, false);
        return m;
    }

    JsonArray BlockList(Block caller, JsonObject a)
    {
        bool all = B(a, "all");
        if (all) RequireCap(caller, Caps.CrossWorkspace, "--all");
        var tabFilter = S(a, "tab") is string t ? ResolveTab(caller, t) : null;
        var view = S(a, "view");
        if (view != null && view is not ("term" or "web")) throw CliError.Usage("unknown --view: use term or web");
        string? urlNorm = S(a, "url") is string u ? Validators.NormalizeUrl(Validators.Url(u)) : null;
        var rtn = new JsonArray();
        foreach (var w in workspaces.Values.Where(w => all || w.Id == caller.WorkspaceId))
        {
            foreach (var tab in w.Tabs.Where(x => tabFilter == null || x.Id == tabFilter.Id))
            {
                foreach (var tile in tab.Tiles)
                {
                    foreach (var id in tile.Members)
                    {
                        var b = blocks[id];
                        if (view != null && b.View != view) continue;
                        if (urlNorm != null && (b.Url == null || Validators.NormalizeUrl(b.Url) != urlNorm)) continue;
                        var o = new JsonObject { ["blockid"] = b.Id, ["workspaceid"] = w.Id, ["tabid"] = tab.Id, ["view"] = b.View };
                        if (tile.IsStack) o["stack"] = tile.Id;
                        o["meta"] = BlockMeta(caller, b, fullUrl: false);
                        if (b.View == "term" && !Owns(caller, b)) o["hidden"] = new JsonArray("cmd", "cmd:args", "cmd:cwd");
                        rtn.Add(o);
                    }
                }
            }
        }
        return rtn;
    }

    JsonObject BlockDelete(Block caller, JsonObject a)
    {
        var b = ResolveBlock(caller, S(a, "block"));
        RequireOwn(caller, b);
        if (!B(a, "force") && b.View == "term" && b.IsRunning)
            throw CliError.Refused($"a program may still be running in the block (status {b.Status}); deleting it stops the program: use --force to delete it anyway");
        DeleteBlockInternal(b);
        return new JsonObject { ["deleted"] = b.Id };
    }

    // ---- meta ----

    JsonObject MetaGet(Block caller, JsonObject a)
    {
        var items = L(a, "items");
        Block target = caller;
        if (items.Count > 0 && (items[0] == "this" || Validators.IsUuid(items[0])))
        {
            target = ResolveBlock(caller, items[0]);
            items.RemoveAt(0);
        }
        var all = BlockMeta(caller, target, fullUrl: target == caller);
        if (items.Count == 0) return all;
        var o = new JsonObject();
        foreach (var k in items)
            if (all.TryGetPropertyValue(k, out var v) && !o.ContainsKey(k)) o[k] = v?.DeepClone();
        return o;
    }

    JsonObject MetaSet(Block caller, JsonObject a, bool unset)
    {
        var target = ResolveBlock(caller, S(a, "block"));
        RequireOwn(caller, target);
        var changes = new Dictionary<string, string?>();
        if (unset)
        {
            var keys = L(a, "keys");
            if (keys.Count == 0) throw CliError.Usage("no keys given");
            foreach (var k in keys)
                if (!changes.TryAdd(k, null)) throw CliError.Usage($"key {TextSafety.Escape(k, false)} given twice");
        }
        else
        {
            foreach (var p in L(a, "pairs"))
            {
                var i = p.IndexOf('=');
                if (i <= 0) throw CliError.Usage($"invalid pair \"{TextSafety.Escape(p, false)}\": want KEY=VALUE");
                if (!changes.TryAdd(p[..i], p[(i + 1)..])) throw CliError.Usage($"key {TextSafety.Escape(p[..i], false)} given twice");
            }
            if (S(a, "json") is string js)
            {
                JsonObject obj;
                try { obj = JsonNode.Parse(js) as JsonObject ?? throw CliError.Usage("--json must be an object"); }
                catch (System.Text.Json.JsonException) { throw CliError.Usage("--json is not valid JSON"); }
                foreach (var kv in obj)
                {
                    string? v;
                    if (kv.Value == null) v = null;
                    else if (kv.Value is JsonValue jv && jv.TryGetValue<string>(out var sv)) v = sv;
                    else throw CliError.Usage($"the value of {TextSafety.Escape(kv.Key, false)} must be a string or null");
                    if (!changes.TryAdd(kv.Key, v)) throw CliError.Usage($"key {TextSafety.Escape(kv.Key, false)} given twice");
                }
            }
            if (changes.Count == 0) throw CliError.Usage("no keys to set");
        }
        var checkedValues = new Dictionary<string, string?>();
        foreach (var kv in changes)
        {
            if (kv.Value == null)
            {
                if (!Validators.IsSettableMetaKey(kv.Key))
                    throw CliError.Usage($"the key {TextSafety.Escape(kv.Key, false)} cannot be removed with meta");
                if (kv.Key == "url") throw CliError.Usage("url cannot be removed");
                checkedValues[kv.Key] = null;
            }
            else checkedValues[kv.Key] = Validators.SettableMetaValue(kv.Key, kv.Value, target.View);
        }
        if (checkedValues.TryGetValue("session:name", out var newName) && newName != null)
            RequireUniqueSessionName(target.WorkspaceId, new Dictionary<string, string> { ["session:name"] = newName }, target);
        var set = new JsonArray();
        var removed = new JsonArray();
        foreach (var kv in checkedValues)
        {
            ApplyMeta(target, kv.Key, kv.Value);
            if (kv.Value == null) removed.Add(kv.Key); else set.Add(kv.Key);
        }
        return new JsonObject { ["id"] = target.Id, ["set"] = set, ["unset"] = removed };
    }

    static void ApplyMeta(Block b, string key, string? value)
    {
        switch (key)
        {
            case "url": b.Url = value; break;
            case "frame:title": b.Title = value; break;
            case "frame:icon": b.Icon = value; break;
            case "icon:color": b.IconColor = value; break;
            case "frame:bordercolor": b.BorderColor = value; break;
            default:
                if (value == null) b.Session.Remove(key); else b.Session[key] = value;
                break;
        }
    }

    // ---- groups (stacks) ----

    JsonObject StackAnswer(Tile tile, string key, string value)
    {
        var o = new JsonObject { ["stack"] = tile.Id, ["members"] = new JsonArray(tile.Members.Select(x => (JsonNode?)x).ToArray()) };
        o[key] = value;
        return o;
    }

    JsonObject GroupAdd(Block caller, JsonObject a)
    {
        var b = ResolveBlock(caller, S(a, "block"));
        var target = ResolveBlock(caller, S(a, "to") ?? throw CliError.Usage("--to is required: the block whose group to join"));
        bool focus = B(a, "focus");
        if (focus) RequireCap(caller, Caps.Focus, "--focus");
        if (b.Id == target.Id) throw CliError.Refused("a block cannot be grouped with itself");
        if (b.TabId != target.TabId) throw CliError.Refused("both blocks must be in the same tab");
        RequireOwn(caller, b);
        var tab = tabs[b.TabId];
        var from = TileOf(tab, b.Id)!;
        var to = TileOf(tab, target.Id)!;
        if (from == to && to.IsStack) throw CliError.Refused("both blocks are already in the same group");
        RemoveFromTiles(tab, b);
        AddMember(tab, to, b, focus);
        return StackAnswer(to, "status", "ok");
    }

    JsonObject GroupRemove(Block caller, JsonObject a)
    {
        var b = ResolveBlock(caller, S(a, "block"));
        bool focus = B(a, "focus");
        if (focus) RequireCap(caller, Caps.Focus, "--focus");
        RequireOwn(caller, b);
        var tab = tabs[b.TabId];
        var tile = TileOf(tab, b.Id)!;
        if (!tile.IsStack || tile.Members.Count < 2) throw CliError.Refused("the block is not in a group");
        RequireOwnTabForTile(caller, tab);
        RemoveFromTiles(tab, b);
        var nt = new Tile { ActiveId = b.Id };
        nt.Members.Add(b.Id);
        tab.Tiles.Insert(tab.Tiles.IndexOf(tile) + 1, nt);
        return new JsonObject { ["unstacked"] = b.Id, ["status"] = "ok" };
    }

    JsonObject GroupMove(Block caller, JsonObject a)
    {
        var from = ResolveBlock(caller, S(a, "from") ?? throw CliError.Usage("--from is required"));
        var to = ResolveBlock(caller, S(a, "to") ?? throw CliError.Usage("--to is required"));
        bool focus = B(a, "focus");
        if (focus) RequireCap(caller, Caps.Focus, "--focus");
        if (from.Id == to.Id) throw CliError.Refused("--from and --to are the same block");
        if (from.TabId != to.TabId) throw CliError.Refused("both blocks must be in the same tab");
        var tab = tabs[from.TabId];
        var fromTile = TileOf(tab, from.Id)!;
        var toTile = TileOf(tab, to.Id)!;
        if (fromTile == toTile) throw CliError.Refused("both blocks are already in the same group");
        var moving = fromTile.IsStack ? fromTile.Members.Select(id => blocks[id]).ToList() : new List<Block> { from };
        foreach (var m in moving) RequireOwn(caller, m);
        var activeBefore = fromTile.ActiveId;
        foreach (var m in moving)
        {
            RemoveFromTiles(tab, m);
            AddMember(tab, toTile, m, false);
        }
        if (focus) toTile.ActiveId = moving.Any(m => m.Id == activeBefore) ? activeBefore : from.Id;
        return StackAnswer(toTile, "status", "ok");
    }

    JsonObject GroupActivate(Block caller, JsonObject a)
    {
        RequireCap(caller, Caps.Focus, "group activate");
        var b = ResolveBlock(caller, S(a, "block"));
        var tile = TileOf(tabs[b.TabId], b.Id)!;
        if (!tile.IsStack || tile.Members.Count < 2) throw CliError.Refused("the block is not in a group");
        tile.ActiveId = b.Id;
        return new JsonObject { ["active"] = b.Id };
    }
}
