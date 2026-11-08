// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// ---------------------------------------------------------------------------------------------------------------------------------
// (c) session start details
// ---------------------------------------------------------------------------------------------------------------------------------
public sealed class CrSessionStartTests : CrBase
{
    static readonly string[] Cmd = { "pwsh", "-File", "x.ps1" };

    Reply Start(Block caller, params (string Key, object? Value)[] kv) => w.Call(caller, "session start", CrH.A(Cmd, kv));

    [Fact]
    public void Stdout_has_exactly_tabid_blockid_and_status_ok()
    {
        var r = Start(w.Manager);
        Assert.Equal(0, r.Exit);
        var o = World.Out(r).AsObject();
        Assert.Equal(new[] { "blockid", "status", "tabid" }, o.Select(kv => kv.Key).OrderBy(x => x).ToArray());
        Assert.Equal("ok", o["status"]!.GetValue<string>());
        Assert.True(Validators.IsUuid(o["tabid"]!.GetValue<string>()));
        Assert.True(Validators.IsUuid(o["blockid"]!.GetValue<string>()));
        Assert.Null(r.Err);
    }

    [Fact]
    public void Without_a_name_the_tabs_are_called_T1_then_T2()
    {
        var a = Start(w.Manager);
        var b = Start(w.Manager);
        Assert.Equal("T1", w.Host.GetTab(CrH.Str(a, "tabid")).Name);
        Assert.Equal("T2", w.Host.GetTab(CrH.Str(b, "tabid")).Name);
    }

    [Fact]
    public void An_empty_name_is_the_same_as_none()
    {
        var a = Start(w.Manager, ("name", ""));
        Assert.Equal("T1", w.Host.GetTab(CrH.Str(a, "tabid")).Name);
    }

    [Fact]
    public void Tab_mode_puts_name_color_and_symbol_on_the_tab_and_not_on_the_block()
    {
        var r = Start(w.S1, ("name", "Work"), ("color", "red"), ("symbol", "rocket"));
        Assert.Equal(0, r.Exit);
        var tab = w.Host.GetTab(CrH.Str(r, "tabid"));
        var b = w.Host.GetBlock(CrH.Str(r, "blockid"));
        Assert.Equal("Work", tab.Name);
        Assert.Equal("red", tab.Color);
        Assert.Equal("rocket", tab.Symbol);
        Assert.Null(b.Title);
        Assert.Null(b.Icon);
        Assert.Null(b.IconColor);
        Assert.Equal(tab.Id, b.TabId);
        Assert.Equal(w.S1.Id, tab.CreatedBy);
        Assert.Equal(w.S1.Id, b.CreatedBy);
    }

    [Fact]
    public void Tab_mode_makes_one_new_tab_holding_one_block()
    {
        int tabs = w.Ws.Tabs.Count;
        var r = Start(w.S1);
        Assert.Equal(tabs + 1, w.Ws.Tabs.Count);
        Assert.Equal(new[] { CrH.Str(r, "blockid") }, w.Host.GetTab(CrH.Str(r, "tabid")).BlockIds.ToArray());
    }

    [Fact]
    public void Stack_mode_puts_name_color_and_symbol_on_the_block_and_not_on_the_tab()
    {
        var before = CrH.Snap(w);
        var r = Start(w.S2, ("stack", w.S1.Id), ("name", "Side"), ("color", "#aabbcc"), ("symbol", "rocket"));
        Assert.Equal(0, r.Exit);
        Assert.Equal(w.Projects.Id, CrH.Str(r, "tabid"));
        Assert.Equal(w.S1.Id, CrH.Str(r, "stackedwith"));
        var b = w.Host.GetBlock(CrH.Str(r, "blockid"));
        Assert.Equal("Side", b.Title);
        Assert.Equal("#aabbcc", b.IconColor);
        Assert.Equal("rocket", b.Icon);
        Assert.Equal("Projects", w.Projects.Name);
        Assert.Null(w.Projects.Color);
        Assert.Null(w.Projects.Symbol);
        Assert.Equal(before.Tabs, w.Ws.Tabs.Count);
        Assert.Equal(before.Blocks + 1, w.Host.AllBlocks().Count);
    }

    [Fact]
    public void Stack_mode_makes_the_new_block_a_hidden_member_without_activate()
    {
        var r = Start(w.S2, ("stack", w.S1.Id));
        var tile = w.Projects.Tiles.Single(t => t.Members.Contains(w.S1.Id));
        Assert.True(tile.IsStack);
        Assert.Contains(CrH.Str(r, "blockid"), tile.Members);
        Assert.Equal(w.S1.Id, tile.ActiveId);
    }

    [Fact]
    public void Stack_mode_with_activate_and_focus_shows_the_new_member()
    {
        var r = Start(w.Manager, ("stack", w.S1.Id), ("activate", true));
        Assert.Equal(0, r.Exit);
        var tile = w.Projects.Tiles.Single(t => t.Members.Contains(w.S1.Id));
        Assert.Equal(CrH.Str(r, "blockid"), tile.ActiveId);
    }

    [Fact]
    public void Stack_mode_with_activate_without_focus_is_exit_3_and_creates_nothing()
    {
        var before = CrH.Snap(w);
        var r = Start(w.S2, ("stack", w.S1.Id), ("activate", true));
        Assert.Equal(3, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
        Assert.Equal(w.S1.Id, w.Projects.Tiles.Single(t => t.Members.Contains(w.S1.Id)).ActiveId);
    }

    [Fact]
    public void Tab_mode_with_activate_and_focus_shows_the_new_tab()
    {
        var r = Start(w.Manager, ("activate", true));
        Assert.Equal(0, r.Exit);
        Assert.Equal(CrH.Str(r, "tabid"), w.Ws.ActiveTabId);
    }

    [Fact]
    public void Tab_mode_without_activate_leaves_the_shown_tab_alone_even_with_focus()
    {
        var r = Start(w.Manager);
        Assert.Equal(0, r.Exit);
        Assert.Equal(w.Home.Id, w.Ws.ActiveTabId);
    }

    [Fact]
    public void Tab_mode_with_activate_without_focus_is_exit_3_and_no_tab_was_created()
    {
        var before = CrH.Snap(w);
        var r = Start(w.S1, ("activate", true));
        Assert.Equal(3, r.Exit);
        Assert.Equal("denied", CrH.Kind(r));
        Assert.Equal(before, CrH.Snap(w));
        Assert.Equal(w.Home.Id, w.Ws.ActiveTabId);
    }

    [Fact]
    public void Activate_without_focus_is_never_exit_7_even_when_a_later_step_would_fail()
    {
        w.Host.Faults.FailAfterCreate = true;
        var before = CrH.Snap(w);
        var r = Start(w.S1, ("activate", true));
        Assert.Equal(3, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Theory]
    [InlineData("rel/dir")]
    [InlineData("work")]
    [InlineData("./work")]
    [InlineData("..")]
    public void A_relative_cwd_is_exit_2_and_creates_nothing(string cwd)
    {
        var before = CrH.Snap(w);
        var r = Start(w.Manager, ("cwd", cwd));
        Assert.Equal(2, r.Exit);
        Assert.Equal("usage", CrH.Kind(r));
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_cwd_that_does_not_exist_is_exit_2_and_creates_nothing()
    {
        var gone = Path.Combine(w.DataDir, "no-such-dir");
        w.Host.DirectoryExists = p => p != gone;
        var before = CrH.Snap(w);
        var r = Start(w.Manager, ("cwd", gone));
        Assert.Equal(2, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void An_existing_absolute_cwd_is_recorded()
    {
        var r = Start(w.Manager, ("cwd", w.DataDir));
        Assert.Equal(0, r.Exit);
        Assert.Equal(w.DataDir, w.Host.GetBlock(CrH.Str(r, "blockid")).Cwd);
    }

    [Fact]
    public void The_default_cwd_is_the_cwd_of_the_calling_block()
    {
        var other = Path.Combine(w.DataDir, "other");
        Directory.CreateDirectory(other);
        var caller = w.Host.StartBlockAsApp(w.Projects, new[] { "pwsh" }, other);
        var r = Start(caller);
        Assert.Equal(other, w.Host.GetBlock(CrH.Str(r, "blockid")).Cwd);
        var r2 = Start(w.S1);
        Assert.Equal(w.Cwd, w.Host.GetBlock(CrH.Str(r2, "blockid")).Cwd);
    }

    [Fact]
    public void A_repeated_env_key_is_exit_2_and_creates_nothing()
    {
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("env", new[] { "A=1", "A=2" })).Exit);
        Assert.Equal(2, Start(w.Manager, ("env", new[] { "A=1", "A=1" })).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Theory]
    [InlineData("1A=x")]
    [InlineData("A-B=x")]
    [InlineData("A B=x")]
    [InlineData("=x")]
    [InlineData("NOEQUALS")]
    [InlineData("")]
    public void A_bad_env_pair_is_exit_2_and_creates_nothing(string pair)
    {
        var before = CrH.Snap(w);
        var r = Start(w.Manager, ("env", new[] { pair }));
        Assert.Equal(2, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void Good_env_pairs_are_stored_and_a_value_may_hold_an_equals_sign_or_be_empty()
    {
        var r = Start(w.Manager, ("env", new[] { "A_1=x", "_B=", "C=a=b" }));
        Assert.Equal(0, r.Exit);
        var env = w.Host.GetBlock(CrH.Str(r, "blockid")).Env;
        Assert.Equal(new Dictionary<string, string> { ["A_1"] = "x", ["_B"] = "", ["C"] = "a=b" }, env);
    }

    [Fact]
    public void The_program_gets_no_inherited_environment_only_env_flags()
    {
        var holder = w.Host.ApplyManagerConfig(w.Ws, CrH.Parse(CrH.Cfg(x => x["env"] = new JsonObject { ["SECRET"] = "s3" })));
        var r = Start(holder, ("env", new[] { "A=1" }));
        Assert.Equal(new Dictionary<string, string> { ["A"] = "1" }, w.Host.GetBlock(CrH.Str(r, "blockid")).Env);
        var r2 = Start(holder);
        Assert.Empty(w.Host.GetBlock(CrH.Str(r2, "blockid")).Env);
        var r3 = w.Call(holder, "run", CrH.A(Cmd));
        Assert.Empty(w.Host.GetBlock(CrH.Str(r3, "blockid")).Env);
    }

    [Theory]
    [InlineData("session:model=opus")]
    [InlineData("session:a.b_c-d=1")]
    [InlineData("session:k=")]
    public void A_good_meta_pair_is_stored_on_the_block(string pair)
    {
        var r = Start(w.Manager, ("meta", new[] { pair }));
        Assert.Equal(0, r.Exit);
        var i = pair.IndexOf('=');
        Assert.Equal(pair[(i + 1)..], w.Host.GetBlock(CrH.Str(r, "blockid")).Session[pair[..i]]);
    }

    [Theory]
    [InlineData("model=opus")]
    [InlineData("session:=x")]
    [InlineData("session:a b=x")]
    [InlineData("session:k")]
    [InlineData("frame:title=x")]
    [InlineData("cmd=x")]
    [InlineData("session:k=a\nb")]
    [InlineData("session:k=a\u202Eb")]
    [InlineData("session:k=a\u200Bb")]
    [InlineData("session:k=a\u0007b")]
    public void A_bad_meta_pair_is_exit_2_and_creates_nothing(string pair)
    {
        var before = CrH.Snap(w);
        var r = Start(w.Manager, ("meta", new[] { pair }));
        Assert.Equal(2, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_meta_key_given_twice_is_exit_2()
    {
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("meta", new[] { "session:k=1", "session:k=2" })).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_meta_value_is_limited_to_200_code_points_not_utf16_units()
    {
        Assert.Equal(0, Start(w.Manager, ("meta", new[] { "session:k=" + CrH.Rep("a", 200) })).Exit);
        Assert.Equal(0, Start(w.Manager, ("meta", new[] { "session:k=" + CrH.Rep("\U0001F600", 200) })).Exit);
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("meta", new[] { "session:k=" + CrH.Rep("a", 201) })).Exit);
        Assert.Equal(2, Start(w.Manager, ("meta", new[] { "session:k=" + CrH.Rep("\U0001F600", 201) })).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_session_name_already_in_use_is_exit_5_case_insensitive_and_creates_nothing()
    {
        Assert.Equal(0, Start(w.Manager, ("meta", new[] { "session:name=Alpha" })).Exit);
        var before = CrH.Snap(w);
        var r = Start(w.Manager, ("meta", new[] { "session:name=alpha" }));
        Assert.Equal(5, r.Exit);
        Assert.Equal("refused", CrH.Kind(r));
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_tab_name_already_in_use_is_exit_5_and_creates_nothing()
    {
        var before = CrH.Snap(w);
        var r = Start(w.Manager, ("name", "Projects"));
        Assert.Equal(5, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_name_of_60_units_is_accepted_and_61_is_exit_2()
    {
        Assert.Equal(0, Start(w.Manager, ("name", CrH.Rep("n", 60))).Exit);
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("name", CrH.Rep("n", 61))).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void The_name_limit_counts_utf16_units()
    {
        Assert.Equal(0, Start(w.Manager, ("name", CrH.Rep("\U0001F600", 30))).Exit);
        Assert.Equal(2, Start(w.Manager, ("name", CrH.Rep("\U0001F600", 31))).Exit);
    }

    [Theory]
    [InlineData("a\nb")]
    [InlineData("a\u202Eb")]
    [InlineData("a\u200Bb")]
    [InlineData("a\tb")]
    public void A_name_with_a_refused_character_is_exit_2(string name)
    {
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("name", name)).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Theory]
    [InlineData("notacolor")]
    [InlineData("#12345")]
    [InlineData("#gggggg")]
    [InlineData("rgb(1,2,3)")]
    public void A_bad_color_is_exit_2_and_creates_nothing(string color)
    {
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("color", color)).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Theory]
    [InlineData("a<b")]
    [InlineData("a&b")]
    [InlineData("Bad Icon!")]
    [InlineData("\U0001F600\U0001F600\U0001F600")]
    public void A_bad_symbol_is_exit_2_and_creates_nothing(string symbol)
    {
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("symbol", symbol)).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_missing_command_is_exit_2()
    {
        var before = CrH.Snap(w);
        var r = w.Call(w.Manager, "session start", ("name", "x"));
        Assert.Equal(2, r.Exit);
        Assert.Equal("usage", CrH.Kind(r));
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void An_empty_argv_or_an_empty_first_element_is_exit_2()
    {
        var before = CrH.Snap(w);
        Assert.Equal(2, w.Call(w.Manager, "session start", CrH.A(Array.Empty<string>())).Exit);
        Assert.Equal(2, w.Call(w.Manager, "session start", CrH.A(new[] { "" })).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void Restart_true_is_recorded_and_the_default_is_false()
    {
        Assert.True(w.Host.GetBlock(CrH.Str(Start(w.Manager, ("restart", true)), "blockid")).Restart);
        Assert.False(w.Host.GetBlock(CrH.Str(Start(w.Manager), "blockid")).Restart);
    }

    [Fact]
    public void Exit_7_leaves_the_ids_in_stdout_and_a_created_block_and_tab()
    {
        w.Host.Faults.FailAfterCreate = true;
        var r = Start(w.Manager, ("name", "Partial"));
        Assert.Equal(7, r.Exit);
        var o = World.Out(r);
        Assert.Equal("partial", o["status"]!.GetValue<string>());
        Assert.False(string.IsNullOrEmpty(o["warning"]?.GetValue<string>()));
        Assert.True(w.Host.HasBlock(o["blockid"]!.GetValue<string>()));
        Assert.True(w.Host.HasTab(o["tabid"]!.GetValue<string>()));
        Assert.Equal("partial", CrH.Kind(r));
        Assert.Equal(7, r.Err!["error"]!["code"]!.GetValue<int>());
    }

    [Fact]
    public void Exit_7_in_stack_mode_leaves_the_block_in_the_stack_and_no_new_tab()
    {
        w.Host.Faults.FailAfterCreate = true;
        int tabs = w.Ws.Tabs.Count;
        var r = Start(w.S2, ("stack", w.S1.Id));
        Assert.Equal(7, r.Exit);
        Assert.Equal(tabs, w.Ws.Tabs.Count);
        Assert.Contains(World.Out(r)["blockid"]!.GetValue<string>(), w.Projects.BlockIds);
    }

    [Fact]
    public void A_refused_call_with_a_pending_fault_still_leaves_nothing_and_is_not_exit_7()
    {
        w.Host.Faults.FailAfterCreate = true;
        var before = CrH.Snap(w);
        Assert.Equal(2, Start(w.Manager, ("cwd", "relative")).Exit);
        Assert.Equal(5, Start(w.Manager, ("name", "Projects")).Exit);
        Assert.Equal(5, w.Call(w.Manager, "session start", CrH.A(new[] { "bash", "-c", "x" }, ("restart", true))).Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void Exit_7_also_applies_to_run_and_leaves_the_block()
    {
        w.Host.Faults.FailAfterCreate = true;
        var r = w.Call(w.Manager, "run", CrH.A(Cmd));
        Assert.Equal(7, r.Exit);
        Assert.True(w.Host.HasBlock(World.Out(r)["blockid"]!.GetValue<string>()));
    }

    [Fact]
    public void The_new_block_records_its_creator_and_starts_with_no_capabilities()
    {
        var b = w.Host.GetBlock(CrH.Str(Start(w.Manager), "blockid"));
        Assert.Equal(w.Manager.Id, b.CreatedBy);
        Assert.Empty(b.Caps);
        Assert.Equal(Cmd, b.Argv);
    }

    [Fact]
    public void The_real_client_passes_the_flags_through_the_dash_dash_command()
    {
        var (exit, stdout, stderr) = w.Cli(w.Manager, "session", "start", "--name", "ViaCli", "--env", "K=V", "--", "pwsh", "-File", "x.ps1");
        Assert.Equal(0, exit);
        Assert.Equal("", stderr);
        var b = w.Host.GetBlock(JsonNode.Parse(stdout)!["blockid"]!.GetValue<string>());
        Assert.Equal(Cmd, b.Argv);
        Assert.Equal("V", b.Env["K"]);
        Assert.Equal("ViaCli", w.Host.GetTab(b.TabId).Name);
    }

    [Fact]
    public void The_real_client_refuses_an_inline_shell_with_restart_with_exit_5()
    {
        var before = CrH.Snap(w);
        var (exit, stdout, stderr) = w.Cli(w.Manager, "session", "start", "--restart", "--", "pwsh", "-Command", "x");
        Assert.Equal(5, exit);
        Assert.Equal("", stdout);
        Assert.Equal("refused", JsonNode.Parse(stderr)!["error"]!["kind"]!.GetValue<string>());
        Assert.Equal(before, CrH.Snap(w));
    }
}
