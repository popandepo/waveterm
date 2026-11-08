// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;

namespace WaveNative.Contract;

// The stub app: an in-memory model that answers the verbs of cli-contract.md with the rules of its sections 3 and 6. It starts no process and draws
// nothing; tests set what a terminal shows or a page contains through the Block test hooks. The real app must pass the same tests.
public sealed partial class ContractHost
{
    public sealed class FaultSwitches
    {
        // The step after the block exists fails: session start, run and web open answer exit 7 with the ids.
        public bool FailAfterCreate { get; set; }
    }

    readonly object gate = new();
    readonly Dictionary<string, Workspace> workspaces = new();
    readonly Dictionary<string, Tab> tabs = new();
    readonly Dictionary<string, Block> blocks = new();
    readonly Dictionary<string, string> tokens = new();
    readonly Dictionary<string, BadgeState> badges = new();
    long badgeSeq;
    int shotSeq;

    public FaultSwitches Faults { get; } = new();
    public Func<string, bool> DirectoryExists { get; set; } = Directory.Exists;
    public string DataDir { get; }

    public ContractHost(string dataDir) => DataDir = dataDir;

    // ---- setup and inspection (the app's own side; no CLI verb reaches these) ----

    public Workspace CreateWorkspace()
    {
        lock (gate)
        {
            var w = new Workspace();
            workspaces[w.Id] = w;
            return w;
        }
    }

    public Tab CreateTabAsApp(Workspace w, string name)
    {
        lock (gate) return MakeTab(w, name, null);
    }

    // A block started by the app or by the owner in the UI: no createdby, optional capabilities. Returns the block with its token set.
    public Block StartBlockAsApp(Tab tab, string[] argv, string cwd, IEnumerable<string>? caps = null, bool restart = false, string? title = null)
    {
        lock (gate)
        {
            var b = NewTermBlock(tab, argv, cwd, new Dictionary<string, string>(), restart, null, caps ?? Array.Empty<string>());
            b.Title = title;
            AddNewTile(tab, b);
            if (!tabs.ContainsKey(tab.Id)) throw new InvalidOperationException("tab vanished");
            if (workspaces[tab.WorkspaceId].ActiveTabId == null) workspaces[tab.WorkspaceId].ActiveTabId = tab.Id;
            return b;
        }
    }

    // The app (or the owner in the UI) groups blocks of one tab into a stack: the first block stays the shown member.
    public Tile MakeStackAsApp(Tab tab, params string[] blockIds)
    {
        lock (gate)
        {
            var first = TileOf(tab, blockIds[0]) ?? throw new InvalidOperationException("block is not in the tab");
            first.IsStack = true;
            foreach (var id in blockIds.Skip(1))
            {
                var b = blocks[id];
                RemoveFromTiles(tab, b);
                first.Members.Add(id);
                b.TabId = tab.Id;
            }
            return first;
        }
    }

    public Block StartWebBlockAsApp(Tab tab, string url)
    {
        lock (gate)
        {
            var b = new Block { TabId = tab.Id, WorkspaceId = tab.WorkspaceId, View = "web", Url = Validators.Url(url), Status = Status.None };
            b.Partition = "persist:" + b.Id;
            Register(b);
            AddNewTile(tab, b);
            return b;
        }
    }

    // The `manager` config entry: the tab (found by name, else made) gets the manager block with its capabilities. Restart-at-start is inherent.
    public Block ApplyManagerConfig(Workspace w, ManagerConfig cfg)
    {
        lock (gate)
        {
            if (cfg.Session.TryGetValue("session:name", out var mgrName) && mgrName.Length > 0 && blocks.Values.Any(x => x.WorkspaceId == w.Id && x.Session.TryGetValue("session:name", out var m) && string.Equals(m, mgrName, StringComparison.OrdinalIgnoreCase)))
                throw new ConfigException($"manager: a session named \"{mgrName}\" already exists");
            foreach (var areaTab in cfg.Tabs)
                if (!w.Tabs.Any(t => string.Equals(t.Name, areaTab, StringComparison.OrdinalIgnoreCase))) MakeTab(w, areaTab, null);
            var tab = w.Tabs.FirstOrDefault(t => string.Equals(t.Name, cfg.Tab, StringComparison.OrdinalIgnoreCase)) ?? MakeTab(w, cfg.Tab, null);
            var b = NewTermBlock(tab, cfg.Argv.ToArray(), cfg.Cwd, cfg.Env, restart: true, createdBy: null, cfg.Caps);
            foreach (var kv in cfg.Session) b.Session[kv.Key] = kv.Value;
            b.Title = cfg.Name;
            AddNewTile(tab, b);
            if (w.ActiveTabId == null) w.ActiveTabId = tab.Id;
            return b;
        }
    }

    // The app starts again: every token changes, restart blocks run again, other programs are not started in this run. Rights stay in the record.
    public Dictionary<string, string> SimulateAppRestart()
    {
        lock (gate)
        {
            tokens.Clear();
            foreach (var b in blocks.Values)
            {
                b.Token = Guid.NewGuid().ToString("N");
                tokens[b.Token] = b.Id;
                if (b.View == "term")
                {
                    b.Status = b.Restart ? Status.Running : Status.None;
                    b.ExitCode = 0;
                }
            }
            return blocks.Values.ToDictionary(b => b.Id, b => b.Token);
        }
    }

    public void FinishProgram(string blockId, int exitCode)
    {
        lock (gate)
        {
            var b = blocks[blockId];
            b.Status = Status.Done;
            b.ExitCode = exitCode;
        }
    }

    public Block GetBlock(string id) { lock (gate) return blocks[id]; }
    public bool HasBlock(string id) { lock (gate) return blocks.ContainsKey(id); }
    public bool HasTab(string id) { lock (gate) return tabs.ContainsKey(id); }
    public Tab GetTab(string id) { lock (gate) return tabs[id]; }
    public Workspace GetWorkspace(string id) { lock (gate) return workspaces[id]; }
    public IReadOnlyList<Block> AllBlocks() { lock (gate) return blocks.Values.ToList(); }
    public bool IsValidToken(string token) { lock (gate) return tokens.ContainsKey(token); }

    // ---- dispatch ----

    public Reply Handle(Request req)
    {
        lock (gate)
        {
            try
            {
                var caller = Caller(req.Token);
                var a = req.Args;
                JsonNode body = req.Verb switch
                {
                    "whoami" => Whoami(caller),
                    "session start" => SessionStart(caller, a, "session start"),
                    "session list" => SessionList(caller, a),
                    "session close" => SessionClose(caller, a),
                    "tab new" => TabNew(caller, a),
                    "tab rename" => TabRename(caller, a),
                    "tab move" => TabMove(caller, a),
                    "tab style" => TabStyle(caller, a),
                    "tab switch" => TabSwitch(caller, a),
                    "workspace style" => WorkspaceStyle(caller, a),
                    "block list" => BlockList(caller, a),
                    "block delete" => BlockDelete(caller, a),
                    "block shot" => BlockShot(caller, a),
                    "meta get" => MetaGet(caller, a),
                    "meta set" => MetaSet(caller, a, unset: false),
                    "meta unset" => MetaSet(caller, a, unset: true),
                    "run" => SessionStart(caller, a, "run"),
                    "group add" => GroupAdd(caller, a),
                    "group remove" => GroupRemove(caller, a),
                    "group move" => GroupMove(caller, a),
                    "group activate" => GroupActivate(caller, a),
                    "web open" => WebOpen(caller, a),
                    "web text" => WebText(caller, a),
                    "send" => Send(caller, a),
                    "screen" => Screen(caller, a),
                    "attention" => Attention(caller, a),
                    "badge" => Badge(caller, a),
                    _ => throw CliError.Usage($"unknown verb \"{TextSafety.Escape(req.Verb, false)}\""),
                };
                return Reply.Ok(body);
            }
            catch (PartialSuccess p)
            {
                return Reply.Failure(Exit.Partial, p.Message, p.Body);
            }
            catch (CliError e)
            {
                return Reply.Failure(e.Code, e.Message, null, e.Reason);
            }
            catch (Exception e) when (e is not OutOfMemoryException)
            {
                // No exception may leave the host: the caller gets exit 1 and the type, never a secret.
                return Reply.Failure(Exit.Internal, "internal error: " + e.GetType().Name);
            }
        }
    }

    sealed class PartialSuccess : Exception
    {
        public JsonObject Body { get; }
        public PartialSuccess(string message, JsonObject body) : base(message) => Body = body;
    }

    // ---- shared helpers ----

    Block Caller(string? token)
    {
        if (string.IsNullOrEmpty(token) || !tokens.TryGetValue(token, out var id) || !blocks.TryGetValue(id, out var b))
            throw CliError.Denied("no valid token: run this inside a Wave block").WithReason("badtoken");
        return b;
    }

    static string? S(JsonObject a, string k) => a[k] is JsonValue v && v.TryGetValue<string>(out var s) ? s : null;

    static bool B(JsonObject a, string k) => a[k] is JsonValue v && v.TryGetValue<bool>(out var b) && b;

    static int? I(JsonObject a, string k)
    {
        if (a[k] is not JsonValue v) return null;
        if (v.TryGetValue<int>(out var i)) return i;
        if (v.TryGetValue<double>(out var d) && d == Math.Floor(d)) return (int)d;
        throw CliError.Usage($"--{k} must be a whole number");
    }

    static List<string> L(JsonObject a, string k) =>
        a[k] is JsonArray arr ? arr.Select(x => x is JsonValue v && v.TryGetValue<string>(out var s) ? s : throw CliError.Usage($"{k} must be strings")).ToList() : new List<string>();

    void Register(Block b)
    {
        b.Token = Guid.NewGuid().ToString("N");
        blocks[b.Id] = b;
        tokens[b.Token] = b.Id;
    }

    Tab MakeTab(Workspace w, string name, string? createdBy)
    {
        if (name.Length > 0 && w.Tabs.Any(x => string.Equals(x.Name, name, StringComparison.OrdinalIgnoreCase))) throw CliError.Refused($"a tab named \"{TextSafety.Escape(name, false)}\" already exists").WithReason("duplicate");
        var t = new Tab { WorkspaceId = w.Id, Name = name, CreatedBy = createdBy };
        if (string.IsNullOrEmpty(t.Name))
        {
            int max = 0;
            foreach (var x in w.Tabs)
                if (x.Name.StartsWith('T') && int.TryParse(x.Name.AsSpan(1), out var n) && n > max) max = n;
            int next = max == int.MaxValue ? 1 : max + 1;
            while (w.Tabs.Any(x => string.Equals(x.Name, "T" + next, StringComparison.OrdinalIgnoreCase))) next++;
            t.Name = "T" + next;
        }
        w.Tabs.Add(t);
        tabs[t.Id] = t;
        if (w.ActiveTabId == null) w.ActiveTabId = t.Id;
        return t;
    }

    Block NewTermBlock(Tab tab, string[] argv, string cwd, Dictionary<string, string> env, bool restart, string? createdBy, IEnumerable<string> caps)
    {
        var b = new Block { TabId = tab.Id, WorkspaceId = tab.WorkspaceId, View = "term", CreatedBy = createdBy, Argv = argv.ToList(), Cwd = cwd, Env = new(env), Restart = restart, Status = Status.Running };
        foreach (var c in caps) b.Caps.Add(c);
        Register(b);
        return b;
    }

    void AddNewTile(Tab tab, Block b)
    {
        var tile = new Tile { ActiveId = b.Id };
        tile.Members.Add(b.Id);
        tab.Tiles.Add(tile);
        b.TabId = tab.Id;
    }

    Tile? TileOf(Tab tab, string blockId) => tab.Tiles.FirstOrDefault(t => t.Members.Contains(blockId));

    // A new member of an existing tile: hidden unless the caller asked for it and may (cli-contract.md 3.2 CREATE).
    void AddMember(Tab tab, Tile target, Block b, bool show)
    {
        if (!target.IsStack) target.IsStack = true;
        target.Members.Add(b.Id);
        b.TabId = tab.Id;
        if (show) target.ActiveId = b.Id;
    }

    void RemoveFromTiles(Tab tab, Block b)
    {
        var tile = TileOf(tab, b.Id);
        if (tile == null) return;
        int idx = tile.Members.IndexOf(b.Id);
        tile.Members.RemoveAt(idx);
        if (tile.Members.Count == 0) tab.Tiles.Remove(tile);
        // A stack that loses its shown member falls back to its first member (the original tile), never to a hidden page someone else added.
        else if (tile.ActiveId == b.Id) tile.ActiveId = tile.Members[0];
    }

    public const string ContentDenied = "that block is not available to this caller";

    // hideAs: the READ CONTENT verbs answer an unknown, an unreachable and a denied block with one message, so existence does not leak.
    Block ResolveBlock(Block caller, string? id, string? hideAs = null)
    {
        if (string.IsNullOrEmpty(id)) throw CliError.Usage("a block id is required");
        if (id == "this") return caller;
        if (!Validators.IsUuid(id)) throw CliError.Usage($"\"{TextSafety.Escape(id, false)}\" is not a block id (a full lower-case id, or this)");
        if (!blocks.TryGetValue(id, out var b) || !Reaches(caller, b.WorkspaceId)) throw CliError.NotFound(hideAs ?? "block not found");
        return b;
    }

    Tab ResolveTab(Block caller, string? id)
    {
        if (string.IsNullOrEmpty(id)) throw CliError.Usage("a tab id is required");
        if (id == "tab") return tabs[caller.TabId];
        if (!Validators.IsUuid(id)) throw CliError.Usage($"\"{TextSafety.Escape(id, false)}\" is not a tab id (a full lower-case id, or tab)");
        if (!tabs.TryGetValue(id, out var t) || !Reaches(caller, t.WorkspaceId)) throw CliError.NotFound("tab not found");
        return t;
    }

    static bool Reaches(Block caller, string workspaceId) => caller.WorkspaceId == workspaceId || caller.Caps.Contains(Caps.CrossWorkspace);

    bool Created(Block caller, string? createdBy)
    {
        // The chain stops where an ancestor no longer exists: authority never passes up (cli-contract.md 3.2).
        var seen = new HashSet<string>();
        while (createdBy != null && seen.Add(createdBy))
        {
            if (createdBy == caller.Id) return true;
            if (!blocks.TryGetValue(createdBy, out var anc)) return false;
            createdBy = anc.CreatedBy;
        }
        return false;
    }

    bool Owns(Block caller, Block target) => caller.Caps.Contains(Caps.Admin) || target == caller || Created(caller, target.CreatedBy);

    // The tab a session sits in is usually a shared area tab made by the owner or the app: sitting in it grants nothing over it (cli-contract.md 3.2).
    bool OwnsTab(Block caller, Tab tab) => caller.Caps.Contains(Caps.Admin) || Created(caller, tab.CreatedBy);

    void RequireOwn(Block caller, Block target)
    {
        if (!Owns(caller, target)) throw CliError.Denied("that block is not yours: it needs the admin capability").WithReason("notowner");
    }

    void RequireCap(Block caller, string cap, string what)
    {
        if (!caller.Caps.Contains(cap)) throw CliError.Denied($"{what} needs the {cap} capability").WithReason("capability");
    }

    void RequireContentAccess(Block caller, Block target)
    {
        if (!Owns(caller, target)) throw CliError.NotFound(ContentDenied);
    }

    // The capabilities a creator may give (cli-contract.md 3.4): needs grant, only what it holds, never grant itself.
    List<string> CapsToGive(Block caller, List<string> asked)
    {
        if (asked.Count == 0) return new List<string>();
        RequireCap(caller, Caps.Grant, "--cap");
        foreach (var c in asked)
        {
            if (!Caps.All.Contains(c)) throw CliError.Usage($"unknown capability \"{TextSafety.Escape(c, false)}\": focus, admin or crossworkspace");
            if (!Caps.Passable.Contains(c)) throw CliError.Denied($"the capability \"{TextSafety.Escape(c, false)}\" cannot be given with --cap").WithReason("capability");
            if (!caller.Caps.Contains(c)) throw CliError.Denied($"the caller does not hold the {c} capability, so it cannot give it").WithReason("capability");
        }
        return asked.Distinct().ToList();
    }

    // A new top-level tile changes what the tab looks like for everyone in it: only the owner of the tab (or admin) may add one. Anyone may add a hidden stack member.
    void RequireOwnTabForTile(Block caller, Tab tab)
    {
        if (!OwnsTab(caller, tab)) throw CliError.Denied("adding a tile to a tab that is not yours needs the admin capability; use --stack to add a hidden member").WithReason("notowner");
    }

    // session:name is the key scripts close sessions by: one per workspace.
    void RequireUniqueSessionName(string workspaceId, Dictionary<string, string> meta, Block? except)
    {
        if (!meta.TryGetValue("session:name", out var n) || n.Length == 0) return;
        if (blocks.Values.Any(x => x != except && x.WorkspaceId == workspaceId && x.Session.TryGetValue("session:name", out var m) && string.Equals(m, n, StringComparison.OrdinalIgnoreCase)))
            throw CliError.Refused($"a session named \"{TextSafety.Escape(n, false)}\" already exists").WithReason("duplicate");
    }

    // The UI fallback of the manager config entry: the block gets the manager rights and no creator, so no session owns it through a chain.
    public void PromoteToManager(string blockId)
    {
        lock (gate)
        {
            var b = blocks[blockId];
            b.CreatedBy = null;
            b.Caps.Clear();
            b.Caps.Add(Caps.Focus);
            b.Caps.Add(Caps.Admin);
            b.Caps.Add(Caps.Grant);
        }
    }

    JsonObject Whoami(Block caller)
    {
        var caps = new JsonArray();
        foreach (var c in Caps.All) if (caller.Caps.Contains(c)) caps.Add(c);
        return new JsonObject { ["blockid"] = caller.Id, ["tabid"] = caller.TabId, ["workspaceid"] = caller.WorkspaceId, ["caps"] = caps };
    }

    // ---- session start and run (one implementation; `run` starts in an existing tab, `session start` makes a tab) ----

    // Order of checks (cli-contract.md 4.1): usage 2, scope 4, rights 3, restart 5, uniqueness 5. Nothing is created before all of them pass.
    JsonObject SessionStart(Block caller, JsonObject a, string verb)
    {
        bool isRun = verb == "run";
        var argv = L(a, "argv");
        if (argv.Count == 0 || string.IsNullOrEmpty(argv[0])) throw CliError.Usage("no command given: put it after -- or in --args-file");
        var cwd = S(a, "cwd") is string c ? Validators.Cwd(c, DirectoryExists) : caller.Cwd;
        var name = S(a, isRun ? "title" : "name");
        if (name != null) name = isRun ? Validators.Title(name) : Validators.Name(name);
        var color = S(a, "color") is string col ? Validators.Color(col) : null;
        var symbol = S(a, "symbol") is string sym ? Validators.Symbol(sym) : null;
        var env = Validators.EnvPairs(L(a, "env"));
        var meta = Validators.SessionMetaPairs(L(a, "meta"));
        bool restart = B(a, "restart");
        bool activate = B(a, isRun ? "focus" : "activate");
        var stackArg = S(a, "stack");
        var capsAsked = L(a, "cap");
        foreach (var cap in capsAsked)
            if (!Caps.All.Contains(cap)) throw CliError.Usage($"unknown capability \"{TextSafety.Escape(cap, false)}\": focus, admin or crossworkspace");

        Block? stackTarget = stackArg != null ? ResolveBlock(caller, stackArg) : null;
        Tab? tab = null;
        if (isRun)
        {
            tab = ResolveTab(caller, S(a, "tab") ?? "tab");
            if (stackTarget != null && stackTarget.TabId != tab.Id && a["tab"] != null)
                throw CliError.Refused("the stack target is in another tab than --tab").WithReason("othertab");
            if (stackTarget != null) tab = tabs[stackTarget.TabId];
        }
        else if (stackTarget != null) tab = tabs[stackTarget.TabId];

        if (activate) RequireCap(caller, Caps.Focus, isRun ? "--focus" : "--activate");
        var give = CapsToGive(caller, capsAsked);
        if (isRun && stackTarget == null) RequireOwnTabForTile(caller, tab!);

        if (restart) Validators.CheckRestartArgv(argv);
        RequireUniqueSessionName(caller.WorkspaceId, meta, null);
        if (!isRun && stackTarget == null && !string.IsNullOrEmpty(name) && workspaces[caller.WorkspaceId].Tabs.Any(t => string.Equals(t.Name, name, StringComparison.OrdinalIgnoreCase)))
            throw CliError.Refused($"a tab named \"{TextSafety.Escape(name, false)}\" already exists").WithReason("duplicate");

        if (!isRun && stackTarget == null)
        {
            tab = MakeTab(workspaces[caller.WorkspaceId], name ?? "", caller.Id);
            tab.Color = color;
            tab.Symbol = symbol;
            name = null;
        }
        var b = NewTermBlock(tab!, argv.ToArray(), cwd, env, restart, caller.Id, give);
        foreach (var kv in meta) b.Session[kv.Key] = kv.Value;
        if (stackTarget != null || isRun)
        {
            b.Title = name;
            if (stackTarget != null)
            {
                b.IconColor = color;
                b.BorderColor = color;
                b.Icon = symbol;
            }
        }
        var result = new JsonObject { ["tabid"] = tab!.Id, ["blockid"] = b.Id, ["status"] = "ok" };
        if (stackTarget != null)
        {
            AddMember(tab, TileOf(tab, stackTarget.Id)!, b, activate);
            result["stackedwith"] = stackTarget.Id;
        }
        else
        {
            AddNewTile(tab, b);
            if (activate && !isRun) workspaces[tab.WorkspaceId].ActiveTabId = tab.Id;
        }
        if (Faults.FailAfterCreate)
        {
            result["status"] = "partial";
            result["warning"] = "the tab and block exist, but a later step failed";
            throw new PartialSuccess("the block was created but a later step failed", result);
        }
        return result;
    }

    // A program with no recorded end is treated as possibly running: status none (not started in this app run) and unknown count as unfinished.
    static bool Unfinished(Block b) => b.View == "term" && b.Status != Status.Done;
}
