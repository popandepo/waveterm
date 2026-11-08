// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;

public sealed class SmokeTests : IDisposable
{
    readonly World w = new();

    public void Dispose() => w.Dispose();

    [Fact]
    public void Whoami_in_process_names_the_caller_and_its_caps()
    {
        var r = w.Call(w.Manager, "whoami");
        Assert.Equal(0, r.Exit);
        Assert.Equal(w.Manager.Id, World.Out(r)["blockid"]!.GetValue<string>());
        Assert.Equal(new[] { "focus", "admin", "grant" }, World.Out(r)["caps"]!.AsArray().Select(x => x!.GetValue<string>()).ToArray());
        Assert.Empty(World.Out(w.Call(w.S1, "whoami"))["caps"]!.AsArray());
    }

    [Fact]
    public void The_real_client_over_the_pipe_prints_json_and_exits_zero()
    {
        var (exit, stdout, stderr) = w.Cli(w.S1, "whoami");
        Assert.Equal(0, exit);
        Assert.Equal("", stderr);
        Assert.Equal(w.S1.Id, JsonNode.Parse(stdout)!["blockid"]!.GetValue<string>());
    }

    [Fact]
    public void The_real_client_without_a_token_is_denied_before_it_connects()
    {
        var (exit, stdout, stderr) = w.Cli(null, "whoami");
        Assert.Equal(3, exit);
        Assert.Equal("", stdout);
        Assert.Equal(3, JsonNode.Parse(stderr)!["error"]!["code"]!.GetValue<int>());
    }

    [Fact]
    public void The_real_client_with_the_pipe_gone_exits_six()
    {
        var (exit, _, stderr) = w.CliWithEnv(w.S1, "wave-no-such-pipe-" + Guid.NewGuid().ToString("N"), null, "whoami");
        Assert.Equal(6, exit);
        Assert.Equal("unreachable", JsonNode.Parse(stderr)!["error"]!["kind"]!.GetValue<string>());
    }
}
