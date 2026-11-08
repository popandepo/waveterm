// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.IO.Pipes;
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Cli;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// Pins for rules an independent review found missing or untested: pipe robustness, the order of checks, honest runonstart, unfinished programs, anchors, config lookups,
// generated names, badge priorities, tab badges, the shot folder, and the client grammar of every flag.
public sealed class ReviewFixTests : IDisposable
{
    readonly World w = new();

    public void Dispose() => w.Dispose();

    string RawPipe(string line, int readTimeoutMs = 8000)
    {
        using var c = new NamedPipeClientStream(".", w.Pipe.PipeName, PipeDirection.InOut);
        c.Connect(5000);
        var bytes = new UTF8Encoding(false).GetBytes(line + "\n");
        c.Write(bytes, 0, bytes.Length);
        c.Flush();
        using var r = new StreamReader(c, new UTF8Encoding(false));
        var task = r.ReadLineAsync();
        Assert.True(task.Wait(readTimeoutMs), "the host did not answer");
        return task.Result ?? "";
    }

    static int ExitOf(string reply) => JsonNode.Parse(reply)!["exit"]!.GetValue<int>();

    // ---- the pipe host survives bad clients ----

    [Theory]
    [InlineData("{\"verb\":1}")]
    [InlineData("{\"verb\":\"x\",\"token\":5}")]
    [InlineData("not json at all")]
    [InlineData("[1,2,3]")]
    [InlineData("{\"verb\":\"whoami\",\"args\":7}")]
    public void A_malformed_request_gets_an_exit_2_or_3_reply_and_the_host_keeps_serving(string request)
    {
        var code = ExitOf(RawPipe(request));
        Assert.True(code is 2 or 3, "exit was " + code);
        Assert.Equal(0, w.Cli(w.S1, "whoami").Exit);
    }

    [Fact]
    public void Many_idle_connections_do_not_stop_the_host()
    {
        w.Pipe.ReadTimeout = TimeSpan.FromSeconds(2);
        var idle = Enumerable.Range(0, 8).Select(_ =>
        {
            var c = new NamedPipeClientStream(".", w.Pipe.PipeName, PipeDirection.InOut);
            c.Connect(5000);
            return c;
        }).ToList();
        try
        {
            Assert.Equal(0, w.Cli(w.S1, "whoami").Exit);
        }
        finally
        {
            foreach (var c in idle) c.Dispose();
        }
    }

    [Fact]
    public void A_line_over_the_limit_drops_only_that_client()
    {
        w.Pipe.MaxLineBytes = 2048;
        using (var c = new NamedPipeClientStream(".", w.Pipe.PipeName, PipeDirection.InOut))
        {
            c.Connect(5000);
            var big = new byte[10000];
            Array.Fill(big, (byte)'a');
            try { c.Write(big, 0, big.Length); c.Flush(); } catch (IOException) { }
        }
        Assert.Equal(0, w.Cli(w.S1, "whoami").Exit);
    }

    [Fact]
    public void Concurrent_requests_all_get_their_own_answer()
    {
        var tasks = Enumerable.Range(0, 30).Select(i => Task.Run(() =>
        {
            var who = i % 2 == 0 ? w.S1 : w.S2;
            var r = JsonNode.Parse(RawPipe(new Request { Verb = "whoami", Token = who.Token }.ToLine()))!;
            return (r["exit"]!.GetValue<int>(), r["out"]!["blockid"]!.GetValue<string>(), who.Id);
        })).ToArray();
        Task.WaitAll(tasks, TimeSpan.FromSeconds(30));
        foreach (var t in tasks)
        {
            Assert.Equal(0, t.Result.Item1);
            Assert.Equal(t.Result.Item3, t.Result.Item2);
        }
    }

    // ---- order of checks: usage 2, scope 4, rights 3, restart 5, uniqueness 5 ----

    [Fact]
    public void Rights_are_checked_before_the_restart_refusal()
    {
        var r = w.Call(w.S1, "session start", ("argv", new[] { "bash", "-c", "x" }), ("restart", true), ("cap", new[] { "admin" }));
        Assert.Equal(3, r.Exit);
    }

    [Fact]
    public void Scope_is_checked_before_the_restart_refusal()
    {
        var r = w.Call(w.S1, "session start", ("argv", new[] { "bash", "-c", "x" }), ("restart", true), ("stack", Guid.NewGuid().ToString()));
        Assert.Equal(4, r.Exit);
    }

    [Fact]
    public void Usage_is_checked_before_rights_for_an_unknown_cap_name()
    {
        var r = w.Call(w.S1, "session start", ("argv", new[] { "pwsh" }), ("cap", new[] { "bogus" }));
        Assert.Equal(2, r.Exit);
    }

    [Fact]
    public void The_tile_rule_is_checked_before_the_uniqueness_refusal()
    {
        w.Call(w.S2, "meta set", ("block", w.S2.Id), ("pairs", new[] { "session:name=taken" }));
        var r = w.Call(w.S1, "run", ("tab", w.Home.Id), ("argv", new[] { "pwsh" }), ("meta", new[] { "session:name=taken" }));
        Assert.Equal(3, r.Exit);
    }

    // ---- honest runonstart and unfinished programs ----

    [Fact]
    public void Runonstart_is_true_only_for_restart_blocks()
    {
        var list = World.Out(w.Call(w.Manager, "block list")).AsArray();
        bool Flag(string id) => list.First(x => x!["blockid"]!.GetValue<string>() == id)!["meta"]!["cmd:runonstart"]!.GetValue<bool>();
        Assert.True(Flag(w.Manager.Id));
        Assert.False(Flag(w.S1.Id));
    }

    [Fact]
    public void A_terminal_with_status_none_is_not_known_to_be_finished_so_delete_needs_force()
    {
        w.Host.SimulateAppRestart();
        var s2 = w.S2;
        Assert.Equal("none", s2.Status);
        var token = s2.Token;
        var managerToken = w.Manager.Token;
        var r = w.Host.Handle(new Request { Verb = "block delete", Args = World.Args(("block", s2.Id)), Token = managerToken });
        Assert.Equal(5, r.Exit);
        Assert.True(w.Host.HasBlock(s2.Id));
        Assert.NotEqual("", token);
        var forced = w.Host.Handle(new Request { Verb = "block delete", Args = World.Args(("block", s2.Id), ("force", true)), Token = managerToken });
        Assert.Equal(0, forced.Exit);
    }

    [Fact]
    public void A_finished_terminal_is_deleted_without_force_and_a_web_block_never_needs_it()
    {
        w.Host.FinishProgram(w.S2.Id, 0);
        Assert.Equal(0, w.Call(w.Manager, "block delete", ("block", w.S2.Id)).Exit);
        var page = w.Host.StartWebBlockAsApp(w.Projects, "http://127.0.0.1:1/x");
        Assert.Equal(0, w.Call(w.Manager, "block delete", ("block", page.Id)).Exit);
    }

    // ---- anchors ----

    [Theory]
    [InlineData("session:x\n")]
    [InlineData("session:name\n")]
    public void A_key_with_a_trailing_newline_is_refused(string key)
    {
        var r = w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { key + "=v" }));
        Assert.Equal(2, r.Exit);
        Assert.DoesNotContain(key, w.S1.Session.Keys);
    }

    [Fact]
    public void An_id_with_a_trailing_newline_is_a_usage_error_not_a_lookup()
    {
        Assert.Equal(2, w.Call(w.Manager, "block delete", ("block", w.S2.Id + "\n")).Exit);
        Assert.Equal(2, w.Call(w.S1, "session start", ("argv", new[] { "pwsh" }), ("env", new[] { "A\n=v" })).Exit);
    }

    [Fact]
    public void A_symbol_or_colour_with_a_trailing_newline_is_refused_as_given()
    {
        Assert.Equal(2, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "icon:color=red\n" })).Exit);
        Assert.Equal(2, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "frame:icon=ab\n" })).Exit);
    }

    // ---- manager config lookups ----

    [Fact]
    public void The_manager_tab_is_found_case_insensitively_and_a_squatted_name_is_a_config_error()
    {
        using var w2 = new World();
        var cfg = ManagerConfig.Parse(JsonNode.Parse("{\"tab\":\"HOME\",\"cwd\":" + JsonNode.Parse(JsonValue.Create(w2.Cwd)!.ToJsonString())!.ToJsonString() + ",\"argv\":[\"pwsh\"]}")!.AsObject(), Directory.Exists);
        var before = w2.Ws.Tabs.Count;
        var b = w2.Host.ApplyManagerConfig(w2.Ws, cfg);
        Assert.Equal(before, w2.Ws.Tabs.Count);
        Assert.Equal(w2.Home.Id, b.TabId);

        var cfg2 = ManagerConfig.Parse(JsonNode.Parse("{\"cwd\":" + JsonValue.Create(w2.Cwd)!.ToJsonString() + ",\"argv\":[\"pwsh\"],\"session\":{\"name\":\"s1\"}}")!.AsObject(), Directory.Exists);
        w2.Host.Handle(new Request { Verb = "meta set", Args = World.Args(("block", "this"), ("pairs", new[] { "session:name=s1" })), Token = w2.S1.Token });
        Assert.Throws<ConfigException>(() => w2.Host.ApplyManagerConfig(w2.Ws, cfg2));
    }

    // ---- generated names, badges, tab badges, the shot folder ----

    [Fact]
    public void A_generated_tab_name_never_collides_case_insensitively()
    {
        Assert.Equal(0, w.Call(w.Manager, "tab new", ("name", "t1")).Exit);
        var r = w.Call(w.Manager, "tab new");
        Assert.Equal(0, r.Exit);
        var names = w.Ws.Tabs.Select(t => t.Name.ToLowerInvariant()).ToList();
        Assert.Equal(names.Count, names.Distinct().Count());
    }

    [Theory]
    [InlineData("NaN")]
    [InlineData("Infinity")]
    [InlineData("-Infinity")]
    public void A_badge_priority_that_is_not_a_finite_number_is_exit_2(string p)
    {
        Assert.Equal(2, w.Call(w.S1, "badge", ("icon", "star"), ("priority", p)).Exit);
    }

    [Fact]
    public void A_badge_on_a_tab_goes_with_the_tab()
    {
        var made = World.Out(w.Call(w.S1, "session start", ("argv", new[] { "pwsh" })));
        var tabId = made["tabid"]!.GetValue<string>();
        Assert.Equal(0, w.Call(w.S1, "badge", ("icon", "star"), ("tab", tabId)).Exit);
        Assert.NotNull(w.Host.BadgeOf(tabId));
        w.Host.FinishProgram(made["blockid"]!.GetValue<string>(), 0);
        Assert.Equal(0, w.Call(w.S1, "session close", ("tab", tabId)).Exit);
        Assert.Null(w.Host.BadgeOf(tabId));
    }

    [Fact]
    public void A_foreign_file_that_looks_like_a_shot_survives_the_pruning()
    {
        var dir = Path.Combine(w.DataDir, "shots");
        Directory.CreateDirectory(dir);
        var foreign = Path.Combine(dir, "shot-mine.png");
        File.WriteAllText(foreign, "not made by the app");
        for (int i = 0; i < 53; i++) Assert.Equal(0, w.Call(w.Manager, "block shot", ("block", w.S1.Id)).Exit);
        Assert.True(File.Exists(foreign));
        Assert.Equal(50, Directory.GetFiles(dir).Count(f => !f.EndsWith("shot-mine.png")));
    }

    // ---- the client: flags given twice, bad file arguments, stdin as UTF-8 ----

    [Theory]
    [InlineData("tab", "move", "x", "--index", "1", "--index", "2")]
    [InlineData("session", "close", "x", "--force", "--force")]
    [InlineData("screen", "x", "--last", "1", "--last", "2")]
    public void An_int_or_bool_flag_given_twice_is_exit_2(params string[] args)
    {
        var e = Assert.Throws<CliError>(() => ArgParser.Parse(args));
        Assert.Equal(2, e.Code);
    }

    [Fact]
    public void An_empty_file_argument_is_a_usage_error_not_a_crash()
    {
        var (exit, _, stderr) = w.Cli(w.S1, "session", "start", "--args-file", "");
        Assert.Equal(2, exit);
        Assert.Equal("usage", JsonNode.Parse(stderr)!["error"]!["kind"]!.GetValue<string>());
        Assert.Equal(2, w.Cli(w.S1, "meta", "set", "this", "--json-file", "").Exit);
    }

    [Fact]
    public void Stdin_is_read_as_utf8_for_send()
    {
        var (exit, _, _) = w.CliWithEnv(w.S1, null, "caf\u00E9 \U0001F600\n", "send", w.S1.Id, "--stdin");
        Assert.Equal(0, exit);
        Assert.Equal(Encoding.UTF8.GetBytes("caf\u00E9 \U0001F600"), w.S1.Input.Last());
    }

    // ---- the grammar of every flag reaches the right key of the request ----

    [Theory]
    [MemberData(nameof(GrammarCases))]
    public void A_flag_reaches_the_request_under_the_documented_key(string[] args, string key, string expected)
    {
        var req = ArgParser.Parse(args);
        var node = req.Args[key];
        Assert.NotNull(node);
        Assert.Equal(expected, node!.ToJsonString().Trim('"'));
    }

    public static IEnumerable<object[]> GrammarCases()
    {
        object[] C(string key, string expected, params string[] args) => new object[] { args, key, expected };
        yield return C("stack", "B", "session", "start", "--stack", "B", "--", "pwsh");
        yield return C("activate", "true", "session", "start", "--activate", "--", "pwsh");
        yield return C("restart", "true", "session", "start", "--restart", "--", "pwsh");
        yield return C("name", "N", "session", "start", "--name=N", "--", "pwsh");
        yield return C("color", "red", "session", "start", "--color", "red", "--", "pwsh");
        yield return C("symbol", "star", "session", "start", "--symbol", "star", "--", "pwsh");
        yield return C("cwd", "C:/x", "session", "start", "--cwd", "C:/x", "--", "pwsh");
        yield return C("all", "true", "session", "list", "--all");
        yield return C("force", "true", "session", "close", "T", "--force");
        yield return C("tab", "T", "session", "close", "T");
        yield return C("activate", "true", "tab", "new", "--activate");
        yield return C("name", "N", "tab", "new", "--name", "N");
        yield return C("name", "NN", "tab", "rename", "T", "NN");
        yield return C("index", "2", "tab", "move", "T", "--index", "2");
        yield return C("tab", "T", "tab", "style", "--tab", "T", "--color", "red");
        yield return C("accent", "blue", "tab", "style", "--accent", "blue");
        yield return C("clear", "true", "tab", "style", "--clear");
        yield return C("tab", "T", "tab", "switch", "T");
        yield return C("accent", "blue", "workspace", "style", "--accent", "blue");
        yield return C("clear", "true", "workspace", "style", "--clear");
        yield return C("tab", "T", "block", "list", "--tab", "T");
        yield return C("view", "web", "block", "list", "--view", "web");
        yield return C("url", "http://x/", "block", "list", "--url", "http://x/");
        yield return C("all", "true", "block", "list", "--all");
        yield return C("force", "true", "block", "delete", "B", "--force");
        yield return C("block", "B", "block", "shot", "B");
        yield return C("block", "B", "meta", "set", "B", "a=b");
        yield return C("json", "{}", "meta", "set", "B", "--json", "{}");
        yield return C("block", "B", "meta", "unset", "B", "k");
        yield return C("tab", "T", "run", "--tab", "T", "--", "pwsh");
        yield return C("title", "TT", "run", "--title", "TT", "--", "pwsh");
        yield return C("focus", "true", "run", "--focus", "--", "pwsh");
        yield return C("stack", "B", "run", "--stack", "B", "--", "pwsh");
        yield return C("to", "T", "group", "add", "B", "--to", "T");
        yield return C("from", "F", "group", "move", "--from", "F", "--to", "T");
        yield return C("focus", "true", "group", "add", "B", "--to", "T", "--focus");
        yield return C("block", "B", "group", "activate", "B");
        yield return C("url", "http://x/", "web", "open", "http://x/");
        yield return C("title", "T", "web", "open", "http://x/", "--title", "T");
        yield return C("stack", "B", "web", "open", "http://x/", "--stack", "B");
        yield return C("focus", "true", "web", "open", "http://x/", "--focus");
        yield return C("selector", "#line", "web", "text", "B", "--selector", "#line");
        yield return C("maxBytes", "10", "web", "text", "B", "--max-bytes", "10");
        yield return C("delay", "5", "send", "B", "key:enter", "--delay", "5");
        yield return C("raw", "true", "send", "B", "x", "--raw");
        yield return C("last", "3", "screen", "B", "--last", "3");
        yield return C("maxBytes", "99", "screen", "B", "--max-bytes", "99");
        yield return C("level", "need", "attention", "need", "--reason", "x");
        yield return C("reason", "x", "attention", "need", "--reason", "x");
        yield return C("onlyIfLevel", "need", "attention", "clear", "--only-if-level", "need");
        yield return C("onlyIfReason", "why", "attention", "clear", "--only-if-reason", "why");
        yield return C("block", "B", "attention", "info", "--block", "B");
        yield return C("icon", "star", "badge", "star");
        yield return C("priority", "3", "badge", "star", "--priority", "3");
        yield return C("clear", "true", "badge", "--clear");
        yield return C("tab", "T", "badge", "star", "--tab", "T");
    }
}
