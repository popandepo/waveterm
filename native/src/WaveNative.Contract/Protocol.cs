// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text.Json;
using System.Text.Json.Nodes;

namespace WaveNative.Contract;

// Exit codes of cli-contract.md section 1.
public static class Exit
{
    public const int Ok = 0;
    public const int Internal = 1;
    public const int Usage = 2;
    public const int Denied = 3;
    public const int NotFound = 4;
    public const int Refused = 5;
    public const int Unreachable = 6;
    public const int Partial = 7;

    public static string Kind(int code) => code switch
    {
        Usage => "usage",
        Denied => "denied",
        NotFound => "notfound",
        Refused => "refused",
        Unreachable => "unreachable",
        Partial => "partial",
        _ => "internal",
    };
}

public sealed class CliError : Exception
{
    public int Code { get; }

    public CliError(int code, string message) : base(message) => Code = code;

    public static CliError Usage(string m) => new(Exit.Usage, m);
    public static CliError Denied(string m) => new(Exit.Denied, m);
    public static CliError NotFound(string m = "not found") => new(Exit.NotFound, m);
    public static CliError Refused(string m) => new(Exit.Refused, m);
}

// What wave.exe sends over the pipe: one JSON line.
public sealed class Request
{
    public string Verb { get; set; } = "";
    public JsonObject Args { get; set; } = new();
    public string? Token { get; set; }

    public string ToLine() => new JsonObject
    {
        ["verb"] = Verb,
        ["args"] = Args.DeepClone(),
        ["token"] = Token,
    }.ToJsonString();

    public static Request Parse(string line)
    {
        try
        {
            var o = JsonNode.Parse(line) as JsonObject ?? throw new JsonException("request is not an object");
            return new Request
            {
                Verb = o["verb"]?.GetValue<string>() ?? "",
                Args = o["args"] as JsonObject ?? new JsonObject(),
                Token = o["token"]?.GetValue<string>(),
            };
        }
        catch (Exception e) when (e is InvalidOperationException or FormatException or ArgumentException)
        {
            throw new JsonException("the request has the wrong shape");
        }
    }
}

// What the app answers: the exit code the CLI must use, the JSON for stdout and the JSON for stderr.
public sealed class Reply
{
    public int Exit { get; set; }
    public JsonNode? Out { get; set; }
    public JsonNode? Err { get; set; }

    public string ToLine() => new JsonObject { ["exit"] = Exit, ["out"] = Out?.DeepClone(), ["err"] = Err?.DeepClone() }.ToJsonString();

    public static Reply Parse(string line)
    {
        try
        {
            var o = JsonNode.Parse(line) as JsonObject ?? throw new JsonException("reply is not an object");
            return new Reply { Exit = o["exit"]?.GetValue<int>() ?? 1, Out = o["out"]?.DeepClone(), Err = o["err"]?.DeepClone() };
        }
        catch (Exception e) when (e is InvalidOperationException or FormatException or ArgumentException)
        {
            throw new JsonException("the reply has the wrong shape");
        }
    }

    public static Reply Ok(JsonNode body) => new() { Exit = Contract.Exit.Ok, Out = body };

    public static Reply Failure(int code, string message, JsonNode? out_ = null) => new()
    {
        Exit = code,
        Out = out_,
        Err = new JsonObject { ["error"] = new JsonObject { ["code"] = code, ["kind"] = Contract.Exit.Kind(code), ["message"] = TextSafety.Escape(message, keepNewlineAndTab: false) } },
    };
}
