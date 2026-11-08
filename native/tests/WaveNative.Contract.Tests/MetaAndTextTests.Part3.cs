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
    [Theory]
    [InlineData("enter", "\r")]
    [InlineData("return", "\r")]
    [InlineData("escape", "\u001B")]
    [InlineData("esc", "\u001B")]
    [InlineData("tab", "\t")]
    [InlineData("backtab", "\u001B[Z")]
    [InlineData("backspace", "\u007F")]
    [InlineData("delete", "\u001B[3~")]
    [InlineData("insert", "\u001B[2~")]
    [InlineData("space", " ")]
    [InlineData("up", "\u001B[A")]
    [InlineData("down", "\u001B[B")]
    [InlineData("right", "\u001B[C")]
    [InlineData("left", "\u001B[D")]
    [InlineData("home", "\u001B[H")]
    [InlineData("end", "\u001B[F")]
    [InlineData("pageup", "\u001B[5~")]
    [InlineData("pagedown", "\u001B[6~")]
    [InlineData("f1", "\u001BOP")]
    [InlineData("f2", "\u001BOQ")]
    [InlineData("f3", "\u001BOR")]
    [InlineData("f4", "\u001BOS")]
    [InlineData("f5", "\u001B[15~")]
    [InlineData("f6", "\u001B[17~")]
    [InlineData("f7", "\u001B[18~")]
    [InlineData("f8", "\u001B[19~")]
    [InlineData("f9", "\u001B[20~")]
    [InlineData("f10", "\u001B[21~")]
    [InlineData("f11", "\u001B[23~")]
    [InlineData("f12", "\u001B[24~")]
    public void Every_named_key_sends_its_terminal_sequence(string name, string expected)
    {
        var r = Send(w.S1, new[] { "key:" + name });
        AssertOk(r);
        Assert.Equal(Encoding.UTF8.GetBytes(expected), Flat(w.S1));
        Assert.Equal(expected.Length, World.Out(r)["bytes"]!.GetValue<int>());
        Assert.Equal(1, World.Out(r)["chunks"]!.GetValue<int>());
    }

    [Fact]
    public void Ctrl_a_to_z_send_bytes_1_to_26_in_any_case()
    {
        for (char c = 'a'; c <= 'z'; c++)
        {
            w.S1.Input.Clear();
            AssertOk(Send(w.S1, new[] { "key:ctrl+" + c }));
            Assert.Equal(new[] { (byte)(c - 'a' + 1) }, Flat(w.S1));
            w.S1.Input.Clear();
            AssertOk(Send(w.S1, new[] { "KEY:CTRL+" + char.ToUpperInvariant(c) }));
            Assert.Equal(new[] { (byte)(c - 'a' + 1) }, Flat(w.S1));
        }
    }

    [Fact]
    public void Alt_letter_sends_escape_then_the_letter_in_any_case()
    {
        AssertOk(Send(w.S1, new[] { "key:alt+x", "Key:ALT+Q", "key:alt+a" }));
        Assert.Equal(new byte[] { 0x1B, (byte)'x', 0x1B, (byte)'q', 0x1B, (byte)'a' }, Flat(w.S1));
    }

    [Theory]
    [InlineData("KEY:ENTER")]
    [InlineData("Key:Enter")]
    [InlineData("kEy:eNtEr")]
    public void Key_prefix_and_key_names_are_case_insensitive(string token)
    {
        AssertOk(Send(w.S1, new[] { token }));
        Assert.Equal(new byte[] { 0x0D }, Flat(w.S1));
    }

    [Theory]
    [InlineData("key:")]
    [InlineData("key:nosuchkey")]
    [InlineData("key:f0")]
    [InlineData("key:f13")]
    [InlineData("key:f01")]
    [InlineData("key:ctrl+")]
    [InlineData("key:ctrl+1")]
    [InlineData("key:ctrl+ab")]
    [InlineData("key:ctrl+@")]
    [InlineData("key:alt+")]
    [InlineData("key:alt+1")]
    [InlineData("key:alt+ab")]
    [InlineData("key:shift+a")]
    [InlineData("key:enterr")]
    [InlineData("key:page up")]
    [InlineData("key:A")]
    public void An_unknown_key_is_exit_2_and_nothing_is_typed_even_after_good_tokens(string bad)
    {
        AssertFail(Send(w.S1, new[] { "text:before", bad }), 2);
        Assert.Empty(w.S1.Input);
    }

    [Fact]
    public void Text_prefix_and_bare_tokens_send_their_utf8_bytes_in_order_and_the_prefix_is_case_insensitive()
    {
        var r = Send(w.S1, new[] { "text:ls -la", "TEXT:Caf\u00E9", "Text:\u65E5", "bare word", "key:enter" });
        AssertOk(r);
        Assert.Equal(new[] { "ls -la", "Caf\u00E9", "\u65E5", "bare word", "\r" }, w.S1.Input.Select(b => Encoding.UTF8.GetString(b)).ToArray());
        Assert.Equal(Encoding.UTF8.GetBytes("ls -laCaf\u00E9\u65E5bare word\r").Length, World.Out(r)["bytes"]!.GetValue<int>());
        Assert.Equal(5, World.Out(r)["chunks"]!.GetValue<int>());
    }

    [Fact]
    public void A_bare_token_that_only_looks_like_a_key_is_text_and_text_prefix_protects_a_literal_key_name()
    {
        AssertOk(Send(w.S1, new[] { "keyboard", "text:key:enter", "enter" }));
        Assert.Equal(new[] { "keyboard", "key:enter", "enter" }, w.S1.Input.Select(b => Encoding.UTF8.GetString(b)).ToArray());
    }

    [Fact]
    public void An_emoji_with_vs16_and_a_zwj_family_can_be_sent_without_raw_and_arrive_byte_exact()
    {
        AssertOk(Send(w.S1, new[] { "text:" + Heart, Family }));
        Assert.Equal(Encoding.UTF8.GetBytes(Heart + Family), Flat(w.S1));
    }

    [Theory]
    [InlineData("a\nb")]
    [InlineData("a\r")]
    [InlineData("a\tb")]
    [InlineData("a\u0007")]
    [InlineData("\u001B[31m")]
    [InlineData("a\u2028")]
    [InlineData("a\u2029")]
    [InlineData("a\u202Eb")]
    [InlineData("a\u2067")]
    [InlineData("a\u200Bb")]
    [InlineData("a\u00ADb")]
    [InlineData("a\uFEFF")]
    [InlineData("a\u200D")]
    [InlineData("a\uFE0F")]
    public void Text_with_a_refused_character_is_exit_2_nothing_is_typed_and_raw_lets_it_through(string text)
    {
        AssertFail(Send(w.S1, new[] { "ok", "text:" + text }), 2);
        AssertFail(Send(w.S1, new[] { text }), 2);
        Assert.Empty(w.S1.Input);
        var r = Send(w.S1, new[] { text }, raw: true);
        AssertOk(r);
        Assert.Equal(Encoding.UTF8.GetBytes(text), Flat(w.S1));
    }

    [Fact]
    public void Raw_does_not_turn_an_unknown_key_into_text()
    {
        AssertFail(Send(w.S1, new[] { "key:nosuchkey" }, raw: true), 2);
        Assert.Empty(w.S1.Input);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(1001)]
    [InlineData(100000)]
    public void Delay_outside_0_to_1000_is_exit_2(int delay)
    {
        AssertFail(Send(w.S1, new[] { "x" }, delay: delay), 2);
        Assert.Empty(w.S1.Input);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(40)]
    [InlineData(1000)]
    public void Delay_inside_0_to_1000_is_accepted(int delay)
    {
        AssertOk(Send(w.S1, new[] { "x" }, delay: delay));
        Assert.Equal(new byte[] { (byte)'x' }, Flat(w.S1));
    }

    [Fact]
    public void Exactly_4096_tokens_pass_and_4097_are_exit_2_with_nothing_typed()
    {
        AssertFail(Send(w.S1, Enumerable.Repeat("a", 4097).ToArray()), 2);
        Assert.Empty(w.S1.Input);
        var r = Send(w.S1, Enumerable.Repeat("a", 4096).ToArray());
        AssertOk(r);
        Assert.Equal(4096, World.Out(r)["bytes"]!.GetValue<int>());
        Assert.Equal(4096, World.Out(r)["chunks"]!.GetValue<int>());
    }

    [Fact]
    public void Exactly_64_KiB_pass_and_one_byte_more_is_exit_2()
    {
        var kib = new string('a', 1024);
        var ok = Enumerable.Repeat(kib, 64).ToArray();
        AssertFail(Send(w.S1, ok.Append("b").ToArray()), 2);
        Assert.Empty(w.S1.Input);
        var r = Send(w.S1, ok);
        AssertOk(r);
        Assert.Equal(65536, World.Out(r)["bytes"]!.GetValue<int>());
    }

    [Fact]
    public void The_64_KiB_limit_counts_bytes_not_characters()
    {
        var wide = string.Concat(Enumerable.Repeat("\u65E5", 21846));
        Assert.True(Encoding.UTF8.GetByteCount(wide) > 65536);
        AssertFail(Send(w.S1, new[] { wide }), 2);
        Assert.Empty(w.S1.Input);
    }

    [Fact]
    public void No_tokens_or_only_empty_tokens_is_exit_2()
    {
        AssertFail(Send(w.S1, Array.Empty<string>()), 2);
        AssertFail(Send(w.S1, new[] { "" }), 2);
        AssertFail(Send(w.S1, new[] { "", "text:" }), 2);
        Assert.Empty(w.S1.Input);
    }

    [Fact]
    public void An_empty_token_between_real_ones_adds_no_bytes()
    {
        var r = Send(w.S1, new[] { "a", "", "b" });
        AssertOk(r);
        Assert.Equal(2, World.Out(r)["bytes"]!.GetValue<int>());
        Assert.Equal(Encoding.UTF8.GetBytes("ab"), Flat(w.S1));
    }

    [Fact]
    public void Send_to_a_web_block_is_exit_5_and_to_a_finished_program_is_exit_5()
    {
        var web = NewWeb();
        AssertFail(Send(web, new[] { "x" }, caller: w.Manager), 5);
        Assert.Empty(web.Input);
        w.Host.FinishProgram(w.S1.Id, 0);
        AssertFail(Send(w.S1, new[] { "x" }), 5);
        Assert.Empty(w.S1.Input);
    }

    [Fact]
    public void Send_to_a_block_the_caller_did_not_create_needs_admin_and_the_manager_may()
    {
        AssertFail(Send(w.S2, new[] { "x" }, caller: w.S1), 3);
        Assert.Empty(w.S2.Input);
        AssertOk(Send(w.S2, new[] { "x" }, caller: w.Manager));
        Assert.Equal(new byte[] { (byte)'x' }, Flat(w.S2));
    }

    [Fact]
    public void Send_to_an_unknown_block_is_exit_4_and_to_a_malformed_id_exit_2()
    {
        var a = World.Args(("block", Guid.NewGuid().ToString()), ("tokens", new[] { "x" }));
        AssertFail(w.Call(w.S1, "send", a), 4);
        var b = World.Args(("block", "abc"), ("tokens", new[] { "x" }));
        AssertFail(w.Call(w.S1, "send", b), 2);
    }

    [Fact]
    public void Client_send_types_tokens_and_raw_reaches_the_app()
    {
        var (exit, stdout, _) = w.Cli(w.S1, "send", "this", "text:hi", "key:enter");
        Assert.Equal(0, exit);
        Assert.Equal(3, JsonNode.Parse(stdout)!["bytes"]!.GetValue<int>());
        Assert.Equal(Encoding.UTF8.GetBytes("hi\r"), Flat(w.S1));
        var (e2, o2, s2) = w.Cli(w.S1, "send", "this", "a\u0007");
        Assert.Equal(2, e2);
        Assert.Equal("", o2);
        Assert.Equal("usage", JsonNode.Parse(s2)!["error"]!["kind"]!.GetValue<string>());
        var (e3, _, _) = w.Cli(w.S1, "send", "this", "a\u0007", "--raw");
        Assert.Equal(0, e3);
        Assert.Equal(Encoding.UTF8.GetBytes("hi\ra\u0007"), Flat(w.S1));
    }

    // ======================================================================
    // (d) screen and web text
    // ======================================================================

    [Fact]
    public void An_empty_screen_answers_an_empty_list_not_an_error()
    {
        var r = Screen(w.S1);
        AssertOk(r);
        Assert.Empty(Lines(r));
        Assert.Equal(0, World.Out(r)["total"]!.GetValue<int>());
        Assert.False(Truncated(r));
        w.S1.Screen.AddRange(new[] { "", "   ", "" });
        var r2 = Screen(w.S1);
        AssertOk(r2);
        Assert.Empty(Lines(r2));
    }

    [Fact]
    public void Trailing_blank_lines_are_removed_but_inner_and_leading_blanks_stay()
    {
        w.S1.Screen.AddRange(new[] { "", "a", "", "b", "  ", "", "" });
        var r = Screen(w.S1);
        Assert.Equal(new[] { "", "a", "", "b" }, Lines(r));
        Assert.Equal(4, World.Out(r)["total"]!.GetValue<int>());
        Assert.False(Truncated(r));
    }

    [Fact]
    public void Last_n_keeps_the_final_n_lines_after_trailing_blanks_are_dropped()
    {
        w.S1.Screen.AddRange(new[] { "l1", "l2", "l3", "l4", "l5", "", "" });
        Assert.Equal(new[] { "l4", "l5" }, Lines(Screen(w.S1, last: 2)));
        Assert.Equal(new[] { "l5" }, Lines(Screen(w.S1, last: 1)));
        Assert.Equal(new[] { "l1", "l2", "l3", "l4", "l5" }, Lines(Screen(w.S1, last: 0)));
        Assert.Equal(new[] { "l1", "l2", "l3", "l4", "l5" }, Lines(Screen(w.S1, last: 5)));
        Assert.Equal(new[] { "l1", "l2", "l3", "l4", "l5" }, Lines(Screen(w.S1, last: 500)));
    }

    [Fact]
    public void Last_that_cuts_nothing_does_not_set_truncated()
    {
        w.S1.Screen.AddRange(new[] { "l1", "l2" });
        Assert.False(Truncated(Screen(w.S1, last: 2)));
        Assert.False(Truncated(Screen(w.S1, last: 10)));
        Assert.False(Truncated(Screen(w.S1, last: 0)));
    }

    [Theory]
    [InlineData(-1, 2)]
    [InlineData(10001, 2)]
    [InlineData(int.MaxValue, 2)]
    [InlineData(0, 0)]
    [InlineData(1, 0)]
    [InlineData(10000, 0)]
    public void Last_is_0_to_10000(int last, int exit)
    {
        w.S1.Screen.Add("x");
        Assert.Equal(exit, Screen(w.S1, last: last).Exit);
    }

    [Fact]
    public void Max_bytes_drops_the_oldest_lines_and_marks_truncated_with_no_marker_line()
    {
        w.S1.Screen.AddRange(new[] { "aaaa", "bbbb", "cccc" });
        var all = Screen(w.S1, max: 100);
        Assert.Equal(new[] { "aaaa", "bbbb", "cccc" }, Lines(all));
        Assert.False(Truncated(all));
        var cut = Screen(w.S1, max: 11);
        Assert.Equal(new[] { "bbbb", "cccc" }, Lines(cut));
        Assert.True(Truncated(cut));
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(4)]
    public void Max_bytes_smaller_than_the_last_line_still_keeps_the_last_line(int max)
    {
        w.S1.Screen.AddRange(new[] { "aaaa", "bbbb", "cccccccc" });
        var r = Screen(w.S1, max: max);
        AssertOk(r);
        Assert.Equal(new[] { "cccccccc" }, Lines(r));
        Assert.True(Truncated(r));
    }

    [Fact]
    public void Max_bytes_counts_the_escaped_text_not_the_raw_text()
    {
        w.S1.Screen.AddRange(new[] { "\u0001", "bb" });
        var r = Screen(w.S1, max: 7);
        Assert.Equal(new[] { "bb" }, Lines(r));
        Assert.True(Truncated(r));
        Assert.Equal(new[] { "<U+0001>", "bb" }, Lines(Screen(w.S1, max: 100)));
    }

    [Fact]
    public void Last_and_max_bytes_combine_last_first_then_the_byte_cap()
    {
        w.S1.Screen.AddRange(new[] { "x1", "x2", "yyyyyyyy", "zzzzzzzz" });
        var r = Screen(w.S1, last: 3, max: 12);
        Assert.Equal(new[] { "zzzzzzzz" }, Lines(r));
        Assert.True(Truncated(r));
    }

    [Fact]
    public void Screen_lines_escape_controls_with_the_section_7_format_and_keep_tab()
    {
        w.S1.Screen.Add("a\u001B[31mred\u001B[0m\tend\u200B" + Heart + "\U000E0100\uFFFD");
        var line = Lines(Screen(w.S1))[0];
        Assert.Equal("a<U+001B>[31mred<U+001B>[0m\tend<U+200B>\u2764<U+FE0F><U+E0100><U+FFFD>", line);
    }

    [Fact]
    public void Screen_lines_never_contain_a_raw_control_or_format_character()
    {
        w.S1.Screen.AddRange(new[] { "\u0001\u0002\u0003", "\u202Eabc\u202C", "x\u2028y\u2029z", "\u007F\u0085" });
        foreach (var l in Lines(Screen(w.S1)))
            foreach (var r in l.EnumerateRunes())
                Assert.False(TextSafety.IsUnsafe(r), "raw U+" + r.Value.ToString("X4") + " in " + l);
    }

    [Fact]
    public void Screen_of_a_web_block_is_exit_5_and_of_a_missing_block_exit_4_and_of_another_session_exit_4()
    {
        var web = NewWeb();
        AssertFail(Screen(web, caller: w.Manager), 5);
        AssertFail(w.Call(w.S1, "screen", ("block", Guid.NewGuid().ToString())), 4);
        w.S2.Screen.Add("secret");
        AssertFail(Screen(w.S2, caller: w.S1), 4);
        Assert.Equal(new[] { "secret" }, Lines(Screen(w.S2, caller: w.Manager)));
    }

    [Fact]
    public void Screen_works_for_a_finished_program_and_answers_the_remaining_lines()
    {
        w.S1.Screen.AddRange(new[] { "out", "bye" });
        w.Host.FinishProgram(w.S1.Id, 3);
        Assert.Equal(new[] { "out", "bye" }, Lines(Screen(w.S1)));
    }

    [Fact]
    public void Client_screen_prints_the_json_shape_and_honours_last()
    {
        w.S1.Screen.AddRange(new[] { "one", "two", "three", "" });
        var (exit, stdout, stderr) = w.Cli(w.S1, "screen", "this", "--last", "2");
        Assert.Equal(0, exit);
        Assert.Equal("", stderr);
        var o = JsonNode.Parse(stdout)!;
        Assert.Equal(new[] { "two", "three" }, o["lines"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
        Assert.NotNull(o["total"]);
        Assert.NotNull(o["truncated"]);
        var (e2, _, s2) = w.Cli(w.S1, "screen", "this", "--last", "10001");
        Assert.Equal(2, e2);
        Assert.Equal(2, JsonNode.Parse(s2)!["error"]!["code"]!.GetValue<int>());
    }

    // ---- web text ----

    [Fact]
    public void Web_text_escapes_unsafe_runes_keeps_newline_and_tab_and_reports_truncated_false()
    {
        var web = NewWeb();
        web.Page[""] = "a\u0001b\nc\td\u200Be\u2028f\uFE0F\U000E0100\uFFFD";
        var r = WebText(web);
        AssertOk(r);
        Assert.Equal("a<U+0001>b\nc\td<U+200B>e<U+2028>f<U+FE0F><U+E0100><U+FFFD>", TextOf(r));
        Assert.False(Truncated(r));
    }

    [Fact]
    public void Web_text_with_a_selector_reads_that_selector_only()
    {
        var web = NewWeb();
        web.Page[""] = "whole page";
        web.Page["#line"] = "just the line";
        Assert.Equal("just the line", TextOf(WebText(web, "#line")));
        Assert.Equal("whole page", TextOf(WebText(web)));
    }

}
