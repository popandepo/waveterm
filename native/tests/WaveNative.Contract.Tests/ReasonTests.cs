// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// A refusal carries a short stable reason next to the message, so a script can tell causes apart without parsing text; and file: urls keep their form.
public sealed class ReasonTests : IDisposable
{
    readonly World w = new();

    public void Dispose() => w.Dispose();

    static string? ReasonOf(Reply r) => r.Err?["error"]?["reason"]?.GetValue<string>();

    void Expect(int exit, string reason, Reply r)
    {
        Assert.Equal(exit, r.Exit);
        Assert.Equal(reason, ReasonOf(r));
    }

    [Fact]
    public void No_token_in_the_client_says_notoken_and_an_unknown_token_in_the_host_says_badtoken()
    {
        var (exit, _, stderr) = w.Cli(null, "whoami");
        Assert.Equal(3, exit);
        Assert.Equal("notoken", JsonNode.Parse(stderr)!["error"]!["reason"]!.GetValue<string>());
        Expect(3, "badtoken", w.Host.Handle(new Request { Verb = "whoami", Token = "not-a-token" }));
    }

    [Fact]
    public void A_missing_capability_and_a_missing_ownership_have_different_reasons()
    {
        Expect(3, "capability", w.Call(w.S1, "tab switch", ("tab", w.Home.Id)));
        Expect(3, "notowner", w.Call(w.S1, "send", ("block", w.S2.Id), ("tokens", new[] { "x" })));
        Expect(3, "capability", w.Call(w.S1, "session start", ("argv", new[] { "pwsh" }), ("cap", new[] { "admin" })));
    }

    [Fact]
    public void Safety_refusals_say_which_rule_refused()
    {
        Expect(5, "unfinished", w.Call(w.Manager, "block delete", ("block", w.S1.Id)));
        Expect(5, "owntab", w.Call(w.Manager, "session close", ("tab", w.Home.Id)));
        Expect(5, "restart", w.Call(w.S1, "session start", ("argv", new[] { "bash", "-c", "x" }), ("restart", true)));
    }

    [Fact]
    public void The_last_tab_of_a_workspace_is_refused_with_its_own_reason()
    {
        var ws2 = w.Host.CreateWorkspace();
        var only = w.Host.CreateTabAsApp(ws2, "Only");
        var admin = w.Host.StartBlockAsApp(w.Managers, new[] { "pwsh" }, w.Cwd, new[] { Caps.Admin, Caps.CrossWorkspace });
        Expect(5, "lasttab", w.Call(admin, "session close", ("tab", only.Id)));
        Assert.True(w.Host.HasTab(only.Id));
    }

    [Fact]
    public void Names_and_groups_and_content_checks_have_their_own_reasons()
    {
        w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:name=dup" }));
        Expect(5, "duplicate", w.Call(w.S2, "meta set", ("block", "this"), ("pairs", new[] { "session:name=dup" })));
        Expect(5, "duplicate", w.Call(w.Manager, "tab new", ("name", "Projects")));
        Expect(5, "notingroup", w.Call(w.Manager, "group remove", ("block", w.S1.Id)));
        Expect(5, "othertab", w.Call(w.Manager, "group add", ("block", w.S1.Id), ("to", w.Manager.Id)));
        var page = w.Host.StartWebBlockAsApp(w.Projects, "http://127.0.0.1:1/x");
        page.Page["#a"] = "text";
        Expect(5, "nomatch", w.Call(w.Manager, "web text", ("block", page.Id), ("selector", "#nothing")));
        Expect(5, "notweb", w.Call(w.Manager, "web text", ("block", w.S1.Id)));
        Expect(5, "notterm", w.Call(w.Manager, "screen", ("block", page.Id)));
        w.Host.FinishProgram(w.S2.Id, 0);
        Expect(5, "notrunning", w.Call(w.Manager, "send", ("block", w.S2.Id), ("tokens", new[] { "x" })));
    }

    [Fact]
    public void Putting_blocks_in_the_group_they_are_already_in_says_samegroup_for_add_and_for_move()
    {
        Assert.Equal(0, w.Call(w.Manager, "group add", ("block", w.S1.Id), ("to", w.S2.Id)).Exit);
        Expect(5, "samegroup", w.Call(w.Manager, "group add", ("block", w.S1.Id), ("to", w.S2.Id)));
        Expect(5, "samegroup", w.Call(w.Manager, "group move", ("from", w.S1.Id), ("to", w.S2.Id)));
    }

    [Fact]
    public void A_manager_config_session_key_with_an_equals_sign_is_a_config_error()
    {
        var o = CrH.Cfg(x => x["session"] = new JsonObject { ["a=b"] = "c" });
        Assert.Throws<ConfigException>(() => CrH.Parse(o));
    }

    [Fact]
    public void A_usage_error_and_a_not_found_carry_no_reason_unless_one_was_named()
    {
        Assert.Null(ReasonOf(w.Call(w.S1, "tab move", ("tab", "tab"), ("index", 99))));
        Assert.Null(ReasonOf(w.Call(w.S1, "block delete", ("block", Guid.NewGuid().ToString()))));
    }

    // ---- file: urls keep their form in listings and in equality tests ----

    [Fact]
    public void A_file_url_is_listed_with_three_slashes_and_found_again_by_url()
    {
        const string url = "file:///C:/x/page%20one.html?a=1#top";
        var made = World.Out(w.Call(w.Manager, "web open", ("url", url), ("tab", w.Managers.Id)));
        var id = made["blockid"]!.GetValue<string>();
        var entry = World.Out(w.Call(w.Manager, "block list", ("view", "web"), ("tab", w.Managers.Id))).AsArray()
            .First(b => b!["blockid"]!.GetValue<string>() == id)!;
        Assert.Equal("file:///C:/x/page%20one.html", entry["meta"]!["url"]!.GetValue<string>());
        var found = World.Out(w.Call(w.Manager, "block list", ("url", url))).AsArray();
        Assert.Contains(found, b => b!["blockid"]!.GetValue<string>() == id);
        var other = World.Out(w.Call(w.Manager, "block list", ("url", "file:///C:/x/page%20one.html?a=2#top"))).AsArray();
        Assert.DoesNotContain(other, b => b!["blockid"]!.GetValue<string>() == id);
    }

    [Theory]
    [InlineData("file:///C:/x/y.html", "file:///C:/x/y.html")]
    [InlineData("file:///C:/x/y.html?q=1#f", "file:///C:/x/y.html")]
    [InlineData("HTTP://U:P@Example.COM:80/a?b=1#c", "http://Example.COM/a")]
    public void The_printed_form_of_a_url_has_no_user_info_query_or_fragment(string input, string expected)
    {
        Assert.Equal(expected.ToLowerInvariant(), Validators.RedactUrl(input).ToLowerInvariant());
    }
}
