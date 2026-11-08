// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// Contract section 11 item 4: ownership matrix of 3.2 / 3.3 / 3.4 with three tokens (Manager: focus+admin+grant, S1, S2) plus a child block that S1
// creates (a createdby chain: S1 -> Child -> Grand). Each test name says what rule it pins; the test is red when that rule is missing.
public sealed partial class OwnershipTests : IDisposable
{
    readonly World w = new();
    readonly Block child;
    readonly Block grand;
    readonly Tab childTab;
    readonly Tab grandTab;

    public OwnershipTests()
    {
        child = Born(Start(w.S1));
        childTab = w.Host.GetTab(child.TabId);
        grand = Born(Start(child));
        grandTab = w.Host.GetTab(grand.TabId);
    }

    public void Dispose() => w.Dispose();

    // ---- helpers ----

    static readonly string[] Prog = { "tool.exe" };

    Reply Start(Block caller, params (string, object?)[] more)
    {
        var l = new List<(string, object?)> { ("argv", Prog) };
        l.AddRange(more);
        return w.Call(caller, "session start", l.ToArray());
    }

    Reply Run(Block caller, params (string, object?)[] more)
    {
        var l = new List<(string, object?)> { ("argv", Prog) };
        l.AddRange(more);
        return w.Call(caller, "run", l.ToArray());
    }

    Block Born(Reply r)
    {
        Assert.True(r.Exit == 0, "expected a block, got exit " + r.Exit + " " + r.Err?.ToJsonString());
        return w.Host.GetBlock(World.Out(r)["blockid"]!.GetValue<string>());
    }

    static void Is(int code, Reply r)
    {
        Assert.True(code == r.Exit, $"expected exit {code}, got {r.Exit} {r.Err?.ToJsonString()}");
        if (code != 0) Assert.Equal(code, r.Err!["error"]!["code"]!.GetValue<int>());
    }

    Block CapBlock(params string[] caps)
    {
        var r = Start(w.Manager, ("cap", caps));
        return Born(r);
    }

    int BlockCount() => w.Host.AllBlocks().Count;

    int TabCount() => w.Host.GetWorkspace(w.Ws.Id).Tabs.Count;

    Tile TileOf(Block b) => w.Host.GetTab(b.TabId).Tiles.First(t => t.Members.Contains(b.Id));

    string Missing() => Guid.NewGuid().ToString();

    (Workspace Ws, Tab Tab, Block Block) SecondWorkspace()
    {
        var ws2 = w.Host.CreateWorkspace();
        var t2 = w.Host.CreateTabAsApp(ws2, "Other");
        var x2 = w.Host.StartBlockAsApp(t2, new[] { "pwsh", "x2.ps1" }, w.Cwd, title: "x2");
        return (ws2, t2, x2);
    }

    Block CrossBlock(params string[] caps) =>
        w.Host.StartBlockAsApp(w.Projects, new[] { "pwsh", "cx.ps1" }, w.Cwd, caps.Append(Caps.CrossWorkspace), title: "cx");

    static string[] CapsOf(Reply r) => World.Out(r)["caps"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray();

    // ---- send (ACT) ----

    [Fact]
    public void Send_to_own_block_is_allowed()
    {
        Is(0, w.Call(w.S1, "send", ("block", "this"), ("tokens", new[] { "hi" })));
        Assert.Single(w.S1.Input);
    }

    [Fact]
    public void Send_to_another_session_is_denied_3_and_nothing_is_typed__red_without_ownership_check()
    {
        Is(3, w.Call(w.S1, "send", ("block", w.S2.Id), ("tokens", new[] { "rm" })));
        Assert.Empty(w.S2.Input);
    }

    [Fact]
    public void Send_to_the_manager_by_a_session_is_denied_3()
    {
        Is(3, w.Call(w.S2, "send", ("block", w.Manager.Id), ("tokens", new[] { "x" })));
        Assert.Empty(w.Manager.Input);
    }

    [Fact]
    public void Send_to_a_block_the_caller_created_is_allowed__red_without_createdby()
    {
        Is(0, w.Call(w.S1, "send", ("block", child.Id), ("tokens", new[] { "x" })));
        Assert.Single(child.Input);
    }

    [Fact]
    public void Send_to_a_grandchild_follows_the_createdby_chain__red_if_only_direct_creator_counts()
    {
        Is(0, w.Call(w.S1, "send", ("block", grand.Id), ("tokens", new[] { "x" })));
        Assert.Single(grand.Input);
    }

    [Fact]
    public void Send_by_a_child_to_its_creator_is_denied_3__red_if_authority_passes_up()
    {
        Is(3, w.Call(child, "send", ("block", w.S1.Id), ("tokens", new[] { "x" })));
        Is(3, w.Call(grand, "send", ("block", child.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Send_by_a_sibling_session_to_the_child_is_denied_3()
    {
        Is(3, w.Call(w.S2, "send", ("block", child.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Send_with_admin_reaches_blocks_it_did_not_create__red_without_admin_rule()
    {
        Is(0, w.Call(w.Manager, "send", ("block", w.S2.Id), ("tokens", new[] { "x" })));
        Assert.Single(w.S2.Input);
    }

    [Fact]
    public void Send_to_an_unknown_id_is_exit_4()
    {
        Is(4, w.Call(w.S1, "send", ("block", Missing()), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Send_with_a_non_uuid_id_is_usage_2()
    {
        Is(2, w.Call(w.S1, "send", ("block", "12345"), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Send_to_a_finished_program_is_refused_5_even_for_the_owner()
    {
        w.Host.FinishProgram(w.S1.Id, 0);
        Is(5, w.Call(w.S1, "send", ("block", "this"), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Send_denial_comes_before_the_state_check__red_if_state_leaks_to_non_owners()
    {
        w.Host.FinishProgram(w.S2.Id, 0);
        Is(3, w.Call(w.S1, "send", ("block", w.S2.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void A_call_without_a_token_is_denied_3()
    {
        Is(3, w.CallNoToken("send", World.Args(("block", w.S1.Id), ("tokens", new[] { "x" }))));
    }

    // ---- block delete (ACT + safety) ----

    [Fact]
    public void Delete_of_another_session_is_denied_3_and_the_block_stays()
    {
        Is(3, w.Call(w.S1, "block delete", ("block", w.S2.Id), ("force", true)));
        Assert.True(w.Host.HasBlock(w.S2.Id));
    }

    [Fact]
    public void Delete_denial_comes_before_the_running_refusal()
    {
        Is(3, w.Call(w.S1, "block delete", ("block", w.S2.Id)));
    }

    [Fact]
    public void Delete_of_the_manager_by_a_session_is_denied_3()
    {
        Is(3, w.Call(w.S1, "block delete", ("block", w.Manager.Id), ("force", true)));
        Assert.True(w.Host.HasBlock(w.Manager.Id));
    }

    [Fact]
    public void Delete_of_a_running_child_without_force_is_refused_5__red_without_safety_rule()
    {
        Is(5, w.Call(w.S1, "block delete", ("block", child.Id)));
        Assert.True(w.Host.HasBlock(child.Id));
    }

    [Fact]
    public void Delete_of_a_running_child_with_force_is_allowed()
    {
        var r = w.Call(w.S1, "block delete", ("block", child.Id), ("force", true));
        Is(0, r);
        Assert.False(w.Host.HasBlock(child.Id));
        Assert.Equal(child.Id, World.Out(r)["deleted"]!.GetValue<string>());
    }

    [Fact]
    public void Delete_of_a_finished_child_needs_no_force()
    {
        w.Host.FinishProgram(child.Id, 0);
        Is(0, w.Call(w.S1, "block delete", ("block", child.Id)));
    }

    [Fact]
    public void Delete_of_the_grandchild_by_the_root_creator_is_allowed()
    {
        Is(0, w.Call(w.S1, "block delete", ("block", grand.Id), ("force", true)));
        Assert.False(w.Host.HasBlock(grand.Id));
    }

    [Fact]
    public void Delete_of_the_creator_by_its_child_is_denied_3()
    {
        Is(3, w.Call(grand, "block delete", ("block", child.Id), ("force", true)));
        Assert.True(w.Host.HasBlock(child.Id));
    }

    [Fact]
    public void Delete_with_admin_is_allowed_with_force_and_refused_5_without()
    {
        Is(5, w.Call(w.Manager, "block delete", ("block", w.S2.Id)));
        Is(0, w.Call(w.Manager, "block delete", ("block", w.S2.Id), ("force", true)));
    }

    [Fact]
    public void A_block_deleting_itself_needs_force_5_without_and_works_with()
    {
        Is(5, w.Call(w.S1, "block delete", ("block", "this")));
        Is(0, w.Call(w.S1, "block delete", ("block", "this"), ("force", true)));
        Assert.False(w.Host.HasBlock(w.S1.Id));
    }

    [Fact]
    public void Delete_of_an_unknown_block_is_exit_4()
    {
        Is(4, w.Call(w.S1, "block delete", ("block", Missing()), ("force", true)));
    }

    [Fact]
    public void Delete_without_an_id_is_usage_2()
    {
        Is(2, w.Call(w.S1, "block delete"));
        Assert.True(w.Host.HasBlock(w.S1.Id));
    }

    [Fact]
    public void Unknown_status_counts_as_running_for_delete__red_if_none_is_treated_as_finished()
    {
        w.Host.GetBlock(child.Id).Status = Status.Init;
        Is(5, w.Call(w.S1, "block delete", ("block", child.Id)));
    }

    // ---- session close ----

    [Fact]
    public void Close_of_a_tab_the_caller_created_with_a_running_block_needs_force()
    {
        var r = w.Call(w.S1, "session close", ("tab", childTab.Id));
        Is(5, r);
        Assert.Contains(child.Id, World.ErrMessage(r));
        Assert.True(w.Host.HasTab(childTab.Id));
    }

    [Fact]
    public void Close_with_force_removes_the_tab_and_its_blocks()
    {
        var r = w.Call(w.S1, "session close", ("tab", childTab.Id), ("force", true));
        Is(0, r);
        Assert.False(w.Host.HasTab(childTab.Id));
        Assert.False(w.Host.HasBlock(child.Id));
        Assert.Equal(1, World.Out(r)["blocks"]!.GetValue<int>());
    }

    [Fact]
    public void Close_of_a_tab_with_only_finished_blocks_needs_no_force()
    {
        w.Host.FinishProgram(child.Id, 0);
        Is(0, w.Call(w.S1, "session close", ("tab", childTab.Id)));
    }

    [Fact]
    public void Close_of_the_grandchild_tab_by_the_root_creator_is_allowed()
    {
        Is(0, w.Call(w.S1, "session close", ("tab", grandTab.Id), ("force", true)));
        Assert.False(w.Host.HasBlock(grand.Id));
    }

    [Fact]
    public void Close_of_own_tab_is_refused_5_even_with_force__red_without_own_tab_rule()
    {
        Is(5, w.Call(child, "session close", ("tab", "tab"), ("force", true)));
        Is(5, w.Call(child, "session close", ("tab", childTab.Id), ("force", true)));
        Is(5, w.Call(w.Manager, "session close", ("tab", w.Home.Id), ("force", true)));
        Assert.True(w.Host.HasTab(childTab.Id));
        Assert.True(w.Host.HasTab(w.Home.Id));
    }

    [Fact]
    public void Close_of_a_tab_holding_another_sessions_block_is_denied_3_for_the_creator_and_nothing_is_closed()
    {
        var foreign = Born(Run(w.S2, ("stack", child.Id)));
        Assert.Equal(childTab.Id, foreign.TabId);
        Is(3, w.Call(w.S1, "session close", ("tab", childTab.Id), ("force", true)));
        Assert.True(w.Host.HasTab(childTab.Id));
        Assert.True(w.Host.HasBlock(child.Id));
        Assert.True(w.Host.HasBlock(foreign.Id));
    }

    [Fact]
    public void Close_with_a_foreign_block_is_all_or_nothing_even_when_the_own_block_is_first()
    {
        var foreign = Born(Run(w.S2, ("stack", child.Id)));
        var before = BlockCount();
        Is(3, w.Call(w.S1, "session close", ("tab", childTab.Id), ("force", true)));
        Assert.Equal(before, BlockCount());
        Assert.Contains(child.Id, TileOf(foreign).Members);
    }

    [Fact]
    public void Close_of_a_tab_with_a_foreign_block_by_admin_is_allowed()
    {
        var foreign = Born(Run(w.S2, ("stack", child.Id)));
        var r = w.Call(w.Manager, "session close", ("tab", childTab.Id), ("force", true));
        Is(0, r);
        Assert.Equal(2, World.Out(r)["blocks"]!.GetValue<int>());
        Assert.False(w.Host.HasBlock(foreign.Id));
    }

    [Fact]
    public void Close_of_a_shared_area_tab_with_another_sessions_block_is_denied_3()
    {
        Is(3, w.Call(w.S1, "session close", ("tab", w.Home.Id), ("force", true)));
        Assert.True(w.Host.HasBlock(w.Manager.Id));
    }

    [Fact]
    public void Close_of_an_empty_area_tab_by_a_session_is_denied_3__a_tab_made_by_the_app_is_not_a_sessions_to_close()
    {
        Is(3, w.Call(w.S1, "session close", ("tab", w.Managers.Id), ("force", true)));
        Assert.True(w.Host.HasTab(w.Managers.Id));
    }

    [Fact]
    public void Close_of_an_empty_area_tab_by_admin_is_allowed()
    {
        Is(0, w.Call(w.Manager, "session close", ("tab", w.Managers.Id)));
        Assert.False(w.Host.HasTab(w.Managers.Id));
    }

    [Fact]
    public void Close_of_the_last_tab_of_a_window_is_refused_5_even_with_force_and_admin()
    {
        var (_, t2, x2) = SecondWorkspace();
        var cx = CrossBlock(Caps.Admin);
        Is(5, w.Call(cx, "session close", ("tab", t2.Id), ("force", true)));
        Assert.True(w.Host.HasTab(t2.Id));
        Assert.True(w.Host.HasBlock(x2.Id));
    }

    [Fact]
    public void Close_of_an_unknown_tab_is_exit_4()
    {
        Is(4, w.Call(w.S1, "session close", ("tab", Missing()), ("force", true)));
    }

    [Fact]
    public void Close_without_a_tab_is_usage_2()
    {
        Is(2, w.Call(w.S1, "session close"));
    }

    [Fact]
    public void Close_of_a_tab_in_another_workspace_without_crossworkspace_is_exit_4_even_for_admin()
    {
        var (_, t2, _) = SecondWorkspace();
        Is(4, w.Call(w.Manager, "session close", ("tab", t2.Id), ("force", true)));
        Assert.True(w.Host.HasTab(t2.Id));
    }

    [Fact]
    public void Close_revokes_the_tokens_of_the_closed_blocks()
    {
        Is(0, w.Call(w.S1, "session close", ("tab", childTab.Id), ("force", true)));
        Is(3, w.Call(child, "whoami"));
    }

    // ---- meta set / unset on others ----

    [Fact]
    public void Meta_set_on_own_block_is_allowed()
    {
        Is(0, w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:model=opus" })));
        Assert.Equal("opus", w.S1.Session["session:model"]);
    }

    [Fact]
    public void Meta_set_on_another_session_is_denied_3_and_nothing_changes()
    {
        Is(3, w.Call(w.S1, "meta set", ("block", w.S2.Id), ("pairs", new[] { "session:model=x" })));
        Assert.False(w.S2.Session.ContainsKey("session:model"));
    }

    [Fact]
    public void Meta_unset_on_another_session_is_denied_3()
    {
        w.S2.Session["session:model"] = "keep";
        Is(3, w.Call(w.S1, "meta unset", ("block", w.S2.Id), ("keys", new[] { "session:model" })));
        Assert.Equal("keep", w.S2.Session["session:model"]);
    }

    [Fact]
    public void Meta_set_on_a_created_child_and_grandchild_is_allowed()
    {
        Is(0, w.Call(w.S1, "meta set", ("block", child.Id), ("pairs", new[] { "frame:title=a" })));
        Is(0, w.Call(w.S1, "meta set", ("block", grand.Id), ("pairs", new[] { "frame:title=b" })));
        Assert.Equal("b", grand.Title);
    }

    [Fact]
    public void Meta_set_by_a_child_on_its_creator_is_denied_3()
    {
        Is(3, w.Call(child, "meta set", ("block", w.S1.Id), ("pairs", new[] { "frame:title=x" })));
    }

    [Fact]
    public void Meta_set_on_a_session_by_admin_is_allowed()
    {
        Is(0, w.Call(w.Manager, "meta set", ("block", w.S2.Id), ("pairs", new[] { "frame:title=by-manager" })));
        Assert.Equal("by-manager", w.S2.Title);
    }

    [Fact]
    public void Meta_set_denial_comes_before_key_validation_for_non_owners__red_if_validation_leaks_state()
    {
        Is(3, w.Call(w.S1, "meta set", ("block", w.S2.Id), ("pairs", new[] { "cmd=evil" })));
    }

}
