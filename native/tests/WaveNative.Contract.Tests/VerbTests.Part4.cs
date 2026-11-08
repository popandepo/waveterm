// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// Per-verb happy paths and every exit code of cli-contract.md section 1 for: whoami, session start/list/close, tab new/rename/move/style/switch,
// workspace style, block list/delete, run, web open, exit 7, and the real client's argument and stream rules. Each test says (in its name or a
// comment) what would make it red.
public sealed partial class VerbTests
{
    [Theory]
    [InlineData("ftp://example.com/x")]
    [InlineData("javascript:alert(1)")]
    [InlineData("mailto:a@b.example")]
    [InlineData("example.com")]
    [InlineData("data:text/html,hi")]
    public void Web_open_with_another_scheme_or_no_scheme_is_exit_2_and_creates_nothing(string url)
    {
        int blocks = Blocks;
        Fail(w.Call(w.Manager, "web open", ("url", url)), 2);
        Assert.Equal(blocks, Blocks);
    }

    [Theory]
    [InlineData("http://example.com/")]
    [InlineData("https://example.com/")]
    [InlineData("file:///C:/tmp/x.html")]
    public void Web_open_accepts_http_https_and_file(string url)
    {
        Ok(w.Call(w.Manager, "web open", ("url", url)));
    }

    [Fact]
    public void Web_open_by_a_session_without_stack_in_an_area_tab_is_exit_3_and_with_stack_adds_a_hidden_member()
    {
        int blocks = Blocks;
        Fail(w.Call(w.S1, "web open", ("url", "https://example.com/")), 3);
        Assert.Equal(blocks, Blocks);
        var o = Ok(w.Call(w.S1, "web open", ("url", "https://example.com/"), ("stack", w.S2.Id)));
        Assert.Equal(w.S2.Id, Shown(w.Projects.Id, w.S2.Id));
        Assert.Equal(Str(BEntry(w.S1, w.S2.Id)["stack"]), Str(BEntry(w.S1, Str(o["blockid"]))["stack"]));
    }

    [Fact]
    public void Web_open_in_an_owned_tab_without_stack_is_a_new_tile()
    {
        var (tab, _) = Sess(w.S1);
        var o = Ok(w.Call(w.S1, "web open", ("url", "https://example.com/"), ("tab", tab)));
        Assert.Null(BEntry(w.S1, Str(o["blockid"]))["stack"]);
    }

    [Fact]
    public void Web_open_focus_without_focus_is_exit_3_and_with_focus_and_stack_the_new_member_is_shown()
    {
        Fail(w.Call(w.S1, "web open", ("url", "https://example.com/"), ("stack", w.S2.Id), ("focus", true)), 3);
        var o = Ok(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("stack", w.S2.Id), ("focus", true)));
        Assert.Equal(Str(o["blockid"]), Shown(w.Projects.Id, w.S2.Id));
    }

    [Fact]
    public void Web_open_title_with_a_newline_is_exit_2_and_a_missing_url_is_exit_2()
    {
        Fail(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("title", "a\nb")), 2);
        Fail(w.Call(w.Manager, "web open"), 2);
    }

    [Fact]
    public void Web_open_unknown_stack_or_tab_is_exit_4()
    {
        Fail(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("stack", NewId())), 4);
        Fail(w.Call(w.Manager, "web open", ("url", "https://example.com/"), ("tab", NewId())), 4);
    }

    // ================= real client: usage errors exit 2 before anything is sent =================

    [Fact]
    public void Client_unknown_verb_is_exit_2_before_the_token_is_even_checked()
    {
        var r = w.Cli(null, "frobnicate");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Theory]
    [InlineData("tab list")]
    [InlineData("blocks list")]
    [InlineData("api list")]
    [InlineData("createblock")]
    [InlineData("deleteblock")]
    [InlineData("session")]
    [InlineData("tab")]
    public void Client_cut_or_incomplete_verbs_are_exit_2_not_a_silent_alias(string words)
    {
        var r = w.Cli(w.Manager, words.Split(' '));
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Fact]
    public void Client_no_arguments_is_exit_2()
    {
        var r = w.Cli(null);
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Fact]
    public void Client_unknown_flag_is_exit_2_and_nothing_is_created()
    {
        int blocks = Blocks;
        var r = w.Cli(w.S1, "session", "start", "--name", "x", "--bogus", "--", "pwsh");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
        Assert.Equal(blocks, Blocks);
        var r2 = w.Cli(null, "whoami", "--verbose");
        CliFail(r2.Stdout, r2.Stderr, r2.Exit, 2);
    }

    [Theory]
    [InlineData("session close")]
    [InlineData("tab rename")]
    [InlineData("tab move")]
    [InlineData("tab switch")]
    [InlineData("block delete")]
    [InlineData("web open")]
    public void Client_missing_positional_is_exit_2(string words)
    {
        var r = w.Cli(null, words.Split(' '));
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Fact]
    public void Client_tab_rename_with_only_the_tab_is_exit_2_and_too_many_positionals_are_exit_2()
    {
        var r = w.Cli(null, "tab", "rename", w.Projects.Id);
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
        var r2 = w.Cli(null, "session", "close", w.Projects.Id, "extra");
        CliFail(r2.Stdout, r2.Stderr, r2.Exit, 2);
    }

    [Theory]
    [InlineData("abc")]
    [InlineData("1.5")]
    [InlineData("")]
    public void Client_bad_int_for_index_is_exit_2_and_the_order_stays(string index)
    {
        var r = w.Cli(w.Manager, "tab", "move", w.Projects.Id, "--index", index);
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
        Assert.Equal("Home", w.Ws.Tabs[0].Name);
    }

    [Fact]
    public void Client_flag_without_its_value_and_a_flag_given_twice_are_exit_2()
    {
        var r = w.Cli(null, "tab", "move", w.Projects.Id, "--index");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
        var r2 = w.Cli(null, "tab", "new", "--name", "a", "--name", "b");
        CliFail(r2.Stdout, r2.Stderr, r2.Exit, 2);
    }

    [Fact]
    public void Client_double_dash_on_a_verb_that_takes_no_command_is_exit_2()
    {
        var r = w.Cli(null, "whoami", "--", "x");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Fact]
    public void Client_args_file_together_with_a_command_after_double_dash_is_exit_2_and_nothing_is_created()
    {
        var f = TempText("a.json", "[\"pwsh\"]");
        int blocks = Blocks;
        var r = w.Cli(w.Manager, "session", "start", "--args-file", f, "--", "pwsh");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
        Assert.Equal(blocks, Blocks);
    }

    [Theory]
    [InlineData("{\"a\":1}")]
    [InlineData("\"pwsh\"")]
    [InlineData("null")]
    [InlineData("[1,2]")]
    [InlineData("[\"pwsh\",null]")]
    [InlineData("not json")]
    [InlineData("")]
    public void Client_args_file_that_is_not_a_json_array_of_strings_is_exit_2_and_nothing_is_created(string text)
    {
        var f = TempText("bad.json", text);
        int blocks = Blocks;
        var r = w.Cli(w.Manager, "session", "start", "--args-file", f);
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
        Assert.Equal(blocks, Blocks);
    }

    [Fact]
    public void Client_args_file_that_does_not_exist_is_exit_2()
    {
        var r = w.Cli(w.Manager, "session", "start", "--args-file", Path.Combine(w.DataDir, "nope.json"));
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Fact]
    public void Client_args_file_with_a_utf8_bom_is_accepted_and_the_array_becomes_the_command()
    {
        var body = Encoding.UTF8.GetBytes("[\"pwsh\",\"-NoLogo\",\"-File\",\"x.ps1\"]\r\n");
        var f = TempFile("bom.json", new byte[] { 0xEF, 0xBB, 0xBF }.Concat(body).ToArray());
        var o = CliOk(w.Cli(w.Manager, "session", "start", "--args-file", f));
        var m = BEntry(w.Manager, Str(o["blockid"]))["meta"]!;
        Assert.Equal("pwsh", Str(m["cmd"]));
        Assert.Equal(new[] { "-NoLogo", "-File", "x.ps1" }, Strs(m["cmd:args"]));
    }

    [Fact]
    public void Client_args_file_without_a_bom_gives_the_same_command()
    {
        var f = TempText("plain.json", "[\"pwsh\",\"-NoLogo\"]");
        var o = CliOk(w.Cli(w.Manager, "session", "start", "--args-file", f));
        Assert.Equal(new[] { "-NoLogo" }, Strs(BEntry(w.Manager, Str(o["blockid"]))["meta"]!["cmd:args"]));
    }

    // ================= real client: streams, error shape, kinds =================

    [Fact]
    public void Client_success_prints_one_json_line_on_stdout_and_nothing_on_stderr()
    {
        var r = w.Cli(w.S1, "session", "list");
        Assert.Equal(0, r.Exit);
        Assert.Equal("", r.Stderr);
        Assert.EndsWith("\n", r.Stdout);
        Assert.Single(r.Stdout.TrimEnd().Split('\n'));
        Assert.Equal(JsonValueKind.Array, J(r.Stdout).GetValueKind());
        Assert.DoesNotContain(w.S1.Token, r.Stdout);
    }

    [Fact]
    public void Client_empty_list_is_exactly_an_empty_json_array()
    {
        var tab = Str(Ok(w.Call(w.S1, "tab new"))["tabid"]);
        var r = w.Cli(w.S1, "block", "list", "--tab", tab);
        Assert.Equal(0, r.Exit);
        Assert.Equal("[]", r.Stdout.Trim());
        Assert.Equal("", r.Stderr);
    }

    [Fact]
    public void Client_denied_is_exit_3_kind_denied_with_empty_stdout()
    {
        var r = w.Cli(w.S1, "tab", "switch", w.Projects.Id);
        CliFail(r.Stdout, r.Stderr, r.Exit, 3);
        Assert.DoesNotContain(w.S1.Token, r.Stderr);
    }

    [Fact]
    public void Client_not_found_is_exit_4_kind_notfound_with_empty_stdout()
    {
        var r = w.Cli(w.S1, "session", "close", NewId());
        CliFail(r.Stdout, r.Stderr, r.Exit, 4);
    }

    [Fact]
    public void Client_refused_is_exit_5_kind_refused_with_empty_stdout()
    {
        var r = w.Cli(w.S1, "block", "delete", "this");
        CliFail(r.Stdout, r.Stderr, r.Exit, 5);
        Assert.True(w.Host.HasBlock(w.S1.Id));
    }

    [Fact]
    public void Client_server_side_usage_error_is_exit_2_kind_usage_with_empty_stdout()
    {
        var r = w.Cli(w.S1, "tab", "style", "--tab", "tab");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Fact]
    public void Client_without_a_token_is_exit_3_for_a_list_verb_too_with_empty_stdout()
    {
        var r = w.Cli(null, "block", "list");
        CliFail(r.Stdout, r.Stderr, r.Exit, 3);
    }

    [Fact]
    public void Client_with_the_pipe_gone_is_exit_6_with_empty_stdout_and_no_token_in_the_message()
    {
        var r = w.CliWithEnv(w.S1, "wave-no-such-pipe-" + Guid.NewGuid().ToString("N"), null, "session", "list");
        CliFail(r.Stdout, r.Stderr, r.Exit, 6);
        Assert.DoesNotContain(w.S1.Token, r.Stderr);
    }

    [Fact]
    public void Client_exit_7_keeps_ids_on_stdout_and_the_error_object_on_stderr()
    {
        w.Host.Faults.FailAfterCreate = true;
        var r = w.Cli(w.S1, "session", "start", "--name", "half", "--", "pwsh");
        Assert.Equal(7, r.Exit);
        var o = J(r.Stdout);
        Assert.Equal("partial", Str(o["status"]));
        Assert.True(w.Host.HasBlock(Str(o["blockid"])));
        Assert.True(w.Host.HasTab(Str(o["tabid"])));
        Assert.False(string.IsNullOrWhiteSpace(Str(o["warning"])));
        var e = J(r.Stderr)["error"]!;
        ErrorKeys(e);
        Assert.Equal(7, e["code"]!.GetValue<int>());
        Assert.Equal("partial", Str(e["kind"]));
    }

    [Fact]
    public void Client_exit_7_for_run_and_web_open_also_keeps_the_streams_apart()
    {
        var (tab, _) = Sess(w.S1);
        w.Host.Faults.FailAfterCreate = true;
        var a = w.Cli(w.S1, "run", "--tab", tab, "--", "pwsh");
        var b = w.Cli(w.Manager, "web", "open", "https://example.com/");
        foreach (var r in new[] { a, b })
        {
            Assert.Equal(7, r.Exit);
            Assert.Equal("partial", Str(J(r.Stdout)["status"]));
            Assert.Equal(7, J(r.Stderr)["error"]!["code"]!.GetValue<int>());
        }
    }

    // ================= real client: argv, repeated flags, = form, meta set =================

    [Fact]
    public void Client_argv_after_double_dash_passes_through_unchanged_including_flag_lookalikes_and_a_second_double_dash()
    {
        var args = new[] { "--name", "foo", "--cwd", "/x", "--", "a b", "say \"hi\"", "-c", "" };
        var o = CliOk(w.Cli(w.Manager, new[] { "session", "start", "--name", "pt", "--" }.Concat(new[] { "pwsh" }).Concat(args).ToArray()));
        var m = BEntry(w.Manager, Str(o["blockid"]))["meta"]!;
        Assert.Equal("pwsh", Str(m["cmd"]));
        Assert.Equal(args, Strs(m["cmd:args"]));
        Assert.Equal("pt", Str(STab(w.Manager, Str(o["tabid"]))["name"]));
    }

    [Fact]
    public void Client_run_argv_after_double_dash_passes_through_unchanged()
    {
        var args = new[] { "--tab", "nope", "--focus", "--restart" };
        var o = CliOk(w.Cli(w.Manager, new[] { "run", "--", "pwsh" }.Concat(args).ToArray()));
        var m = BEntry(w.Manager, Str(o["blockid"]))["meta"]!;
        Assert.Equal(args, Strs(m["cmd:args"]));
        Assert.False(Rec(Str(o["blockid"])).Restart);
    }

    [Fact]
    public void Client_repeated_meta_env_and_cap_flags_all_arrive()
    {
        var o = CliOk(w.Cli(w.Manager, "session", "start", "--meta", "session:a=1", "--meta", "session:b=2", "--env", "A=1", "--env", "B=2",
            "--cap", "focus", "--cap", "admin", "--", "pwsh"));
        var id = Str(o["blockid"]);
        var meta = STab(w.Manager, Str(o["tabid"]))["blocks"]![0]!["meta"]!;
        Keys(meta, "session:a", "session:b");
        Assert.Equal(2, Rec(id).Env.Count);
        Assert.Equal(new[] { "focus", "admin" }, Strs(Ok(w.Call(Rec(id), "whoami"))["caps"]));
    }

    [Fact]
    public void Client_flag_equals_value_form_is_the_same_as_the_spaced_form_and_only_the_first_equals_splits()
    {
        var o = CliOk(w.Cli(w.Manager, "session", "start", "--name=a=b", "--color=red", "--meta=session:k=v=w", "--", "pwsh"));
        var t = STab(w.Manager, Str(o["tabid"]));
        Assert.Equal("a=b", Str(t["name"]));
        Assert.Equal("red", Str(t["color"]));
        Assert.Equal("v=w", Str(t["blocks"]![0]!["meta"]!["session:k"]));
    }

    [Fact]
    public void Client_int_flag_equals_form_works_for_tab_move()
    {
        var o = CliOk(w.Cli(w.Manager, "tab", "move", w.Projects.Id, "--index=0"));
        Assert.Equal(0, o["index"]!.GetValue<int>());
    }

    [Fact]
    public void Client_meta_set_json_file_dash_reads_the_object_from_stdin_and_prints_id_set_unset()
    {
        var r = w.CliWithEnv(w.Manager, null, "{\"frame:title\":\"Ti\",\"session:name\":\"mgr\"}", "meta", "set", "this", "--json-file", "-");
        var o = CliOk((r.Exit, r.Stdout, r.Stderr));
        Keys(o, "id", "set", "unset");
        Assert.Equal(w.Manager.Id, Str(o["id"]));
        Assert.Equal(new[] { "frame:title", "session:name" }, Strs(o["set"]).OrderBy(x => x, StringComparer.Ordinal).ToArray());
        Assert.Empty(o["unset"]!.AsArray());
        Assert.Equal("Ti", w.Manager.Title);
        Assert.Equal("mgr", w.Manager.Session["session:name"]);
    }

    [Fact]
    public void Client_meta_set_json_file_dash_with_null_deletes_the_key()
    {
        Ok(w.Call(w.Manager, "meta set", ("block", "this"), ("pairs", new[] { "session:name=mgr" })));
        var r = w.CliWithEnv(w.Manager, null, "{\"session:name\":null}", "meta", "set", "this", "--json-file", "-");
        var o = CliOk((r.Exit, r.Stdout, r.Stderr));
        Assert.Equal(new[] { "session:name" }, Strs(o["unset"]));
        Assert.Empty(o["set"]!.AsArray());
        Assert.False(w.Manager.Session.ContainsKey("session:name"));
    }

    [Fact]
    public void Client_meta_set_json_file_path_reads_a_file_and_tolerates_a_bom()
    {
        var f = TempFile("m.json", new byte[] { 0xEF, 0xBB, 0xBF }.Concat(Encoding.UTF8.GetBytes("{\"session:effort\":\"high\"}")).ToArray());
        var o = CliOk(w.Cli(w.Manager, "meta", "set", "this", "--json-file", f));
        Assert.Equal(new[] { "session:effort" }, Strs(o["set"]));
        Assert.Equal("high", w.Manager.Session["session:effort"]);
    }

    [Fact]
    public void Client_meta_set_with_both_json_and_json_file_is_exit_2_before_anything_is_sent()
    {
        var r = w.Cli(null, "meta", "set", "this", "--json", "{\"session:a\":\"1\"}", "--json-file", "-");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
    }

    [Fact]
    public void Client_meta_set_json_file_that_is_missing_or_whose_stdin_is_not_json_is_exit_2_and_changes_nothing()
    {
        var a = w.Cli(w.Manager, "meta", "set", "this", "--json-file", Path.Combine(w.DataDir, "nope.json"));
        CliFail(a.Stdout, a.Stderr, a.Exit, 2);
        var b = w.CliWithEnv(w.Manager, null, "not json", "meta", "set", "this", "--json-file", "-");
        CliFail(b.Stdout, b.Stderr, b.Exit, 2);
        Assert.Equal("manager", w.Manager.Title);
    }

    [Fact]
    public void Client_meta_set_with_one_unsettable_key_changes_nothing_all_or_none()
    {
        var r = w.CliWithEnv(w.Manager, null, "{\"frame:title\":\"X\",\"cmd\":\"evil\"}", "meta", "set", "this", "--json-file", "-");
        CliFail(r.Stdout, r.Stderr, r.Exit, 2);
        Assert.Equal("manager", w.Manager.Title);
    }

    [Fact]
    public void Client_meta_set_url_through_stdin_keeps_the_secret_off_the_listing_and_is_readable_in_full_by_the_block_itself()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://example.com/start");
        var json = "{\"url\":\"https://home.example/?entry=deadbeef01\",\"frame:title\":\"Home\"}";
        var r = w.CliWithEnv(web, null, json, "meta", "set", "this", "--json-file", "-");
        Assert.Equal(0, r.Exit);
        var listing = w.Cli(w.S1, "block", "list");
        Assert.DoesNotContain("deadbeef01", listing.Stdout);
        Assert.Contains("https://home.example/", listing.Stdout);
        var own = CliOk(w.Cli(web, "meta", "get", "url"));
        Assert.Equal("https://home.example/?entry=deadbeef01", Str(own["url"]));
    }
}
