// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// Contract section 11 item 4: ownership matrix of 3.2 / 3.3 / 3.4 with three tokens (Manager: focus+admin+grant, S1, S2) plus a child block that S1
// creates (a createdby chain: S1 -> Child -> Grand). Each test name says what rule it pins; the test is red when that rule is missing.
public sealed partial class OwnershipTests
{
    [Fact]
    public void Admin_lets_a_session_act_on_others_but_not_move_focus()
    {
        var adm = CapBlock(Caps.Admin);
        Is(0, w.Call(adm, "send", ("block", w.S2.Id), ("tokens", new[] { "x" })));
        Is(0, w.Call(adm, "meta set", ("block", w.S2.Id), ("pairs", new[] { "frame:title=t" })));
        Is(0, w.Call(adm, "tab rename", ("tab", w.Projects.Id), ("name", "P2")));
        Is(0, w.Call(adm, "workspace style", ("accent", "red")));
        Is(0, w.Call(adm, "attention", ("level", "info"), ("block", w.S2.Id)));
        Is(0, Run(adm, ("tab", w.Projects.Id)));
        Is(3, w.Call(adm, "tab switch", ("tab", w.Projects.Id)));
        Is(0, w.Call(adm, "block delete", ("block", w.S2.Id), ("force", true)));
    }

    [Fact]
    public void Admin_does_not_reach_across_workspaces_without_crossworkspace()
    {
        var (_, _, x2) = SecondWorkspace();
        var adm = CapBlock(Caps.Admin);
        Is(4, w.Call(adm, "send", ("block", x2.Id), ("tokens", new[] { "x" })));
        Is(4, w.Call(adm, "block delete", ("block", x2.Id), ("force", true)));
    }

    // ---- READ CONTENT ----

    [Fact]
    public void Screen_of_own_block_is_allowed()
    {
        w.S1.Screen.Add("hello");
        var r = w.Call(w.S1, "screen", ("block", "this"));
        Is(0, r);
        Assert.Equal("hello", World.Out(r)["lines"]![0]!.GetValue<string>());
    }

    [Fact]
    public void Screen_of_another_session_is_exit_4_not_3_and_leaks_nothing()
    {
        w.S2.Screen.Add("password=hunter2");
        var r = w.Call(w.S1, "screen", ("block", w.S2.Id));
        Is(4, r);
        Assert.Null(r.Out);
        Assert.DoesNotContain("hunter2", r.Err!.ToJsonString());
    }

    [Fact]
    public void Block_shot_and_web_text_of_another_session_are_exit_4()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://example.com/");
        web.Page[""] = "secret page";
        Is(4, w.Call(w.S1, "block shot", ("block", w.S2.Id)));
        Is(4, w.Call(w.S1, "block shot", ("block", web.Id)));
        var r = w.Call(w.S1, "web text", ("block", web.Id));
        Is(4, r);
        Assert.DoesNotContain("secret", r.Err!.ToJsonString());
    }

    [Fact]
    public void Screen_and_block_shot_of_another_session_give_the_same_message()
    {
        var a = w.Call(w.S1, "screen", ("block", w.S2.Id));
        var b = w.Call(w.S1, "block shot", ("block", w.S2.Id));
        Assert.Equal(World.ErrMessage(a), World.ErrMessage(b));
    }

    [Fact]
    public void Screen_of_another_session_has_the_same_message_as_an_unknown_id__existence_must_not_leak()
    {
        var other = w.Call(w.S1, "screen", ("block", w.S2.Id));
        var unknown = w.Call(w.S1, "screen", ("block", Missing()));
        Is(4, other);
        Is(4, unknown);
        Assert.Equal(World.ErrMessage(unknown), World.ErrMessage(other));
    }

    [Fact]
    public void Block_shot_of_another_session_has_the_same_message_as_an_unknown_id()
    {
        var other = w.Call(w.S1, "block shot", ("block", w.S2.Id));
        var unknown = w.Call(w.S1, "block shot", ("block", Missing()));
        Assert.Equal(World.ErrMessage(unknown), World.ErrMessage(other));
    }

    [Fact]
    public void Web_text_of_another_session_has_the_same_message_as_an_unknown_id()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://example.com/");
        var other = w.Call(w.S1, "web text", ("block", web.Id));
        var unknown = w.Call(w.S1, "web text", ("block", Missing()));
        Assert.Equal(World.ErrMessage(unknown), World.ErrMessage(other));
    }

    [Fact]
    public void Screen_denial_message_is_the_same_for_all_three_content_verbs_contract_3_2()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://example.com/");
        var s = World.ErrMessage(w.Call(w.S1, "screen", ("block", web.Id)));
        var b = World.ErrMessage(w.Call(w.S1, "block shot", ("block", web.Id)));
        var t = World.ErrMessage(w.Call(w.S1, "web text", ("block", web.Id)));
        Assert.Equal(s, b);
        Assert.Equal(s, t);
    }

    [Fact]
    public void Screen_of_the_manager_by_a_session_is_exit_4()
    {
        Is(4, w.Call(w.S1, "screen", ("block", w.Manager.Id)));
    }

    [Fact]
    public void Content_of_a_created_child_and_grandchild_is_readable_by_the_creator()
    {
        child.Screen.Add("c");
        grand.Screen.Add("g");
        Is(0, w.Call(w.S1, "screen", ("block", child.Id)));
        Is(0, w.Call(w.S1, "screen", ("block", grand.Id)));
        Is(0, w.Call(w.S1, "block shot", ("block", grand.Id)));
    }

    [Fact]
    public void Content_of_the_creator_is_not_readable_by_its_child_exit_4()
    {
        Is(4, w.Call(child, "screen", ("block", w.S1.Id)));
        Is(4, w.Call(grand, "block shot", ("block", child.Id)));
    }

    [Fact]
    public void Web_text_of_a_page_the_caller_created_is_allowed()
    {
        var r = w.Call(w.S1, "web open", ("url", "https://example.com/"), ("stack", w.S1.Id));
        var web = Born(r);
        web.Page[""] = "mine";
        var t = w.Call(w.S1, "web text", ("block", web.Id));
        Is(0, t);
        Assert.Equal("mine", World.Out(t)["text"]!.GetValue<string>());
        Is(4, w.Call(w.S2, "web text", ("block", web.Id)));
    }

    [Fact]
    public void Content_with_admin_is_readable_for_screen_shot_and_web_text()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://example.com/");
        web.Page[""] = "p";
        w.S2.Screen.Add("s");
        Is(0, w.Call(w.Manager, "screen", ("block", w.S2.Id)));
        Is(0, w.Call(w.Manager, "block shot", ("block", w.S2.Id)));
        Is(0, w.Call(w.Manager, "web text", ("block", web.Id)));
    }

    [Fact]
    public void Content_with_focus_but_no_admin_is_not_readable__focus_is_not_admin()
    {
        var f = CapBlock(Caps.Focus);
        Is(4, w.Call(f, "screen", ("block", w.S2.Id)));
    }

    [Fact]
    public void Content_with_crossworkspace_but_no_admin_is_not_readable_across_workspaces()
    {
        var (_, _, x2) = SecondWorkspace();
        var cx = CrossBlock();
        Is(4, w.Call(cx, "screen", ("block", x2.Id)));
    }

    [Fact]
    public void Content_across_workspaces_needs_admin_and_crossworkspace_together()
    {
        var (_, _, x2) = SecondWorkspace();
        x2.Screen.Add("other ws");
        var cx = CrossBlock(Caps.Admin);
        Is(0, w.Call(cx, "screen", ("block", x2.Id)));
        Is(4, w.Call(w.Manager, "screen", ("block", x2.Id)));
    }

    [Fact]
    public void A_block_outside_the_workspace_has_the_same_message_as_an_unknown_id_for_every_verb()
    {
        var (_, t2, x2) = SecondWorkspace();
        var unknown = World.ErrMessage(w.Call(w.S1, "send", ("block", Missing()), ("tokens", new[] { "x" })));
        Is(4, w.Call(w.S1, "send", ("block", x2.Id), ("tokens", new[] { "x" })));
        Assert.Equal(unknown, World.ErrMessage(w.Call(w.S1, "send", ("block", x2.Id), ("tokens", new[] { "x" }))));
        Assert.Equal(unknown, World.ErrMessage(w.Call(w.S1, "block delete", ("block", x2.Id))));
        Assert.Equal(World.ErrMessage(w.Call(w.S1, "screen", ("block", Missing()))), World.ErrMessage(w.Call(w.S1, "screen", ("block", x2.Id))));
        Assert.Equal(unknown, World.ErrMessage(w.Call(w.S1, "meta set", ("block", x2.Id), ("pairs", new[] { "frame:title=x" }))));
        Assert.NotNull(t2);
    }

    // ---- LIST scope ----

    [Fact]
    public void Block_list_is_limited_to_the_own_workspace()
    {
        var (_, _, x2) = SecondWorkspace();
        var r = w.Call(w.S1, "block list");
        Is(0, r);
        var ids = World.Out(r).AsArray().Select(x => x!["blockid"]!.GetValue<string>()).ToList();
        Assert.Contains(w.S2.Id, ids);
        Assert.DoesNotContain(x2.Id, ids);
        var m = World.Out(w.Call(w.Manager, "block list")).AsArray().Select(x => x!["blockid"]!.GetValue<string>()).ToList();
        Assert.DoesNotContain(x2.Id, m);
    }

    [Fact]
    public void Session_list_is_limited_to_the_own_workspace()
    {
        var (ws2, _, _) = SecondWorkspace();
        var r = w.Call(w.Manager, "session list");
        Is(0, r);
        Assert.All(World.Out(r).AsArray(), x => Assert.Equal(w.Ws.Id, x!["workspaceid"]!.GetValue<string>()));
        Assert.NotNull(ws2);
    }

    [Fact]
    public void List_all_needs_crossworkspace_exit_3_for_a_session_and_for_the_manager()
    {
        Is(3, w.Call(w.S1, "block list", ("all", true)));
        Is(3, w.Call(w.S1, "session list", ("all", true)));
        Is(3, w.Call(w.Manager, "block list", ("all", true)));
        Is(3, w.Call(w.Manager, "session list", ("all", true)));
    }

    [Fact]
    public void List_all_with_crossworkspace_shows_both_workspaces()
    {
        var (ws2, _, x2) = SecondWorkspace();
        var cx = CrossBlock();
        var bl = World.Out(w.Call(cx, "block list", ("all", true))).AsArray().Select(x => x!["blockid"]!.GetValue<string>()).ToList();
        Assert.Contains(x2.Id, bl);
        Assert.Contains(w.S1.Id, bl);
        var sl = World.Out(w.Call(cx, "session list", ("all", true))).AsArray().Select(x => x!["workspaceid"]!.GetValue<string>()).Distinct().ToList();
        Assert.Contains(ws2.Id, sl);
        Assert.Contains(w.Ws.Id, sl);
    }

    [Fact]
    public void Block_list_hides_cmd_of_other_sessions_and_marks_it_hidden()
    {
        var list = World.Out(w.Call(w.S1, "block list")).AsArray();
        var s2 = list.First(x => x!["blockid"]!.GetValue<string>() == w.S2.Id)!;
        Assert.Null(s2["meta"]!["cmd"]);
        Assert.Null(s2["meta"]!["cmd:args"]);
        Assert.Null(s2["meta"]!["cmd:cwd"]);
        Assert.Equal(new[] { "cmd", "cmd:args", "cmd:cwd" }, s2["hidden"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
    }

    [Fact]
    public void Block_list_shows_cmd_of_own_and_created_blocks_without_the_hidden_marker()
    {
        var list = World.Out(w.Call(w.S1, "block list")).AsArray();
        var own = list.First(x => x!["blockid"]!.GetValue<string>() == w.S1.Id)!;
        var kid = list.First(x => x!["blockid"]!.GetValue<string>() == grand.Id)!;
        Assert.Equal("pwsh", own["meta"]!["cmd"]!.GetValue<string>());
        Assert.Null(own["hidden"]);
        Assert.Equal("tool.exe", kid["meta"]!["cmd"]!.GetValue<string>());
        Assert.Null(kid["hidden"]);
    }

    [Fact]
    public void Block_list_shows_cmd_of_every_block_with_admin()
    {
        var list = World.Out(w.Call(w.Manager, "block list")).AsArray();
        Assert.All(list.Where(x => x!["view"]!.GetValue<string>() == "term"), x =>
        {
            Assert.NotNull(x!["meta"]!["cmd"]);
            Assert.Null(x["hidden"]);
        });
    }

    [Fact]
    public void Block_list_never_shows_cmd_env_and_session_id_stays_visible_to_every_token()
    {
        w.S2.Session["session:id"] = "abc";
        w.S2.Env["SECRET"] = "x";
        var raw = w.Call(w.S1, "block list").Out!.ToJsonString();
        Assert.DoesNotContain("cmd:env", raw);
        Assert.DoesNotContain("SECRET", raw);
        var s2 = World.Out(w.Call(w.S1, "block list")).AsArray().First(x => x!["blockid"]!.GetValue<string>() == w.S2.Id)!;
        Assert.Equal("abc", s2["meta"]!["session:id"]!.GetValue<string>());
    }

    [Fact]
    public void Block_list_with_a_tab_of_another_workspace_is_exit_4_and_unknown_tab_is_exit_4()
    {
        var (_, t2, _) = SecondWorkspace();
        Is(4, w.Call(w.Manager, "block list", ("tab", t2.Id)));
        Is(4, w.Call(w.Manager, "block list", ("tab", Missing())));
    }

    [Fact]
    public void Meta_get_of_another_session_does_not_return_cmd()
    {
        var r = w.Call(w.S1, "meta get", World.Args(("items", new[] { w.S2.Id, "cmd" })));
        Is(0, r);
        Assert.Null(World.Out(r)["cmd"]);
        var own = w.Call(w.S1, "meta get", World.Args(("items", new[] { "this", "cmd" })));
        Assert.NotNull(World.Out(own)["cmd"]);
    }

    [Fact]
    public void Meta_get_of_a_block_in_another_workspace_is_exit_4()
    {
        var (_, _, x2) = SecondWorkspace();
        Is(4, w.Call(w.S1, "meta get", World.Args(("items", new[] { x2.Id }))));
    }

    // ---- scope across workspaces ----

    [Fact]
    public void Foreign_workspace_ids_are_exit_4_not_3_without_crossworkspace()
    {
        var (_, t2, x2) = SecondWorkspace();
        Is(4, w.Call(w.S1, "send", ("block", x2.Id), ("tokens", new[] { "x" })));
        Is(4, w.Call(w.S1, "meta set", ("block", x2.Id), ("pairs", new[] { "frame:title=x" })));
        Is(4, w.Call(w.S1, "attention", ("level", "info"), ("block", x2.Id)));
        Is(4, w.Call(w.S1, "tab rename", ("tab", t2.Id), ("name", "x")));
        Is(4, w.Call(w.S1, "badge", ("icon", "star"), ("tab", t2.Id)));
        Is(4, Run(w.S1, ("tab", t2.Id)));
        Is(4, w.Call(w.S1, "web open", ("url", "https://example.com/"), ("tab", t2.Id)));
        Is(4, Start(w.S1, ("stack", x2.Id)));
        Is(4, w.Call(w.S1, "group add", ("block", x2.Id), ("to", w.S1.Id)));
        Is(4, w.Call(w.Manager, "tab switch", ("tab", t2.Id)));
    }

    [Fact]
    public void Crossworkspace_gives_reach_but_not_ownership_exit_3_for_act_verbs()
    {
        var (_, t2, x2) = SecondWorkspace();
        var cx = CrossBlock();
        Is(3, w.Call(cx, "send", ("block", x2.Id), ("tokens", new[] { "x" })));
        Is(3, w.Call(cx, "meta set", ("block", x2.Id), ("pairs", new[] { "frame:title=x" })));
        Is(3, w.Call(cx, "tab rename", ("tab", t2.Id), ("name", "x")));
        Is(3, Run(cx, ("tab", t2.Id)));
        Is(3, w.Call(cx, "tab switch", ("tab", t2.Id)));
    }

    [Fact]
    public void Crossworkspace_with_admin_acts_in_the_other_workspace()
    {
        var (_, t2, x2) = SecondWorkspace();
        var cx = CrossBlock(Caps.Admin);
        Is(0, w.Call(cx, "send", ("block", x2.Id), ("tokens", new[] { "x" })));
        Is(0, w.Call(cx, "tab rename", ("tab", t2.Id), ("name", "Renamed")));
        Is(0, Run(cx, ("tab", t2.Id)));
        Is(0, w.Call(cx, "block delete", ("block", x2.Id), ("force", true)));
    }

    [Fact]
    public void Crossworkspace_with_focus_switches_tabs_in_the_other_workspace()
    {
        var (ws2, t2, _) = SecondWorkspace();
        var t3 = w.Host.CreateTabAsApp(ws2, "Other2");
        var cx = CrossBlock(Caps.Focus);
        Is(0, w.Call(cx, "tab switch", ("tab", t3.Id)));
        Assert.Equal(t3.Id, w.Host.GetWorkspace(ws2.Id).ActiveTabId);
        Assert.NotNull(t2);
    }

    [Fact]
    public void Workspace_style_changes_only_the_callers_workspace()
    {
        var (ws2, _, _) = SecondWorkspace();
        Is(0, w.Call(w.Manager, "workspace style", ("accent", "red")));
        Assert.Null(w.Host.GetWorkspace(ws2.Id).Accent);
    }

    // ---- chain with a deleted ancestor ----

    [Fact]
    public void Orphan_after_its_creator_is_deleted_is_unowned_by_the_root_creator_send_denied_3()
    {
        Is(0, w.Call(w.S1, "block delete", ("block", child.Id), ("force", true)));
        Is(3, w.Call(w.S1, "send", ("block", grand.Id), ("tokens", new[] { "x" })));
        Assert.Equal(child.Id, w.Host.GetBlock(grand.Id).CreatedBy);
    }

    [Fact]
    public void Orphan_is_unowned_for_every_act_verb_and_content_read()
    {
        Is(0, w.Call(w.S1, "block delete", ("block", child.Id), ("force", true)));
        Is(3, w.Call(w.S1, "meta set", ("block", grand.Id), ("pairs", new[] { "frame:title=x" })));
        Is(3, w.Call(w.S1, "attention", ("level", "info"), ("block", grand.Id)));
        Is(3, w.Call(w.S1, "block delete", ("block", grand.Id), ("force", true)));
        Is(4, w.Call(w.S1, "screen", ("block", grand.Id)));
        Is(4, w.Call(w.S1, "block shot", ("block", grand.Id)));
        Assert.True(w.Host.HasBlock(grand.Id));
    }

    [Fact]
    public void Orphan_is_still_reachable_by_admin()
    {
        Is(0, w.Call(w.S1, "block delete", ("block", child.Id), ("force", true)));
        Is(0, w.Call(w.Manager, "send", ("block", grand.Id), ("tokens", new[] { "x" })));
        Is(0, w.Call(w.Manager, "block delete", ("block", grand.Id), ("force", true)));
    }

    [Fact]
    public void Orphan_tab_created_by_a_deleted_block_is_unowned_for_rename_and_run()
    {
        var gt = grandTab;
        Is(0, w.Call(w.S1, "block delete", ("block", child.Id), ("force", true)));
        Is(3, w.Call(w.S1, "tab rename", ("tab", gt.Id), ("name", "Mine")));
        Is(3, Run(w.S1, ("tab", gt.Id)));
        Is(0, w.Call(w.Manager, "tab rename", ("tab", gt.Id), ("name", "Mgr")));
    }

    [Fact]
    public void Orphan_tab_close_by_the_root_creator_is_denied_3_for_the_orphan_block()
    {
        Is(0, w.Call(w.S1, "block delete", ("block", child.Id), ("force", true)));
        Is(3, w.Call(w.S1, "session close", ("tab", grandTab.Id), ("force", true)));
        Assert.True(w.Host.HasTab(grandTab.Id));
    }

    [Fact]
    public void Deleting_the_middle_of_a_chain_by_closing_its_tab_orphans_the_grandchild_too()
    {
        Is(0, w.Call(w.S1, "session close", ("tab", childTab.Id), ("force", true)));
        Is(3, w.Call(w.S1, "send", ("block", grand.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void The_chain_below_a_live_ancestor_still_works_after_a_sibling_is_deleted()
    {
        var other = Born(Start(w.S1));
        Is(0, w.Call(w.S1, "block delete", ("block", other.Id), ("force", true)));
        Is(0, w.Call(w.S1, "send", ("block", grand.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void A_block_whose_creator_was_replaced_by_a_reused_id_is_not_possible_ids_are_unique()
    {
        Is(0, w.Call(w.S1, "block delete", ("block", child.Id), ("force", true)));
        var fresh = Born(Start(w.S1));
        Assert.NotEqual(child.Id, fresh.Id);
        Is(3, w.Call(w.S1, "send", ("block", grand.Id), ("tokens", new[] { "x" })));
    }

    // ---- promotion ----

    [Fact]
    public void Promotion_clears_createdby_so_no_session_owns_the_manager_through_a_chain()
    {
        var promoted = Born(Start(w.S1));
        Is(0, w.Call(w.S1, "send", ("block", promoted.Id), ("tokens", new[] { "x" })));
        w.Host.PromoteToManager(promoted.Id);
        Assert.Null(w.Host.GetBlock(promoted.Id).CreatedBy);
        Is(3, w.Call(w.S1, "send", ("block", promoted.Id), ("tokens", new[] { "x" })));
        Assert.Equal(new[] { "focus", "admin", "grant" }, CapsOf(w.Call(promoted, "whoami")));
    }

    // ---- token binding ----

}
