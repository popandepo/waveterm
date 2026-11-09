// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// Anything that changes which tab or which stack member is shown is a FOCUS change (cli-contract.md 3.2): closing the shown tab and taking the shown
// member out of its stack need the focus capability, wherever the caller got its other rights from.
public sealed class FocusShownTests : IDisposable
{
    readonly World w = new();

    public void Dispose() => w.Dispose();

    string StackWithShownHiddenMember()
    {
        var made = World.Out(w.Call(w.S1, "run", ("stack", w.S2.Id), ("argv", new[] { "pwsh" })));
        var id = made["blockid"]!.GetValue<string>();
        Assert.Equal(0, w.Call(w.Manager, "group activate", ("block", id)).Exit);
        return id;
    }

    [Fact]
    public void A_session_cannot_close_the_shown_tab_it_owns_but_the_manager_can()
    {
        var started = World.Out(w.Call(w.S1, "session start", ("name", "scratch"), ("argv", new[] { "pwsh" })));
        var tab = started["tabid"]!.GetValue<string>();
        Assert.Equal(0, w.Call(w.Manager, "tab switch", ("tab", tab)).Exit);
        var r = w.Call(w.S1, "session close", ("tab", tab), ("force", true));
        Assert.Equal(3, r.Exit);
        Assert.Equal("capability", r.Err?["error"]?["reason"]?.GetValue<string>());
        Assert.True(w.Host.HasTab(tab));
        Assert.Equal(0, w.Call(w.Manager, "session close", ("tab", tab), ("force", true)).Exit);
        Assert.False(w.Host.HasTab(tab));
    }

    [Fact]
    public void A_session_can_still_close_a_tab_that_is_not_shown()
    {
        var started = World.Out(w.Call(w.S1, "session start", ("name", "scratch"), ("argv", new[] { "pwsh" })));
        var tab = started["tabid"]!.GetValue<string>();
        Assert.Equal(0, w.Call(w.S1, "session close", ("tab", tab), ("force", true)).Exit);
    }

    [Fact]
    public void A_session_cannot_delete_or_regroup_the_shown_member_of_a_stack_without_focus()
    {
        var shown = StackWithShownHiddenMember();
        var del = w.Call(w.S1, "block delete", ("block", shown), ("force", true));
        Assert.Equal(3, del.Exit);
        Assert.Equal("capability", del.Err?["error"]?["reason"]?.GetValue<string>());
        Assert.Equal(3, w.Call(w.S1, "group remove", ("block", shown)).Exit);
        Assert.Equal(3, w.Call(w.S1, "group add", ("block", shown), ("to", w.S1.Id)).Exit);
        var tile = w.Host.GetTab(w.Projects.Id).Tiles.First(t => t.Members.Contains(w.S2.Id));
        Assert.Equal(shown, tile.ActiveId);
        Assert.Contains(shown, tile.Members);
    }

    [Fact]
    public void A_session_can_still_delete_a_hidden_member_that_is_not_the_shown_one()
    {
        var shown = StackWithShownHiddenMember();
        var other = World.Out(w.Call(w.S1, "run", ("stack", w.S2.Id), ("argv", new[] { "pwsh" })))["blockid"]!.GetValue<string>();
        Assert.Equal(0, w.Call(w.S1, "block delete", ("block", other), ("force", true)).Exit);
        var tile = w.Host.GetTab(w.Projects.Id).Tiles.First(t => t.Members.Contains(w.S2.Id));
        Assert.Equal(shown, tile.ActiveId);
    }

    [Fact]
    public void The_manager_with_focus_may_take_the_shown_member_out()
    {
        var shown = StackWithShownHiddenMember();
        Assert.Equal(0, w.Call(w.Manager, "group remove", ("block", shown)).Exit);
        var tile = w.Host.GetTab(w.Projects.Id).Tiles.First(t => t.Members.Contains(w.S2.Id));
        Assert.Equal(w.S2.Id, tile.ActiveId);
    }
}
