// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// Per-verb happy paths and every exit code of cli-contract.md section 1 for: whoami, session start/list/close, tab new/rename/move/style/switch,
// workspace style, block list/delete, run, web open, exit 7, and the real client's argument and stream rules. Each test says (in its name or a
// comment) what would make it red.
public sealed partial class VerbTests : IDisposable
{
    readonly World w = new();
    public void Dispose() => w.Dispose();

    static readonly string[] Pwsh = { "pwsh", "-NoLogo" };

    // ---- helpers ----

    static string Str(JsonNode? n) => n!.GetValue<string>();
    static string[] Strs(JsonNode? n) => n!.AsArray().Select(x => x!.GetValue<string>()).ToArray();
    static string[] Names(JsonNode n) => n.AsObject().Select(kv => kv.Key).ToArray();
    static string NewId() => Guid.NewGuid().ToString();

    static void Keys(JsonNode n, params string[] keys) => Assert.Equal(keys.OrderBy(x => x, StringComparer.Ordinal), Names(n).OrderBy(x => x, StringComparer.Ordinal));

    static readonly Dictionary<int, string> KindOf = new() { [2] = "usage", [3] = "denied", [4] = "notfound", [5] = "refused", [6] = "unreachable", [7] = "partial" };

    static JsonNode Ok(Reply r)
    {
        Assert.True(r.Exit == 0, "exit " + r.Exit + " " + r.Err?.ToJsonString());
        Assert.Null(r.Err);
        return r.Out!;
    }

    static void Fail(Reply r, int code)
    {
        Assert.True(r.Exit == code, "wanted exit " + code + " got " + r.Exit + " out " + r.Out?.ToJsonString() + " err " + r.Err?.ToJsonString());
        Assert.Null(r.Out);
        var e = r.Err!["error"]!;
        Keys(e, "code", "kind", "message");
        Assert.Equal(code, e["code"]!.GetValue<int>());
        Assert.Equal(KindOf[code], Str(e["kind"]));
        Assert.False(string.IsNullOrWhiteSpace(Str(e["message"])));
    }

    Reply Start(Block c, params (string, object?)[] extra) =>
        w.Call(c, "session start", World.Args(new (string, object?)[] { ("argv", Pwsh) }.Concat(extra).ToArray()));

    Reply Run(Block c, params (string, object?)[] extra) =>
        w.Call(c, "run", World.Args(new (string, object?)[] { ("argv", Pwsh) }.Concat(extra).ToArray()));

    (string Tab, string Block) Sess(Block c, string? name = null)
    {
        var o = Ok(name == null ? Start(c) : Start(c, ("name", name)));
        return (Str(o["tabid"]), Str(o["blockid"]));
    }

    JsonArray SList(Block c) => Ok(w.Call(c, "session list")).AsArray();
    JsonNode STab(Block c, string tabId) => SList(c).Single(t => Str(t!["tabid"]) == tabId)!;
    JsonArray BList(Block c, params (string, object?)[] kv) => Ok(w.Call(c, "block list", kv)).AsArray();
    JsonNode BEntry(Block c, string blockId) => BList(c).Single(b => Str(b!["blockid"]) == blockId)!;
    string Shown(string tabId, string blockId) => w.Host.GetTab(tabId).Tiles.Single(t => t.Members.Contains(blockId)).ActiveId;
    int Blocks => w.Host.AllBlocks().Count;
    int Tabs => w.Ws.Tabs.Count;
    Block Rec(string blockId) => w.Host.GetBlock(blockId);

    static JsonNode J(string s) => JsonNode.Parse(s)!;

    static void CliFail(string stdout, string stderr, int exit, int code)
    {
        Assert.Equal(code, exit);
        Assert.Equal("", stdout);
        var e = J(stderr)["error"]!;
        Keys(e, "code", "kind", "message");
        Assert.Equal(code, e["code"]!.GetValue<int>());
        Assert.Equal(KindOf[code], Str(e["kind"]));
        Assert.False(string.IsNullOrWhiteSpace(Str(e["message"])));
        Assert.Single(stderr.Trim().Split('\n'));
    }

    JsonNode CliOk((int Exit, string Stdout, string Stderr) r)
    {
        Assert.True(r.Exit == 0, "exit " + r.Exit + " stderr " + r.Stderr);
        Assert.Equal("", r.Stderr);
        return J(r.Stdout);
    }

    string TempFile(string name, byte[] bytes)
    {
        var p = Path.Combine(w.DataDir, name);
        File.WriteAllBytes(p, bytes);
        return p;
    }

    string TempText(string name, string text) => TempFile(name, new UTF8Encoding(false).GetBytes(text));

    // ================= whoami =================

    [Fact]
    public void Whoami_has_exactly_four_fields_and_they_name_the_calling_record()
    {
        var o = Ok(w.Call(w.S1, "whoami"));
        Keys(o, "blockid", "tabid", "workspaceid", "caps");
        Assert.Equal(w.S1.Id, Str(o["blockid"]));
        Assert.Equal(w.Projects.Id, Str(o["tabid"]));
        Assert.Equal(w.Ws.Id, Str(o["workspaceid"]));
        Assert.Equal(JsonValueKind.Array, o["caps"]!.GetValueKind());
    }

    [Fact]
    public void Whoami_of_a_block_given_only_focus_and_admin_lists_those_two_in_contract_order()
    {
        var id = Str(Ok(Start(w.Manager, ("cap", new[] { "admin", "focus" })))["blockid"]);
        var o = Ok(w.Call(Rec(id), "whoami"));
        Assert.Equal(new[] { "focus", "admin" }, Strs(o["caps"]));
    }

    [Fact]
    public void Whoami_never_shows_grant_on_a_block_made_by_cap_because_grant_is_not_passed_on()
    {
        var id = Str(Ok(Start(w.Manager, ("cap", new[] { "focus" })))["blockid"]);
        Assert.DoesNotContain("grant", Strs(Ok(w.Call(Rec(id), "whoami"))["caps"]));
    }

    [Fact]
    public void Whoami_without_a_token_is_exit_3_with_no_stdout()
    {
        Fail(w.CallNoToken("whoami"), 3);
    }

    [Fact]
    public void Whoami_with_a_made_up_token_is_exit_3()
    {
        var r = w.Host.Handle(new Request { Verb = "whoami", Token = "not-a-token" });
        Fail(r, 3);
    }

    [Fact]
    public void Whoami_with_the_token_of_a_deleted_block_is_exit_3_because_the_token_is_revoked()
    {
        var (_, b) = Sess(w.S1);
        var rec = Rec(b);
        Ok(w.Call(w.S1, "block delete", ("block", b), ("force", true)));
        Fail(w.Call(rec, "whoami"), 3);
    }

    // ================= session start =================

    [Fact]
    public void Session_start_happy_path_prints_tabid_blockid_status_ok_and_nothing_else()
    {
        var o = Ok(Start(w.S1, ("name", "alpha")));
        Keys(o, "tabid", "blockid", "status");
        Assert.Equal("ok", Str(o["status"]));
        Assert.True(w.Host.HasTab(Str(o["tabid"])));
        Assert.Equal(Str(o["tabid"]), Rec(Str(o["blockid"])).TabId);
        Assert.Equal("alpha", Str(STab(w.S1, Str(o["tabid"]))["name"]));
    }

    [Fact]
    public void Session_start_without_name_gets_T_n_and_the_next_one_gets_the_next_number()
    {
        var (t1, _) = Sess(w.S1);
        var (t2, _) = Sess(w.S1);
        Assert.Equal("T1", Str(STab(w.S1, t1)["name"]));
        Assert.Equal("T2", Str(STab(w.S1, t2)["name"]));
    }

    [Fact]
    public void Session_start_without_stack_puts_color_and_symbol_on_the_tab_not_the_block()
    {
        var o = Ok(Start(w.S1, ("name", "styled"), ("color", "RED"), ("symbol", "rocket")));
        var t = STab(w.S1, Str(o["tabid"]));
        Assert.Equal("red", Str(t["color"]));
        Assert.Equal("rocket", Str(t["symbol"]));
        var m = BEntry(w.Manager, Str(o["blockid"]))["meta"]!;
        Assert.Null(m["icon:color"]);
        Assert.Null(m["frame:icon"]);
        Assert.Null(m["frame:title"]);
    }

    [Fact]
    public void Session_start_with_stack_makes_no_tab_and_the_block_is_a_hidden_member_with_stackedwith()
    {
        int tabs = Tabs;
        var o = Ok(Start(w.S1, ("stack", w.S2.Id)));
        Keys(o, "tabid", "blockid", "status", "stackedwith");
        Assert.Equal(w.S2.Id, Str(o["stackedwith"]));
        Assert.Equal(w.Projects.Id, Str(o["tabid"]));
        Assert.Equal(tabs, Tabs);
        Assert.Equal(w.S2.Id, Shown(w.Projects.Id, w.S2.Id));
        var a = BEntry(w.S1, w.S2.Id);
        var b = BEntry(w.S1, Str(o["blockid"]));
        Assert.Equal(Str(a["stack"]), Str(b["stack"]));
    }

    [Fact]
    public void Session_start_with_stack_puts_name_color_symbol_on_the_block_and_leaves_the_tab_alone()
    {
        var o = Ok(Start(w.S1, ("stack", w.S2.Id), ("name", "pg"), ("color", "blue"), ("symbol", "rocket")));
        var m = BEntry(w.S1, Str(o["blockid"]))["meta"]!;
        Assert.Equal("pg", Str(m["frame:title"]));
        Assert.Equal("blue", Str(m["icon:color"]));
        Assert.Equal("rocket", Str(m["frame:icon"]));
        Assert.Equal("Projects", Str(STab(w.S1, w.Projects.Id)["name"]));
    }

    [Fact]
    public void Session_start_activate_without_focus_is_exit_3_and_creates_nothing()
    {
        int blocks = Blocks, tabs = Tabs;
        Fail(Start(w.S1, ("activate", true)), 3);
        Assert.Equal(blocks, Blocks);
        Assert.Equal(tabs, Tabs);
    }

    [Fact]
    public void Session_start_activate_with_focus_makes_the_new_tab_the_active_one()
    {
        var o = Ok(Start(w.Manager, ("name", "go"), ("activate", true)));
        var list = SList(w.Manager);
        Assert.Single(list, t => t!["active"]!.GetValue<bool>());
        Assert.True(STab(w.Manager, Str(o["tabid"]))["active"]!.GetValue<bool>());
    }

    [Fact]
    public void Session_start_without_activate_leaves_the_shown_tab_alone()
    {
        Ok(Start(w.Manager, ("name", "quiet")));
        Assert.True(STab(w.Manager, w.Home.Id)["active"]!.GetValue<bool>());
    }

    [Theory]
    [InlineData("relative\\dir")]
    [InlineData("C:\\definitely\\not\\here\\x1y2z3")]
    public void Session_start_cwd_that_is_relative_or_missing_is_exit_2_and_nothing_is_created(string cwd)
    {
        int blocks = Blocks, tabs = Tabs;
        Fail(Start(w.S1, ("cwd", cwd)), 2);
        Assert.Equal(blocks, Blocks);
        Assert.Equal(tabs, Tabs);
    }

    [Fact]
    public void Session_start_cwd_defaults_to_the_callers_cwd_and_an_explicit_one_wins()
    {
        var sub = Path.Combine(w.Cwd, "sub");
        Directory.CreateDirectory(sub);
        var (_, a) = Sess(w.S1);
        var b = Str(Ok(Start(w.S1, ("cwd", sub)))["blockid"]);
        Assert.Equal(w.Cwd, Str(BEntry(w.Manager, a)["meta"]!["cmd:cwd"]));
        Assert.Equal(sub, Str(BEntry(w.Manager, b)["meta"]!["cmd:cwd"]));
    }

    [Fact]
    public void Session_start_without_a_command_is_exit_2()
    {
        Fail(w.Call(w.S1, "session start", ("name", "x")), 2);
    }

    [Theory]
    [InlineData("pwsh|-Command|Write-Host hi")]
    [InlineData("bash|-c|echo hi")]
    [InlineData("claude|--session-id|abc")]
    public void Session_start_restart_with_inline_code_or_session_id_is_exit_5_and_creates_nothing(string argvText)
    {
        int blocks = Blocks, tabs = Tabs;
        Fail(w.Call(w.S1, "session start", ("argv", argvText.Split('|')), ("restart", true)), 5);
        Assert.Equal(blocks, Blocks);
        Assert.Equal(tabs, Tabs);
    }

    [Fact]
    public void Session_start_restart_with_a_script_file_is_ok_and_the_block_is_listed_as_runonstart()
    {
        var o = Ok(w.Call(w.S1, "session start", ("argv", new[] { "pwsh", "-File", "x.ps1" }), ("restart", true)));
        Assert.True(Rec(Str(o["blockid"])).Restart);
        Assert.True(BEntry(w.Manager, Str(o["blockid"]))["meta"]!["cmd:runonstart"]!.GetValue<bool>());
    }

    [Fact]
    public void Session_start_with_a_tab_name_that_already_exists_is_exit_5_and_creates_nothing()
    {
        int blocks = Blocks, tabs = Tabs;
        Fail(Start(w.S1, ("name", "Projects")), 5);
        Assert.Equal(blocks, Blocks);
        Assert.Equal(tabs, Tabs);
    }

    [Fact]
    public void Session_start_with_a_session_name_that_exists_differing_only_in_case_is_exit_5_and_creates_nothing()
    {
        Ok(Start(w.S1, ("meta", new[] { "session:name=Alpha" })));
        int blocks = Blocks, tabs = Tabs;
        Fail(Start(w.S2, ("meta", new[] { "session:name=aLPHA" })), 5);
        Assert.Equal(blocks, Blocks);
        Assert.Equal(tabs, Tabs);
    }

    [Fact]
    public void Session_start_cap_without_grant_is_exit_3_and_creates_nothing()
    {
        int blocks = Blocks, tabs = Tabs;
        Fail(Start(w.S1, ("cap", new[] { "focus" })), 3);
        Assert.Equal(blocks, Blocks);
        Assert.Equal(tabs, Tabs);
    }

    [Fact]
    public void Session_start_cap_without_grant_is_exit_3_never_exit_7_even_when_the_fault_switch_is_on()
    {
        w.Host.Faults.FailAfterCreate = true;
        int blocks = Blocks;
        Fail(Start(w.S1, ("cap", new[] { "focus" })), 3);
        Fail(Start(w.S1, ("activate", true)), 3);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Session_start_cap_grant_is_refused_even_for_the_manager_who_holds_it()
    {
        int blocks = Blocks;
        Fail(Start(w.Manager, ("cap", new[] { "grant" })), 3);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Session_start_cap_beyond_what_the_creator_holds_is_exit_3()
    {
        var granter = Rec(Str(Ok(Start(w.Manager, ("cap", new[] { "focus" })))["blockid"]));
        Fail(Start(granter, ("cap", new[] { "admin" })), 3);
    }

    [Fact]
    public void Session_start_cap_is_denied_for_a_creator_that_has_focus_but_no_grant()
    {
        var focusOnly = Rec(Str(Ok(Start(w.Manager, ("cap", new[] { "focus" })))["blockid"]));
        Fail(Start(focusOnly, ("cap", new[] { "focus" })), 3);
    }

    [Fact]
    public void Session_start_meta_shows_up_in_session_list_blocks_meta()
    {
        var o = Ok(Start(w.S1, ("meta", new[] { "session:id=abc", "session:name=n1", "session:model=m" })));
        var meta = STab(w.S1, Str(o["tabid"]))["blocks"]![0]!["meta"]!;
        Keys(meta, "session:id", "session:name", "session:model");
        Assert.Equal("abc", Str(meta["session:id"]));
    }

    [Theory]
    [InlineData("name=x")]
    [InlineData("session:=x")]
    [InlineData("session:a b=x")]
    [InlineData("session:k")]
    public void Session_start_bad_meta_pair_is_exit_2_and_creates_nothing(string pair)
    {
        int blocks = Blocks;
        Fail(Start(w.S1, ("meta", new[] { pair })), 2);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Session_start_meta_key_twice_is_exit_2()
    {
        Fail(Start(w.S1, ("meta", new[] { "session:a=1", "session:a=2" })), 2);
    }

    [Fact]
    public void Session_start_meta_value_of_201_code_points_is_exit_2_and_200_is_ok()
    {
        Fail(Start(w.S1, ("meta", new[] { "session:a=" + new string('x', 201) })), 2);
        Ok(Start(w.S1, ("meta", new[] { "session:a=" + new string('x', 200) })));
    }

    [Fact]
    public void Session_start_env_repeated_key_or_empty_key_is_exit_2_and_valid_env_is_kept_exactly()
    {
        Fail(Start(w.S1, ("env", new[] { "A=1", "A=2" })), 2);
        Fail(Start(w.S1, ("env", new[] { "=1" })), 2);
        var o = Ok(Start(w.S1, ("env", new[] { "A=1", "B=two words" })));
        var env = Rec(Str(o["blockid"])).Env;
        Assert.Equal(2, env.Count);
        Assert.Equal("1", env["A"]);
        Assert.Equal("two words", env["B"]);
    }

    [Fact]
    public void Session_start_name_longer_than_60_is_exit_2_and_exactly_60_is_ok()
    {
        Fail(Start(w.S1, ("name", new string('n', 61))), 2);
        Ok(Start(w.S1, ("name", new string('n', 60))));
    }

    [Fact]
    public void Session_start_name_with_a_newline_is_exit_2()
    {
        Fail(Start(w.S1, ("name", "a\nb")), 2);
    }

    [Fact]
    public void Session_start_stack_that_does_not_exist_is_exit_4_and_creates_nothing()
    {
        int blocks = Blocks;
        Fail(Start(w.S1, ("stack", NewId())), 4);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Session_start_records_createdby_so_the_creator_can_delete_its_block_but_a_sibling_cannot()
    {
        var (_, b) = Sess(w.S1);
        Fail(w.Call(w.S2, "block delete", ("block", b), ("force", true)), 3);
        Ok(w.Call(w.S1, "block delete", ("block", b), ("force", true)));
    }

    // ================= exit 7 =================

    [Fact]
    public void Exit_7_session_start_prints_ids_with_partial_status_and_a_warning_on_stdout_and_the_error_on_stderr()
    {
        w.Host.Faults.FailAfterCreate = true;
        int blocks = Blocks, tabs = Tabs;
        var r = Start(w.S1, ("name", "half"));
        Assert.Equal(7, r.Exit);
        Keys(r.Out!, "tabid", "blockid", "status", "warning");
        Assert.Equal("partial", Str(r.Out!["status"]));
        Assert.False(string.IsNullOrWhiteSpace(Str(r.Out!["warning"])));
        var e = r.Err!["error"]!;
        Keys(e, "code", "kind", "message");
        Assert.Equal(7, e["code"]!.GetValue<int>());
        Assert.Equal("partial", Str(e["kind"]));
        Assert.Equal(blocks + 1, Blocks);
        Assert.Equal(tabs + 1, Tabs);
        Assert.True(w.Host.HasBlock(Str(r.Out["blockid"])));
        Assert.True(w.Host.HasTab(Str(r.Out["tabid"])));
    }

    [Fact]
    public void Exit_7_loses_nothing_the_new_tab_and_block_are_listed_and_a_close_with_force_cleans_up()
    {
        w.Host.Faults.FailAfterCreate = true;
        var r = Start(w.S1, ("name", "half"));
        w.Host.Faults.FailAfterCreate = false;
        var tabId = Str(r.Out!["tabid"]);
        Assert.Equal(Str(r.Out["blockid"]), Str(STab(w.S1, tabId)["blocks"]![0]!["blockid"]));
        Assert.Equal(1, Ok(w.Call(w.S1, "session close", ("tab", tabId), ("force", true)))["blocks"]!.GetValue<int>());
        Assert.False(w.Host.HasTab(tabId));
    }

    [Fact]
    public void Exit_7_session_start_in_stack_mode_carries_stackedwith_and_block_delete_force_cleans_up()
    {
        w.Host.Faults.FailAfterCreate = true;
        var r = Start(w.S1, ("stack", w.S2.Id));
        w.Host.Faults.FailAfterCreate = false;
        Assert.Equal(7, r.Exit);
        Assert.Equal("partial", Str(r.Out!["status"]));
        Assert.Equal(w.S2.Id, Str(r.Out["stackedwith"]));
        Ok(w.Call(w.S1, "block delete", ("block", Str(r.Out["blockid"])), ("force", true)));
    }

}
