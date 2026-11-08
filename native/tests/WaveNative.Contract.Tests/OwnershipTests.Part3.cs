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
    public void Tab_switch_to_an_unknown_tab_by_the_manager_is_exit_4()
    {
        Is(4, w.Call(w.Manager, "tab switch", ("tab", Missing())));
    }

    [Fact]
    public void Group_activate_by_a_session_is_denied_3_and_the_shown_member_stays()
    {
        var h = Born(Run(w.S1, ("stack", w.S2.Id)));
        Is(3, w.Call(w.S1, "group activate", ("block", h.Id)));
        Assert.Equal(w.S2.Id, TileOf(w.S2).ActiveId);
    }

    [Fact]
    public void Group_activate_by_the_manager_shows_the_member()
    {
        var h = Born(Run(w.S1, ("stack", w.S2.Id)));
        Is(0, w.Call(w.Manager, "group activate", ("block", h.Id)));
        Assert.Equal(h.Id, TileOf(w.S2).ActiveId);
    }

    [Fact]
    public void Group_activate_of_a_block_not_in_a_group_is_refused_5_for_focus_holder()
    {
        Is(5, w.Call(w.Manager, "group activate", ("block", w.S1.Id)));
    }

    [Fact]
    public void Group_activate_denial_is_3_even_for_a_block_the_caller_owns_focus_is_not_ownership()
    {
        var c2 = Sibling(w.S1);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        Is(3, w.Call(w.S1, "group activate", ("block", c2.Id)));
    }

    [Fact]
    public void Focus_flag_on_group_add_by_a_session_is_denied_3_and_nothing_changes()
    {
        var c2 = Sibling(w.S1);
        Is(3, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id), ("focus", true)));
        Assert.False(TileOf(child).IsStack);
    }

    [Fact]
    public void Focus_flag_on_group_add_by_a_focus_holder_shows_the_new_member()
    {
        var c2 = Sibling(w.S1);
        var f = CapBlock(Caps.Focus, Caps.Admin);
        Is(0, w.Call(f, "group add", ("block", c2.Id), ("to", child.Id), ("focus", true)));
        Assert.Equal(c2.Id, TileOf(child).ActiveId);
    }

    [Fact]
    public void Focus_flag_on_group_remove_and_move_by_a_session_is_denied_3()
    {
        var c2 = Sibling(w.S1);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        Is(3, w.Call(w.S1, "group remove", ("block", c2.Id), ("focus", true)));
        var c3 = Sibling(w.S1);
        Is(3, w.Call(w.S1, "group move", ("from", child.Id), ("to", c3.Id), ("focus", true)));
    }

    [Fact]
    public void Activate_on_tab_new_by_a_session_is_denied_3_and_no_tab_is_made()
    {
        var n = TabCount();
        Is(3, w.Call(w.S1, "tab new", ("name", "Sneaky"), ("activate", true)));
        Assert.Equal(n, TabCount());
    }

    [Fact]
    public void Activate_on_session_start_by_a_session_is_denied_3_not_7_and_nothing_is_created()
    {
        var n = BlockCount();
        var t = TabCount();
        Is(3, Start(w.S1, ("activate", true)));
        Assert.Equal(n, BlockCount());
        Assert.Equal(t, TabCount());
    }

    [Fact]
    public void Focus_on_run_and_web_open_by_a_session_is_denied_3_and_nothing_is_created()
    {
        var n = BlockCount();
        Is(3, Run(w.S1, ("tab", childTab.Id), ("focus", true)));
        Is(3, w.Call(w.S1, "web open", ("url", "https://example.com/"), ("stack", w.S2.Id), ("focus", true)));
        Assert.Equal(n, BlockCount());
    }

    [Fact]
    public void Activate_on_session_start_by_the_manager_shows_the_new_tab()
    {
        var r = Start(w.Manager, ("activate", true));
        Is(0, r);
        Assert.Equal(World.Out(r)["tabid"]!.GetValue<string>(), w.Host.GetWorkspace(w.Ws.Id).ActiveTabId);
    }

    [Fact]
    public void Without_activate_a_new_session_tab_is_never_the_shown_tab()
    {
        var before = w.Host.GetWorkspace(w.Ws.Id).ActiveTabId;
        Start(w.S1);
        Start(w.Manager);
        Assert.Equal(before, w.Host.GetWorkspace(w.Ws.Id).ActiveTabId);
    }

    // ---- hidden stack members ----

    [Fact]
    public void Web_open_stack_by_a_session_is_a_hidden_member()
    {
        var r = w.Call(w.S1, "web open", ("url", "https://example.com/"), ("stack", w.S2.Id));
        Is(0, r);
        var nb = World.Out(r)["blockid"]!.GetValue<string>();
        var tile = TileOf(w.S2);
        Assert.Contains(nb, tile.Members);
        Assert.Equal(w.S2.Id, tile.ActiveId);
    }

    [Fact]
    public void Run_stack_by_a_session_is_a_hidden_member()
    {
        var h = Born(Run(w.S1, ("stack", w.S2.Id)));
        Assert.Equal(w.S2.Id, TileOf(h).ActiveId);
        Assert.Contains(h.Id, TileOf(w.S2).Members);
    }

    [Fact]
    public void Session_start_stack_by_a_session_is_a_hidden_member_and_makes_no_tab()
    {
        var t = TabCount();
        var h = Born(Start(w.S1, ("stack", w.S2.Id)));
        Assert.Equal(w.S2.Id, TileOf(h).ActiveId);
        Assert.Equal(t, TabCount());
    }

    [Fact]
    public void A_session_may_hide_a_page_in_the_managers_stack_but_not_show_it()
    {
        var r = w.Call(w.S1, "web open", ("url", "https://example.com/"), ("stack", w.Manager.Id));
        Is(0, r);
        Assert.Equal(w.Manager.Id, TileOf(w.Manager).ActiveId);
    }

    [Fact]
    public void Focus_holder_with_focus_flag_shows_the_new_web_member()
    {
        var f = CapBlock(Caps.Focus);
        var r = w.Call(f, "web open", ("url", "https://example.com/"), ("stack", w.S2.Id), ("focus", true));
        Is(0, r);
        Assert.Equal(World.Out(r)["blockid"]!.GetValue<string>(), TileOf(w.S2).ActiveId);
    }

    [Fact]
    public void Focus_holder_with_focus_flag_shows_the_new_run_member_and_without_flag_does_not()
    {
        var f = CapBlock(Caps.Focus);
        var hidden = Born(Run(f, ("stack", w.S2.Id)));
        Assert.Equal(w.S2.Id, TileOf(w.S2).ActiveId);
        var shown = Born(Run(f, ("stack", w.S2.Id), ("focus", true)));
        Assert.Equal(shown.Id, TileOf(w.S2).ActiveId);
        Assert.Contains(hidden.Id, TileOf(w.S2).Members);
    }

    [Fact]
    public void Focus_holder_with_activate_shows_the_new_session_start_member()
    {
        var f = CapBlock(Caps.Focus);
        var shown = Born(Start(f, ("stack", w.S2.Id), ("activate", true)));
        Assert.Equal(shown.Id, TileOf(w.S2).ActiveId);
    }

    [Fact]
    public void Admin_without_focus_still_adds_only_hidden_members__admin_is_not_focus()
    {
        var adm = CapBlock(Caps.Admin);
        var h = Born(Run(adm, ("stack", w.S2.Id)));
        Assert.Equal(w.S2.Id, TileOf(w.S2).ActiveId);
        Is(3, Run(adm, ("stack", w.S2.Id), ("focus", true)));
        Assert.Contains(h.Id, TileOf(w.S2).Members);
    }

    [Fact]
    public void Stack_target_in_another_tab_than_tab_flag_is_refused_5()
    {
        Is(5, Run(w.S1, ("stack", w.S2.Id), ("tab", childTab.Id)));
    }

    // ---- top-level tile rule ----

    [Fact]
    public void Run_without_stack_into_the_callers_unowned_tab_is_denied_3_and_nothing_is_created()
    {
        var n = BlockCount();
        Is(3, Run(w.S1));
        Is(3, Run(w.S1, ("tab", "tab")));
        Is(3, Run(w.S1, ("tab", w.Projects.Id)));
        Assert.Equal(n, BlockCount());
        Assert.Equal(2, w.Host.GetTab(w.Projects.Id).Tiles.Count);
    }

    [Fact]
    public void Web_open_without_stack_into_an_unowned_tab_is_denied_3()
    {
        var n = BlockCount();
        Is(3, w.Call(w.S1, "web open", ("url", "https://example.com/")));
        Is(3, w.Call(w.S1, "web open", ("url", "https://example.com/"), ("tab", w.Home.Id)));
        Assert.Equal(n, BlockCount());
    }

    [Fact]
    public void Run_and_web_open_without_stack_into_an_owned_tab_work()
    {
        Is(0, Run(w.S1, ("tab", childTab.Id)));
        Is(0, w.Call(w.S1, "web open", ("url", "https://example.com/"), ("tab", childTab.Id)));
        Assert.Equal(3, w.Host.GetTab(childTab.Id).Tiles.Count);
    }

    [Fact]
    public void Run_into_a_tab_created_by_the_callers_creator_is_denied_3__authority_never_passes_up()
    {
        Is(3, Run(child));
        Is(3, Run(child, ("tab", childTab.Id)));
    }

    [Fact]
    public void Run_into_a_tab_created_by_a_descendant_is_allowed()
    {
        Is(0, Run(w.S1, ("tab", grandTab.Id)));
    }

    [Fact]
    public void Run_without_stack_with_admin_into_any_tab_works()
    {
        var r = Run(w.Manager, ("tab", w.Projects.Id));
        Is(0, r);
        Assert.Equal(3, w.Host.GetTab(w.Projects.Id).Tiles.Count);
    }

    [Fact]
    public void Run_into_an_owned_tab_by_a_session_that_has_no_other_rights_is_a_top_level_tile_not_a_stack_member()
    {
        var h = Born(Run(w.S1, ("tab", childTab.Id)));
        Assert.False(TileOf(h).IsStack);
    }

    [Fact]
    public void Session_start_creates_a_tab_the_creator_owns_and_a_block_created_by_it()
    {
        var r = Start(w.S2, ("name", "Mine2"));
        Is(0, r);
        var tab = w.Host.GetTab(World.Out(r)["tabid"]!.GetValue<string>());
        var b = Born(r);
        Assert.Equal(w.S2.Id, tab.CreatedBy);
        Assert.Equal(w.S2.Id, b.CreatedBy);
        Is(0, Run(w.S2, ("tab", tab.Id)));
        Is(0, w.Call(w.S2, "tab rename", ("tab", tab.Id), ("name", "Mine3")));
        Is(3, Run(w.S1, ("tab", tab.Id)));
    }

    // ---- unique names ----

    [Fact]
    public void Session_start_with_a_duplicate_tab_name_is_refused_5_and_leaves_nothing_behind()
    {
        var n = BlockCount();
        var t = TabCount();
        Is(5, Start(w.S1, ("name", "Projects")));
        Is(5, Start(w.S1, ("name", "Home")));
        Assert.Equal(n, BlockCount());
        Assert.Equal(t, TabCount());
    }

    [Fact]
    public void Tab_new_with_a_duplicate_name_is_refused_5()
    {
        var t = TabCount();
        Is(5, w.Call(w.S1, "tab new", ("name", "Managers")));
        Assert.Equal(t, TabCount());
    }

    [Fact]
    public void Two_unnamed_tabs_get_distinct_generated_names()
    {
        var a = Start(w.S1);
        var b = Start(w.S1);
        Is(0, a);
        Is(0, b);
        Assert.NotEqual(w.Host.GetTab(World.Out(a)["tabid"]!.GetValue<string>()).Name, w.Host.GetTab(World.Out(b)["tabid"]!.GetValue<string>()).Name);
    }

    [Fact]
    public void Tab_rename_to_an_existing_name_is_refused_5_by_a_session_and_by_admin()
    {
        Is(5, w.Call(w.S1, "tab rename", ("tab", childTab.Id), ("name", "Home")));
        Is(5, w.Call(w.Manager, "tab rename", ("tab", w.Managers.Id), ("name", "Projects")));
        Assert.Equal("Managers", w.Host.GetTab(w.Managers.Id).Name);
    }

    [Fact]
    public void Session_name_duplicate_on_session_start_is_refused_5_case_insensitive_and_nothing_is_made()
    {
        Is(0, Start(w.S1, ("meta", new[] { "session:name=Alpha" })));
        var n = BlockCount();
        var t = TabCount();
        Is(5, Start(w.S2, ("meta", new[] { "session:name=Alpha" })));
        Is(5, Start(w.S2, ("meta", new[] { "session:name=aLPHA" })));
        Assert.Equal(n, BlockCount());
        Assert.Equal(t, TabCount());
    }

    [Fact]
    public void Session_name_duplicate_on_run_and_in_a_stack_is_refused_5()
    {
        Is(0, Start(w.S1, ("meta", new[] { "session:name=Beta" })));
        Is(5, Run(w.S1, ("tab", childTab.Id), ("meta", new[] { "session:name=beta" })));
        Is(5, Start(w.S2, ("stack", w.S1.Id), ("meta", new[] { "session:name=BETA" })));
    }

    [Fact]
    public void Session_name_duplicate_via_meta_set_is_refused_5_for_owner_and_for_admin()
    {
        Is(0, w.Call(w.S2, "meta set", ("block", "this"), ("pairs", new[] { "session:name=Gamma" })));
        Is(5, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:name=gamma" })));
        Is(5, w.Call(w.Manager, "meta set", ("block", w.S1.Id), ("pairs", new[] { "session:name=GAMMA" })));
        Assert.False(w.S1.Session.ContainsKey("session:name"));
    }

    [Fact]
    public void Setting_the_same_session_name_on_the_same_block_again_is_not_a_duplicate()
    {
        Is(0, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:name=Delta" })));
        Is(0, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:name=delta" })));
    }

    [Fact]
    public void Session_name_is_unique_per_workspace_not_globally()
    {
        var (_, t2, x2) = SecondWorkspace();
        var cx = CrossBlock(Caps.Admin);
        Is(0, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:name=Eps" })));
        Is(0, w.Call(cx, "meta set", ("block", x2.Id), ("pairs", new[] { "session:name=eps" })));
        Assert.Equal("eps", x2.Session["session:name"]);
        Assert.NotNull(t2);
    }

    // ---- capabilities at creation ----

    [Fact]
    public void Cap_needs_grant_a_session_is_denied_3_and_nothing_is_created()
    {
        var n = BlockCount();
        Is(3, Start(w.S1, ("cap", new[] { "focus" })));
        Is(3, Run(w.S1, ("tab", childTab.Id), ("cap", new[] { "admin" })));
        Assert.Equal(n, BlockCount());
    }

    [Fact]
    public void Cap_by_the_manager_gives_exactly_what_was_asked()
    {
        var b = CapBlock(Caps.Focus);
        Assert.Equal(new[] { "focus" }, CapsOf(w.Call(b, "whoami")));
        var c = CapBlock(Caps.Focus, Caps.Admin);
        Assert.Equal(new[] { "focus", "admin" }, CapsOf(w.Call(c, "whoami")));
    }

    [Fact]
    public void Cap_grant_is_refused_3_even_for_the_manager()
    {
        var n = BlockCount();
        Is(3, Start(w.Manager, ("cap", new[] { "grant" })));
        Is(3, Run(w.Manager, ("tab", w.Home.Id), ("cap", new[] { "grant" })));
        Assert.Equal(n, BlockCount());
    }

    [Fact]
    public void Cap_beyond_the_creators_rights_is_refused_3__the_manager_lacks_crossworkspace()
    {
        var n = BlockCount();
        Is(3, Start(w.Manager, ("cap", new[] { "crossworkspace" })));
        Is(3, Start(w.Manager, ("cap", new[] { "focus", "crossworkspace" })));
        Assert.Equal(n, BlockCount());
    }

    [Fact]
    public void A_block_with_admin_only_cannot_give_caps_it_lacks_grant()
    {
        var adm = CapBlock(Caps.Admin);
        Is(3, Start(adm, ("cap", new[] { "admin" })));
    }

    [Fact]
    public void A_block_that_holds_grant_through_the_app_but_not_focus_cannot_give_focus()
    {
        var g = w.Host.StartBlockAsApp(w.Managers, new[] { "pwsh", "g.ps1" }, w.Cwd, new[] { Caps.Grant, Caps.Admin });
        Is(3, Start(g, ("cap", new[] { "focus" })));
        Is(0, Start(g, ("cap", new[] { "admin" })));
    }

    [Fact]
    public void Grant_is_not_passed_on_a_child_of_the_manager_cannot_grant_or_forge_createdby()
    {
        var c = CapBlock(Caps.Focus, Caps.Admin);
        Is(3, Start(c, ("cap", new[] { "focus" })));
    }

    [Fact]
    public void Createdby_cannot_be_forged_through_arguments()
    {
        var r = Start(w.S1, ("createdby", w.Manager.Id), ("caps", new[] { "admin" }));
        var b = Born(r);
        Assert.Equal(w.S1.Id, b.CreatedBy);
        Assert.Empty(b.Caps);
        Is(0, w.Call(w.S1, "send", ("block", b.Id), ("tokens", new[] { "x" })));
        Is(3, w.Call(w.S2, "send", ("block", b.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Created_by_a_session_with_a_forged_creator_claim_on_run_is_still_owned_by_the_real_caller()
    {
        var b = Born(Run(w.S2, ("stack", w.S1.Id), ("createdby", w.S1.Id)));
        Assert.Equal(w.S2.Id, b.CreatedBy);
        Is(3, w.Call(w.S1, "send", ("block", b.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Restart_with_session_id_of_the_manager_gives_no_rights_even_after_an_app_restart()
    {
        var b = Born(Start(w.S1, ("restart", true), ("meta", new[] { "session:id=" + w.Manager.Id })));
        Assert.Empty(CapsOf(w.Call(b, "whoami")));
        w.Host.SimulateAppRestart();
        Assert.Empty(CapsOf(w.Call(b, "whoami")));
        Is(3, w.Call(b, "tab switch", ("tab", w.Home.Id)));
        Is(3, w.Call(b, "send", ("block", w.S2.Id), ("tokens", new[] { "x" })));
        Assert.Equal(new[] { "focus", "admin", "grant" }, CapsOf(w.Call(w.Manager, "whoami")));
    }

    [Fact]
    public void Rights_live_in_the_record_a_granted_block_keeps_them_after_an_app_restart()
    {
        var adm = CapBlock(Caps.Admin);
        w.Host.SimulateAppRestart();
        Assert.Equal(new[] { "admin" }, CapsOf(w.Call(adm, "whoami")));
        Is(0, w.Call(adm, "meta set", ("block", w.S2.Id), ("pairs", new[] { "frame:title=x" })));
    }

}
