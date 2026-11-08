// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;
using Xunit;

namespace WaveNative.Contract.Tests;


// ---------------------------------------------------------------------------------------------------------------------------------
// (e) block shot
// ---------------------------------------------------------------------------------------------------------------------------------
public sealed class CrShotTests : CrBase
{
    string Shots => Path.Combine(w.DataDir, "shots");

    static readonly byte[] PngSignature = { 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A };

    string Shot(Block caller, string blockId)
    {
        var r = w.Call(caller, "block shot", ("block", blockId));
        Assert.Equal(0, r.Exit);
        return CrH.Str(r, "path");
    }

    [Fact]
    public void A_shot_is_a_png_file_under_the_data_dir_shots_folder()
    {
        var path = Shot(w.Manager, w.S1.Id);
        Assert.True(File.Exists(path));
        Assert.Equal(Path.GetFullPath(Shots), Path.GetFullPath(Path.GetDirectoryName(path)!));
        Assert.EndsWith(".png", path);
        Assert.Equal(PngSignature, File.ReadAllBytes(path).Take(8).ToArray());
    }

    [Fact]
    public void The_width_and_height_fields_match_the_png_header()
    {
        var r = w.Call(w.Manager, "block shot", ("block", w.S1.Id));
        var o = World.Out(r).AsObject();
        Assert.Equal(new[] { "height", "path", "width" }, o.Select(kv => kv.Key).OrderBy(x => x).ToArray());
        var bytes = File.ReadAllBytes(o["path"]!.GetValue<string>());
        int width = (bytes[16] << 24) | (bytes[17] << 16) | (bytes[18] << 8) | bytes[19];
        int height = (bytes[20] << 24) | (bytes[21] << 16) | (bytes[22] << 8) | bytes[23];
        Assert.True(width > 0);
        Assert.True(height > 0);
        Assert.Equal(width, o["width"]!.GetValue<int>());
        Assert.Equal(height, o["height"]!.GetValue<int>());
    }

    [Fact]
    public void Two_shots_are_two_different_files()
    {
        var a = Shot(w.Manager, w.S1.Id);
        var b = Shot(w.Manager, w.S1.Id);
        Assert.NotEqual(a, b);
        Assert.True(File.Exists(a));
        Assert.True(File.Exists(b));
    }

    [Fact]
    public void A_session_may_shoot_its_own_block_and_one_it_created()
    {
        Assert.True(File.Exists(Shot(w.S1, "this")));
        var id = CrH.Str(w.Call(w.S1, "session start", CrH.A(new[] { "pwsh", "-File", "x.ps1" })), "blockid");
        Assert.True(File.Exists(Shot(w.S1, id)));
    }

    [Fact]
    public void A_web_block_can_be_shot()
    {
        var web = w.Host.StartWebBlockAsApp(w.Projects, "https://example.com/");
        Assert.True(File.Exists(Shot(w.Manager, web.Id)));
    }

    [Fact]
    public void Another_sessions_block_is_exit_4_and_writes_no_file()
    {
        var r = w.Call(w.S1, "block shot", ("block", w.S2.Id));
        Assert.Equal(4, r.Exit);
        Assert.Contains("not available to this caller", World.ErrMessage(r));
        Assert.False(Directory.Exists(Shots) && Directory.GetFiles(Shots).Length > 0);
    }

    [Fact]
    public void An_unknown_block_gives_the_same_exit_and_message_as_a_forbidden_one()
    {
        var forbidden = w.Call(w.S1, "block shot", ("block", w.S2.Id));
        var unknown = w.Call(w.S1, "block shot", ("block", Guid.NewGuid().ToString()));
        Assert.Equal(4, unknown.Exit);
        Assert.Equal(forbidden.Exit, unknown.Exit);
    }

    [Fact]
    public void Pruning_keeps_the_newest_50_and_nothing_is_deleted_before_the_51st()
    {
        var paths = new List<string>();
        for (int i = 0; i < 50; i++) paths.Add(Shot(w.Manager, w.S1.Id));
        Assert.Equal(50, Directory.GetFiles(Shots, "shot-*.png").Length);
        Assert.All(paths, p => Assert.True(File.Exists(p)));
        paths.Add(Shot(w.Manager, w.S1.Id));
        Assert.Equal(50, Directory.GetFiles(Shots, "shot-*.png").Length);
        Assert.False(File.Exists(paths[0]));
        Assert.True(File.Exists(paths[1]));
        Assert.True(File.Exists(paths[50]));
    }

    [Fact]
    public void Pruning_after_many_shots_leaves_exactly_the_newest_50()
    {
        var paths = new List<string>();
        for (int i = 0; i < 60; i++) paths.Add(Shot(w.Manager, w.S1.Id));
        var left = Directory.GetFiles(Shots, "shot-*.png").OrderBy(x => x, StringComparer.Ordinal).ToArray();
        Assert.Equal(50, left.Length);
        Assert.Equal(paths.Skip(10).OrderBy(x => x, StringComparer.Ordinal).ToArray(), left);
        Assert.All(paths.Take(10), p => Assert.False(File.Exists(p)));
    }

    [Fact]
    public void Pruning_deletes_only_its_own_shot_files_and_never_a_subfolder()
    {
        Directory.CreateDirectory(Shots);
        var sub = Path.Combine(Shots, "archive");
        Directory.CreateDirectory(sub);
        var foreign = new[]
        {
            Path.Combine(Shots, "notes.txt"), Path.Combine(Shots, "photo.png"), Path.Combine(Shots, "shot-notes.txt"), Path.Combine(Shots, "a-shot-1.png"),
            Path.Combine(sub, "shot-0001.png"), Path.Combine(sub, "keep.txt"),
        };
        foreach (var f in foreign) File.WriteAllText(f, "mine");
        for (int i = 0; i < 60; i++) Shot(w.Manager, w.S1.Id);
        Assert.All(foreign, f => Assert.True(File.Exists(f), f));
        Assert.True(Directory.Exists(sub));
        Assert.Equal(50, Directory.GetFiles(Shots, "shot-*.png").Length);
    }

    [Fact]
    public void Foreign_files_do_not_count_toward_the_50()
    {
        Directory.CreateDirectory(Shots);
        for (int i = 0; i < 5; i++) File.WriteAllText(Path.Combine(Shots, "photo" + i + ".png"), "x");
        var paths = new List<string>();
        for (int i = 0; i < 50; i++) paths.Add(Shot(w.Manager, w.S1.Id));
        Assert.All(paths, p => Assert.True(File.Exists(p)));
    }
}
