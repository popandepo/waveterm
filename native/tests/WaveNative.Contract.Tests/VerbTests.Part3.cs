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
    [Theory]
    [InlineData("#AABBCC80", "#aabbcc80")]
    [InlineData("Tomato", "tomato")]
    public void Tab_style_accepts_css_names_and_eight_digit_hex_and_stores_them_lower_case(string given, string stored)
    {
        var (tab, _) = Sess(w.S1);
        Assert.Equal(stored, Str(Ok(w.Call(w.S1, "tab style", ("tab", tab), ("color", given)))["color"]));
    }

    [Fact]
    public void Tab_style_symbol_accepts_a_single_emoji_and_a_41_character_icon_name_is_exit_2()
    {
        var (tab, _) = Sess(w.S1);
        Ok(w.Call(w.S1, "tab style", ("tab", tab), ("symbol", "\U0001F680")));
        Fail(w.Call(w.S1, "tab style", ("tab", tab), ("symbol", new string('a', 41))), 2);
    }

    [Fact]
    public void Tab_style_without_tab_from_a_session_in_an_area_tab_is_exit_3_and_the_tab_stays_unstyled()
    {
        Fail(w.Call(w.S1, "tab style", ("color", "red")), 3);
        Fail(w.Call(w.S1, "tab style", ("tab", w.Projects.Id), ("color", "red")), 3);
        Assert.Null(w.Projects.Color);
    }

    [Fact]
    public void Tab_style_without_tab_from_the_manager_styles_its_own_tab()
    {
        var o = Ok(w.Call(w.Manager, "tab style", ("color", "teal")));
        Assert.Equal(w.Home.Id, Str(o["tabid"]));
        Assert.Equal("teal", w.Home.Color);
    }

    // ================= tab switch =================

    [Fact]
    public void Tab_switch_by_a_session_without_focus_is_exit_3_and_the_active_tab_stays()
    {
        Fail(w.Call(w.S1, "tab switch", ("tab", w.Projects.Id)), 3);
        Assert.True(STab(w.S1, w.Home.Id)["active"]!.GetValue<bool>());
    }

    [Fact]
    public void Tab_switch_with_focus_prints_active_and_changes_exactly_one_active_mark()
    {
        var o = Ok(w.Call(w.Manager, "tab switch", ("tab", w.Projects.Id)));
        Keys(o, "active");
        Assert.Equal(w.Projects.Id, Str(o["active"]));
        var list = SList(w.Manager);
        Assert.Single(list, t => t!["active"]!.GetValue<bool>());
        Assert.True(STab(w.Manager, w.Projects.Id)["active"]!.GetValue<bool>());
    }

    [Fact]
    public void Tab_switch_keyword_tab_means_the_callers_tab_and_an_unknown_id_is_exit_4()
    {
        Ok(w.Call(w.Manager, "tab switch", ("tab", w.Projects.Id)));
        Assert.Equal(w.Home.Id, Str(Ok(w.Call(w.Manager, "tab switch", ("tab", "tab")))["active"]));
        Fail(w.Call(w.Manager, "tab switch", ("tab", NewId())), 4);
    }

    [Fact]
    public void Tab_switch_does_not_depend_on_ownership_only_on_focus()
    {
        var focusOnly = Rec(Str(Ok(Start(w.Manager, ("cap", new[] { "focus" })))["blockid"]));
        Ok(w.Call(focusOnly, "tab switch", ("tab", w.Projects.Id)));
    }

    // ================= workspace style =================

    [Fact]
    public void Workspace_style_by_a_session_is_exit_3_and_the_accent_stays_unset()
    {
        Fail(w.Call(w.S1, "workspace style", ("accent", "red")), 3);
        Assert.Null(w.Ws.Accent);
    }

    [Fact]
    public void Workspace_style_by_the_manager_prints_workspaceid_and_the_lower_cased_accent_and_clear_drops_it()
    {
        var o = Ok(w.Call(w.Manager, "workspace style", ("accent", "#FF0000")));
        Keys(o, "workspaceid", "accent");
        Assert.Equal(w.Ws.Id, Str(o["workspaceid"]));
        Assert.Equal("#ff0000", Str(o["accent"]));
        Keys(Ok(w.Call(w.Manager, "workspace style", ("clear", true))), "workspaceid");
        Assert.Null(w.Ws.Accent);
    }

    [Fact]
    public void Workspace_style_clear_with_accent_none_or_a_bad_colour_is_exit_2()
    {
        Fail(w.Call(w.Manager, "workspace style", ("accent", "red"), ("clear", true)), 2);
        Fail(w.Call(w.Manager, "workspace style"), 2);
        Fail(w.Call(w.Manager, "workspace style", ("accent", "notacolor")), 2);
    }

    // ================= block list =================

    [Fact]
    public void Block_list_as_manager_has_exact_fields_and_meta_with_cmd_cmd_args_cmd_cwd_runonstart_and_no_cmd_env()
    {
        var e = BEntry(w.Manager, w.S1.Id);
        Keys(e, "blockid", "workspaceid", "tabid", "view", "meta");
        Assert.Equal(w.Projects.Id, Str(e["tabid"]));
        Assert.Equal(w.Ws.Id, Str(e["workspaceid"]));
        var m = e["meta"]!;
        Assert.Equal("pwsh", Str(m["cmd"]));
        Assert.Equal(new[] { "-NoLogo", "-File", "s1.ps1" }, Strs(m["cmd:args"]));
        Assert.Equal(w.Cwd, Str(m["cmd:cwd"]));
        Assert.Equal(JsonValueKind.True, m["cmd:runonstart"]!.GetValueKind());
        Assert.Null(m["cmd:env"]);
        Assert.Null(e["hidden"]);
    }

    [Fact]
    public void Block_list_cmd_env_is_never_listed_even_for_the_owner_and_the_manager()
    {
        var id = Str(Ok(Start(w.S1, ("env", new[] { "SECRET=hunter2" })))["blockid"]);
        var all = w.Call(w.Manager, "block list").Out!.ToJsonString() + w.Call(w.S1, "block list").Out!.ToJsonString();
        Assert.DoesNotContain("hunter2", all);
        Assert.DoesNotContain("cmd:env", all);
        Assert.NotNull(id);
    }

    [Fact]
    public void Block_list_hides_cmd_keys_of_another_sessions_block_and_marks_it_hidden_but_session_id_stays()
    {
        Ok(w.Call(w.Manager, "meta set", ("block", w.S2.Id), ("pairs", new[] { "session:id=u-2" })));
        var other = BEntry(w.S1, w.S2.Id);
        Assert.Equal(new[] { "cmd", "cmd:args", "cmd:cwd" }, Strs(other["hidden"]));
        Assert.Null(other["meta"]!["cmd"]);
        Assert.Null(other["meta"]!["cmd:args"]);
        Assert.Null(other["meta"]!["cmd:cwd"]);
        Assert.Equal("u-2", Str(other["meta"]!["session:id"]));
    }

    [Fact]
    public void Block_list_shows_cmd_keys_without_hidden_for_the_callers_own_and_created_blocks_but_not_a_siblings()
    {
        var (_, mine) = Sess(w.S1);
        Assert.Null(BEntry(w.S1, w.S1.Id)["hidden"]);
        Assert.NotNull(BEntry(w.S1, w.S1.Id)["meta"]!["cmd"]);
        Assert.Null(BEntry(w.S1, mine)["hidden"]);
        Assert.NotNull(BEntry(w.S1, mine)["meta"]!["cmd"]);
        Assert.NotNull(BEntry(w.S2, w.S2.Id)["meta"]!["cmd"]);
        Assert.NotNull(BEntry(w.S2, mine)["hidden"]);
    }

    [Fact]
    public void Block_list_of_an_empty_tab_and_of_a_view_with_no_blocks_is_an_empty_array()
    {
        var tab = Str(Ok(w.Call(w.S1, "tab new"))["tabid"]);
        Assert.Equal(JsonValueKind.Array, w.Call(w.S1, "block list", ("tab", tab)).Out!.GetValueKind());
        Assert.Empty(BList(w.S1, ("tab", tab)));
        Assert.Empty(BList(w.S1, ("view", "web")));
    }

    [Fact]
    public void Block_list_is_in_tab_order_then_layout_order_and_follows_a_tab_move()
    {
        var (_, x) = Sess(w.Manager, "ext");
        Assert.Equal(new[] { w.Manager.Id, w.S1.Id, w.S2.Id, x }, BList(w.Manager).Select(b => Str(b!["blockid"])).ToArray());
        Ok(w.Call(w.Manager, "tab move", ("tab", w.Projects.Id), ("index", 0)));
        Assert.Equal(new[] { w.S1.Id, w.S2.Id, w.Manager.Id, x }, BList(w.Manager).Select(b => Str(b!["blockid"])).ToArray());
    }

    [Fact]
    public void Block_list_tab_filter_returns_only_that_tabs_blocks_and_an_unknown_tab_is_exit_4()
    {
        Assert.Equal(new[] { w.S1.Id, w.S2.Id }, BList(w.S1, ("tab", w.Projects.Id)).Select(b => Str(b!["blockid"])).ToArray());
        Fail(w.Call(w.S1, "block list", ("tab", NewId())), 4);
    }

    [Fact]
    public void Block_list_view_filter_separates_term_and_web_and_an_unknown_view_is_exit_2()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://example.com/");
        Assert.Equal(new[] { web.Id }, BList(w.S1, ("view", "web")).Select(b => Str(b!["blockid"])).ToArray());
        Assert.DoesNotContain(BList(w.S1, ("view", "term")), b => Str(b!["view"]) == "web");
        Fail(w.Call(w.S1, "block list", ("view", "image")), 2);
    }

    [Fact]
    public void Block_list_stack_field_is_present_for_stack_members_only_and_equal_across_the_members()
    {
        var o = Ok(Start(w.S1, ("stack", w.S2.Id)));
        var s2 = BEntry(w.S1, w.S2.Id);
        var added = BEntry(w.S1, Str(o["blockid"]));
        Assert.Equal(JsonValueKind.String, s2["stack"]!.GetValueKind());
        Assert.Equal(Str(s2["stack"]), Str(added["stack"]));
        Assert.Null(BEntry(w.S1, w.S1.Id)["stack"]);
    }

    [Fact]
    public void Block_list_redacts_url_user_info_query_and_fragment()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://user:pw@Example.com:8443/p?token=abc123#frag");
        var e = BEntry(w.S1, web.Id);
        var url = Str(e["meta"]!["url"]);
        Assert.StartsWith("https://example.com:8443/p", url, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("user", url);
        Assert.DoesNotContain("pw", url);
        Assert.DoesNotContain("abc123", url);
        Assert.DoesNotContain("frag", url);
        Assert.DoesNotContain("abc123", w.Call(w.Manager, "block list").Out!.ToJsonString());
        Assert.Equal("persist:" + web.Id, Str(e["meta"]!["web:partition"]));
        Assert.Equal("web", Str(e["meta"]!["view"]));
    }

    [Theory]
    [InlineData("HTTPS://EXAMPLE.com:443/a?x=1", "https://example.com/a?x=1", true)]
    [InlineData("https://example.com", "https://example.com/", true)]
    [InlineData("http://example.com:80/a", "http://EXAMPLE.com/a", true)]
    [InlineData("https://example.com/a?x=1", "https://example.com/a?x=2", false)]
    [InlineData("https://example.com/a", "https://example.com/a?x=1", false)]
    [InlineData("https://example.com/a#f1", "https://example.com/a#f2", false)]
    [InlineData("https://example.com/a", "https://example.com/b", false)]
    [InlineData("https://example.com:8443/a", "https://example.com/a", false)]
    [InlineData("https://example.com/a", "http://example.com/a", false)]
    public void Block_list_url_is_an_equality_test_after_normalization_query_and_fragment_count(string stored, string asked, bool matches)
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, stored);
        var hits = BList(w.S1, ("url", asked)).Select(b => Str(b!["blockid"])).ToArray();
        Assert.Equal(matches ? new[] { web.Id } : Array.Empty<string>(), hits);
    }

    [Fact]
    public void Block_list_url_other_than_http_https_file_is_exit_2()
    {
        Fail(w.Call(w.S1, "block list", ("url", "ftp://example.com/")), 2);
    }

    [Fact]
    public void Block_list_url_combines_with_view_and_finds_the_tile_web_open_just_made()
    {
        var o = Ok(w.Call(w.Manager, "web open", ("url", "https://Example.com/page?a=1")));
        var hits = BList(w.S1, ("view", "web"), ("url", "https://example.com/page?a=1"));
        Assert.Equal(new[] { Str(o["blockid"]) }, hits.Select(b => Str(b!["blockid"])).ToArray());
    }

    [Fact]
    public void Block_list_all_without_crossworkspace_is_exit_3()
    {
        Fail(w.Call(w.S1, "block list", ("all", true)), 3);
    }

    [Fact]
    public void Block_list_title_is_reported_as_frame_title_in_meta()
    {
        Assert.Equal("s1", Str(BEntry(w.S1, w.S1.Id)["meta"]!["frame:title"]));
    }

    // ================= block delete =================

    [Fact]
    public void Block_delete_of_a_running_own_block_without_force_is_exit_5_and_with_force_prints_deleted()
    {
        var (_, b) = Sess(w.S1);
        Fail(w.Call(w.S1, "block delete", ("block", b)), 5);
        Assert.True(w.Host.HasBlock(b));
        var o = Ok(w.Call(w.S1, "block delete", ("block", b), ("force", true)));
        Keys(o, "deleted");
        Assert.Equal(b, Str(o["deleted"]));
        Assert.False(w.Host.HasBlock(b));
        Assert.DoesNotContain(BList(w.S1), e => Str(e!["blockid"]) == b);
    }

    [Fact]
    public void Block_delete_of_a_finished_block_needs_no_force()
    {
        var (_, b) = Sess(w.S1);
        w.Host.FinishProgram(b, 0);
        Ok(w.Call(w.S1, "block delete", ("block", b)));
    }

    [Fact]
    public void Block_delete_of_a_block_the_caller_did_not_create_is_exit_3_even_with_force()
    {
        Fail(w.Call(w.S1, "block delete", ("block", w.S2.Id), ("force", true)), 3);
        Fail(w.Call(w.S1, "block delete", ("block", w.Manager.Id), ("force", true)), 3);
        Assert.True(w.Host.HasBlock(w.S2.Id));
        Assert.True(w.Host.HasBlock(w.Manager.Id));
    }

    [Fact]
    public void Block_delete_by_the_manager_needs_force_for_a_running_block_and_then_succeeds()
    {
        Fail(w.Call(w.Manager, "block delete", ("block", w.S2.Id)), 5);
        Ok(w.Call(w.Manager, "block delete", ("block", w.S2.Id), ("force", true)));
        Assert.False(w.Host.HasBlock(w.S2.Id));
    }

    [Fact]
    public void Block_delete_this_with_force_deletes_the_caller_and_its_token_stops_working()
    {
        var (_, b) = Sess(w.S1);
        var rec = Rec(b);
        Fail(w.Call(rec, "block delete", ("block", "this")), 5);
        var o = Ok(w.Call(rec, "block delete", ("block", "this"), ("force", true)));
        Assert.Equal(b, Str(o["deleted"]));
        Fail(w.Call(rec, "whoami"), 3);
    }

    [Fact]
    public void Block_delete_unknown_id_is_exit_4_and_a_non_id_is_exit_2()
    {
        Fail(w.Call(w.Manager, "block delete", ("block", NewId()), ("force", true)), 4);
        Fail(w.Call(w.Manager, "block delete", ("block", "12345678"), ("force", true)), 2);
    }

    [Fact]
    public void Block_delete_without_an_id_is_exit_2_and_deletes_nothing_not_the_callers_own_block()
    {
        int blocks = Blocks;
        Fail(w.Call(w.S1, "block delete", ("force", true)), 2);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Block_delete_of_a_web_block_needs_no_force()
    {
        var o = Ok(w.Call(w.Manager, "web open", ("url", "https://example.com/")));
        Ok(w.Call(w.Manager, "block delete", ("block", Str(o["blockid"]))));
    }

    [Fact]
    public void Block_delete_of_a_stack_member_leaves_the_rest_and_the_stack_shows_another_member()
    {
        var added = Str(Ok(Start(w.S1, ("stack", w.S2.Id), ("activate", false)))["blockid"]);
        Ok(w.Call(w.Manager, "block delete", ("block", w.S2.Id), ("force", true)));
        Assert.Equal(added, Shown(w.Projects.Id, added));
    }

    // ================= run =================

    [Fact]
    public void Run_happy_path_in_an_owned_tab_prints_tabid_blockid_status_ok()
    {
        var (tab, _) = Sess(w.S1);
        var o = Ok(Run(w.S1, ("tab", tab)));
        Assert.Equal("ok", Str(o["status"]));
        Assert.Equal(tab, Str(o["tabid"]));
        Assert.Contains("blockid", Names(o));
        Assert.Equal(2, STab(w.S1, tab)["blocks"]!.AsArray().Count);
    }

    [Fact]
    public void Run_without_tab_means_the_callers_own_tab_for_the_manager_it_is_home()
    {
        var o = Ok(Run(w.Manager));
        Assert.Equal(w.Home.Id, Str(o["tabid"]));
    }

    [Fact]
    public void Run_in_the_area_tab_a_session_sits_in_is_exit_3_because_a_new_top_level_tile_needs_an_owned_tab()
    {
        int blocks = Blocks;
        Fail(Run(w.S1), 3);
        Fail(Run(w.S1, ("tab", w.Projects.Id)), 3);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Run_with_stack_by_a_session_adds_a_hidden_member_and_leaves_the_shown_one()
    {
        var o = Ok(Run(w.S1, ("stack", w.S2.Id)));
        Assert.Equal(w.Projects.Id, Str(o["tabid"]));
        Assert.Equal(w.S2.Id, Shown(w.Projects.Id, w.S2.Id));
        Assert.Equal(Str(BEntry(w.S1, w.S2.Id)["stack"]), Str(BEntry(w.S1, Str(o["blockid"]))["stack"]));
    }

    [Fact]
    public void Run_focus_without_the_focus_capability_is_exit_3_and_with_it_the_new_member_becomes_shown()
    {
        int blocks = Blocks;
        Fail(Run(w.S1, ("stack", w.S2.Id), ("focus", true)), 3);
        Assert.Equal(blocks, Blocks);
        var o = Ok(Run(w.Manager, ("stack", w.S2.Id), ("focus", true)));
        Assert.Equal(Str(o["blockid"]), Shown(w.Projects.Id, w.S2.Id));
    }

    [Fact]
    public void Run_without_a_command_is_exit_2()
    {
        Fail(w.Call(w.Manager, "run", ("tab", w.Home.Id)), 2);
    }

    [Fact]
    public void Run_gets_only_the_given_env_and_title_becomes_frame_title()
    {
        var o = Ok(Run(w.Manager, ("env", new[] { "A=1" }), ("title", "hello")));
        var rec = Rec(Str(o["blockid"]));
        Assert.Single(rec.Env);
        Assert.Equal("1", rec.Env["A"]);
        Assert.Equal("hello", Str(BEntry(w.Manager, rec.Id)["meta"]!["frame:title"]));
    }

    [Fact]
    public void Run_title_over_200_code_points_or_with_a_newline_is_exit_2_and_200_is_ok()
    {
        Fail(Run(w.Manager, ("title", new string('t', 201))), 2);
        Fail(Run(w.Manager, ("title", "a\nb")), 2);
        Ok(Run(w.Manager, ("title", new string('t', 200))));
    }

    [Fact]
    public void Run_cap_without_grant_is_exit_3_restart_with_inline_shell_is_exit_5_and_repeated_env_key_is_exit_2()
    {
        var (tab, _) = Sess(w.S1);
        int blocks = Blocks;
        Fail(Run(w.S1, ("tab", tab), ("cap", new[] { "focus" })), 3);
        Fail(w.Call(w.S1, "run", ("tab", tab), ("argv", new[] { "pwsh", "-Command", "x" }), ("restart", true)), 5);
        Fail(Run(w.S1, ("tab", tab), ("env", new[] { "A=1", "A=2" })), 2);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Run_in_a_tab_of_another_workspace_is_exit_4()
    {
        var ws2 = w.Host.CreateWorkspace();
        var other = w.Host.CreateTabAsApp(ws2, "Elsewhere");
        Fail(Run(w.Manager, ("tab", other.Id)), 4);
        Fail(Run(w.Manager, ("tab", NewId())), 4);
    }

    [Fact]
    public void Run_records_createdby_so_a_sibling_session_cannot_delete_the_new_block()
    {
        var (tab, _) = Sess(w.S1);
        var b = Str(Ok(Run(w.S1, ("tab", tab)))["blockid"]);
        Fail(w.Call(w.S2, "block delete", ("block", b), ("force", true)), 3);
    }

    // ================= web open =================

    [Fact]
    public void Web_open_happy_path_prints_tabid_blockid_status_and_makes_a_web_tile_with_its_own_partition()
    {
        var o = Ok(w.Call(w.Manager, "web open", ("url", "https://example.com/a"), ("title", "Docs")));
        Keys(o, "tabid", "blockid", "status");
        Assert.Equal("ok", Str(o["status"]));
        Assert.Equal(w.Home.Id, Str(o["tabid"]));
        var m = BEntry(w.Manager, Str(o["blockid"]))["meta"]!;
        Assert.Equal("web", Str(m["view"]));
        Assert.Equal("persist:" + Str(o["blockid"]), Str(m["web:partition"]));
        Assert.Equal("Docs", Str(m["frame:title"]));
        Assert.Equal("https://example.com/a", Str(m["url"]));
    }

}
