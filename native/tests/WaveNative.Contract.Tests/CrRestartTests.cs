// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// ---------------------------------------------------------------------------------------------------------------------------------
// (b) --restart refusals
// ---------------------------------------------------------------------------------------------------------------------------------
public sealed class CrRestartTests : CrBase
{
    static readonly (string Cmd, string[] Flags)[] Table =
    {
        ("pwsh", new[] { "-command", "-c", "-encodedcommand", "-ec", "-e", "-commandwithargs", "-cwa" }),
        ("powershell", new[] { "-command", "-c", "-encodedcommand", "-ec", "-e", "-commandwithargs", "-cwa" }),
        ("cmd", new[] { "/c", "/k" }),
        ("bash", new[] { "-c", "-lc", "-ic", "-cl" }),
        ("sh", new[] { "-c", "-lc", "-ic", "-cl" }),
        ("zsh", new[] { "-c", "-lc", "-ic", "-cl" }),
        ("fish", new[] { "-c", "-lc", "-ic", "-cl" }),
        ("dash", new[] { "-c" }),
        ("nu", new[] { "-c", "--commands" }),
        ("node", new[] { "-e", "--eval", "-p", "--print" }),
        ("python", new[] { "-c" }),
        ("python3", new[] { "-c" }),
        ("py", new[] { "-c" }),
        ("ruby", new[] { "-e" }),
        ("perl", new[] { "-e", "-E" }),
    };

    public static IEnumerable<object[]> InlineCases()
    {
        foreach (var (cmd, flags) in Table)
            foreach (var f in flags)
                yield return new object[] { cmd + "|" + f + "|echo hi" };
    }

    public static IEnumerable<object[]> Variants() => new[]
    {
        "PWSH|-COMMAND|x", "Pwsh|-Command|x", "pwsh.exe|-Command|x", "PWSH.EXE|-c|x", "BASH|-C|x", "cmd.exe|/K|dir", "CMD|/C|dir", "Node|--EVAL|1", "Perl|-e|1", "PERL|-E|1",
        "C:\\x\\pwsh.exe|-c|x", "C:\\Program Files\\PowerShell\\7\\pwsh.exe|-Command|x", "C:/Program Files/PowerShell/7/pwsh.exe|-Command|x",
        "/usr/bin/bash|-lc|x", "/bin/sh|-c|x", "C:\\Windows\\System32\\cmd.exe|/c|dir", "C:\\Python\\python.exe|-c|print(1)", "/usr/local/bin/node|-e|1",
        "wsl|ls", "wsl", "wsl.exe|-e|ls", "WSL.EXE|--|ls", "C:\\Windows\\System32\\wsl.exe|ls", "/mnt/c/Windows/wsl|ls",
    }.Select(s => new object[] { s });

    public static IEnumerable<object[]> Accepted() => new[]
    {
        "pwsh|-File|x.ps1", "pwsh|-NoLogo|-File|C:\\s\\Start.ps1|-Name|a", "powershell|-File|x.ps1", "C:\\x\\pwsh.exe|-File|x.ps1", "pwsh",
        "bash|script.sh", "/usr/bin/bash|script.sh", "sh|run.sh|arg", "python|-m|http.server", "python3|script.py", "node|app.js", "ruby|a.rb", "perl|a.pl",
        "claude|--resume|abc", "cmd|/?", "tether.exe|serve", "wslpath|x",
    }.Select(s => new object[] { s });

    [Theory]
    [MemberData(nameof(InlineCases))]
    public void An_interpreter_with_an_inline_code_flag_is_refused_with_exit_5_and_nothing_is_created(string argv)
    {
        var before = CrH.Snap(w);
        var r = w.Call(w.Manager, "session start", CrH.A(CrH.Argv(argv), ("restart", true)));
        Assert.Equal(5, r.Exit);
        Assert.Equal("refused", CrH.Kind(r));
        Assert.Equal(before, CrH.Snap(w));
    }

    [Theory]
    [MemberData(nameof(Variants))]
    public void Path_forms_case_differences_and_wsl_are_refused(string argv)
    {
        var before = CrH.Snap(w);
        var r = w.Call(w.Manager, "session start", CrH.A(CrH.Argv(argv), ("restart", true)));
        Assert.Equal(5, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Theory]
    [InlineData("pwsh|-Command|x")]
    [InlineData("bash|-lc|x")]
    [InlineData("/usr/bin/bash|-c|x")]
    [InlineData("C:\\x\\PWSH.EXE|-EC|x")]
    [InlineData("python3|-c|1")]
    [InlineData("wsl|ls")]
    public void Run_refuses_the_same_commands_with_restart(string argv)
    {
        var before = CrH.Snap(w);
        var r = w.Call(w.Manager, "run", CrH.A(CrH.Argv(argv), ("restart", true)));
        Assert.Equal(5, r.Exit);
        Assert.Equal(before, CrH.Snap(w));
    }

    [Theory]
    [InlineData("pwsh|-Command|x")]
    [InlineData("bash|-c|x")]
    [InlineData("wsl|ls")]
    [InlineData("claude|--session-id|abc")]
    public void Without_restart_the_same_commands_are_accepted(string argv)
    {
        var r = w.Call(w.Manager, "session start", CrH.A(CrH.Argv(argv)));
        Assert.Equal(0, r.Exit);
        Assert.False(w.Host.GetBlock(CrH.Str(r, "blockid")).Restart);
        Assert.Equal(0, w.Call(w.Manager, "run", CrH.A(CrH.Argv(argv))).Exit);
    }

    [Theory]
    [InlineData("claude|--session-id|abc")]
    [InlineData("claude|--session-id=abc")]
    [InlineData("pwsh|-File|x.ps1|--session-id|abc")]
    [InlineData("tool|x--session-idx")]
    public void An_argv_containing_session_id_is_refused_with_restart(string argv)
    {
        foreach (var verb in new[] { "session start", "run" })
        {
            var before = CrH.Snap(w);
            var r = w.Call(w.Manager, verb, CrH.A(CrH.Argv(argv), ("restart", true)));
            Assert.Equal(5, r.Exit);
            Assert.Contains("--session-id", World.ErrMessage(r));
            Assert.Equal(before, CrH.Snap(w));
        }
    }

    [Theory]
    [MemberData(nameof(Accepted))]
    public void Accepted_forms_start_and_the_block_restarts(string argv)
    {
        var r = w.Call(w.Manager, "session start", CrH.A(CrH.Argv(argv), ("restart", true)));
        Assert.Equal(0, r.Exit);
        Assert.True(w.Host.GetBlock(CrH.Str(r, "blockid")).Restart);
        var r2 = w.Call(w.Manager, "run", CrH.A(CrH.Argv(argv), ("restart", true)));
        Assert.Equal(0, r2.Exit);
        Assert.True(w.Host.GetBlock(CrH.Str(r2, "blockid")).Restart);
    }

    [Fact]
    public void A_refused_restart_is_exit_5_even_when_other_checks_would_also_fail_later()
    {
        // 4.1 order: usage (2) first, then restart (5): a bad --cwd alongside an inline shell is a usage error.
        var r = w.Call(w.Manager, "session start", CrH.A(new[] { "pwsh", "-Command", "x" }, ("restart", true), ("cwd", "relative")));
        Assert.Equal(2, r.Exit);
        var r2 = w.Call(w.Manager, "session start", CrH.A(new[] { "pwsh", "-Command", "x" }, ("restart", true)));
        Assert.Equal(5, r2.Exit);
    }

    [Theory]
    [InlineData("pwsh|-Command|x")]
    [InlineData("bash|-c|x")]
    [InlineData("wsl|ls")]
    [InlineData("claude|--session-id|abc")]
    public void The_manager_config_entry_is_not_subject_to_the_check(string argv)
    {
        var cfg = CrH.Parse(CrH.Cfg(x => x["argv"] = new JsonArray(CrH.Argv(argv).Select(a => (JsonNode?)a).ToArray())));
        var b = w.Host.ApplyManagerConfig(w.Ws, cfg);
        Assert.True(b.Restart);
        Assert.Equal(CrH.Argv(argv), b.Argv);
    }
}
