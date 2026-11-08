// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// cli-contract.md v0.3 sections 4.7, 4.10, 4.11, 6 and 7: meta keys, the two text-safety predicates, send, screen and web text.
public sealed partial class MetaAndTextTests
{
    [Fact]
    public void A_json_with_the_same_key_twice_is_exit_2()
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaJson(w.S1, "this", "{\"session:a\":\"1\",\"session:a\":\"2\"}"), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Fact]
    public void A_key_given_twice_in_pairs_or_in_pairs_and_json_is_exit_2()
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaSet(w.S1, "this", "session:a=1", "session:a=2"), 2);
        AssertFail(MetaSet(w.S1, "this", "session:a=1", "session:a=1"), 2);
        var r = w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:a=1" }), ("json", "{\"session:a\":\"2\"}"));
        AssertFail(r, 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Theory]
    [InlineData("frame:title")]
    [InlineData("=value")]
    [InlineData("")]
    public void A_malformed_pair_is_exit_2(string pair)
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaSet(w.S1, "this", pair), 2);
        AssertFail(MetaSet(w.S1, "this", "session:ok=1", pair), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Fact]
    public void Set_without_any_pair_or_json_is_exit_2()
    {
        AssertFail(MetaSet(w.S1, "this"), 2);
    }

    [Fact]
    public void A_value_with_equals_signs_keeps_everything_after_the_first_one()
    {
        AssertOk(MetaSet(w.S1, "this", "session:q=a=b=c"));
        Assert.Equal("a=b=c", Get(w.S1, "session:q"));
    }

    [Fact]
    public void The_block_argument_is_this_or_a_full_id_else_usage_or_not_found()
    {
        AssertOk(MetaSet(w.S1, w.S1.Id, "session:a=1"));
        AssertFail(MetaSet(w.S1, "12345678", "session:a=1"), 2);
        AssertFail(MetaSet(w.S1, "tab", "session:a=1"), 2);
        AssertFail(MetaSet(w.S1, "", "session:a=1"), 2);
        AssertFail(MetaSet(w.S1, Guid.NewGuid().ToString(), "session:a=1"), 4);
    }

    [Fact]
    public void Setting_meta_on_a_block_the_caller_did_not_create_needs_admin()
    {
        AssertFail(MetaSet(w.S1, w.S2.Id, "frame:title=x"), 3);
        Assert.Equal("s2", Get(w.S2, "frame:title"));
        AssertOk(MetaSet(w.Manager, w.S2.Id, "frame:title=x"));
        Assert.Equal("x", Get(w.S2, "frame:title"));
        AssertFail(MetaUnset(w.S1, w.S2.Id, "frame:title"), 3);
        Assert.Equal("x", Get(w.S2, "frame:title"));
    }

    [Fact]
    public void Meta_set_with_a_json_value_and_session_name_uniqueness_leaves_nothing_changed_on_exit_5()
    {
        AssertOk(MetaSet(w.S1, "this", "session:name=alpha"));
        var before = Snapshot(w.S2);
        AssertFail(MetaSet(w.S2, "this", "frame:title=new", "session:name=ALPHA"), 5);
        Assert.Equal(before, Snapshot(w.S2));
    }

    // ---- meta get ----

    [Fact]
    public void Get_on_the_own_web_block_shows_the_full_url_and_another_block_a_redacted_one()
    {
        const string full = "https://user:pw@Example.com:8443/path/x?token=secret#frag";
        var web = NewWeb();
        AssertOk(MetaSet(web, "this", "url=" + full));
        Assert.Equal(full, Get(web, "url"));
        foreach (var reader in new[] { w.S1, w.Manager })
        {
            var o = World.Out(w.Call(reader, "meta get", ("items", new[] { web.Id, "url" })));
            var url = o["url"]!.GetValue<string>();
            Assert.Equal("https://example.com:8443/path/x", url, ignoreCase: false);
            Assert.DoesNotContain("secret", url);
            Assert.DoesNotContain("pw", url);
            Assert.DoesNotContain("frag", url);
        }
    }

    [Fact]
    public void Get_with_keys_returns_only_those_keys_and_a_missing_key_is_absent()
    {
        AssertOk(MetaSet(w.S1, "this", "session:a=1", "frame:title=t"));
        var o = World.Out(w.Call(w.S1, "meta get", ("items", new[] { "this", "session:a", "session:missing", "icon:color" }))).AsObject();
        Assert.Equal(new[] { "session:a" }, o.Select(kv => kv.Key).ToArray());
        Assert.Empty(World.Out(w.Call(w.S1, "meta get", ("items", new[] { "session:missing" }))).AsObject());
    }

    [Fact]
    public void Get_with_a_first_argument_that_is_not_this_or_an_id_reads_that_key_of_the_own_block()
    {
        AssertOk(MetaSet(w.S1, "this", "session:a=mine"));
        AssertOk(MetaSet(w.S2, "this", "session:a=theirs"));
        var o = World.Out(w.Call(w.S1, "meta get", ("items", new[] { "session:a" })));
        Assert.Equal("mine", o["session:a"]!.GetValue<string>());
        var other = World.Out(w.Call(w.S1, "meta get", ("items", new[] { w.S2.Id, "session:a" })));
        Assert.Equal("theirs", other["session:a"]!.GetValue<string>());
    }

    [Fact]
    public void Get_of_an_unknown_block_id_is_exit_4()
    {
        AssertFail(w.Call(w.S1, "meta get", ("items", new[] { Guid.NewGuid().ToString() })), 4);
    }

    [Fact]
    public void Get_never_returns_cmd_env_and_shows_cmd_keys_only_for_an_owned_block()
    {
        var own = World.Out(w.Call(w.S1, "meta get")).AsObject();
        Assert.False(own.ContainsKey("cmd:env"));
        Assert.True(own.ContainsKey("cmd"));
        Assert.True(own.ContainsKey("cmd:args"));
        Assert.True(own.ContainsKey("cmd:cwd"));
        Assert.False(World.Out(w.Call(w.Manager, "meta get", ("items", new[] { w.S1.Id }))).AsObject().ContainsKey("cmd:env"));
    }

    [Fact]
    public void Get_of_another_session_hides_cmd_keys_and_says_so_in_the_hidden_field()
    {
        var o = World.Out(w.Call(w.S2, "meta get", ("items", new[] { w.S1.Id }))).AsObject();
        Assert.False(o.ContainsKey("cmd"));
        Assert.False(o.ContainsKey("cmd:args"));
        Assert.False(o.ContainsKey("cmd:cwd"));
        Assert.NotNull(o["hidden"]);
        Assert.Equal(new[] { "cmd", "cmd:args", "cmd:cwd" }, o["hidden"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
        Assert.DoesNotContain(World.Out(w.Call(w.S2, "meta get", ("items", new[] { w.S1.Id, "cmd" }))).AsObject(), kv => kv.Key == "cmd");
    }

    [Fact]
    public void Get_of_the_own_block_or_with_admin_has_no_hidden_field()
    {
        Assert.False(World.Out(w.Call(w.S1, "meta get")).AsObject().ContainsKey("hidden"));
        Assert.False(World.Out(w.Call(w.Manager, "meta get", ("items", new[] { w.S1.Id }))).AsObject().ContainsKey("hidden"));
    }

    [Fact]
    public void Session_id_is_visible_to_every_token()
    {
        AssertOk(MetaSet(w.S1, "this", "session:id=abc-123"));
        var o = World.Out(w.Call(w.S2, "meta get", ("items", new[] { w.S1.Id, "session:id" })));
        Assert.Equal("abc-123", o["session:id"]!.GetValue<string>());
    }

    [Fact]
    public void Get_escapes_a_title_through_the_output_rule_while_set_accepted_the_zwj_family()
    {
        var title = "\U0001F468\u200D\U0001F469";
        AssertOk(MetaSet(w.S1, "this", "frame:title=" + title));
        Assert.Equal("\U0001F468<U+200D>\U0001F469", Get(w.S1, "frame:title"));
    }

    // ---- meta through the real client ----

    [Fact]
    public void Client_meta_set_and_get_with_pairs_roundtrip_and_a_duplicate_key_is_exit_2()
    {
        var (exit, stdout, _) = w.Cli(w.S1, "meta", "set", "this", "session:a=010", "frame:title=hi");
        Assert.Equal(0, exit);
        Assert.Equal(new[] { "frame:title", "session:a" }, JsonNode.Parse(stdout)!["set"]!.AsArray().Select(x => x!.GetValue<string>()).OrderBy(x => x, StringComparer.Ordinal).ToArray());
        var (e2, o2, _) = w.Cli(w.S1, "meta", "get", "session:a");
        Assert.Equal(0, e2);
        Assert.Equal("010", JsonNode.Parse(o2)!["session:a"]!.GetValue<string>());
        var (e3, so3, se3) = w.Cli(w.S1, "meta", "set", "this", "session:a=1", "session:a=2");
        Assert.Equal(2, e3);
        Assert.Equal("", so3);
        Assert.Equal("usage", JsonNode.Parse(se3)!["error"]!["kind"]!.GetValue<string>());
    }

    [Fact]
    public void Client_meta_set_json_file_dash_reads_the_object_from_stdin_and_json_with_json_file_is_exit_2()
    {
        var (exit, stdout, _) = w.CliWithEnv(w.S1, null, "{\"frame:title\":\"from stdin\",\"session:z\":\"1\"}", "meta", "set", "this", "--json-file", "-");
        Assert.Equal(0, exit);
        Assert.Equal(2, JsonNode.Parse(stdout)!["set"]!.AsArray().Count);
        Assert.Equal("from stdin", Get(w.S1, "frame:title"));
        var (e2, _, se2) = w.CliWithEnv(w.S1, null, "{\"session:z\":\"2\"}", "meta", "set", "this", "--json", "{\"session:z\":\"3\"}", "--json-file", "-");
        Assert.Equal(2, e2);
        Assert.Equal(2, JsonNode.Parse(se2)!["error"]!["code"]!.GetValue<int>());
        Assert.Equal("1", Get(w.S1, "session:z"));
    }

    [Fact]
    public void Client_meta_unset_deletes()
    {
        AssertOk(MetaSet(w.S1, "this", "session:gone=1"));
        var (exit, stdout, _) = w.Cli(w.S1, "meta", "unset", "this", "session:gone");
        Assert.Equal(0, exit);
        Assert.Equal("session:gone", JsonNode.Parse(stdout)!["unset"]![0]!.GetValue<string>());
        Assert.Null(Get(w.S1, "session:gone"));
    }

    // ======================================================================
    // (b) text safety, section 7
    // ======================================================================

    [Theory]
    [InlineData("\u0001", "<U+0001>")]
    [InlineData("\u001B", "<U+001B>")]
    [InlineData("\u007F", "<U+007F>")]
    [InlineData("\u0085", "<U+0085>")]
    [InlineData("\u009F", "<U+009F>")]
    [InlineData("\u00AD", "<U+00AD>")]
    [InlineData("\u200B", "<U+200B>")]
    [InlineData("\u200D", "<U+200D>")]
    [InlineData("\u2060", "<U+2060>")]
    [InlineData("\u202E", "<U+202E>")]
    [InlineData("\u2066", "<U+2066>")]
    [InlineData("\uFEFF", "<U+FEFF>")]
    [InlineData("\u2028", "<U+2028>")]
    [InlineData("\u2029", "<U+2029>")]
    [InlineData("\uFFFD", "<U+FFFD>")]
    [InlineData("\uFE00", "<U+FE00>")]
    [InlineData("\uFE0E", "<U+FE0E>")]
    [InlineData("\uFE0F", "<U+FE0F>")]
    [InlineData("\u180B", "<U+180B>")]
    [InlineData("\u180F", "<U+180F>")]
    [InlineData("\U000E0100", "<U+E0100>")]
    [InlineData("\U000E01EF", "<U+E01EF>")]
    [InlineData("\u034F", "<U+034F>")]
    [InlineData("\u115F", "<U+115F>")]
    [InlineData("\u1160", "<U+1160>")]
    [InlineData("\u17B4", "<U+17B4>")]
    [InlineData("\u2065", "<U+2065>")]
    [InlineData("\u3164", "<U+3164>")]
    [InlineData("\uFFA0", "<U+FFA0>")]
    [InlineData("\uFFF0", "<U+FFF0>")]
    [InlineData("\U000E0000", "<U+E0000>")]
    [InlineData("\U000E0020", "<U+E0020>")]
    [InlineData("\U0001D173", "<U+1D173>")]
    public void Unsafe_runes_become_upper_case_hex_escapes_with_at_least_four_digits(string input, string expected)
    {
        Assert.Equal(expected, TextSafety.Escape(input));
        Assert.Equal(expected, TextSafety.Escape("a" + input + "b").Substring(1, expected.Length));
    }

    [Theory]
    [InlineData("abc")]
    [InlineData("hello world")]
    [InlineData("\u00A0")]
    [InlineData("\u00E9")]
    [InlineData("\u3000")]
    [InlineData("\u2603")]
    [InlineData("\u65E5\u672C")]
    [InlineData("\U0001F600")]
    [InlineData("\U00010000")]
    public void Safe_text_passes_through_unchanged(string input)
    {
        Assert.Equal(input, TextSafety.Escape(input));
    }

    [Fact]
    public void Escape_keeps_newline_and_tab_when_asked_and_escapes_them_otherwise_but_always_escapes_carriage_return()
    {
        Assert.Equal("a\nb\tc", TextSafety.Escape("a\nb\tc"));
        Assert.Equal("a<U+000A>b<U+0009>c", TextSafety.Escape("a\nb\tc", keepNewlineAndTab: false));
        Assert.Equal("a<U+000D>\nb", TextSafety.Escape("a\r\nb"));
    }

    [Fact]
    public void Escape_of_a_lone_surrogate_does_not_throw_and_a_valid_pair_stays_one_rune()
    {
        Assert.Equal("<U+FFFD>", TextSafety.Escape("\uD800"));
        Assert.Equal("x<U+FFFD>y", TextSafety.Escape("x\uDC00y"));
        Assert.Equal("a\U0001F600b", TextSafety.Escape("a\U0001F600b"));
    }

    [Fact]
    public void Escape_never_leaves_an_emoji_variation_selector_or_joiner_unescaped()
    {
        Assert.Equal("\u2764<U+FE0F>", TextSafety.Escape(Heart));
        Assert.Equal("\U0001F468<U+200D>\U0001F469<U+200D>\U0001F467", TextSafety.Escape(Family));
    }

    [Theory]
    [InlineData("\u0001")]
    [InlineData("\u0007")]
    [InlineData("\u001B")]
    [InlineData("\u007F")]
    [InlineData("\u0085")]
    [InlineData("\n")]
    [InlineData("\r")]
    [InlineData("\t")]
    [InlineData("\u2028")]
    [InlineData("\u2029")]
    [InlineData("\u202A")]
    [InlineData("\u202B")]
    [InlineData("\u202C")]
    [InlineData("\u202D")]
    [InlineData("\u202E")]
    [InlineData("\u2066")]
    [InlineData("\u2067")]
    [InlineData("\u2068")]
    [InlineData("\u2069")]
    [InlineData("\u200B")]
    [InlineData("\u200C")]
    [InlineData("\u00AD")]
    [InlineData("\u2060")]
    [InlineData("\uFEFF")]
    [InlineData("\U0001D173")]
    [InlineData("\U000E0020")]
    [InlineData("a\u200Db")]
    [InlineData("a\u200D")]
    [InlineData("\u200D")]
    [InlineData("\uFE0F")]
    [InlineData("\uFE0E")]
    [InlineData("a\uFE0F")]
    [InlineData("a\uFE0E")]
    [InlineData("\U0001F600\u200B")]
    [InlineData("\U0001F600\u202E")]
    public void Refuse_names_control_separator_bidi_format_and_stray_joiners(string input)
    {
        Assert.True(TextSafety.IsRefused(input, out var cp));
        Assert.True(TextSafety.HasRefused(input));
        Assert.NotEqual(0, cp);
    }

    [Theory]
    [InlineData("plain text")]
    [InlineData(" ")]
    [InlineData("\u00A0")]
    [InlineData("\u00E9")]
    [InlineData("\u65E5\u672C\u8A9E")]
    [InlineData("\U0001F600")]
    [InlineData(Heart)]
    [InlineData("\u2764\uFE0E")]
    [InlineData("\U0001F44D\uFE0F")]
    [InlineData(Family)]
    [InlineData("\U0001F468\u200D\U0001F4BB")]
    [InlineData("\U0001F3F3\uFE0F\u200D\U0001F308")]
    [InlineData("1\uFE0F\u20E3")]
    [InlineData("x " + Heart + " y " + Family + " z")]
    public void Refuse_accepts_ordinary_text_and_emoji_sequences(string input)
    {
        Assert.False(TextSafety.IsRefused(input, out var cp), "U+" + cp.ToString("X4"));
        Assert.Equal(0, cp);
    }

    [Fact]
    public void Refuse_reports_the_first_offending_code_point()
    {
        Assert.True(TextSafety.IsRefused("ok\u200Bmore\u0001", out var cp));
        Assert.Equal(0x200B, cp);
        Assert.True(TextSafety.IsRefused("a\U000E0020", out var astral));
        Assert.Equal(0xE0020, astral);
    }

    [Fact]
    public void Refuse_is_less_strict_than_unsafe_for_emoji_joiners_on_purpose()
    {
        var zwj = new Rune(0x200D);
        var vs16 = new Rune(0xFE0F);
        Assert.True(TextSafety.IsUnsafe(zwj));
        Assert.True(TextSafety.IsUnsafe(vs16));
        Assert.False(TextSafety.HasRefused(Family));
        Assert.False(TextSafety.HasRefused(Heart));
    }

    [Theory]
    [InlineData(0x0041, false)]
    [InlineData(0x00A0, false)]
    [InlineData(0x0001, true)]
    [InlineData(0x200B, true)]
    [InlineData(0xFFFD, true)]
    [InlineData(0xE0100, true)]
    [InlineData(0x1F600, false)]
    public void IsUnsafe_by_code_point(int value, bool unsafeExpected)
    {
        Assert.Equal(unsafeExpected, TextSafety.IsUnsafe(new Rune(value)));
    }

    // ---- through the verbs ----

    [Fact]
    public void Names_of_sixty_units_pass_and_longer_ones_are_exit_2()
    {
        AssertOk(w.Call(w.Manager, "tab new", ("name", new string('n', 60))));
        AssertFail(w.Call(w.Manager, "tab new", ("name", new string('m', 61))), 2);
        AssertOk(w.Call(w.Manager, "tab new", ("name", string.Concat(Enumerable.Repeat("\U0001F600", 30)))));
        AssertFail(w.Call(w.Manager, "tab new", ("name", string.Concat(Enumerable.Repeat("\U0001F601", 31)))), 2);
    }

    [Theory]
    [InlineData("a\u0001b")]
    [InlineData("a\nb")]
    [InlineData("a\u2028b")]
    [InlineData("a\u202Eb")]
    [InlineData("a\u200Bb")]
    [InlineData("a\u200D")]
    public void A_name_with_a_refused_character_is_exit_2_and_makes_no_tab(string name)
    {
        var count = w.Ws.Tabs.Count;
        AssertFail(w.Call(w.Manager, "tab new", ("name", name)), 2);
        Assert.Equal(count, w.Ws.Tabs.Count);
    }

    [Fact]
    public void A_name_may_hold_an_emoji_with_vs16_or_a_zwj_family()
    {
        AssertOk(w.Call(w.Manager, "tab new", ("name", "ok " + Heart)));
        AssertOk(w.Call(w.Manager, "tab new", ("name", "fam " + Family)));
    }

    [Theory]
    [InlineData("a\u0007")]
    [InlineData("a\nb")]
    [InlineData("a\u200Bb")]
    [InlineData("a\u202Eb")]
    [InlineData("a\u2029")]
    public void An_attention_reason_with_a_refused_character_is_exit_2_and_sets_no_mark(string reason)
    {
        AssertFail(w.Call(w.S1, "attention", ("level", "need"), ("reason", reason)), 2);
        Assert.Null(w.S1.AttentionLevel);
    }

    [Fact]
    public void An_attention_reason_is_limited_to_120_code_points_and_may_hold_emoji()
    {
        AssertOk(w.Call(w.S1, "attention", ("level", "need"), ("reason", string.Concat(Enumerable.Repeat("\U0001F600", 120)))));
        AssertFail(w.Call(w.S1, "attention", ("level", "need"), ("reason", new string('r', 121))), 2);
        AssertOk(w.Call(w.S1, "attention", ("level", "info"), ("reason", "done " + Family + Heart)));
    }

    [Fact]
    public void Web_open_title_follows_the_title_rule()
    {
        AssertOk(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("tab", w.Projects.Id), ("title", new string('t', 200))));
        AssertFail(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("tab", w.Projects.Id), ("title", new string('t', 201))), 2);
        AssertFail(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("tab", w.Projects.Id), ("title", "x\u202Ey")), 2);
        AssertOk(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("tab", w.Projects.Id), ("title", "x" + Family)));
    }

    // ======================================================================
    // (c) send
    // ======================================================================

}
