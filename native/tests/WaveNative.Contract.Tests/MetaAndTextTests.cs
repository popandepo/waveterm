// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// cli-contract.md v0.3 sections 4.7, 4.10, 4.11, 6 and 7: meta keys, the two text-safety predicates, send, screen and web text.
public sealed partial class MetaAndTextTests : IDisposable
{
    readonly World w = new();

    public void Dispose() => w.Dispose();

    const string Family = "\U0001F468\u200D\U0001F469\u200D\U0001F467";
    const string Heart = "\u2764\uFE0F";

    // ---- helpers ----

    static void AssertFail(Reply r, int code)
    {
        Assert.Equal(code, r.Exit);
        Assert.NotNull(r.Err);
        Assert.Equal(code, r.Err!["error"]!["code"]!.GetValue<int>());
        Assert.Equal(Exit.Kind(code), r.Err["error"]!["kind"]!.GetValue<string>());
    }

    static void AssertOk(Reply r) => Assert.True(r.Exit == 0, "exit " + r.Exit + " " + r.Err?.ToJsonString());

    Reply MetaSet(Block caller, string block, params string[] pairs) => w.Call(caller, "meta set", ("block", block), ("pairs", pairs));

    Reply MetaJson(Block caller, string block, string json) => w.Call(caller, "meta set", ("block", block), ("pairs", Array.Empty<string>()), ("json", json));

    Reply MetaUnset(Block caller, string block, params string[] keys) => w.Call(caller, "meta unset", ("block", block), ("keys", keys));

    string? Get(Block b, string key) => World.Out(w.Call(b, "meta get", ("items", new[] { "this", key })))[key]?.GetValue<string>();

    string Snapshot(Block b) => World.Out(w.Call(b, "meta get")).ToJsonString();

    Block NewWeb(string url = "https://example.com/")
    {
        var r = w.Call(w.Manager, "web open", ("url", url), ("tab", w.Projects.Id));
        AssertOk(r);
        return w.Host.GetBlock(World.Out(r)["blockid"]!.GetValue<string>());
    }

    Reply Send(Block block, string[] tokens, bool raw = false, int? delay = null, Block? caller = null)
    {
        var a = World.Args(("block", block.Id), ("tokens", tokens), ("raw", raw));
        if (delay != null) a["delay"] = delay;
        return w.Call(caller ?? block, "send", a);
    }

    static byte[] Flat(Block b) => b.Input.SelectMany(x => x).ToArray();

    Reply Screen(Block block, int? last = null, int? max = null, Block? caller = null)
    {
        var a = World.Args(("block", block.Id));
        if (last != null) a["last"] = last;
        if (max != null) a["maxBytes"] = max;
        return w.Call(caller ?? block, "screen", a);
    }

    static string[] Lines(Reply r) => World.Out(r)["lines"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray();

    Reply WebText(Block web, string? selector = null, int? max = null, Block? caller = null)
    {
        var a = World.Args(("block", web.Id));
        if (selector != null) a["selector"] = selector;
        if (max != null) a["maxBytes"] = max;
        return w.Call(caller ?? w.Manager, "web text", a);
    }

    static string TextOf(Reply r) => World.Out(r)["text"]!.GetValue<string>();

    static bool Truncated(Reply r) => World.Out(r)["truncated"]!.GetValue<bool>();

    // ======================================================================
    // (a) meta keys of section 6
    // ======================================================================

    [Theory]
    [InlineData("frame:title", "hello world")]
    [InlineData("frame:title", "")]
    [InlineData("frame:icon", "bell")]
    [InlineData("frame:icon", "a-1")]
    [InlineData("frame:icon", "AB")]
    [InlineData("frame:icon", "\U0001F600")]
    [InlineData("icon:color", "red")]
    [InlineData("icon:color", "rebeccapurple")]
    [InlineData("icon:color", "#00ff00")]
    [InlineData("icon:color", "#00ff0080")]
    [InlineData("frame:bordercolor", "navy")]
    [InlineData("frame:bordercolor", "#123456")]
    [InlineData("frame:bordercolor", "#12345678")]
    [InlineData("session:id", "11111111-2222-3333-4444-555555555555")]
    [InlineData("session:name", "build")]
    [InlineData("session:model", "opus")]
    [InlineData("session:effort", "high")]
    [InlineData("session:permissionmode", "plan")]
    [InlineData("session:Custom.k_e-y9", "v")]
    [InlineData("session:empty", "")]
    public void A_valid_value_of_every_settable_key_is_stored_and_read_back(string key, string value)
    {
        var r = MetaSet(w.S1, "this", key + "=" + value);
        AssertOk(r);
        Assert.Equal(w.S1.Id, World.Out(r)["id"]!.GetValue<string>());
        Assert.Equal(new[] { key }, World.Out(r)["set"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
        Assert.Empty(World.Out(r)["unset"]!.AsArray());
        Assert.Equal(value, Get(w.S1, key));
    }

    [Theory]
    [InlineData("https://example.com/a")]
    [InlineData("http://example.com:8080/x?y=1#z")]
    [InlineData("file:///C:/temp/a.html")]
    public void Url_accepts_http_https_and_file_on_a_web_block(string url)
    {
        var web = NewWeb();
        AssertOk(MetaSet(web, "this", "url=" + url));
        Assert.Equal(url, Get(web, "url"));
    }

    [Theory]
    [InlineData("ftp://example.com/")]
    [InlineData("javascript:alert(1)")]
    [InlineData("data:text/html,hi")]
    [InlineData("mailto:a@b.c")]
    [InlineData("about:blank")]
    [InlineData("example.com")]
    [InlineData("//example.com/")]
    [InlineData("")]
    [InlineData("not a url")]
    public void Url_with_another_scheme_or_no_scheme_is_exit_2_and_changes_nothing(string url)
    {
        var web = NewWeb();
        AssertFail(MetaSet(web, "this", "url=" + url), 2);
        Assert.Equal("https://example.com/", Get(web, "url"));
    }

    [Fact]
    public void Url_on_a_terminal_block_is_exit_2_and_creates_no_key()
    {
        AssertFail(MetaSet(w.S1, "this", "url=https://example.com/"), 2);
        Assert.Null(Get(w.S1, "url"));
    }

    public static IEnumerable<object[]> RefusedValues()
    {
        string n(int c, char ch = 'a') => new string(ch, c);
        var rows = new List<(string Key, string Value)>
        {
            ("frame:title", n(201)),
            ("frame:title", string.Concat(Enumerable.Repeat("\U0001F600", 201))),
            ("frame:title", "a\u0001b"),
            ("frame:title", "a\nb"),
            ("frame:title", "a\tb"),
            ("frame:title", "a\u200Bb"),
            ("frame:title", "a\u202Eb"),
            ("frame:title", "a\u2066b"),
            ("frame:title", "a\u2028b"),
            ("frame:title", "a\u2029b"),
            ("frame:title", "a\u007Fb"),
            ("frame:icon", n(41)),
            ("frame:icon", "solid@bell"),
            ("frame:icon", "regular@bell"),
            ("frame:icon", "brands@github"),
            ("frame:icon", "custom@x"),
            ("frame:icon", "Bell"),
            ("frame:icon", "bell_x"),
            ("frame:icon", "ABC"),
            ("frame:icon", "\u65E5\u672C\u8A9E"),
            ("frame:icon", "<"),
            ("frame:icon", ">"),
            ("frame:icon", "&"),
            ("frame:icon", "\""),
            ("frame:icon", "'"),
            ("frame:icon", "`"),
            ("frame:icon", "\\"),
            ("frame:icon", "A<"),
            ("frame:icon", Family + Family),
            ("frame:icon", "x\uFE0F"),
            ("frame:icon", "a\u200D"),
            ("frame:icon", "\u202E"),
            ("frame:icon", "A\n"),
            ("icon:color", "notacolor"),
            ("icon:color", "#12345"),
            ("icon:color", "#1234567"),
            ("icon:color", "#123456789"),
            ("icon:color", "#12345g"),
            ("icon:color", "#abc"),
            ("icon:color", "rgb(1,2,3)"),
            ("icon:color", "red;"),
            ("icon:color", "red blue"),
            ("frame:bordercolor", "notacolor"),
            ("frame:bordercolor", "#12345"),
            ("frame:bordercolor", "#zzzzzz"),
            ("frame:bordercolor", "123456"),
            ("session:name", n(201)),
            ("session:model", string.Concat(Enumerable.Repeat("\U0001F600", 201))),
            ("session:effort", "a\u0001"),
            ("session:effort", "a\nb"),
            ("session:effort", "a\u200Bb"),
            ("session:effort", "a\u202Db"),
            ("session:effort", "a\u2069b"),
            ("session:effort", "a\u2028b"),
            ("session:", "v"),
            ("session:a b", "v"),
            ("session:a/b", "v"),
            ("session:a:b", "v"),
            ("session:a*", "v"),
            ("session:*", "v"),
            ("session:\u00E9", "v"),
            ("Session:x", "v"),
            ("sessions:x", "v"),
            ("session", "v"),
        };
        foreach (var (k, v) in rows) yield return new object[] { k, v };
    }

    [Theory]
    [MemberData(nameof(RefusedValues))]
    public void A_refused_value_or_key_is_exit_2_and_changes_nothing(string key, string value)
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaSet(w.S1, "this", key + "=" + value), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Theory]
    [InlineData("frame:title", 200)]
    [InlineData("session:name", 200)]
    [InlineData("session:model", 200)]
    public void The_length_limit_of_200_code_points_is_inclusive(string key, int length)
    {
        AssertOk(MetaSet(w.S1, "this", key + "=" + new string('x', length)));
        Assert.Equal(length, Get(w.S1, key)!.Length);
    }

    [Fact]
    public void The_length_limit_counts_code_points_not_utf16_units()
    {
        var s = string.Concat(Enumerable.Repeat("\U0001F600", 200));
        AssertOk(MetaSet(w.S1, "this", "frame:title=" + s));
        AssertOk(MetaSet(w.S1, "this", "session:model=" + s));
    }

    [Fact]
    public void Icon_names_of_forty_characters_pass_and_a_symbol_may_be_one_family_emoji_or_a_heart_with_vs16()
    {
        AssertOk(MetaSet(w.S1, "this", "frame:icon=" + new string('a', 40)));
        AssertOk(MetaSet(w.S1, "this", "frame:icon=" + Family));
        Assert.Equal(Family, Get(w.S1, "frame:icon"));
        AssertOk(MetaSet(w.S1, "this", "frame:icon=" + Heart));
        Assert.Equal(Heart, Get(w.S1, "frame:icon"));
    }

    [Theory]
    [InlineData("view")]
    [InlineData("cmd")]
    [InlineData("cmd:args")]
    [InlineData("cmd:cwd")]
    [InlineData("cmd:runonstart")]
    [InlineData("cmd:runonce")]
    [InlineData("cmd:clearonstart")]
    [InlineData("cmd:env")]
    [InlineData("web:partition")]
    [InlineData("attention:level")]
    [InlineData("attention:reason")]
    [InlineData("tab:flagcolor")]
    [InlineData("tab:accent")]
    [InlineData("tab:symbol")]
    [InlineData("ws:accent")]
    [InlineData("slot")]
    [InlineData("display:order")]
    [InlineData("ai:model")]
    [InlineData("term:fontsize")]
    [InlineData("icon")]
    [InlineData("frame:other")]
    [InlineData("URL")]
    [InlineData("Frame:Title")]
    [InlineData("unknownkey")]
    [InlineData("a/b")]
    public void A_key_that_is_not_settable_is_exit_2_for_set_json_and_unset_and_changes_nothing(string key)
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaSet(w.S1, "this", key + "=x"), 2);
        AssertFail(MetaJson(w.S1, "this", new JsonObject { [key] = "x" }.ToJsonString()), 2);
        AssertFail(MetaJson(w.S1, "this", new JsonObject { [key] = null }.ToJsonString()), 2);
        AssertFail(MetaUnset(w.S1, "this", key), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Theory]
    [InlineData("view", "web")]
    [InlineData("cmd", "evil")]
    [InlineData("cmd:cwd", "C:\\")]
    [InlineData("attention:level", "urgent")]
    public void Admin_does_not_make_a_read_only_key_settable(string key, string value)
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaSet(w.Manager, w.S1.Id, key + "=" + value), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Fact]
    public void Web_partition_stays_the_one_the_app_set_at_creation()
    {
        var web = NewWeb();
        AssertFail(MetaSet(web, "this", "web:partition=persist:other"), 2);
        Assert.Equal("persist:" + web.Id, Get(web, "web:partition"));
    }

    [Fact]
    public void Atomic_one_bad_key_among_good_ones_changes_nothing_in_pairs_form()
    {
        AssertOk(MetaSet(w.S1, "this", "frame:title=before"));
        var before = Snapshot(w.S1);
        AssertFail(MetaSet(w.S1, "this", "frame:title=after", "session:a=1", "view=web"), 2);
        Assert.Equal(before, Snapshot(w.S1));
        AssertFail(MetaSet(w.S1, "this", "view=web", "frame:title=after", "session:a=1"), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Fact]
    public void Atomic_a_bad_value_of_a_settable_key_changes_nothing()
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaSet(w.S1, "this", "frame:title=fine", "icon:color=notacolor"), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Fact]
    public void Atomic_in_json_form_and_in_a_mixed_pairs_and_json_call()
    {
        AssertOk(MetaSet(w.S1, "this", "frame:title=before"));
        var before = Snapshot(w.S1);
        AssertFail(MetaJson(w.S1, "this", "{\"frame:title\":\"after\",\"slot\":\"x\"}"), 2);
        Assert.Equal(before, Snapshot(w.S1));
        var r = w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "frame:title=after" }), ("json", "{\"icon:color\":\"nope\"}"));
        AssertFail(r, 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Fact]
    public void Atomic_unset_with_one_bad_key_removes_none()
    {
        AssertOk(MetaSet(w.S1, "this", "frame:title=keep", "session:a=keep"));
        var before = Snapshot(w.S1);
        AssertFail(MetaUnset(w.S1, "this", "frame:title", "view", "session:a"), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

    [Fact]
    public void Set_answers_id_set_and_unset_and_json_null_deletes()
    {
        AssertOk(MetaSet(w.S1, "this", "frame:title=t", "session:a=1"));
        var r = MetaJson(w.S1, "this", "{\"frame:title\":null,\"session:b\":\"2\"}");
        AssertOk(r);
        var o = World.Out(r);
        Assert.Equal(w.S1.Id, o["id"]!.GetValue<string>());
        Assert.Equal(new[] { "session:b" }, o["set"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
        Assert.Equal(new[] { "frame:title" }, o["unset"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
        Assert.Null(Get(w.S1, "frame:title"));
        Assert.Equal("1", Get(w.S1, "session:a"));
        Assert.Equal("2", Get(w.S1, "session:b"));
    }

    [Fact]
    public void Unset_deletes_the_keys_and_answers_them_under_unset()
    {
        AssertOk(MetaSet(w.S1, "this", "frame:title=t", "icon:color=red", "session:a=1"));
        var r = MetaUnset(w.S1, "this", "frame:title", "session:a");
        AssertOk(r);
        Assert.Empty(World.Out(r)["set"]!.AsArray());
        Assert.Equal(new[] { "frame:title", "session:a" }, World.Out(r)["unset"]!.AsArray().Select(x => x!.GetValue<string>()).OrderBy(x => x, StringComparer.Ordinal).ToArray());
        Assert.Null(Get(w.S1, "frame:title"));
        Assert.Null(Get(w.S1, "session:a"));
        Assert.Equal("red", Get(w.S1, "icon:color"));
    }

    [Fact]
    public void Unset_of_an_absent_settable_key_is_not_an_error()
    {
        AssertOk(MetaUnset(w.S1, "this", "session:never-set"));
    }

    [Fact]
    public void Unset_without_keys_is_exit_2()
    {
        AssertFail(MetaUnset(w.S1, "this"), 2);
    }

    [Fact]
    public void Unset_with_a_repeated_key_is_exit_2()
    {
        AssertOk(MetaSet(w.S1, "this", "session:a=1"));
        AssertFail(MetaUnset(w.S1, "this", "session:a", "session:a"), 2);
        Assert.Equal("1", Get(w.S1, "session:a"));
    }

    [Fact]
    public void An_empty_string_is_a_valid_value_and_is_not_a_delete()
    {
        AssertOk(MetaSet(w.S1, "this", "frame:title=x"));
        AssertOk(MetaSet(w.S1, "this", "frame:title="));
        Assert.Equal("", Get(w.S1, "frame:title"));
        var r = MetaJson(w.S1, "this", "{\"session:e\":\"\"}");
        AssertOk(r);
        Assert.Equal("", Get(w.S1, "session:e"));
        Assert.Equal(new[] { "session:e" }, World.Out(r)["set"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
    }

    [Theory]
    [InlineData("010")]
    [InlineData("0x10")]
    [InlineData("1e3")]
    [InlineData("1.50")]
    [InlineData("true")]
    [InlineData("null")]
    [InlineData("-0")]
    [InlineData("+5")]
    public void Values_stay_strings_no_number_or_hex_coercion(string value)
    {
        AssertOk(MetaSet(w.S1, "this", "session:v=" + value));
        var node = World.Out(w.Call(w.S1, "meta get", ("items", new[] { "this", "session:v" })))["session:v"]!;
        Assert.Equal(System.Text.Json.JsonValueKind.String, node.GetValueKind());
        Assert.Equal(value, node.GetValue<string>());
    }

    [Theory]
    [InlineData("{\"session:n\":1}")]
    [InlineData("{\"session:n\":1.5}")]
    [InlineData("{\"session:n\":true}")]
    [InlineData("{\"session:n\":[\"a\"]}")]
    [InlineData("{\"session:n\":{\"a\":\"b\"}}")]
    [InlineData("[\"session:n\"]")]
    [InlineData("\"text\"")]
    [InlineData("42")]
    [InlineData("null")]
    [InlineData("{not json")]
    [InlineData("")]
    [InlineData("{}")]
    public void Json_must_be_an_object_of_strings_and_nulls_with_at_least_one_key(string json)
    {
        var before = Snapshot(w.S1);
        AssertFail(MetaJson(w.S1, "this", json), 2);
        Assert.Equal(before, Snapshot(w.S1));
    }

}
