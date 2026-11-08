// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

// Contract section 11 item 4: ownership matrix of 3.2 / 3.3 / 3.4 with three tokens (Manager: focus+admin+grant, S1, S2) plus a child block that S1
// creates (a createdby chain: S1 -> Child -> Grand). Each test name says what rule it pins; the test is red when that rule is missing.
public sealed partial class OwnershipTests
{
    [Fact]
    public void A_token_of_a_deleted_block_is_refused_3()
    {
        var token = w.S2.Token;
        Is(0, w.Call(w.Manager, "block delete", ("block", w.S2.Id), ("force", true)));
        Is(3, w.Host.Handle(new Request { Verb = "whoami", Token = token }));
        Is(3, w.Host.Handle(new Request { Verb = "send", Token = token, Args = World.Args(("block", w.S1.Id), ("tokens", new[] { "x" })) }));
        Assert.Empty(w.S1.Input);
    }

    [Fact]
    public void A_token_of_a_block_deleted_by_itself_is_refused_3()
    {
        var token = w.S1.Token;
        Is(0, w.Call(w.S1, "block delete", ("block", "this"), ("force", true)));
        Is(3, w.Host.Handle(new Request { Verb = "whoami", Token = token }));
    }

    [Fact]
    public void An_invented_or_empty_token_is_refused_3()
    {
        Is(3, w.Host.Handle(new Request { Verb = "whoami", Token = "deadbeef" }));
        Is(3, w.Host.Handle(new Request { Verb = "whoami", Token = "" }));
        Is(3, w.Host.Handle(new Request { Verb = "whoami", Token = w.S1.Id }));
    }

    [Fact]
    public void Block_id_is_not_a_token()
    {
        Is(3, w.Host.Handle(new Request { Verb = "send", Token = w.Manager.Id, Args = World.Args(("block", w.S1.Id), ("tokens", new[] { "x" })) }));
    }

    [Fact]
    public void Tokens_rotate_at_app_start_the_old_token_is_refused_3()
    {
        var old = w.S1.Token;
        w.Host.SimulateAppRestart();
        Is(3, w.Host.Handle(new Request { Verb = "whoami", Token = old }));
        Is(0, w.Call(w.S1, "whoami"));
    }

    [Fact]
    public void A_token_cannot_be_used_to_claim_another_blocks_identity_via_this()
    {
        var r = w.Call(w.S1, "whoami");
        Assert.Equal(w.S1.Id, World.Out(r)["blockid"]!.GetValue<string>());
        Is(3, w.Call(w.S1, "send", ("block", w.Manager.Id), ("tokens", new[] { "x" })));
        Is(0, w.Call(w.S1, "send", ("block", "this"), ("tokens", new[] { "x" })));
        Assert.Empty(w.Manager.Input);
    }

    [Fact]
    public void The_tab_keyword_is_the_callers_own_tab_read_from_the_record()
    {
        Is(0, w.Call(w.Manager, "tab rename", ("tab", "tab"), ("name", "HomeX")));
        Assert.Equal("HomeX", w.Host.GetTab(w.Home.Id).Name);
        Is(3, w.Call(w.S1, "tab rename", ("tab", "tab"), ("name", "ProjX")));
    }
}
