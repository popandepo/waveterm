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
    public void Capabilities_are_not_meta_a_session_cannot_set_admin_on_itself_usage_2()
    {
        Is(2, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "admin=true" })));
        Is(2, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "caps=admin" })));
        Is(2, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "createdby=" + w.Manager.Id })));
        Assert.Empty(CapsOf(w.Call(w.S1, "whoami")));
    }

    [Fact]
    public void A_session_key_that_looks_like_a_capability_grants_nothing()
    {
        Is(0, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:admin=true", "session:caps=focus,admin,grant" })));
        Assert.Empty(CapsOf(w.Call(w.S1, "whoami")));
        Is(3, w.Call(w.S1, "send", ("block", w.S2.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void A_session_id_equal_to_the_managers_gives_no_rights()
    {
        Is(0, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:id=" + w.Manager.Id })));
        Assert.Empty(CapsOf(w.Call(w.S1, "whoami")));
        Is(3, w.Call(w.S1, "tab switch", ("tab", w.Home.Id)));
    }

    [Fact]
    public void Meta_set_json_cannot_forge_createdby_either()
    {
        Is(2, w.Call(w.S1, "meta set", ("block", "this"), ("json", "{\"createdby\":\"" + w.Manager.Id + "\"}")));
    }

    [Fact]
    public void Meta_set_with_one_bad_key_changes_nothing_all_or_nothing()
    {
        Is(2, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:model=x", "cmd=bad" })));
        Assert.False(w.S1.Session.ContainsKey("session:model"));
    }

    // ---- attention and badge ----

    [Fact]
    public void Attention_on_own_block_is_allowed_with_default_block()
    {
        var r = w.Call(w.S1, "attention", ("level", "need"), ("reason", "look"));
        Is(0, r);
        Assert.Equal("need", w.S1.AttentionLevel);
    }

    [Fact]
    public void Attention_on_another_block_is_denied_3_and_no_mark_is_set()
    {
        Is(3, w.Call(w.S1, "attention", ("level", "urgent"), ("reason", "x"), ("block", w.S2.Id)));
        Assert.Null(w.S2.AttentionLevel);
    }

    [Fact]
    public void Attention_clear_on_another_block_is_denied_3_and_keeps_the_mark()
    {
        w.S2.AttentionLevel = "need";
        w.S2.AttentionReason = "mine";
        Is(3, w.Call(w.S1, "attention", ("level", "clear"), ("block", w.S2.Id)));
        Assert.Equal("need", w.S2.AttentionLevel);
    }

    [Fact]
    public void Attention_on_a_created_child_is_allowed_and_on_the_manager_is_denied()
    {
        Is(0, w.Call(w.S1, "attention", ("level", "info"), ("block", grand.Id)));
        Is(3, w.Call(w.S1, "attention", ("level", "info"), ("block", w.Manager.Id)));
    }

    [Fact]
    public void Attention_on_another_block_by_admin_is_allowed()
    {
        Is(0, w.Call(w.Manager, "attention", ("level", "need"), ("reason", "r"), ("block", w.S2.Id)));
        Assert.Equal("need", w.S2.AttentionLevel);
    }

    [Fact]
    public void Badge_on_own_block_is_allowed_and_on_another_denied_3()
    {
        Is(0, w.Call(w.S1, "badge", ("icon", "star")));
        Assert.NotNull(w.Host.BadgeOf(w.S1.Id));
        Is(3, w.Call(w.S1, "badge", ("icon", "star"), ("block", w.S2.Id)));
        Assert.Null(w.Host.BadgeOf(w.S2.Id));
    }

    [Fact]
    public void Badge_clear_on_another_block_is_denied_3_and_keeps_the_badge()
    {
        Is(0, w.Call(w.Manager, "badge", ("icon", "star"), ("block", w.S2.Id)));
        Is(3, w.Call(w.S1, "badge", ("clear", true), ("block", w.S2.Id)));
        Assert.NotNull(w.Host.BadgeOf(w.S2.Id));
    }

    [Fact]
    public void Badge_on_an_area_tab_by_a_session_is_denied_3_and_on_its_own_created_tab_allowed()
    {
        Is(3, w.Call(w.S1, "badge", ("icon", "star"), ("tab", w.Projects.Id)));
        Assert.Null(w.Host.BadgeOf(w.Projects.Id));
        Is(0, w.Call(w.S1, "badge", ("icon", "star"), ("tab", childTab.Id)));
        Is(0, w.Call(w.Manager, "badge", ("icon", "star"), ("tab", w.Projects.Id)));
    }

    [Fact]
    public void Badge_and_attention_on_an_unknown_block_are_exit_4()
    {
        Is(4, w.Call(w.S1, "badge", ("icon", "star"), ("block", Missing())));
        Is(4, w.Call(w.S1, "attention", ("level", "info"), ("block", Missing())));
    }

    // ---- tab rename / move / style and workspace style ----

    [Fact]
    public void Rename_of_an_area_tab_by_a_session_sitting_in_it_is_denied_3__sitting_in_a_tab_grants_nothing()
    {
        Is(3, w.Call(w.S1, "tab rename", ("tab", w.Projects.Id), ("name", "Hijacked")));
        Is(3, w.Call(w.S1, "tab rename", ("tab", "tab"), ("name", "Hijacked")));
        Assert.Equal("Projects", w.Host.GetTab(w.Projects.Id).Name);
    }

    [Fact]
    public void Rename_of_the_managers_tab_by_a_session_is_denied_3()
    {
        Is(3, w.Call(w.S1, "tab rename", ("tab", w.Home.Id), ("name", "X")));
        Assert.Equal("Home", w.Host.GetTab(w.Home.Id).Name);
    }

    [Fact]
    public void Rename_of_a_tab_the_caller_created_is_allowed()
    {
        var r = w.Call(w.S1, "tab rename", ("tab", childTab.Id), ("name", "Mine"));
        Is(0, r);
        Assert.Equal("Mine", w.Host.GetTab(childTab.Id).Name);
    }

    [Fact]
    public void Rename_by_a_child_of_the_tab_its_creator_made_is_denied_3__red_if_authority_passes_up()
    {
        Is(3, w.Call(child, "tab rename", ("tab", childTab.Id), ("name", "Up")));
    }

    [Fact]
    public void Rename_by_the_root_creator_of_a_tab_made_by_its_child_is_allowed()
    {
        Is(0, w.Call(w.S1, "tab rename", ("tab", grandTab.Id), ("name", "Down")));
    }

    [Fact]
    public void Rename_of_an_area_tab_by_admin_is_allowed()
    {
        Is(0, w.Call(w.Manager, "tab rename", ("tab", w.Projects.Id), ("name", "Work")));
        Assert.Equal("Work", w.Host.GetTab(w.Projects.Id).Name);
    }

    [Fact]
    public void Move_of_an_area_tab_by_a_session_is_denied_3_and_order_is_unchanged()
    {
        var before = w.Host.GetWorkspace(w.Ws.Id).Tabs.Select(t => t.Id).ToList();
        Is(3, w.Call(w.S1, "tab move", ("tab", w.Projects.Id), ("index", 0)));
        Assert.Equal(before, w.Host.GetWorkspace(w.Ws.Id).Tabs.Select(t => t.Id).ToList());
    }

    [Fact]
    public void Move_of_an_own_created_tab_and_any_tab_by_admin_is_allowed()
    {
        Is(0, w.Call(w.S1, "tab move", ("tab", childTab.Id), ("index", 0)));
        Assert.Equal(childTab.Id, w.Host.GetWorkspace(w.Ws.Id).Tabs[0].Id);
        Is(0, w.Call(w.Manager, "tab move", ("tab", w.Projects.Id), ("index", 0)));
        Assert.Equal(w.Projects.Id, w.Host.GetWorkspace(w.Ws.Id).Tabs[0].Id);
    }

    [Fact]
    public void Style_of_an_area_tab_by_a_session_is_denied_3()
    {
        Is(3, w.Call(w.S1, "tab style", ("tab", w.Projects.Id), ("color", "red")));
        Assert.Null(w.Host.GetTab(w.Projects.Id).Color);
    }

    [Fact]
    public void Style_with_no_tab_from_a_stacked_session_is_denied_3__its_own_tab_is_an_area_tab()
    {
        Is(3, w.Call(w.S1, "tab style", ("color", "red")));
        Is(3, w.Call(w.S2, "tab style", ("symbol", "star")));
        Assert.Null(w.Host.GetTab(w.Projects.Id).Color);
    }

    [Fact]
    public void Style_with_no_tab_from_a_session_in_a_tab_it_made_is_denied_3_for_the_child_and_allowed_for_the_creator()
    {
        Is(3, w.Call(child, "tab style", ("color", "red")));
        Is(0, w.Call(w.S1, "tab style", ("tab", childTab.Id), ("color", "red")));
        Assert.Equal("red", w.Host.GetTab(childTab.Id).Color);
    }

    [Fact]
    public void Style_of_any_tab_by_admin_is_allowed()
    {
        Is(0, w.Call(w.Manager, "tab style", ("tab", w.Projects.Id), ("accent", "#112233")));
        Assert.Equal("#112233", w.Host.GetTab(w.Projects.Id).Accent);
    }

    [Fact]
    public void Tab_new_makes_a_tab_the_caller_owns()
    {
        var r = w.Call(w.S1, "tab new", ("name", "Scratch"));
        Is(0, r);
        var id = World.Out(r)["tabid"]!.GetValue<string>();
        Assert.Equal(w.S1.Id, w.Host.GetTab(id).CreatedBy);
        Is(0, w.Call(w.S1, "tab style", ("tab", id), ("color", "blue")));
        Is(0, w.Call(w.S1, "tab rename", ("tab", id), ("name", "Scratch2")));
        Is(3, w.Call(w.S2, "tab rename", ("tab", id), ("name", "Nope")));
    }

    [Fact]
    public void Tab_rename_unknown_tab_is_exit_4_and_foreign_workspace_tab_is_exit_4()
    {
        var (_, t2, _) = SecondWorkspace();
        Is(4, w.Call(w.Manager, "tab rename", ("tab", Missing()), ("name", "x")));
        Is(4, w.Call(w.Manager, "tab rename", ("tab", t2.Id), ("name", "x")));
    }

    [Fact]
    public void Workspace_style_is_admin_only_a_session_is_denied_3()
    {
        Is(3, w.Call(w.S1, "workspace style", ("accent", "red")));
        Assert.Null(w.Host.GetWorkspace(w.Ws.Id).Accent);
    }

    [Fact]
    public void Workspace_style_by_admin_is_allowed()
    {
        Is(0, w.Call(w.Manager, "workspace style", ("accent", "red")));
        Assert.Equal("red", w.Host.GetWorkspace(w.Ws.Id).Accent);
        Is(0, w.Call(w.Manager, "workspace style", ("clear", true)));
        Assert.Null(w.Host.GetWorkspace(w.Ws.Id).Accent);
    }

    [Fact]
    public void Workspace_style_with_only_focus_is_denied_3__focus_is_not_admin()
    {
        var f = CapBlock(Caps.Focus);
        Is(3, w.Call(f, "workspace style", ("accent", "red")));
    }

    // ---- group ----

    Block Sibling(Block caller) => Born(Run(caller, ("tab", childTab.Id)));

    [Fact]
    public void Group_add_of_own_blocks_in_an_owned_tab_is_allowed_and_the_new_member_is_hidden()
    {
        var c2 = Sibling(w.S1);
        var r = w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id));
        Is(0, r);
        var tile = TileOf(child);
        Assert.Equal(new[] { child.Id, c2.Id }, tile.Members.ToArray());
        Assert.Equal(child.Id, tile.ActiveId);
    }

    [Fact]
    public void Group_add_of_another_sessions_block_is_denied_3()
    {
        Is(3, w.Call(w.S2, "group add", ("block", w.S1.Id), ("to", w.S2.Id)));
        Assert.False(TileOf(w.S1).IsStack);
    }

    [Fact]
    public void Group_add_of_a_foreign_block_by_admin_is_allowed()
    {
        Is(0, w.Call(w.Manager, "group add", ("block", w.S1.Id), ("to", w.S2.Id)));
        Assert.True(TileOf(w.S2).IsStack);
    }

    [Fact]
    public void Group_add_across_tabs_is_refused_5_same_tab_rule()
    {
        Is(5, w.Call(w.S1, "group add", ("block", child.Id), ("to", w.S1.Id)));
        Is(5, w.Call(w.Manager, "group add", ("block", w.S1.Id), ("to", w.Manager.Id)));
    }

    [Fact]
    public void Group_add_when_already_in_the_same_group_is_refused_5()
    {
        var c2 = Sibling(w.S1);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        Is(5, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
    }

    [Fact]
    public void Group_add_without_to_is_usage_2_and_unknown_target_is_4()
    {
        var c2 = Sibling(w.S1);
        Is(2, w.Call(w.S1, "group add", ("block", c2.Id)));
        Is(4, w.Call(w.S1, "group add", ("block", c2.Id), ("to", Missing())));
    }

    [Fact]
    public void Group_remove_of_a_block_not_in_a_group_is_refused_5()
    {
        Is(5, w.Call(w.S1, "group remove", ("block", child.Id)));
    }

    [Fact]
    public void Group_remove_needs_an_owned_tab_a_session_removing_its_own_hidden_member_from_an_area_tab_is_denied_3()
    {
        var h = Born(Run(w.S1, ("stack", w.S2.Id)));
        Assert.Equal(w.Projects.Id, h.TabId);
        Is(3, w.Call(w.S1, "group remove", ("block", h.Id)));
        Assert.Contains(h.Id, TileOf(w.S2).Members);
    }

    [Fact]
    public void Group_remove_in_an_owned_tab_is_allowed_and_makes_a_top_level_tile()
    {
        var c2 = Sibling(w.S1);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        Is(0, w.Call(w.S1, "group remove", ("block", c2.Id)));
        Assert.False(TileOf(c2).IsStack);
        Assert.NotEqual(TileOf(c2).Id, TileOf(child).Id);
    }

    [Fact]
    public void Group_remove_by_admin_in_an_area_tab_is_allowed()
    {
        var h = Born(Run(w.S1, ("stack", w.S2.Id)));
        Is(0, w.Call(w.Manager, "group remove", ("block", h.Id)));
        Assert.False(TileOf(h).IsStack);
    }

    [Fact]
    public void Group_remove_of_a_block_the_caller_did_not_create_is_denied_3()
    {
        var h = Born(Run(w.S1, ("stack", w.S2.Id)));
        Is(3, w.Call(w.S2, "group remove", ("block", h.Id)));
    }

    [Fact]
    public void Group_move_of_own_stack_in_an_owned_tab_is_allowed()
    {
        var c2 = Sibling(w.S1);
        var c3 = Sibling(w.S1);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        var r = w.Call(w.S1, "group move", ("from", child.Id), ("to", c3.Id));
        Is(0, r);
        Assert.Equal(new[] { c3.Id, child.Id, c2.Id }, TileOf(c3).Members.ToArray());
    }

    [Fact]
    public void Group_move_needs_ownership_of_every_member_a_foreign_hidden_member_denies_3_and_nothing_moves()
    {
        var foreign = Born(Run(w.S2, ("stack", child.Id)));
        var c3 = Sibling(w.S1);
        Is(3, w.Call(w.S1, "group move", ("from", child.Id), ("to", c3.Id)));
        Assert.Contains(foreign.Id, TileOf(child).Members);
        Assert.DoesNotContain(child.Id, TileOf(c3).Members);
    }

    [Fact]
    public void Group_move_of_a_foreign_stack_by_admin_moves_all_members()
    {
        var foreign = Born(Run(w.S2, ("stack", child.Id)));
        var c3 = Sibling(w.S1);
        Is(0, w.Call(w.Manager, "group move", ("from", child.Id), ("to", c3.Id)));
        Assert.Equal(3, TileOf(c3).Members.Count);
        Assert.Contains(foreign.Id, TileOf(c3).Members);
    }

    [Fact]
    public void Group_move_across_tabs_is_refused_5_and_to_the_same_group_is_refused_5()
    {
        Is(5, w.Call(w.Manager, "group move", ("from", w.S1.Id), ("to", w.Manager.Id)));
        var c2 = Sibling(w.S1);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        Is(5, w.Call(w.S1, "group move", ("from", c2.Id), ("to", child.Id)));
    }

    [Fact]
    public void Group_move_without_from_is_usage_2_and_unknown_is_4()
    {
        Is(2, w.Call(w.S1, "group move", ("to", child.Id)));
        Is(4, w.Call(w.S1, "group move", ("from", Missing()), ("to", child.Id)));
    }

    [Fact]
    public void Group_remove_of_the_shown_member_makes_the_next_member_shown()
    {
        var c2 = Sibling(w.S1);
        var c3 = Sibling(w.S1);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        Is(0, w.Call(w.S1, "group add", ("block", c3.Id), ("to", child.Id)));
        Assert.Equal(child.Id, TileOf(child).ActiveId);
        Is(3, w.Call(w.S1, "group remove", ("block", child.Id)));
        Is(0, w.Call(w.Manager, "group remove", ("block", child.Id)));
        Assert.Equal(c2.Id, TileOf(c2).ActiveId);
    }

    [Fact]
    public void Deleting_the_shown_member_shows_the_first_member_not_a_hidden_page()
    {
        var h1 = Born(Run(w.S1, ("stack", w.S2.Id)));
        var h2 = Born(Run(w.S1, ("stack", w.S2.Id)));
        Is(0, w.Call(w.Manager, "group activate", ("block", h1.Id)));
        Assert.Equal(h1.Id, TileOf(w.S2).ActiveId);
        Is(3, w.Call(w.S1, "block delete", ("block", h1.Id), ("force", true)));
        Is(0, w.Call(w.Manager, "block delete", ("block", h1.Id), ("force", true)));
        Assert.Equal(w.S2.Id, TileOf(w.S2).ActiveId);
        Assert.NotEqual(h2.Id, TileOf(w.S2).ActiveId);
    }

    [Fact]
    public void Group_move_of_the_shown_member_stack_with_focus_keeps_the_shown_member()
    {
        var c2 = Sibling(w.S1);
        var c3 = Sibling(w.S1);
        var f = CapBlock(Caps.Focus, Caps.Admin);
        Is(0, w.Call(w.S1, "group add", ("block", c2.Id), ("to", child.Id)));
        Is(0, w.Call(f, "group activate", ("block", c2.Id)));
        Is(0, w.Call(f, "group move", ("from", child.Id), ("to", c3.Id), ("focus", true)));
        Assert.Equal(c2.Id, TileOf(c3).ActiveId);
    }

    // ---- FOCUS ----

    [Fact]
    public void Tab_switch_by_a_session_is_denied_3_and_the_shown_tab_stays()
    {
        var before = w.Host.GetWorkspace(w.Ws.Id).ActiveTabId;
        Is(3, w.Call(w.S1, "tab switch", ("tab", w.Projects.Id)));
        Is(3, w.Call(w.S1, "tab switch", ("tab", childTab.Id)));
        Assert.Equal(before, w.Host.GetWorkspace(w.Ws.Id).ActiveTabId);
    }

    [Fact]
    public void Tab_switch_by_the_manager_changes_the_shown_tab()
    {
        var r = w.Call(w.Manager, "tab switch", ("tab", w.Projects.Id));
        Is(0, r);
        Assert.Equal(w.Projects.Id, w.Host.GetWorkspace(w.Ws.Id).ActiveTabId);
        Assert.Equal(w.Projects.Id, World.Out(r)["active"]!.GetValue<string>());
    }

    [Fact]
    public void Tab_switch_with_admin_but_no_focus_is_denied_3__admin_is_not_focus()
    {
        var adm = CapBlock(Caps.Admin);
        Is(3, w.Call(adm, "tab switch", ("tab", w.Projects.Id)));
    }

}
