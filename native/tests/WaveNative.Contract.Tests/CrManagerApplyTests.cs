// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// ---------------------------------------------------------------------------------------------------------------------------------
// (a) ApplyManagerConfig, restart survival, grant, promotion
// ---------------------------------------------------------------------------------------------------------------------------------
public sealed class CrManagerApplyTests : CrBase
{
    Block Apply(Action<JsonObject>? mut = null) => w.Host.ApplyManagerConfig(w.Ws, CrH.Parse(CrH.Cfg(mut)));

    [Fact]
    public void A_missing_tab_is_created_with_the_configured_name_and_no_creator()
    {
        int before = w.Ws.Tabs.Count;
        var b = Apply(x => x["tab"] = "Ops");
        Assert.Equal(before + 1, w.Ws.Tabs.Count);
        var tab = w.Ws.Tabs.Single(t => t.Name == "Ops");
        Assert.Null(tab.CreatedBy);
        Assert.Equal(tab.Id, b.TabId);
        Assert.Contains(b.Id, tab.BlockIds);
    }

    [Fact]
    public void A_present_tab_is_found_by_name_and_not_duplicated()
    {
        int before = w.Ws.Tabs.Count;
        var b = Apply(x => x["tab"] = "Managers");
        Assert.Equal(before, w.Ws.Tabs.Count);
        Assert.Equal(w.Managers.Id, b.TabId);
        Assert.Contains(b.Id, w.Managers.BlockIds);
        Assert.Single(w.Ws.Tabs, t => t.Name == "Managers");
    }

    [Fact]
    public void The_default_tab_is_Home_and_is_reused()
    {
        int before = w.Ws.Tabs.Count;
        var b = Apply();
        Assert.Equal(before, w.Ws.Tabs.Count);
        Assert.Equal(w.Home.Id, b.TabId);
    }

    [Fact]
    public void The_block_has_the_default_capabilities_no_creator_restart_and_running()
    {
        var b = Apply();
        Assert.Equal(new[] { "admin", "focus", "grant" }, b.Caps.OrderBy(x => x).ToArray());
        Assert.Null(b.CreatedBy);
        Assert.True(b.Restart);
        Assert.Equal("running", b.Status);
        Assert.Equal("manager", b.Title);
    }

    [Fact]
    public void The_block_has_exactly_the_listed_capabilities()
    {
        var b = Apply(x => x["caps"] = new JsonArray("focus"));
        Assert.Equal(new[] { "focus" }, b.Caps.ToArray());
        var who = World.Out(w.Call(b, "whoami"));
        Assert.Equal(new[] { "focus" }, who["caps"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
    }

    [Fact]
    public void Whoami_lists_capabilities_in_the_contract_order()
    {
        var b = Apply(x => x["caps"] = new JsonArray("grant", "crossworkspace", "admin", "focus"));
        var caps = World.Out(w.Call(b, "whoami"))["caps"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray();
        Assert.Equal(new[] { "focus", "admin", "crossworkspace", "grant" }, caps);
    }

    [Fact]
    public void Command_cwd_env_name_and_session_meta_are_recorded()
    {
        var b = Apply(x =>
        {
            x["name"] = "boss";
            x["env"] = new JsonObject { ["K"] = "V" };
            x["session"] = new JsonObject { ["name"] = "boss", ["id"] = "abc" };
        });
        Assert.Equal(new[] { "pwsh", "-File", "m.ps1" }, b.Argv);
        Assert.Equal(CrH.Abs, b.Cwd);
        Assert.Equal(new Dictionary<string, string> { ["K"] = "V" }, b.Env);
        Assert.Equal("boss", b.Title);
        Assert.Equal("boss", b.Session["session:name"]);
        Assert.Equal("abc", b.Session["session:id"]);
        var list = World.Out(w.Call(w.Manager, "session list")).AsArray();
        var meta = list.SelectMany(t => t!["blocks"]!.AsArray()).Single(x => x!["blockid"]!.GetValue<string>() == b.Id)!["meta"]!;
        Assert.Equal("abc", meta["session:id"]!.GetValue<string>());
    }

    [Fact]
    public void The_config_block_may_start_a_block_with_a_capability_it_holds_but_never_grant()
    {
        var b = Apply();
        var ok = w.Call(b, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }, ("cap", new[] { "focus", "admin" })));
        Assert.Equal(0, ok.Exit);
        var child = w.Host.GetBlock(CrH.Str(ok, "blockid"));
        Assert.Equal(new[] { "admin", "focus" }, child.Caps.OrderBy(x => x).ToArray());
        Assert.Equal(b.Id, child.CreatedBy);
        var before = CrH.Snap(w);
        var no = w.Call(b, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }, ("cap", new[] { "grant" })));
        Assert.Equal(3, no.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void Cap_grant_is_refused_for_a_session_without_grant_and_nothing_is_created()
    {
        var before = CrH.Snap(w);
        var r = w.Call(w.S1, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }, ("cap", new[] { "grant" })));
        Assert.Equal(3, r.Exit);
        Assert.Equal("denied", CrH.Kind(r));
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void Cap_grant_is_refused_even_for_the_manager_who_holds_grant()
    {
        var before = CrH.Snap(w);
        var r = w.Call(w.Manager, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }, ("cap", new[] { "grant" })));
        Assert.Equal(3, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_cap_beyond_the_creators_own_rights_is_refused()
    {
        var m = Apply(x => x["caps"] = new JsonArray("grant", "focus"));
        var before = CrH.Snap(w);
        var r = w.Call(m, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }, ("cap", new[] { "admin" })));
        Assert.Equal(3, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Fact]
    public void A_config_error_starts_nothing_and_gives_no_rights()
    {
        var before = CrH.Snap(w);
        int grantHolders = w.Host.AllBlocks().Count(b => b.Caps.Contains("grant"));
        Assert.Throws<ConfigException>(() => w.Host.ApplyManagerConfig(w.Ws, CrH.Parse(CrH.Cfg(x => { x["tab"] = "Ops"; x["caps"] = new JsonArray("focus", "focus"); }))));
        Assert.Equal(before, CrH.Snap(w));
        Assert.DoesNotContain(w.Ws.Tabs, t => t.Name == "Ops");
        Assert.Equal(grantHolders, w.Host.AllBlocks().Count(b => b.Caps.Contains("grant")));
    }

    [Fact]
    public void A_config_error_does_not_give_a_session_the_rights_of_the_default_caps()
    {
        Assert.Throws<ConfigException>(() => CrH.Parse(CrH.Cfg(x => x["caps"] = new JsonArray("root"))));
        var who = World.Out(w.Call(w.S1, "whoami"));
        Assert.Empty(who["caps"]!.AsArray());
    }

    [Fact]
    public void Rights_survive_an_app_restart_but_tokens_change()
    {
        var b = Apply(x => x["caps"] = new JsonArray("focus", "admin", "grant"));
        var oldToken = b.Token;
        w.Host.SimulateAppRestart();
        Assert.NotEqual(oldToken, b.Token);

        var stale = w.Host.Handle(new Request { Verb = "whoami", Token = oldToken });
        Assert.Equal(3, stale.Exit);
        Assert.Null(stale.Out);

        var fresh = w.Call(b, "whoami");
        Assert.Equal(0, fresh.Exit);
        Assert.Equal(new[] { "focus", "admin", "grant" }, World.Out(fresh)["caps"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
        Assert.Null(b.CreatedBy);
    }

    [Fact]
    public void After_a_restart_every_old_token_is_refused_and_every_new_one_works()
    {
        var olds = w.Host.AllBlocks().ToDictionary(b => b.Id, b => b.Token);
        w.Host.SimulateAppRestart();
        foreach (var b in w.Host.AllBlocks())
        {
            Assert.NotEqual(olds[b.Id], b.Token);
            Assert.Equal(3, w.Host.Handle(new Request { Verb = "whoami", Token = olds[b.Id] }).Exit);
            Assert.Equal(0, w.Call(b, "whoami").Exit);
        }
    }

    [Fact]
    public void After_a_restart_restart_blocks_run_again_and_others_have_status_none()
    {
        var r = Apply();
        var started = w.Call(w.Manager, "session start", CrH.A(new[] { "pwsh", "-File", "keep.ps1" }, ("restart", true)));
        var plain = w.Call(w.Manager, "session start", CrH.A(new[] { "pwsh", "-File", "once.ps1" }));
        var keepId = CrH.Str(started, "blockid");
        var plainId = CrH.Str(plain, "blockid");
        w.Host.FinishProgram(keepId, 0);
        w.Host.FinishProgram(plainId, 0);
        w.Host.SimulateAppRestart();
        Assert.Equal("running", r.Status);
        Assert.Equal("running", w.Host.GetBlock(keepId).Status);
        Assert.Equal("none", w.Host.GetBlock(plainId).Status);
        Assert.Equal("none", w.S1.Status);
        Assert.Equal("none", w.S2.Status);
        Assert.Equal("running", w.Manager.Status);
    }

    [Fact]
    public void Capabilities_and_creator_of_a_created_block_are_unchanged_by_a_restart()
    {
        var ok = w.Call(w.Manager, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }, ("restart", true), ("cap", new[] { "focus" })));
        var child = w.Host.GetBlock(CrH.Str(ok, "blockid"));
        w.Host.SimulateAppRestart();
        Assert.Equal(new[] { "focus" }, child.Caps.ToArray());
        Assert.Equal(w.Manager.Id, child.CreatedBy);
        var who = World.Out(w.Call(child, "whoami"));
        Assert.Equal(new[] { "focus" }, who["caps"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
    }

    [Fact]
    public void Meta_cannot_carry_rights_across_a_restart_or_at_all()
    {
        var r = w.Call(w.S1, "meta set", ("block", "this"), ("pairs", new[] { "session:caps=admin" }));
        Assert.Equal(0, r.Exit);
        w.Host.SimulateAppRestart();
        Assert.Empty(World.Out(w.Call(w.S1, "whoami"))["caps"]!.AsArray());
    }

    [Fact]
    public void Promotion_clears_the_creator_and_sets_focus_admin_and_grant()
    {
        var started = w.Call(w.S1, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }));
        var b = w.Host.GetBlock(CrH.Str(started, "blockid"));
        Assert.Equal(w.S1.Id, b.CreatedBy);
        Assert.Empty(b.Caps);
        w.Host.PromoteToManager(b.Id);
        Assert.Null(b.CreatedBy);
        Assert.Equal(new[] { "admin", "focus", "grant" }, b.Caps.OrderBy(x => x).ToArray());
        Assert.Equal(new[] { "focus", "admin", "grant" }, World.Out(w.Call(b, "whoami"))["caps"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
    }

    [Fact]
    public void After_promotion_the_session_that_created_the_block_no_longer_owns_it()
    {
        var started = w.Call(w.S1, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }));
        var id = CrH.Str(started, "blockid");
        Assert.Equal(0, w.Call(w.S1, "meta set", ("block", id), ("pairs", new[] { "frame:title=mine" })).Exit);
        Assert.Equal(0, w.Call(w.S1, "attention", ("level", "info"), ("block", id)).Exit);

        w.Host.PromoteToManager(id);

        var meta = w.Call(w.S1, "meta set", ("block", id), ("pairs", new[] { "frame:title=still mine" }));
        Assert.Equal(3, meta.Exit);
        Assert.Equal("mine", w.Host.GetBlock(id).Title);
        Assert.Equal(3, w.Call(w.S1, "attention", ("level", "info"), ("block", id)).Exit);
        Assert.Equal(3, w.Call(w.S1, "send", ("block", id), ("tokens", new[] { "key:enter" })).Exit);
        Assert.Equal(3, w.Call(w.S1, "block delete", ("block", id), ("force", true)).Exit);
        Assert.True(w.Host.HasBlock(id));
    }

    [Fact]
    public void After_promotion_the_blocks_children_still_belong_to_the_chain_of_the_promoted_block()
    {
        var started = w.Call(w.S1, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" }));
        var id = CrH.Str(started, "blockid");
        var b = w.Host.GetBlock(id);
        w.Host.PromoteToManager(id);
        var child = w.Call(b, "session start", CrH.A(new[] { "pwsh", "-File", "y.ps1" }, ("cap", new[] { "focus" })));
        Assert.Equal(0, child.Exit);
        Assert.Equal(id, w.Host.GetBlock(CrH.Str(child, "blockid")).CreatedBy);
    }
}
