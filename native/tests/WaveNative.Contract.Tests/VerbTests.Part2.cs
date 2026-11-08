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
public sealed partial class VerbTests
{
    [Fact]
    public void Exit_7_run_carries_ids_and_partial_status()
    {
        var (tab, _) = Sess(w.S1);
        w.Host.Faults.FailAfterCreate = true;
        var r = Run(w.S1, ("tab", tab));
        Assert.Equal(7, r.Exit);
        Assert.Equal("partial", Str(r.Out!["status"]));
        Assert.True(w.Host.HasBlock(Str(r.Out["blockid"])));
        Assert.Equal(7, r.Err!["error"]!["code"]!.GetValue<int>());
    }

    [Fact]
    public void Exit_7_web_open_carries_ids_and_partial_status()
    {
        w.Host.Faults.FailAfterCreate = true;
        var r = w.Call(w.Manager, "web open", ("url", "https://example.com/"));
        Assert.Equal(7, r.Exit);
        Assert.Equal("partial", Str(r.Out!["status"]));
        Assert.True(w.Host.HasBlock(Str(r.Out["blockid"])));
        Assert.Equal("partial", Str(r.Err!["error"]!["kind"]));
    }

    [Fact]
    public void Exit_7_is_only_for_the_creating_verbs_tab_new_stays_exit_0_with_the_fault_on()
    {
        w.Host.Faults.FailAfterCreate = true;
        Ok(w.Call(w.S1, "tab new"));
    }

    [Fact]
    public void Exit_7_does_not_hide_usage_errors_a_bad_cwd_is_still_exit_2_and_creates_nothing()
    {
        w.Host.Faults.FailAfterCreate = true;
        int blocks = Blocks;
        Fail(Start(w.S1, ("cwd", "relative")), 2);
        Assert.Equal(blocks, Blocks);
    }

    // ================= session list =================

    [Fact]
    public void Session_list_has_tab_bar_order_zero_based_index_and_exactly_one_active_tab()
    {
        var list = SList(w.S1);
        Assert.Equal(new[] { "Home", "Managers", "Projects" }, list.Select(t => Str(t!["name"])).ToArray());
        Assert.Equal(new[] { 0, 1, 2 }, list.Select(t => t!["index"]!.GetValue<int>()).ToArray());
        Assert.Equal(new[] { true, false, false }, list.Select(t => t!["active"]!.GetValue<bool>()).ToArray());
    }

    [Fact]
    public void Session_list_tab_object_has_exactly_the_contract_fields_and_omits_unset_color_and_symbol()
    {
        var t = SList(w.S1)[2]!;
        Keys(t, "tabid", "name", "workspaceid", "index", "active", "running", "blocks");
        Assert.Equal(w.Projects.Id, Str(t["tabid"]));
        Assert.Equal(w.Ws.Id, Str(t["workspaceid"]));
        Assert.Equal(JsonValueKind.Number, t["index"]!.GetValueKind());
        Assert.Contains(t["active"]!.GetValueKind(), new[] { JsonValueKind.True, JsonValueKind.False });
        Assert.Contains(t["running"]!.GetValueKind(), new[] { JsonValueKind.True, JsonValueKind.False });
    }

    [Fact]
    public void Session_list_block_object_has_blockid_view_title_status_exitcode_meta_and_no_cmd()
    {
        var b = STab(w.S1, w.Projects.Id)["blocks"]![0]!;
        Keys(b, "blockid", "view", "title", "status", "exitcode", "meta");
        Assert.Equal(w.S1.Id, Str(b["blockid"]));
        Assert.Equal("term", Str(b["view"]));
        Assert.Equal("s1", Str(b["title"]));
        Assert.Equal("running", Str(b["status"]));
        Assert.Equal(JsonValueKind.Number, b["exitcode"]!.GetValueKind());
        Assert.Equal(JsonValueKind.Object, b["meta"]!.GetValueKind());
        Assert.Empty(b["meta"]!.AsObject());
    }

    [Fact]
    public void Session_list_tab_without_blocks_has_an_empty_array_and_running_false()
    {
        var id = Str(Ok(w.Call(w.S1, "tab new"))["tabid"]);
        var t = STab(w.S1, id);
        Assert.Equal(JsonValueKind.Array, t["blocks"]!.GetValueKind());
        Assert.Empty(t["blocks"]!.AsArray());
        Assert.False(t["running"]!.GetValue<bool>());
    }

    [Fact]
    public void Session_list_blocks_meta_carries_only_session_keys_and_stays_visible_to_another_session()
    {
        Ok(w.Call(w.Manager, "meta set", ("block", w.S2.Id), ("pairs", new[] { "session:id=u-2", "session:effort=high", "frame:title=zzz" })));
        var meta = STab(w.S1, w.Projects.Id)["blocks"]!.AsArray().Single(b => Str(b!["blockid"]) == w.S2.Id)!["meta"]!;
        Keys(meta, "session:id", "session:effort");
        Assert.Equal("u-2", Str(meta["session:id"]));
    }

    [Fact]
    public void Session_list_running_follows_the_programs_and_status_done_carries_the_exit_code()
    {
        var (tab, b1) = Sess(w.S1);
        var b2 = Str(Ok(Start(w.S1, ("stack", b1)))["blockid"]);
        Assert.True(STab(w.S1, tab)["running"]!.GetValue<bool>());
        w.Host.FinishProgram(b1, 3);
        Assert.True(STab(w.S1, tab)["running"]!.GetValue<bool>());
        w.Host.FinishProgram(b2, 0);
        var t = STab(w.S1, tab);
        Assert.False(t["running"]!.GetValue<bool>());
        var first = t["blocks"]![0]!;
        Assert.Equal("done", Str(first["status"]));
        Assert.Equal(3, first["exitcode"]!.GetValue<int>());
    }

    [Fact]
    public void Session_list_web_block_has_status_none_and_does_not_make_the_tab_running()
    {
        var tab = Str(Ok(w.Call(w.Manager, "tab new"))["tabid"]);
        Ok(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("tab", tab)));
        var t = STab(w.Manager, tab);
        Assert.Equal("web", Str(t["blocks"]![0]!["view"]));
        Assert.Equal("none", Str(t["blocks"]![0]!["status"]));
        Assert.False(t["running"]!.GetValue<bool>());
    }

    [Fact]
    public void Session_list_index_and_order_follow_tab_move()
    {
        Ok(w.Call(w.Manager, "tab move", ("tab", w.Projects.Id), ("index", 0)));
        var list = SList(w.S1);
        Assert.Equal(new[] { "Projects", "Home", "Managers" }, list.Select(t => Str(t!["name"])).ToArray());
        Assert.Equal(new[] { 0, 1, 2 }, list.Select(t => t!["index"]!.GetValue<int>()).ToArray());
    }

    [Fact]
    public void Session_list_all_without_crossworkspace_is_exit_3()
    {
        Fail(w.Call(w.S1, "session list", ("all", true)), 3);
    }

    [Fact]
    public void Session_list_shows_only_the_callers_workspace()
    {
        var ws2 = w.Host.CreateWorkspace();
        w.Host.CreateTabAsApp(ws2, "Elsewhere");
        Assert.DoesNotContain(SList(w.S1), t => Str(t!["name"]) == "Elsewhere");
        var big = w.Host.StartBlockAsApp(w.Projects, Pwsh, w.Cwd, new[] { "crossworkspace" });
        var all = Ok(w.Call(big, "session list", ("all", true))).AsArray();
        Assert.Contains(all, t => Str(t!["name"]) == "Elsewhere");
    }

    [Fact]
    public void Session_list_color_and_symbol_appear_after_tab_style_and_vanish_after_clear()
    {
        var (tab, _) = Sess(w.S1);
        Ok(w.Call(w.S1, "tab style", ("tab", tab), ("color", "green"), ("symbol", "rocket")));
        var t = STab(w.S1, tab);
        Assert.Equal("green", Str(t["color"]));
        Assert.Equal("rocket", Str(t["symbol"]));
        Ok(w.Call(w.S1, "tab style", ("tab", tab), ("clear", true)));
        Assert.Null(STab(w.S1, tab)["color"]);
    }

    // ================= session close =================

    [Fact]
    public void Session_close_of_a_running_program_without_force_is_exit_5_names_the_block_and_keeps_everything()
    {
        var (tab, b) = Sess(w.S1);
        var r = w.Call(w.S1, "session close", ("tab", tab));
        Fail(r, 5);
        Assert.Contains(b, World.ErrMessage(r));
        Assert.True(w.Host.HasTab(tab));
        Assert.True(w.Host.HasBlock(b));
    }

    [Fact]
    public void Session_close_of_a_finished_tab_prints_closed_and_blocks_count_and_revokes_the_token()
    {
        var (tab, b) = Sess(w.S1);
        var rec = Rec(b);
        w.Host.FinishProgram(b, 0);
        var o = Ok(w.Call(w.S1, "session close", ("tab", tab)));
        Keys(o, "closed", "blocks");
        Assert.Equal(tab, Str(o["closed"]));
        Assert.Equal(1, o["blocks"]!.GetValue<int>());
        Assert.False(w.Host.HasTab(tab));
        Assert.False(w.Host.HasBlock(b));
        Fail(w.Call(rec, "whoami"), 3);
    }

    [Fact]
    public void Session_close_force_closes_a_running_tab_and_counts_every_block_of_a_stack()
    {
        var (tab, b) = Sess(w.S1);
        Ok(Start(w.S1, ("stack", b)));
        var o = Ok(w.Call(w.S1, "session close", ("tab", tab), ("force", true)));
        Assert.Equal(2, o["blocks"]!.GetValue<int>());
    }

    [Fact]
    public void Session_close_of_an_empty_tab_reports_zero_blocks_and_moves_the_active_mark_to_a_remaining_tab()
    {
        var tab = Str(Ok(w.Call(w.Manager, "tab new", ("activate", true)))["tabid"]);
        var o = Ok(w.Call(w.Manager, "session close", ("tab", tab)));
        Assert.Equal(0, o["blocks"]!.GetValue<int>());
        var list = SList(w.Manager);
        Assert.Single(list, t => t!["active"]!.GetValue<bool>());
        Assert.DoesNotContain(list, t => Str(t!["tabid"]) == tab);
    }

    [Theory]
    [InlineData("id")]
    [InlineData("tab")]
    public void Session_close_of_the_callers_own_tab_is_exit_5_even_with_force_and_by_id_or_keyword(string how)
    {
        var target = how == "tab" ? "tab" : w.Home.Id;
        Fail(w.Call(w.Manager, "session close", ("tab", target), ("force", true)), 5);
        Assert.True(w.Host.HasTab(w.Home.Id));
        Assert.True(w.Host.HasBlock(w.Manager.Id));
    }

    [Fact]
    public void Session_close_of_the_last_tab_of_a_window_is_exit_5()
    {
        var ws2 = w.Host.CreateWorkspace();
        var only = w.Host.CreateTabAsApp(ws2, "Only");
        var big = w.Host.StartBlockAsApp(w.Projects, Pwsh, w.Cwd, new[] { "admin", "crossworkspace" });
        Fail(w.Call(big, "session close", ("tab", only.Id), ("force", true)), 5);
        Assert.True(w.Host.HasTab(only.Id));
    }

    [Fact]
    public void Session_close_of_a_tab_another_session_made_is_exit_3_and_keeps_it()
    {
        var (tab, b) = Sess(w.S2);
        Fail(w.Call(w.S1, "session close", ("tab", tab), ("force", true)), 3);
        Assert.True(w.Host.HasBlock(b));
    }

    [Fact]
    public void Session_close_is_all_or_nothing_a_block_of_another_session_in_my_tab_stops_the_close_with_exit_3()
    {
        var (tab, x) = Sess(w.S1);
        var y = Str(Ok(Start(w.S2, ("stack", x)))["blockid"]);
        Fail(w.Call(w.S1, "session close", ("tab", tab), ("force", true)), 3);
        Assert.True(w.Host.HasTab(tab));
        Assert.True(w.Host.HasBlock(x));
        Assert.True(w.Host.HasBlock(y));
        Assert.Equal(2, Ok(w.Call(w.Manager, "session close", ("tab", tab), ("force", true)))["blocks"]!.GetValue<int>());
    }

    [Fact]
    public void Session_close_unknown_tab_and_a_tab_of_another_workspace_give_exit_4_with_the_same_message()
    {
        var ws2 = w.Host.CreateWorkspace();
        var other = w.Host.CreateTabAsApp(ws2, "Elsewhere");
        var a = w.Call(w.S1, "session close", ("tab", NewId()));
        var b = w.Call(w.S1, "session close", ("tab", other.Id));
        Fail(a, 4);
        Fail(b, 4);
        Assert.Equal(World.ErrMessage(a), World.ErrMessage(b));
        Assert.True(w.Host.HasTab(other.Id));
    }

    // ================= tab new =================

    [Fact]
    public void Tab_new_prints_tabid_and_status_ok_and_the_tab_has_no_blocks()
    {
        var o = Ok(w.Call(w.S1, "tab new"));
        Keys(o, "tabid", "status");
        Assert.Equal("ok", Str(o["status"]));
        Assert.Empty(STab(w.S1, Str(o["tabid"]))["blocks"]!.AsArray());
        Assert.Empty(BList(w.S1, ("tab", Str(o["tabid"]))));
    }

    [Fact]
    public void Tab_new_name_is_used_and_a_blank_name_becomes_T_n()
    {
        var a = Str(Ok(w.Call(w.S1, "tab new", ("name", "Notes")))["tabid"]);
        var b = Str(Ok(w.Call(w.S1, "tab new"))["tabid"]);
        Assert.Equal("Notes", Str(STab(w.S1, a)["name"]));
        Assert.Equal("T1", Str(STab(w.S1, b)["name"]));
    }

    [Fact]
    public void Tab_new_with_an_existing_name_is_exit_5_and_with_61_characters_is_exit_2_and_no_tab_appears()
    {
        int tabs = Tabs;
        Fail(w.Call(w.S1, "tab new", ("name", "Home")), 5);
        Fail(w.Call(w.S1, "tab new", ("name", new string('x', 61))), 2);
        Assert.Equal(tabs, Tabs);
    }

    [Fact]
    public void Tab_new_activate_without_focus_is_exit_3_and_creates_no_tab_with_focus_it_becomes_active()
    {
        int tabs = Tabs;
        Fail(w.Call(w.S1, "tab new", ("activate", true)), 3);
        Assert.Equal(tabs, Tabs);
        var id = Str(Ok(w.Call(w.Manager, "tab new", ("activate", true)))["tabid"]);
        Assert.True(STab(w.Manager, id)["active"]!.GetValue<bool>());
        Assert.False(STab(w.Manager, w.Home.Id)["active"]!.GetValue<bool>());
    }

    // ================= tab rename =================

    [Fact]
    public void Tab_rename_prints_tabid_and_name_and_the_list_shows_it()
    {
        var id = Str(Ok(w.Call(w.S1, "tab new", ("name", "old")))["tabid"]);
        var o = Ok(w.Call(w.S1, "tab rename", ("tab", id), ("name", "fresh")));
        Keys(o, "tabid", "name");
        Assert.Equal(id, Str(o["tabid"]));
        Assert.Equal("fresh", Str(o["name"]));
        Assert.Equal("fresh", Str(STab(w.S1, id)["name"]));
    }

    [Fact]
    public void Tab_rename_of_an_area_tab_by_a_session_is_exit_3_even_for_the_tab_it_sits_in_and_the_name_stays()
    {
        Fail(w.Call(w.S1, "tab rename", ("tab", w.Projects.Id), ("name", "mine")), 3);
        Fail(w.Call(w.S1, "tab rename", ("tab", "tab"), ("name", "mine")), 3);
        Assert.Equal("Projects", w.Projects.Name);
    }

    [Fact]
    public void Tab_rename_by_the_manager_works_on_an_area_tab_and_the_tab_keyword_means_the_callers_tab()
    {
        Ok(w.Call(w.Manager, "tab rename", ("tab", w.Projects.Id), ("name", "Work")));
        Ok(w.Call(w.Manager, "tab rename", ("tab", "tab"), ("name", "Base")));
        Assert.Equal("Work", w.Projects.Name);
        Assert.Equal("Base", w.Home.Name);
    }

    [Fact]
    public void Tab_rename_to_a_name_another_tab_has_is_exit_5_and_to_a_61_character_name_is_exit_2()
    {
        Fail(w.Call(w.Manager, "tab rename", ("tab", w.Projects.Id), ("name", "Home")), 5);
        Fail(w.Call(w.Manager, "tab rename", ("tab", w.Projects.Id), ("name", new string('x', 61))), 2);
        Assert.Equal("Projects", w.Projects.Name);
    }

    [Fact]
    public void Tab_rename_unknown_tab_is_exit_4()
    {
        Fail(w.Call(w.Manager, "tab rename", ("tab", NewId()), ("name", "x")), 4);
    }

    // ================= tab move =================

    [Fact]
    public void Tab_move_prints_tabid_and_index_and_moves_the_tab_to_that_position()
    {
        var id = Str(Ok(w.Call(w.S1, "tab new", ("name", "last")))["tabid"]);
        var o = Ok(w.Call(w.S1, "tab move", ("tab", id), ("index", 0)));
        Keys(o, "tabid", "index");
        Assert.Equal(0, o["index"]!.GetValue<int>());
        Assert.Equal(id, Str(SList(w.S1)[0]!["tabid"]));
        Assert.Equal(new[] { "last", "Home", "Managers", "Projects" }, SList(w.S1).Select(t => Str(t!["name"])).ToArray());
    }

    [Fact]
    public void Tab_move_to_the_end_keeps_the_other_tabs_in_their_order_and_loses_none()
    {
        Ok(w.Call(w.Manager, "tab move", ("tab", w.Home.Id), ("index", 2)));
        Assert.Equal(new[] { "Managers", "Projects", "Home" }, SList(w.S1).Select(t => Str(t!["name"])).ToArray());
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(3)]
    [InlineData(99)]
    public void Tab_move_index_outside_the_tab_range_is_exit_2_and_the_order_stays(int index)
    {
        Fail(w.Call(w.Manager, "tab move", ("tab", w.Projects.Id), ("index", index)), 2);
        Assert.Equal(new[] { "Home", "Managers", "Projects" }, SList(w.S1).Select(t => Str(t!["name"])).ToArray());
    }

    [Fact]
    public void Tab_move_of_an_area_tab_by_a_session_is_exit_3_and_the_order_stays()
    {
        Fail(w.Call(w.S1, "tab move", ("tab", w.Projects.Id), ("index", 0)), 3);
        Assert.Equal("Home", Str(SList(w.S1)[0]!["name"]));
    }

    [Fact]
    public void Tab_move_without_index_is_exit_2()
    {
        Fail(w.Call(w.Manager, "tab move", ("tab", w.Projects.Id)), 2);
    }

    // ================= tab style =================

    [Fact]
    public void Tab_style_of_own_tab_prints_tabid_and_only_the_keys_that_are_set_with_colors_lower_cased()
    {
        var (tab, _) = Sess(w.S1);
        var o = Ok(w.Call(w.S1, "tab style", ("tab", tab), ("color", "RED"), ("symbol", "rocket")));
        Keys(o, "tabid", "color", "symbol");
        Assert.Equal("red", Str(o["color"]));
        var o2 = Ok(w.Call(w.S1, "tab style", ("tab", tab), ("accent", "#AABBCC")));
        Keys(o2, "tabid", "color", "symbol", "accent");
        Assert.Equal("#aabbcc", Str(o2["accent"]));
    }

    [Fact]
    public void Tab_style_clear_leaves_only_tabid()
    {
        var (tab, _) = Sess(w.S1);
        Ok(w.Call(w.S1, "tab style", ("tab", tab), ("color", "red")));
        var o = Ok(w.Call(w.S1, "tab style", ("tab", tab), ("clear", true)));
        Keys(o, "tabid");
    }

    [Fact]
    public void Tab_style_clear_together_with_another_flag_is_exit_2_and_nothing_changes()
    {
        var (tab, _) = Sess(w.S1);
        Ok(w.Call(w.S1, "tab style", ("tab", tab), ("color", "red")));
        Fail(w.Call(w.S1, "tab style", ("tab", tab), ("clear", true), ("color", "blue")), 2);
        Assert.Equal("red", Str(STab(w.S1, tab)["color"]));
    }

    [Fact]
    public void Tab_style_with_nothing_to_set_is_exit_2()
    {
        var (tab, _) = Sess(w.S1);
        Fail(w.Call(w.S1, "tab style", ("tab", tab)), 2);
    }

    [Theory]
    [InlineData("color", "notacolor")]
    [InlineData("color", "#12345")]
    [InlineData("color", "#GGGGGG")]
    [InlineData("accent", "nope")]
    [InlineData("symbol", "<")]
    [InlineData("symbol", "\\")]
    [InlineData("symbol", "TooLongText")]
    public void Tab_style_invalid_value_is_exit_2_and_leaves_the_style_unchanged(string flag, string value)
    {
        var (tab, _) = Sess(w.S1);
        Fail(w.Call(w.S1, "tab style", ("tab", tab), (flag, value)), 2);
        Keys(Ok(w.Call(w.S1, "tab style", ("tab", tab), ("color", "red"))), "tabid", "color");
    }

}
