// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// ---------------------------------------------------------------------------------------------------------------------------------
// (d) attention and badge
// ---------------------------------------------------------------------------------------------------------------------------------
public sealed class CrAttentionBadgeTests : CrBase
{
    Reply Att(Block caller, string level, params (string Key, object? Value)[] kv) =>
        w.Call(caller, "attention", kv.Prepend(("level", (object?)level)).ToArray());

    Reply Badge(Block caller, string? icon, double? priority = null, string? color = null, bool clear = false, string? block = null, string? tab = null)
    {
        var o = new JsonObject();
        if (icon != null) o["icon"] = icon;
        if (priority != null) o["priority"] = priority.Value;
        if (color != null) o["color"] = color;
        if (clear) o["clear"] = true;
        if (block != null) o["block"] = block;
        if (tab != null) o["tab"] = tab;
        return w.Call(caller, "badge", o);
    }

    [Fact]
    public void Info_needs_no_reason_and_prints_block_and_level_only()
    {
        var r = Att(w.S1, "info");
        Assert.Equal(0, r.Exit);
        var o = World.Out(r).AsObject();
        Assert.Equal(new[] { "block", "level" }, o.Select(kv => kv.Key).OrderBy(x => x).ToArray());
        Assert.Equal(w.S1.Id, o["block"]!.GetValue<string>());
        Assert.Equal("info", o["level"]!.GetValue<string>());
        Assert.Equal("info", w.S1.AttentionLevel);
        Assert.Null(w.S1.AttentionReason);
    }

    [Theory]
    [InlineData("need")]
    [InlineData("urgent")]
    public void Need_and_urgent_with_a_reason_print_block_level_and_reason(string level)
    {
        var r = Att(w.S1, level, ("reason", "build is red"));
        Assert.Equal(0, r.Exit);
        var o = World.Out(r).AsObject();
        Assert.Equal(new[] { "block", "level", "reason" }, o.Select(kv => kv.Key).OrderBy(x => x).ToArray());
        Assert.Equal(level, o["level"]!.GetValue<string>());
        Assert.Equal("build is red", o["reason"]!.GetValue<string>());
        Assert.Equal(level, w.S1.AttentionLevel);
        Assert.Equal("build is red", w.S1.AttentionReason);
    }

    [Theory]
    [InlineData("need")]
    [InlineData("urgent")]
    public void Need_and_urgent_without_a_reason_are_exit_2_and_change_nothing(string level)
    {
        Assert.Equal(2, Att(w.S1, level).Exit);
        Assert.Null(w.S1.AttentionLevel);
    }

    [Theory]
    [InlineData("need")]
    [InlineData("urgent")]
    public void A_blank_reason_counts_as_missing(string level)
    {
        Assert.Equal(2, Att(w.S1, level, ("reason", "   ")).Exit);
        Assert.Equal(2, Att(w.S1, level, ("reason", "")).Exit);
        Assert.Null(w.S1.AttentionLevel);
    }

    [Theory]
    [InlineData("loud")]
    [InlineData("NEED")]
    [InlineData("")]
    public void An_unknown_level_is_exit_2(string level)
    {
        Assert.Equal(2, Att(w.S1, level, ("reason", "x")).Exit);
        Assert.Null(w.S1.AttentionLevel);
    }

    [Fact]
    public void A_missing_level_is_exit_2()
    {
        Assert.Equal(2, w.Call(w.S1, "attention", ("reason", "x")).Exit);
    }

    [Fact]
    public void A_reason_is_limited_to_120_code_points()
    {
        Assert.Equal(0, Att(w.S1, "need", ("reason", CrH.Rep("a", 120))).Exit);
        Assert.Equal(0, Att(w.S1, "need", ("reason", CrH.Rep("\U0001F600", 120))).Exit);
        Assert.Equal(2, Att(w.S1, "need", ("reason", CrH.Rep("a", 121))).Exit);
        Assert.Equal(2, Att(w.S1, "need", ("reason", CrH.Rep("\U0001F600", 121))).Exit);
        Assert.Equal(120, w.S1.AttentionReason!.EnumerateRunes().Count());
    }

    [Fact]
    public void A_refused_reason_leaves_the_previous_mark_alone()
    {
        Att(w.S1, "need", ("reason", "first"));
        Assert.Equal(2, Att(w.S1, "urgent", ("reason", CrH.Rep("a", 121))).Exit);
        Assert.Equal("need", w.S1.AttentionLevel);
        Assert.Equal("first", w.S1.AttentionReason);
    }

    [Fact]
    public void A_reason_is_trimmed_and_the_limit_applies_after_trimming()
    {
        var r = Att(w.S1, "need", ("reason", "  hi there   "));
        Assert.Equal(0, r.Exit);
        Assert.Equal("hi there", World.Out(r)["reason"]!.GetValue<string>());
        Assert.Equal("hi there", w.S1.AttentionReason);
        Assert.Equal(0, Att(w.S1, "need", ("reason", "  " + CrH.Rep("a", 120) + "  ")).Exit);
    }

    [Theory]
    [InlineData("a\nb")]
    [InlineData("\nabc")]
    [InlineData("abc\n")]
    [InlineData("a\u202Eb")]
    [InlineData("a\u200Bb")]
    [InlineData("a\u0007b")]
    [InlineData("a\u2028b")]
    public void A_reason_with_a_refused_character_is_exit_2_even_at_the_edges(string reason)
    {
        var r = Att(w.S1, "need", ("reason", reason));
        Assert.Equal(2, r.Exit);
        Assert.Null(w.S1.AttentionLevel);
    }

    [Fact]
    public void An_emoji_with_a_variation_selector_is_a_valid_reason()
    {
        Assert.Equal(0, Att(w.S1, "need", ("reason", "ok \u2764\uFE0F")).Exit);
    }

    [Fact]
    public void Attention_on_another_block_needs_admin()
    {
        Assert.Equal(3, Att(w.S1, "need", ("reason", "x"), ("block", w.S2.Id)).Exit);
        Assert.Null(w.S2.AttentionLevel);
        Assert.Equal(0, Att(w.Manager, "need", ("reason", "x"), ("block", w.S2.Id)).Exit);
        Assert.Equal("need", w.S2.AttentionLevel);
    }

    [Fact]
    public void Attention_on_a_block_the_caller_created_is_allowed()
    {
        var id = CrH.Str(w.Call(w.S1, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" })), "blockid");
        Assert.Equal(0, Att(w.S1, "info", ("block", id)).Exit);
        Assert.Equal("info", w.Host.GetBlock(id).AttentionLevel);
    }

    [Fact]
    public void Attention_on_an_unknown_block_is_exit_4()
    {
        Assert.Equal(4, Att(w.Manager, "info", ("block", Guid.NewGuid().ToString())).Exit);
    }

    [Fact]
    public void Clear_empties_the_mark_and_prints_an_empty_level_without_reason()
    {
        Att(w.S1, "urgent", ("reason", "x"));
        var r = Att(w.S1, "clear");
        Assert.Equal(0, r.Exit);
        var o = World.Out(r).AsObject();
        Assert.Equal("", o["level"]!.GetValue<string>());
        Assert.False(o.ContainsKey("reason"));
        Assert.Equal(w.S1.Id, o["block"]!.GetValue<string>());
        Assert.Null(w.S1.AttentionLevel);
        Assert.Null(w.S1.AttentionReason);
    }

    [Fact]
    public void Clear_on_an_unmarked_block_is_fine_and_prints_an_empty_level()
    {
        var r = Att(w.S1, "clear");
        Assert.Equal(0, r.Exit);
        Assert.Equal("", World.Out(r)["level"]!.GetValue<string>());
    }

    [Fact]
    public void Compare_and_clear_clears_when_level_and_reason_match()
    {
        Att(w.S1, "need", ("reason", "waiting"));
        var r = Att(w.S1, "clear", ("onlyIfLevel", "need"), ("onlyIfReason", "waiting"));
        Assert.Equal(0, r.Exit);
        Assert.Equal("", World.Out(r)["level"]!.GetValue<string>());
        Assert.Null(w.S1.AttentionLevel);
    }

    [Fact]
    public void Compare_and_clear_leaves_a_changed_level_alone_and_returns_the_current_mark()
    {
        Att(w.S1, "need", ("reason", "waiting"));
        Att(w.S1, "urgent", ("reason", "waiting"));
        var r = Att(w.S1, "clear", ("onlyIfLevel", "need"), ("onlyIfReason", "waiting"));
        Assert.Equal(0, r.Exit);
        Assert.Equal("urgent", World.Out(r)["level"]!.GetValue<string>());
        Assert.Equal("waiting", World.Out(r)["reason"]!.GetValue<string>());
        Assert.Equal("urgent", w.S1.AttentionLevel);
    }

    [Fact]
    public void Compare_and_clear_leaves_a_changed_reason_alone_and_returns_the_current_mark()
    {
        Att(w.S1, "need", ("reason", "new reason"));
        var r = Att(w.S1, "clear", ("onlyIfLevel", "need"), ("onlyIfReason", "old reason"));
        Assert.Equal("need", World.Out(r)["level"]!.GetValue<string>());
        Assert.Equal("new reason", World.Out(r)["reason"]!.GetValue<string>());
        Assert.Equal("need", w.S1.AttentionLevel);
        Assert.Equal("new reason", w.S1.AttentionReason);
    }

    [Fact]
    public void Compare_and_clear_on_an_unmarked_block_changes_nothing()
    {
        var r = Att(w.S1, "clear", ("onlyIfLevel", "need"), ("onlyIfReason", "x"));
        Assert.Equal(0, r.Exit);
        Assert.Equal("", World.Out(r)["level"]!.GetValue<string>());
        Assert.Null(w.S1.AttentionLevel);
    }

    [Fact]
    public void Compare_and_clear_compares_the_trimmed_stored_reason()
    {
        Att(w.S1, "need", ("reason", "  padded  "));
        Att(w.S1, "clear", ("onlyIfLevel", "need"), ("onlyIfReason", "padded"));
        Assert.Null(w.S1.AttentionLevel);
    }

    [Fact]
    public void Attention_is_visible_in_block_list_meta_for_every_token()
    {
        Att(w.S1, "need", ("reason", "see me"));
        var list = World.Out(w.Call(w.S2, "block list")).AsArray();
        var meta = list.Single(x => x!["blockid"]!.GetValue<string>() == w.S1.Id)!["meta"]!;
        Assert.Equal("need", meta["attention:level"]!.GetValue<string>());
        Assert.Equal("see me", meta["attention:reason"]!.GetValue<string>());
    }

    // ---- badge ----

    [Fact]
    public void A_first_badge_is_applied_and_stored()
    {
        var r = Badge(w.S1, "rocket", 5, "red");
        Assert.Equal(0, r.Exit);
        var o = World.Out(r).AsObject();
        Assert.True(o["applied"]!.GetValue<bool>());
        Assert.False(o.ContainsKey("reason"));
        var s = w.Host.BadgeOf(w.S1.Id)!;
        Assert.Equal("rocket", s.Icon);
        Assert.Equal("red", s.Color);
        Assert.Equal(5, s.Priority);
    }

    [Fact]
    public void A_lower_priority_returns_applied_false_with_a_reason_and_keeps_the_badge()
    {
        Badge(w.S1, "rocket", 5);
        var r = Badge(w.S1, "star", 3);
        Assert.Equal(0, r.Exit);
        var o = World.Out(r).AsObject();
        Assert.False(o["applied"]!.GetValue<bool>());
        Assert.Equal("lower priority", o["reason"]!.GetValue<string>());
        Assert.Equal("rocket", w.Host.BadgeOf(w.S1.Id)!.Icon);
        Assert.Equal(5, w.Host.BadgeOf(w.S1.Id)!.Priority);
    }

    [Fact]
    public void An_equal_priority_replaces_the_badge()
    {
        Badge(w.S1, "rocket", 5);
        var r = Badge(w.S1, "star", 5);
        Assert.True(World.Out(r)["applied"]!.GetValue<bool>());
        Assert.Equal("star", w.Host.BadgeOf(w.S1.Id)!.Icon);
    }

    [Fact]
    public void A_higher_priority_replaces_the_badge()
    {
        Badge(w.S1, "rocket", 5);
        var r = Badge(w.S1, "star", 6.5);
        Assert.True(World.Out(r)["applied"]!.GetValue<bool>());
        Assert.Equal("star", w.Host.BadgeOf(w.S1.Id)!.Icon);
        Assert.Equal(6.5, w.Host.BadgeOf(w.S1.Id)!.Priority);
    }

    [Fact]
    public void Clear_removes_the_badge_and_a_lower_priority_then_applies()
    {
        Badge(w.S1, "rocket", 9);
        var r = Badge(w.S1, null, clear: true);
        Assert.Equal(0, r.Exit);
        Assert.True(World.Out(r)["applied"]!.GetValue<bool>());
        Assert.Null(w.Host.BadgeOf(w.S1.Id));
        Assert.True(World.Out(Badge(w.S1, "star", 1))["applied"]!.GetValue<bool>());
    }

    [Fact]
    public void Clear_without_a_badge_is_fine()
    {
        var r = Badge(w.S1, null, clear: true);
        Assert.Equal(0, r.Exit);
        Assert.Null(w.Host.BadgeOf(w.S1.Id));
    }

    [Fact]
    public void A_badge_on_one_block_does_not_touch_another()
    {
        Badge(w.S1, "rocket", 5);
        Assert.Null(w.Host.BadgeOf(w.S2.Id));
        Badge(w.Manager, "star", 1, block: w.S2.Id);
        Assert.Equal("rocket", w.Host.BadgeOf(w.S1.Id)!.Icon);
        Assert.Equal("star", w.Host.BadgeOf(w.S2.Id)!.Icon);
    }

    [Fact]
    public void A_badge_on_another_block_needs_admin()
    {
        var r = Badge(w.S1, "rocket", 5, block: w.S2.Id);
        Assert.Equal(3, r.Exit);
        Assert.Null(w.Host.BadgeOf(w.S2.Id));
        Assert.Equal(0, Badge(w.Manager, "rocket", 5, block: w.S2.Id).Exit);
        Assert.NotNull(w.Host.BadgeOf(w.S2.Id));
    }

    [Fact]
    public void A_tab_target_badges_the_tab_the_caller_owns()
    {
        var tabId = CrH.Str(w.Call(w.S1, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" })), "tabid");
        var r = Badge(w.S1, "rocket", 5, tab: tabId);
        Assert.Equal(0, r.Exit);
        Assert.True(World.Out(r)["applied"]!.GetValue<bool>());
        Assert.Equal("rocket", w.Host.BadgeOf(tabId)!.Icon);
        Assert.Null(w.Host.BadgeOf(w.S1.Id));
        Assert.False(World.Out(Badge(w.S1, "star", 2, tab: tabId))["applied"]!.GetValue<bool>());
        Assert.Equal(0, Badge(w.S1, null, clear: true, tab: tabId).Exit);
        Assert.Null(w.Host.BadgeOf(tabId));
    }

    [Fact]
    public void A_tab_target_that_is_not_the_callers_needs_admin()
    {
        Assert.Equal(3, Badge(w.S1, "rocket", 5, tab: w.Home.Id).Exit);
        Assert.Null(w.Host.BadgeOf(w.Home.Id));
        Assert.Equal(0, Badge(w.Manager, "rocket", 5, tab: w.Projects.Id).Exit);
        Assert.NotNull(w.Host.BadgeOf(w.Projects.Id));
    }

    [Fact]
    public void Both_block_and_tab_is_exit_2_and_sets_nothing()
    {
        var r = Badge(w.Manager, "rocket", 5, block: "this", tab: "tab");
        Assert.Equal(2, r.Exit);
        Assert.Null(w.Host.BadgeOf(w.Manager.Id));
        Assert.Null(w.Host.BadgeOf(w.Manager.TabId));
        var r2 = Badge(w.Manager, "rocket", 5, block: w.S1.Id, tab: w.Projects.Id);
        Assert.Equal(2, r2.Exit);
        Assert.Null(w.Host.BadgeOf(w.S1.Id));
        Assert.Null(w.Host.BadgeOf(w.Projects.Id));
    }

    [Fact]
    public void Both_block_and_tab_is_exit_2_for_a_clear_too()
    {
        Badge(w.Manager, "rocket", 5);
        Assert.Equal(2, Badge(w.Manager, null, clear: true, block: "this", tab: "tab").Exit);
        Assert.NotNull(w.Host.BadgeOf(w.Manager.Id));
    }

    [Theory]
    [InlineData("rocket")]
    [InlineData("a-b-9")]
    [InlineData("\U0001F600")]
    [InlineData("OK")]
    public void A_valid_icon_is_applied(string icon)
    {
        Assert.True(World.Out(Badge(w.S1, icon, 1))["applied"]!.GetValue<bool>());
        Assert.Equal(icon, w.Host.BadgeOf(w.S1.Id)!.Icon);
    }

    [Theory]
    [InlineData("a<b")]
    [InlineData("a&b")]
    [InlineData("Bad Icon!")]
    [InlineData("\U0001F600\U0001F600\U0001F600")]
    [InlineData("a\nb")]
    public void An_invalid_icon_is_exit_2_and_changes_nothing(string icon)
    {
        Assert.Equal(2, Badge(w.S1, icon, 1).Exit);
        Assert.Null(w.Host.BadgeOf(w.S1.Id));
    }

    [Theory]
    [InlineData("red")]
    [InlineData("Red")]
    [InlineData("#ff0000")]
    [InlineData("#ff000080")]
    public void A_valid_color_is_accepted(string color)
    {
        Assert.Equal(0, Badge(w.S1, "rocket", 1, color).Exit);
        Assert.Equal(color.ToLowerInvariant(), w.Host.BadgeOf(w.S1.Id)!.Color!.ToLowerInvariant());
    }

    [Theory]
    [InlineData("notacolor")]
    [InlineData("#12345")]
    [InlineData("#ff00")]
    [InlineData("#gggggg")]
    public void An_invalid_color_is_exit_2_and_changes_nothing(string color)
    {
        Assert.Equal(2, Badge(w.S1, "rocket", 1, color).Exit);
        Assert.Null(w.Host.BadgeOf(w.S1.Id));
    }

    [Fact]
    public void The_real_client_priority_flag_is_honoured()
    {
        Assert.Equal(0, w.Cli(w.S1, "badge", "rocket", "--priority", "5").Exit);
        var (exit, stdout, _) = w.Cli(w.S1, "badge", "star", "--priority", "3");
        Assert.Equal(0, exit);
        Assert.False(JsonNode.Parse(stdout)!["applied"]!.GetValue<bool>());
        Assert.Equal("rocket", w.Host.BadgeOf(w.S1.Id)!.Icon);
    }

    [Fact]
    public void The_real_client_prints_the_attention_shape()
    {
        var (exit, stdout, stderr) = w.Cli(w.S1, "attention", "need", "--reason", "via cli");
        Assert.Equal(0, exit);
        Assert.Equal("", stderr);
        var o = JsonNode.Parse(stdout)!;
        Assert.Equal("need", o["level"]!.GetValue<string>());
        Assert.Equal("via cli", o["reason"]!.GetValue<string>());
        var (e2, s2, _) = w.Cli(w.S1, "attention", "clear", "--only-if-level", "urgent", "--only-if-reason", "via cli");
        Assert.Equal(0, e2);
        Assert.Equal("need", JsonNode.Parse(s2)!["level"]!.GetValue<string>());
    }
}
