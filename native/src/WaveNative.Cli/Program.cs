// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.IO.Pipes;
using System.Text;
using System.Text.Json.Nodes;
using WaveNative.Contract;

namespace WaveNative.Cli;

static class Program
{
    static int Main(string[] args)
    {
        var stdout = Console.OpenStandardOutput();
        var stderr = Console.OpenStandardError();
        try
        {
            var req = ArgParser.Parse(args);
            req.Token = Environment.GetEnvironmentVariable("WAVE_TOKEN");
            if (string.IsNullOrEmpty(req.Token)) return Fail(stderr, Exit.Denied, "no token: run this inside a Wave block (WAVE_TOKEN is not set)");
            var pipe = Environment.GetEnvironmentVariable("WAVE_PIPE");
            if (string.IsNullOrEmpty(pipe)) return Fail(stderr, Exit.Unreachable, "WAVE_PIPE is not set");
            Reply reply;
            try
            {
                using var client = new NamedPipeClientStream(".", pipe, PipeDirection.InOut);
                client.Connect(5000);
                var bytes = new UTF8Encoding(false).GetBytes(req.ToLine() + "\n");
                client.Write(bytes, 0, bytes.Length);
                client.Flush();
                using var reader = new StreamReader(client, new UTF8Encoding(false));
                var line = reader.ReadLine() ?? throw new IOException("the app closed the connection without an answer");
                reply = Reply.Parse(line);
            }
            catch (Exception e) when (e is TimeoutException or IOException or UnauthorizedAccessException)
            {
                return Fail(stderr, Exit.Unreachable, "the app is not reachable: " + e.Message);
            }
            if (reply.Out != null) Write(stdout, reply.Out.ToJsonString());
            if (reply.Err != null) Write(stderr, reply.Err.ToJsonString());
            return reply.Exit;
        }
        catch (CliError e)
        {
            return Fail(stderr, e.Code, e.Message);
        }
        catch (Exception e) when (e is not OutOfMemoryException)
        {
            return Fail(stderr, Exit.Internal, "internal error: " + e.GetType().Name);
        }
    }

    static int Fail(Stream err, int code, string message)
    {
        Write(err, Reply.Failure(code, message).Err!.ToJsonString());
        return code;
    }

    static void Write(Stream s, string text)
    {
        var b = new UTF8Encoding(false).GetBytes(text + "\n");
        s.Write(b, 0, b.Length);
        s.Flush();
    }
}

// The argument grammar of cli-contract.md section 4: which words make a verb, which positionals and flags it takes. Unknown flags and missing
// positionals are exit 2 before anything is sent.
public static class ArgParser
{
    enum Kind { Str, Bool, Int, Repeat }

    sealed record Spec(string[] Positionals, string? Rest, Dictionary<string, Kind> Flags, string? ArgvAfterDashDash = null);

    static Dictionary<string, Kind> F(params (string, Kind)[] flags) => flags.ToDictionary(f => f.Item1, f => f.Item2);

    static readonly Dictionary<string, Spec> Verbs = new()
    {
        ["whoami"] = new(Array.Empty<string>(), null, F()),
        ["session start"] = new(Array.Empty<string>(), null, F(("name", Kind.Str), ("cwd", Kind.Str), ("args-file", Kind.Str), ("color", Kind.Str), ("symbol", Kind.Str), ("stack", Kind.Str),
            ("activate", Kind.Bool), ("restart", Kind.Bool), ("meta", Kind.Repeat), ("env", Kind.Repeat), ("cap", Kind.Repeat)), "argv"),
        ["session list"] = new(Array.Empty<string>(), null, F(("all", Kind.Bool))),
        ["session close"] = new(new[] { "tab" }, null, F(("force", Kind.Bool))),
        ["tab new"] = new(Array.Empty<string>(), null, F(("name", Kind.Str), ("activate", Kind.Bool))),
        ["tab rename"] = new(new[] { "tab", "name" }, null, F()),
        ["tab move"] = new(new[] { "tab" }, null, F(("index", Kind.Int))),
        ["tab style"] = new(Array.Empty<string>(), null, F(("tab", Kind.Str), ("color", Kind.Str), ("accent", Kind.Str), ("symbol", Kind.Str), ("clear", Kind.Bool))),
        ["tab switch"] = new(new[] { "tab" }, null, F()),
        ["workspace style"] = new(Array.Empty<string>(), null, F(("accent", Kind.Str), ("clear", Kind.Bool))),
        ["block list"] = new(Array.Empty<string>(), null, F(("tab", Kind.Str), ("view", Kind.Str), ("url", Kind.Str), ("all", Kind.Bool))),
        ["block delete"] = new(new[] { "block" }, null, F(("force", Kind.Bool))),
        ["block shot"] = new(new[] { "block" }, null, F()),
        ["meta get"] = new(Array.Empty<string>(), "items", F()),
        ["meta set"] = new(new[] { "block" }, "pairs", F(("json", Kind.Str), ("json-file", Kind.Str))),
        ["meta unset"] = new(new[] { "block" }, "keys", F()),
        ["run"] = new(Array.Empty<string>(), null, F(("tab", Kind.Str), ("stack", Kind.Str), ("cwd", Kind.Str), ("env", Kind.Repeat), ("restart", Kind.Bool), ("meta", Kind.Repeat),
            ("cap", Kind.Repeat), ("title", Kind.Str), ("focus", Kind.Bool)), "argv"),
        ["group add"] = new(new[] { "block" }, null, F(("to", Kind.Str), ("focus", Kind.Bool))),
        ["group remove"] = new(new[] { "block" }, null, F(("focus", Kind.Bool))),
        ["group move"] = new(Array.Empty<string>(), null, F(("from", Kind.Str), ("to", Kind.Str), ("focus", Kind.Bool))),
        ["group activate"] = new(new[] { "block" }, null, F()),
        ["web open"] = new(new[] { "url" }, null, F(("tab", Kind.Str), ("stack", Kind.Str), ("title", Kind.Str), ("focus", Kind.Bool))),
        ["web text"] = new(new[] { "block" }, null, F(("selector", Kind.Str), ("max-bytes", Kind.Int))),
        ["send"] = new(new[] { "block" }, "tokens", F(("delay", Kind.Int), ("stdin", Kind.Bool), ("raw", Kind.Bool))),
        ["screen"] = new(new[] { "block" }, null, F(("last", Kind.Int), ("max-bytes", Kind.Int))),
        ["attention"] = new(new[] { "level" }, null, F(("reason", Kind.Str), ("block", Kind.Str), ("only-if-level", Kind.Str), ("only-if-reason", Kind.Str))),
        ["badge"] = new(Array.Empty<string>(), "icon-list", F(("color", Kind.Str), ("priority", Kind.Str), ("block", Kind.Str), ("tab", Kind.Str), ("clear", Kind.Bool))),
    };

    static readonly HashSet<string> TwoWord = new() { "session", "tab", "workspace", "block", "meta", "group", "web" };

    static string Camel(string kebab)
    {
        var parts = kebab.Split('-');
        return parts[0] + string.Concat(parts.Skip(1).Select(p => char.ToUpperInvariant(p[0]) + p[1..]));
    }

    public static Request Parse(string[] args)
    {
        if (args.Length == 0) throw CliError.Usage("no verb given");
        string verb;
        int next;
        if (TwoWord.Contains(args[0]))
        {
            if (args.Length < 2) throw CliError.Usage($"{args[0]} needs a verb");
            verb = args[0] + " " + args[1];
            next = 2;
        }
        else
        {
            verb = args[0];
            next = 1;
        }
        if (!Verbs.TryGetValue(verb, out var spec)) throw CliError.Usage($"unknown verb \"{verb}\"");
        var o = new JsonObject();
        var positionals = new List<string>();
        var rest = new List<string>();
        var repeats = new Dictionary<string, JsonArray>();
        for (int i = next; i < args.Length; i++)
        {
            var a = args[i];
            if (a == "--")
            {
                if (spec.ArgvAfterDashDash == null) throw CliError.Usage("-- is not used by this verb");
                var argv = new JsonArray();
                foreach (var x in args.Skip(i + 1)) argv.Add(x);
                o[spec.ArgvAfterDashDash] = argv;
                break;
            }
            if (a.StartsWith("--", StringComparison.Ordinal) && a.Length > 2)
            {
                ApplyFlag(verb, spec, args, ref i, o, repeats);
                continue;
            }
            if (spec.Rest != null && positionals.Count >= spec.Positionals.Length) rest.Add(a);
            else positionals.Add(a);
        }
        foreach (var kv in repeats) o[kv.Key] = kv.Value;
        for (int i = 0; i < spec.Positionals.Length; i++)
        {
            if (i < positionals.Count) o[Camel(spec.Positionals[i])] = positionals[i];
            else if (verb is not ("meta get" or "badge")) throw CliError.Usage($"{verb} needs {spec.Positionals[i].ToUpperInvariant()}");
        }
        if (positionals.Count > spec.Positionals.Length)
        {
            if (spec.Rest == null) throw CliError.Usage($"too many arguments for {verb}");
        }
        if (spec.Rest != null && verb != "badge") o[Camel(spec.Rest)] = new JsonArray(rest.Select(x => (JsonNode?)x).ToArray());
        if (verb == "badge" && (rest.Count > 0 || positionals.Count > 0))
        {
            var all = positionals.Concat(rest).ToList();
            if (all.Count > 1) throw CliError.Usage("badge takes at most one ICON");
            o["icon"] = all[0];
        }
        if (verb == "meta get" && positionals.Count > 0) o["items"] = new JsonArray(positionals.Concat(rest).Select(x => (JsonNode?)x).ToArray());
        PostProcess(verb, spec, o, args);
        return new Request { Verb = verb, Args = o };
    }

    static void ApplyFlag(string verb, Spec spec, string[] args, ref int i, JsonObject o, Dictionary<string, JsonArray> repeats)
    {
        var a = args[i];
        var eq = a.IndexOf('=');
        var name = eq > 0 ? a[2..eq] : a[2..];
        if (!spec.Flags.TryGetValue(name, out var kind)) throw CliError.Usage($"unknown flag --{name} for {verb}");
        string? value = eq > 0 ? a[(eq + 1)..] : null;
        if (kind != Kind.Bool && value == null)
        {
            if (i + 1 >= args.Length) throw CliError.Usage($"--{name} needs a value");
            value = args[++i];
        }
        var key = Camel(name);
        switch (kind)
        {
            case Kind.Bool:
                if (o.ContainsKey(key)) throw CliError.Usage($"--{name} given twice");
                if (value != null && !bool.TryParse(value, out _)) throw CliError.Usage($"--{name} takes no value");
                o[key] = value == null || bool.Parse(value);
                break;
            case Kind.Int:
                if (o.ContainsKey(key)) throw CliError.Usage($"--{name} given twice");
                if (!int.TryParse(value, out var n)) throw CliError.Usage($"--{name} must be a whole number");
                o[key] = n;
                break;
            case Kind.Repeat:
                if (!repeats.TryGetValue(key, out var arr)) repeats[key] = arr = new JsonArray();
                arr.Add(value);
                break;
            default:
                if (o.ContainsKey(key)) throw CliError.Usage($"--{name} given twice");
                o[key] = value;
                break;
        }
    }

    static string ReadStdin()
    {
        using var reader = new StreamReader(Console.OpenStandardInput(), new UTF8Encoding(false), detectEncodingFromByteOrderMarks: true);
        return reader.ReadToEnd().TrimStart('\uFEFF');
    }

    // Things only the client can do: read a file or stdin.
    static void PostProcess(string verb, Spec spec, JsonObject o, string[] args)
    {
        if (verb == "session start")
        {
            bool dashDash = o.ContainsKey("argv");
            if (o["argsFile"] is JsonValue f)
            {
                if (dashDash) throw CliError.Usage("give the command after -- or in --args-file, not both");
                string text;
                try { text = File.ReadAllText(f.GetValue<string>(), new UTF8Encoding(false)).TrimStart('\uFEFF'); }
                catch (Exception e) when (e is IOException or UnauthorizedAccessException or ArgumentException or NotSupportedException) { throw CliError.Usage("reading --args-file: " + e.Message); }
                try
                {
                    var arr = JsonNode.Parse(text) as JsonArray ?? throw CliError.Usage("--args-file must be a JSON array of strings");
                    foreach (var x in arr) if (x is not JsonValue v || !v.TryGetValue<string>(out _)) throw CliError.Usage("--args-file must be a JSON array of strings");
                    o["argv"] = arr.DeepClone();
                }
                catch (System.Text.Json.JsonException) { throw CliError.Usage("--args-file must be a JSON array of strings"); }
                o.Remove("argsFile");
            }
        }
        if (verb == "meta set" && o["jsonFile"] is JsonValue jf)
        {
            if (o.ContainsKey("json")) throw CliError.Usage("give --json or --json-file, not both");
            var path = jf.GetValue<string>();
            try
            {
                o["json"] = path == "-" ? ReadStdin() : File.ReadAllText(path, new UTF8Encoding(false)).TrimStart('\uFEFF');
            }
            catch (Exception e) when (e is IOException or UnauthorizedAccessException or ArgumentException or NotSupportedException) { throw CliError.Usage("reading --json-file: " + e.Message); }
            o.Remove("jsonFile");
        }
        if (verb == "send" && o["stdin"] is JsonValue sv && sv.GetValue<bool>())
        {
            var text = ReadStdin().TrimEnd('\r', '\n');
            var tokens = o["tokens"] as JsonArray ?? new JsonArray();
            tokens.Add("text:" + text);
            o["tokens"] = tokens;
            o.Remove("stdin");
        }
    }
}
