// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// ---------------------------------------------------------------------------------------------------------------------------------
// (a) ManagerConfig.Parse
// ---------------------------------------------------------------------------------------------------------------------------------
public sealed class CrManagerParseTests
{
    [Fact]
    public void Minimal_entry_gets_the_documented_defaults()
    {
        var c = CrH.Parse(CrH.Cfg());
        Assert.Equal("manager", c.Name);
        Assert.Equal("Home", c.Tab);
        Assert.Equal(new[] { "focus", "admin", "grant" }, c.Caps.OrderBy(x => Array.IndexOf(Caps.All, x)).ToArray());
        Assert.Equal(3, c.Caps.Count);
        Assert.Empty(c.Env);
        Assert.Empty(c.Session);
        Assert.Equal(new[] { "pwsh", "-File", "m.ps1" }, c.Argv);
    }

    [Fact]
    public void Full_entry_is_read_as_written()
    {
        var o = CrH.Cfg(x =>
        {
            x["name"] = "boss";
            x["tab"] = "Ops";
            x["env"] = new JsonObject { ["K"] = "V", ["_B2"] = "" };
            x["caps"] = new JsonArray("focus", "crossworkspace");
            x["session"] = new JsonObject { ["name"] = "boss", ["id"] = "11111111-2222-3333-4444-555555555555" };
        });
        var c = CrH.Parse(o);
        Assert.Equal("boss", c.Name);
        Assert.Equal("Ops", c.Tab);
        Assert.Equal("V", c.Env["K"]);
        Assert.Equal("", c.Env["_B2"]);
        Assert.Equal(new[] { "focus", "crossworkspace" }, c.Caps);
        Assert.Equal("boss", c.Session["session:name"]);
        Assert.Equal("11111111-2222-3333-4444-555555555555", c.Session["session:id"]);
    }

    [Theory]
    [InlineData("extra")]
    [InlineData("createdby")]
    [InlineData("token")]
    [InlineData("restart")]
    [InlineData("Cwd")]
    public void An_unknown_key_is_a_config_error(string key)
    {
        var o = CrH.Cfg(x => x[key] = "x");
        Assert.Throws<ConfigException>(() => CrH.Parse(o));
    }

    [Theory]
    [InlineData("work")]
    [InlineData("./work")]
    [InlineData("../work")]
    public void A_relative_cwd_is_a_config_error(string cwd)
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["cwd"] = cwd)));
    }

    [Fact]
    public void A_cwd_that_does_not_exist_is_a_config_error_and_the_check_sees_the_given_path()
    {
        string? asked = null;
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(), p => { asked = p; return false; }));
        Assert.Equal(CrH.Abs, asked);
    }

    [Fact]
    public void A_cwd_that_exists_passes()
    {
        Assert.Equal(CrH.Abs, CrH.Parse(CrH.Cfg(), p => p == CrH.Abs).Cwd);
    }

    [Fact]
    public void A_missing_cwd_key_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x.Remove("cwd"))));
    }

    [Fact]
    public void A_cwd_that_is_not_a_string_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["cwd"] = 5)));
    }

    [Fact]
    public void A_missing_argv_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x.Remove("argv"))));
    }

    [Fact]
    public void An_empty_argv_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["argv"] = new JsonArray())));
    }

    [Fact]
    public void An_argv_that_is_a_string_and_not_an_array_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["argv"] = "pwsh -File m.ps1")));
    }

    [Fact]
    public void An_empty_first_argv_element_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["argv"] = new JsonArray("", "-File"))));
    }

    [Fact]
    public void An_empty_later_argv_element_is_allowed()
    {
        Assert.Equal(new[] { "pwsh", "" }, CrH.Parse(CrH.Cfg(x => x["argv"] = new JsonArray("pwsh", ""))).Argv);
    }

    [Fact]
    public void A_number_in_argv_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["argv"] = new JsonArray("pwsh", 2))));
    }

    [Fact]
    public void A_null_or_nested_item_in_argv_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["argv"] = new JsonArray("pwsh", null))));
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["argv"] = new JsonArray("pwsh", new JsonArray("x")))));
    }

    [Theory]
    [InlineData("1A")]
    [InlineData("A-B")]
    [InlineData("A B")]
    [InlineData("")]
    [InlineData("A=B")]
    public void A_bad_env_name_is_a_config_error(string name)
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["env"] = new JsonObject { [name] = "v" })));
    }

    [Fact]
    public void An_env_value_that_is_not_a_string_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["env"] = new JsonObject { ["K"] = 5 })));
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["env"] = new JsonObject { ["K"] = true })));
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["env"] = new JsonObject { ["K"] = null })));
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["env"] = new JsonObject { ["K"] = new JsonObject() })));
    }

    [Fact]
    public void An_unknown_capability_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["caps"] = new JsonArray("focus", "root"))));
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["caps"] = new JsonArray("Focus"))));
    }

    [Fact]
    public void A_duplicate_capability_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["caps"] = new JsonArray("focus", "admin", "focus"))));
    }

    [Fact]
    public void A_capability_that_is_not_a_string_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["caps"] = new JsonArray("focus", 1))));
    }

    [Fact]
    public void Every_known_capability_may_be_listed_including_grant_and_crossworkspace()
    {
        var c = CrH.Parse(CrH.Cfg(x => x["caps"] = new JsonArray("focus", "admin", "crossworkspace", "grant")));
        Assert.Equal(4, c.Caps.Count);
        Assert.Equal(new[] { "crossworkspace" }, CrH.Parse(CrH.Cfg(x => x["caps"] = new JsonArray("crossworkspace"))).Caps);
    }

    // The caps default is focus+admin+grant: a value of the wrong type must not silently fall back to it (rights escalation).
    [Fact]
    public void Caps_that_is_a_string_and_not_an_array_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["caps"] = "focus")));
    }

    [Fact]
    public void Env_that_is_an_array_and_not_an_object_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["env"] = new JsonArray("K=V"))));
    }

    [Fact]
    public void Session_that_is_a_string_and_not_an_object_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["session"] = "manager")));
    }

    [Theory]
    [InlineData("a\nb")]
    [InlineData("a\u202Eb")]
    [InlineData("a\u200Bb")]
    [InlineData("a\u0007b")]
    public void A_session_value_with_a_refused_character_is_a_config_error(string value)
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["session"] = new JsonObject { ["name"] = value })));
    }

    [Fact]
    public void A_session_value_over_200_code_points_is_a_config_error_and_200_passes()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["session"] = new JsonObject { ["id"] = CrH.Rep("a", 201) })));
        Assert.Equal(200, CrH.Parse(CrH.Cfg(x => x["session"] = new JsonObject { ["id"] = CrH.Rep("a", 200) })).Session["session:id"].Length);
        Assert.Equal(200, CrH.Parse(CrH.Cfg(x => x["session"] = new JsonObject { ["id"] = CrH.Rep("\U0001F600", 200) })).Session["session:id"].EnumerateRunes().Count());
    }

    [Theory]
    [InlineData("bad key")]
    [InlineData("")]
    [InlineData("a/b")]
    [InlineData("a:b")]
    public void A_session_key_outside_the_key_grammar_is_a_config_error(string key)
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["session"] = new JsonObject { [key] = "v" })));
    }

    [Fact]
    public void A_session_value_that_is_not_a_string_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["session"] = new JsonObject { ["id"] = 5 })));
    }

    [Fact]
    public void A_name_over_60_units_is_a_config_error_and_60_passes()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["name"] = CrH.Rep("n", 61))));
        Assert.Equal(60, CrH.Parse(CrH.Cfg(x => x["name"] = CrH.Rep("n", 60))).Name.Length);
    }

    [Fact]
    public void A_name_with_a_refused_character_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["name"] = "a\nb")));
    }

    [Fact]
    public void A_name_that_is_not_a_string_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["name"] = 5)));
    }

    [Fact]
    public void A_tab_over_60_units_is_a_config_error_and_60_passes()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["tab"] = CrH.Rep("t", 61))));
        Assert.Equal(60, CrH.Parse(CrH.Cfg(x => x["tab"] = CrH.Rep("t", 60))).Tab.Length);
    }

    [Fact]
    public void A_tab_with_a_refused_character_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["tab"] = "Ho\u202Eme")));
    }

    [Fact]
    public void A_tab_that_is_not_a_string_is_a_config_error()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["tab"] = new JsonArray("Home"))));
    }
}
