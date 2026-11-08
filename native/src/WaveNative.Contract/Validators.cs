// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Globalization;
using System.Text;
using System.Text.RegularExpressions;

namespace WaveNative.Contract;

// The input rules of cli-contract.md sections 4 and 6. A failed rule is exit 2 (usage) and changes nothing.
public static partial class Validators
{
    public static readonly HashSet<string> CssColorNames = new(StringComparer.OrdinalIgnoreCase)
    {
        "aliceblue","antiquewhite","aqua","aquamarine","azure","beige","bisque","black","blanchedalmond","blue","blueviolet","brown","burlywood","cadetblue","chartreuse",
        "chocolate","coral","cornflowerblue","cornsilk","crimson","cyan","darkblue","darkcyan","darkgoldenrod","darkgray","darkgreen","darkgrey","darkkhaki","darkmagenta",
        "darkolivegreen","darkorange","darkorchid","darkred","darksalmon","darkseagreen","darkslateblue","darkslategray","darkslategrey","darkturquoise","darkviolet",
        "deeppink","deepskyblue","dimgray","dimgrey","dodgerblue","firebrick","floralwhite","forestgreen","fuchsia","gainsboro","ghostwhite","gold","goldenrod","gray","green",
        "greenyellow","grey","honeydew","hotpink","indianred","indigo","ivory","khaki","lavender","lavenderblush","lawngreen","lemonchiffon","lightblue","lightcoral","lightcyan",
        "lightgoldenrodyellow","lightgray","lightgreen","lightgrey","lightpink","lightsalmon","lightseagreen","lightskyblue","lightslategray","lightslategrey","lightsteelblue",
        "lightyellow","lime","limegreen","linen","magenta","maroon","mediumaquamarine","mediumblue","mediumorchid","mediumpurple","mediumseagreen","mediumslateblue",
        "mediumspringgreen","mediumturquoise","mediumvioletred","midnightblue","mintcream","mistyrose","moccasin","navajowhite","navy","oldlace","olive","olivedrab","orange",
        "orangered","orchid","palegoldenrod","palegreen","paleturquoise","palevioletred","papayawhip","peachpuff","peru","pink","plum","powderblue","purple","rebeccapurple","red",
        "rosybrown","royalblue","saddlebrown","salmon","sandybrown","seagreen","seashell","sienna","silver","skyblue","slateblue","slategray","slategrey","snow","springgreen",
        "steelblue","tan","teal","thistle","tomato","turquoise","violet","wheat","white","whitesmoke","yellow","yellowgreen",
    };

    [GeneratedRegex("^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})\\z")] private static partial Regex HexColorRe();
    [GeneratedRegex("^[a-z0-9-]{1,40}\\z")] private static partial Regex IconNameRe();
    [GeneratedRegex("^session:[A-Za-z0-9._-]+\\z")] private static partial Regex SessionKeyRe();
    [GeneratedRegex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\\z")] private static partial Regex UuidRe();
    [GeneratedRegex("^[A-Za-z_][A-Za-z0-9_]*\\z")] private static partial Regex EnvKeyRe();

    public static bool IsUuid(string s) => UuidRe().IsMatch(s);

    static void NoRefused(string what, string value)
    {
        if (TextSafety.IsRefused(value, out var cp)) throw CliError.Usage($"{what} contains a character that is not allowed (U+{cp:X4})");
    }

    public static string Name(string value, string what = "name")
    {
        if (value.Length > 60) throw CliError.Usage($"{what} is at most 60 characters");
        NoRefused(what, value);
        return value;
    }

    public static string Title(string value)
    {
        if (TextSafety.CodePointCount(value) > 200) throw CliError.Usage("title is at most 200 code points");
        NoRefused("title", value);
        return value;
    }

    // Colour name (stored lower-case) or #RRGGBB / #RRGGBBAA.
    public static string Color(string value, string what = "color")
    {
        NoRefused(what, value);
        var v = value.Trim();
        if (HexColorRe().IsMatch(v)) return v.ToLowerInvariant();
        if (v.StartsWith('#')) throw CliError.Usage($"{what} must be #RRGGBB or #RRGGBBAA");
        if (CssColorNames.Contains(v)) return v.ToLowerInvariant();
        throw CliError.Usage($"invalid {what} \"{TextSafety.Escape(v, false)}\": use a CSS colour name or #RRGGBB or #RRGGBBAA");
    }

    // A Font Awesome name (optional solid@ regular@ brands@) or at most two graphemes / 32 bytes of text without <>&"' backtick backslash.
    public static string Symbol(string value)
    {
        NoRefused("symbol", value);
        var v = value.Trim();
        if (IconNameRe().IsMatch(v)) return v;
        if (v.Length > 0 && v.All(c => char.IsAsciiLetterLower(c) || char.IsAsciiDigit(c) || c is '-' or '@'))
            throw CliError.Usage("icon names are lowercase letters, digits and dashes, at most 40 characters");
        if (Encoding.UTF8.GetByteCount(v) > 32) throw CliError.Usage("a symbol is at most 32 bytes");
        if (new StringInfo(v).LengthInTextElements > 2) throw CliError.Usage("a symbol is at most 2 characters");
        if (v.IndexOfAny(new[] { '<', '>', '&', '"', '\'', '`', '\\' }) >= 0) throw CliError.Usage("a symbol cannot contain <>&\"' backtick or backslash");
        if (v.Length == 0) throw CliError.Usage("symbol is empty");
        return v;
    }

    public static string SessionMetaKey(string key)
    {
        if (!SessionKeyRe().IsMatch(key)) throw CliError.Usage($"invalid meta key \"{TextSafety.Escape(key, false)}\": want session:KEY with KEY of letters, digits, . _ -");
        return key;
    }

    public static string SessionMetaValue(string key, string value)
    {
        if (TextSafety.CodePointCount(value) > 200) throw CliError.Usage($"the value of {key} is at most 200 code points");
        NoRefused($"the value of {key}", value);
        return value;
    }

    // "session:KEY=VALUE" pairs of --meta: no key twice.
    public static Dictionary<string, string> SessionMetaPairs(IEnumerable<string> pairs)
    {
        var d = new Dictionary<string, string>();
        foreach (var p in pairs)
        {
            var i = p.IndexOf('=');
            if (i < 0) throw CliError.Usage($"invalid --meta \"{TextSafety.Escape(p, false)}\": want session:KEY=VALUE");
            var k = SessionMetaKey(p[..i]);
            var v = SessionMetaValue(k, p[(i + 1)..]);
            if (!d.TryAdd(k, v)) throw CliError.Usage($"--meta {k} given twice");
        }
        return d;
    }

    public static string EnvKey(string k)
    {
        if (!EnvKeyRe().IsMatch(k)) throw CliError.Usage($"invalid --env key \"{TextSafety.Escape(k, false)}\"");
        return k;
    }

    public static Dictionary<string, string> EnvPairs(IEnumerable<string> pairs)
    {
        var d = new Dictionary<string, string>();
        foreach (var p in pairs)
        {
            var i = p.IndexOf('=');
            if (i <= 0) throw CliError.Usage($"invalid --env \"{TextSafety.Escape(p, false)}\": want KEY=VALUE");
            var k = EnvKey(p[..i]);
            if (!d.TryAdd(k, p[(i + 1)..])) throw CliError.Usage($"--env {k} given twice");
        }
        return d;
    }

    public static string Cwd(string value, Func<string, bool> directoryExists)
    {
        bool absolute = Path.IsPathFullyQualified(value);
        if (!absolute) throw CliError.Usage("--cwd must be an absolute path");
        if (!directoryExists(value)) throw CliError.Usage("--cwd is not an existing directory");
        return value;
    }

    public static string Url(string value)
    {
        // Uri.TryCreate reads "//host/path" as a file: UNC address on Windows: the text itself must start with an allowed scheme.
        bool schemeText = value.StartsWith("http://", StringComparison.OrdinalIgnoreCase) || value.StartsWith("https://", StringComparison.OrdinalIgnoreCase) || value.StartsWith("file:", StringComparison.OrdinalIgnoreCase);
        if (!schemeText || !Uri.TryCreate(value, UriKind.Absolute, out var u) || u.Scheme is not ("http" or "https" or "file"))
            throw CliError.Usage("the address must be an http:, https: or file: URL");
        return value;
    }

    // Both sides of an equality test are reduced the same way: scheme and host lower-case, default port dropped, empty path = "/".
    public static string NormalizeUrl(string value)
    {
        var u = new Uri(value, UriKind.Absolute);
        if (u.Scheme == "file") return u.GetLeftPart(UriPartial.Path) + u.Query + u.Fragment;
        var sb = new StringBuilder();
        sb.Append(u.Scheme).Append("://").Append(u.Host.ToLowerInvariant());
        if (!u.IsDefaultPort && u.Port > 0) sb.Append(':').Append(u.Port);
        sb.Append(string.IsNullOrEmpty(u.AbsolutePath) ? "/" : u.AbsolutePath).Append(u.Query).Append(u.Fragment);
        return sb.ToString();
    }

    // Printed form: no user info, query or fragment (the home tile's entry token lives in the query).
    public static string RedactUrl(string value)
    {
        if (!Uri.TryCreate(value, UriKind.Absolute, out var u)) return "";
        if (u.Scheme == "file") return TextSafety.Escape(u.GetLeftPart(UriPartial.Path), false);
        var sb = new StringBuilder();
        sb.Append(u.Scheme).Append("://").Append(u.Host);
        if (!u.IsDefaultPort && u.Port > 0) sb.Append(':').Append(u.Port);
        sb.Append(u.AbsolutePath);
        return TextSafety.Escape(sb.ToString(), false);
    }

    public static string Selector(string value)
    {
        if (TextSafety.CodePointCount(value) > 512) throw CliError.Usage("--selector is at most 512 characters");
        foreach (var r in value.EnumerateRunes())
            if (Rune.IsControl(r)) throw CliError.Usage("--selector cannot contain control characters");
        return value;
    }

    // The inline-code table of the Go tool (cmd/wsh/cmd/wshcmd-session.go:171-187), copied as is.
    static readonly Dictionary<string, string[]> InlineCodeFlags = new()
    {
        ["pwsh"] = new[] { "-command", "-c", "-encodedcommand", "-ec", "-e", "-commandwithargs", "-cwa" },
        ["powershell"] = new[] { "-command", "-c", "-encodedcommand", "-ec", "-e", "-commandwithargs", "-cwa" },
        ["cmd"] = new[] { "/c", "/k" },
        ["bash"] = new[] { "-c", "-lc", "-ic", "-cl" },
        ["sh"] = new[] { "-c", "-lc", "-ic", "-cl" },
        ["zsh"] = new[] { "-c", "-lc", "-ic", "-cl" },
        ["fish"] = new[] { "-c", "-lc", "-ic", "-cl" },
        ["dash"] = new[] { "-c" },
        ["nu"] = new[] { "-c", "--commands" },
        ["node"] = new[] { "-e", "--eval", "-p", "--print" },
        ["python"] = new[] { "-c" },
        ["python3"] = new[] { "-c" },
        ["py"] = new[] { "-c" },
        ["ruby"] = new[] { "-e" },
        ["perl"] = new[] { "-e", "-E" },
    };

    // --restart refusals: exit 5. Text matching only, a best effort (cli-contract.md 4.1).
    public static void CheckRestartArgv(IReadOnlyList<string> argv)
    {
        if (argv.Any(a => a.Contains("--session-id", StringComparison.Ordinal)))
            throw CliError.Refused("--restart cannot be used with --session-id in the command: the second start must resume the session (--resume), not create it again; use a starter script that makes that choice").WithReason("restart");
        var cmd = argv[0].ToLowerInvariant();
        var slash = cmd.LastIndexOfAny(new[] { '/', '\\' });
        if (slash >= 0) cmd = cmd[(slash + 1)..];
        if (cmd.EndsWith(".exe", StringComparison.Ordinal)) cmd = cmd[..^4];
        bool inline = cmd == "wsl" || (InlineCodeFlags.TryGetValue(cmd, out var flags) && argv.Skip(1).Any(a => flags.Contains(a.ToLowerInvariant())));
        if (inline)
            throw CliError.Refused($"--restart cannot be used with an inline shell command ({TextSafety.Escape(argv[0], false)} ...): start a script file instead").WithReason("restart");
    }

    // The meta keys of section 6 that `meta set` may write on a block, with their rules. Returns the value to store.
    public static string SettableMetaValue(string key, string value, string view)
    {
        switch (key)
        {
            case "url":
                if (view != "web") throw CliError.Usage("url can be set on a web block only");
                return Url(value);
            case "frame:title": return Title(value);
            case "frame:icon": return Symbol(value);
            case "icon:color":
            case "frame:bordercolor": return Color(value, key);
        }
        if (key.StartsWith("session:", StringComparison.Ordinal))
        {
            SessionMetaKey(key);
            return SessionMetaValue(key, value);
        }
        throw CliError.Usage(KnownUnsettable.Contains(key) ? $"the key {key} cannot be set with meta set" : $"unknown meta key {TextSafety.Escape(key, false)}");
    }

    public static bool IsSettableMetaKey(string key) =>
        key is "url" or "frame:title" or "frame:icon" or "icon:color" or "frame:bordercolor" || SessionKeyRe().IsMatch(key);

    static readonly HashSet<string> KnownUnsettable = new()
    {
        "view", "cmd", "cmd:args", "cmd:cwd", "cmd:runonstart", "cmd:runonce", "cmd:clearonstart", "cmd:env", "web:partition",
        "attention:level", "attention:reason", "tab:flagcolor", "tab:accent", "tab:symbol", "ws:accent",
    };
}
