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
    public void A_selector_of_512_characters_is_accepted_and_513_is_exit_2()
    {
        var web = NewWeb();
        var s512 = "." + new string('a', 511);
        web.Page[s512] = "hit";
        Assert.Equal("hit", TextOf(WebText(web, s512)));
        AssertFail(WebText(web, s512 + "a"), 2);
    }

    [Theory]
    [InlineData("a\nb")]
    [InlineData("a\tb")]
    [InlineData("a\u0000b")]
    [InlineData("a\u001Bb")]
    [InlineData("a\u007Fb")]
    [InlineData("a\u0085b")]
    public void A_selector_with_a_control_character_is_exit_2(string selector)
    {
        var web = NewWeb();
        web.Page[selector] = "should never be read";
        AssertFail(WebText(web, selector), 2);
    }

    [Theory]
    [InlineData(0, 2)]
    [InlineData(-1, 2)]
    [InlineData(262145, 2)]
    [InlineData(1, 0)]
    [InlineData(262144, 0)]
    public void Web_text_max_bytes_is_1_to_262144(int max, int exit)
    {
        var web = NewWeb();
        web.Page[""] = "abc";
        Assert.Equal(exit, WebText(web, max: max).Exit);
    }

    [Fact]
    public void Web_text_max_bytes_cuts_and_sets_truncated_only_when_something_was_cut()
    {
        var web = NewWeb();
        web.Page[""] = "abcdef";
        var cut = WebText(web, max: 4);
        Assert.Equal("abcd", TextOf(cut));
        Assert.True(Truncated(cut));
        var exact = WebText(web, max: 6);
        Assert.Equal("abcdef", TextOf(exact));
        Assert.False(Truncated(exact));
        var more = WebText(web, max: 600);
        Assert.Equal("abcdef", TextOf(more));
        Assert.False(Truncated(more));
    }

    [Fact]
    public void Web_text_max_bytes_counts_the_escaped_text()
    {
        var web = NewWeb();
        web.Page[""] = "\u0001";
        var cut = WebText(web, max: 7);
        Assert.Equal("", TextOf(cut));
        Assert.True(Truncated(cut));
        var fit = WebText(web, max: 8);
        Assert.Equal("<U+0001>", TextOf(fit));
        Assert.False(Truncated(fit));
    }

    [Theory]
    [InlineData(2, "ab")]
    [InlineData(3, "ab")]
    [InlineData(9, "ab")]
    [InlineData(10, "ab<U+200B>")]
    [InlineData(11, "ab<U+200B>c")]
    public void Web_text_never_cuts_inside_an_escape(int max, string expected)
    {
        var web = NewWeb();
        web.Page[""] = "ab\u200Bcd";
        var r = WebText(web, max: max);
        Assert.Equal(expected, TextOf(r));
        Assert.True(Truncated(r));
    }

    [Fact]
    public void Web_text_cut_at_every_size_leaves_only_whole_escapes_and_no_lone_surrogate()
    {
        var web = NewWeb();
        web.Page[""] = "a\u200B\U0001F600b\U000E0100\U0001D173c\U0001F468\u200D\U0001F469";
        var full = TextOf(WebText(web));
        var fullBytes = Encoding.UTF8.GetByteCount(full);
        for (int max = 1; max <= fullBytes; max++)
        {
            var text = TextOf(WebText(web, max: max));
            Assert.True(Encoding.UTF8.GetByteCount(text) <= max, "max " + max);
            Assert.StartsWith(text, full, StringComparison.Ordinal);
            Assert.DoesNotMatch(new Regex("<(?!U\\+[0-9A-F]{4,6}>)"), text.Replace("<U+", "").Length == text.Length ? text : Regex.Replace(text, "<U\\+[0-9A-F]{4,6}>", ""));
            Assert.Equal(text.Length, text.EnumerateRunes().Sum(r => r.Utf16SequenceLength));
            foreach (var r in text.EnumerateRunes()) Assert.NotEqual(0xFFFD, r.Value);
        }
    }

    [Theory]
    [InlineData(1, "a")]
    [InlineData(4, "a")]
    [InlineData(5, "a\U0001F600")]
    [InlineData(6, "a\U0001F600b")]
    public void Web_text_cut_never_splits_a_surrogate_pair(int max, string expected)
    {
        var web = NewWeb();
        web.Page[""] = "a\U0001F600bc";
        Assert.Equal(expected, TextOf(WebText(web, max: max)));
    }

    [Fact]
    public void Web_text_of_a_terminal_is_exit_5_and_of_a_missing_block_exit_4_and_of_another_session_exit_4()
    {
        AssertFail(WebText(w.S1, caller: w.Manager), 5);
        AssertFail(w.Call(w.Manager, "web text", ("block", Guid.NewGuid().ToString())), 4);
        var web = NewWeb();
        web.Page[""] = "private";
        AssertFail(WebText(web, caller: w.S1), 4);
        Assert.Equal("private", TextOf(WebText(web, caller: w.Manager)));
    }

    [Fact]
    public void A_web_block_reads_its_own_page_without_admin()
    {
        var web = NewWeb();
        web.Page[""] = "mine";
        var r = w.Call(web, "web text", ("block", "this"));
        AssertOk(r);
        Assert.Equal("mine", TextOf(r));
    }

    [Fact]
    public void Client_web_text_prints_text_and_truncated_and_a_bad_max_bytes_is_exit_2()
    {
        var web = NewWeb();
        web.Page[""] = "line1\nline2";
        var (exit, stdout, stderr) = w.Cli(w.Manager, "web", "text", web.Id);
        Assert.Equal(0, exit);
        Assert.Equal("", stderr);
        Assert.Equal("line1\nline2", JsonNode.Parse(stdout)!["text"]!.GetValue<string>());
        Assert.False(JsonNode.Parse(stdout)!["truncated"]!.GetValue<bool>());
        var (e2, _, _) = w.Cli(w.Manager, "web", "text", web.Id, "--max-bytes", "0");
        Assert.Equal(2, e2);
    }
}
