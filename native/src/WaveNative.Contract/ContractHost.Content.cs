// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Text;
using System.Text.Json.Nodes;

namespace WaveNative.Contract;

public sealed partial class ContractHost
{
    // ---- web ----

    JsonObject WebOpen(Block caller, JsonObject a)
    {
        var url = Validators.Url(S(a, "url") ?? throw CliError.Usage("an address is required"));
        var title = S(a, "title") is string ti ? Validators.Title(ti) : null;
        bool focus = B(a, "focus");
        if (focus) RequireCap(caller, Caps.Focus, "--focus");
        var stackArg = S(a, "stack");
        var stackTarget = stackArg != null ? ResolveBlock(caller, stackArg) : null;
        var tab = ResolveTab(caller, S(a, "tab") ?? "tab");
        if (stackTarget != null)
        {
            if (a["tab"] != null && stackTarget.TabId != tab.Id) throw CliError.Refused("the stack target is in another tab than --tab");
            tab = tabs[stackTarget.TabId];
        }
        if (stackTarget == null) RequireOwnTabForTile(caller, tab);
        var b = new Block { TabId = tab.Id, WorkspaceId = tab.WorkspaceId, View = "web", Url = url, CreatedBy = caller.Id, Status = Status.None, Title = title };
        b.Partition = "persist:" + b.Id;
        Register(b);
        var result = new JsonObject { ["tabid"] = tab.Id, ["blockid"] = b.Id, ["status"] = "ok" };
        if (stackTarget != null)
        {
            AddMember(tab, TileOf(tab, stackTarget.Id)!, b, focus);
            result["stackedwith"] = stackTarget.Id;
        }
        else AddNewTile(tab, b);
        if (Faults.FailAfterCreate)
        {
            result["status"] = "partial";
            result["warning"] = "the tile exists, but a later step failed";
            throw new PartialSuccess("the block was created but a later step failed", result);
        }
        return result;
    }

    JsonObject WebText(Block caller, JsonObject a)
    {
        var selector = S(a, "selector");
        if (selector != null) Validators.Selector(selector);
        int max = I(a, "maxBytes") ?? 262144;
        if (max < 1 || max > 262144) throw CliError.Usage("--max-bytes must be between 1 and 262144");
        var b = ResolveBlock(caller, S(a, "block"), ContentDenied);
        RequireContentAccess(caller, b);
        if (b.View != "web") throw CliError.Refused("the block is not a web tile");
        if (!b.Page.TryGetValue(selector ?? "", out var raw)) throw CliError.Refused("nothing on the page matches the selector");
        var escaped = TextSafety.Escape(raw);
        var (text, truncated) = CutBytes(escaped, max);
        return new JsonObject { ["text"] = text, ["truncated"] = truncated };
    }

    // Cuts after `max` bytes of UTF-8 on a character boundary and never inside an <U+XXXX> escape.
    static (string, bool) CutBytes(string s, int max)
    {
        if (Encoding.UTF8.GetByteCount(s) <= max) return (s, false);
        var sb = new StringBuilder();
        int bytes = 0;
        int i = 0;
        while (i < s.Length)
        {
            int len = s[i] == '<' && s.AsSpan(i).StartsWith("<U+") && s.IndexOf('>', i) is int close and > 0 ? close - i + 1 : char.IsHighSurrogate(s[i]) ? 2 : 1;
            var piece = s.Substring(i, len);
            int n = Encoding.UTF8.GetByteCount(piece);
            if (bytes + n > max) break;
            sb.Append(piece);
            bytes += n;
            i += len;
        }
        return (sb.ToString(), true);
    }

    // ---- send ----

    static readonly Dictionary<string, string> KeyTable = new()
    {
        ["enter"] = "\r", ["return"] = "\r", ["escape"] = "\x1b", ["esc"] = "\x1b", ["tab"] = "\t", ["backtab"] = "\x1b[Z", ["backspace"] = "\x7f",
        ["delete"] = "\x1b[3~", ["insert"] = "\x1b[2~", ["space"] = " ", ["up"] = "\x1b[A", ["down"] = "\x1b[B", ["right"] = "\x1b[C", ["left"] = "\x1b[D",
        ["home"] = "\x1b[H", ["end"] = "\x1b[F", ["pageup"] = "\x1b[5~", ["pagedown"] = "\x1b[6~",
        ["f1"] = "\x1bOP", ["f2"] = "\x1bOQ", ["f3"] = "\x1bOR", ["f4"] = "\x1bOS", ["f5"] = "\x1b[15~", ["f6"] = "\x1b[17~", ["f7"] = "\x1b[18~",
        ["f8"] = "\x1b[19~", ["f9"] = "\x1b[20~", ["f10"] = "\x1b[21~", ["f11"] = "\x1b[23~", ["f12"] = "\x1b[24~",
    };

    public static byte[] EncodeSendToken(string token, bool raw, int index)
    {
        if (token.StartsWith("key:", StringComparison.OrdinalIgnoreCase))
        {
            var name = token[4..].Trim().ToLowerInvariant();
            if (KeyTable.TryGetValue(name, out var seq)) return Encoding.UTF8.GetBytes(seq);
            if (name.StartsWith("ctrl+", StringComparison.Ordinal) && name.Length == 6 && name[5] is >= 'a' and <= 'z')
                return new[] { (byte)(name[5] - 'a' + 1) };
            if (name.StartsWith("alt+", StringComparison.Ordinal) && name.Length == 5 && name[4] is >= 'a' and <= 'z')
                return new[] { (byte)0x1b, (byte)name[4] };
            throw CliError.Usage($"unknown key \"{TextSafety.Escape(token[4..], false)}\"");
        }
        var text = token.StartsWith("text:", StringComparison.OrdinalIgnoreCase) ? token[5..] : token;
        if (!raw && TextSafety.IsRefused(text, out var cp))
            throw CliError.Usage($"token {index}: the text contains the control or invisible character U+{cp:X4}; use key:NAME for keys, or --raw");
        return Encoding.UTF8.GetBytes(text);
    }

    JsonObject Send(Block caller, JsonObject a)
    {
        var tokensIn = L(a, "tokens");
        bool raw = B(a, "raw");
        int delay = I(a, "delay") ?? 40;
        if (delay < 0 || delay > 1000) throw CliError.Usage("--delay must be between 0 and 1000");
        if (tokensIn.Count == 0) throw CliError.Usage("nothing to send: give key:NAME or text tokens, or --stdin");
        if (tokensIn.Count > 4096) throw CliError.Usage("at most 4096 tokens in one call");
        var chunks = new List<byte[]>();
        for (int i = 0; i < tokensIn.Count; i++) chunks.Add(EncodeSendToken(tokensIn[i], raw, i + 1));
        int total = chunks.Sum(c => c.Length);
        if (total > 64 * 1024) throw CliError.Usage("at most 64 KiB in one call");
        if (total == 0) throw CliError.Usage("nothing to send: every token was empty");
        var b = ResolveBlock(caller, S(a, "block"));
        RequireOwn(caller, b);
        if (b.View != "term" || !b.IsRunning) throw CliError.Refused("the block is not a running terminal");
        int written = 0;
        foreach (var c in chunks)
        {
            if (c.Length == 0) continue;
            b.Input.Add(c);
            written++;
        }
        return new JsonObject { ["bytes"] = total, ["chunks"] = written };
    }

    // ---- screen and shot ----

    JsonObject Screen(Block caller, JsonObject a)
    {
        int last = I(a, "last") ?? 0;
        int max = I(a, "maxBytes") ?? 200000;
        if (last < 0 || last > 10000) throw CliError.Usage("--last must be between 0 and 10000");
        if (max < 1) throw CliError.Usage("--max-bytes must be at least 1");
        var b = ResolveBlock(caller, S(a, "block"), ContentDenied);
        RequireContentAccess(caller, b);
        if (b.View != "term") throw CliError.Refused("the block is not a terminal");
        var lines = b.Screen.ToList();
        while (lines.Count > 0 && string.IsNullOrWhiteSpace(lines[^1])) lines.RemoveAt(lines.Count - 1);
        int total = lines.Count;
        bool truncated = false;
        if (last > 0 && lines.Count > last)
        {
            lines = lines.Skip(lines.Count - last).ToList();
            truncated = true;
        }
        var escaped = lines.Select(l => TextSafety.Escape(l)).ToList();
        int bytes = escaped.Sum(l => Encoding.UTF8.GetByteCount(l) + 1);
        while (escaped.Count > 1 && bytes > max)
        {
            bytes -= Encoding.UTF8.GetByteCount(escaped[0]) + 1;
            escaped.RemoveAt(0);
            truncated = true;
        }
        return new JsonObject { ["lines"] = new JsonArray(escaped.Select(l => (JsonNode?)l).ToArray()), ["total"] = total, ["truncated"] = truncated };
    }

    static readonly System.Text.RegularExpressions.Regex ShotName = new(@"^shot-\d{17}-\d{6}\.png\z");

    static readonly byte[] OnePixelPng = Convert.FromBase64String("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==");

    JsonObject BlockShot(Block caller, JsonObject a)
    {
        var b = ResolveBlock(caller, S(a, "block"), ContentDenied);
        RequireContentAccess(caller, b);
        var dir = Path.Combine(DataDir, "shots");
        Directory.CreateDirectory(dir);
        var path = Path.Combine(dir, $"shot-{DateTime.UtcNow:yyyyMMddHHmmssfff}-{++shotSeq:D6}.png");
        File.WriteAllBytes(path, OnePixelPng);
        // The app deletes only its own files in its own folder, never a subfolder, and keeps the newest 50.
        var own = new DirectoryInfo(dir).GetFiles("shot-*.png").Where(f => ShotName.IsMatch(f.Name)).OrderByDescending(f => f.Name, StringComparer.Ordinal).ToList();
        foreach (var old in own.Skip(50)) old.Delete();
        return new JsonObject { ["path"] = path, ["width"] = 1, ["height"] = 1 };
    }

    // ---- attention and badge ----

    JsonObject Attention(Block caller, JsonObject a)
    {
        var level = S(a, "level") ?? throw CliError.Usage("a level is required: info, need, urgent or clear");
        if (level is not ("info" or "need" or "urgent" or "clear")) throw CliError.Usage("level must be info, need, urgent or clear");
        var reasonRaw = S(a, "reason");
        string? reason = null;
        if (level != "clear")
        {
            if (reasonRaw != null)
            {
                if (TextSafety.IsRefused(reasonRaw, out _)) throw CliError.Usage("the reason has control or hidden characters");
                reason = reasonRaw.Trim();
                if (TextSafety.CodePointCount(reason) > 120) throw CliError.Usage("the reason is longer than 120 code points");
            }
            if (string.IsNullOrEmpty(reason) && level is "need" or "urgent") throw CliError.Usage($"attention level \"{level}\" needs a reason (--reason \"...\")");
            if (string.IsNullOrEmpty(reason)) reason = null;
        }
        var b = ResolveBlock(caller, S(a, "block") ?? "this");
        RequireOwn(caller, b);
        if (level == "clear")
        {
            var onlyLevel = S(a, "onlyIfLevel");
            var onlyReason = S(a, "onlyIfReason");
            bool matches = (onlyLevel == null || b.AttentionLevel == onlyLevel) && (onlyReason == null || b.AttentionReason == onlyReason);
            if (matches) { b.AttentionLevel = null; b.AttentionReason = null; }
        }
        else
        {
            b.AttentionLevel = level;
            b.AttentionReason = reason;
        }
        var o = new JsonObject { ["block"] = b.Id, ["level"] = b.AttentionLevel ?? "" };
        if (b.AttentionReason != null) o["reason"] = TextSafety.Escape(b.AttentionReason, false);
        return o;
    }

    JsonObject Badge(Block caller, JsonObject a)
    {
        bool clear = B(a, "clear");
        var icon = S(a, "icon") ?? "circle-small";
        var color = S(a, "color") is string c ? Validators.Color(c) : null;
        double priority = 10;
        if (a["priority"] is JsonValue pv)
        {
            if (pv.TryGetValue<double>(out var pd)) priority = pd;
            else if (pv.TryGetValue<string>(out var ps) && double.TryParse(ps, System.Globalization.NumberStyles.Float, System.Globalization.CultureInfo.InvariantCulture, out var pp)) priority = pp;
            else throw CliError.Usage("--priority must be a number");
        }
        if (double.IsNaN(priority) || double.IsInfinity(priority)) throw CliError.Usage("--priority must be a finite number");
        if (!clear) icon = Validators.Symbol(icon);
        if (S(a, "block") != null && S(a, "tab") != null) throw CliError.Usage("give --block or --tab, not both");
        string targetId;
        if (S(a, "tab") is string tabArg)
        {
            var t = ResolveTab(caller, tabArg);
            if (!OwnsTab(caller, t)) throw CliError.Denied("that tab is not yours: it needs the admin capability");
            targetId = t.Id;
        }
        else
        {
            var b = ResolveBlock(caller, S(a, "block") ?? "this");
            RequireOwn(caller, b);
            targetId = b.Id;
        }
        if (clear)
        {
            badges.Remove(targetId);
            return new JsonObject { ["applied"] = true };
        }
        var next = new BadgeState { Seq = ++badgeSeq, Priority = priority, Icon = icon, Color = color };
        if (badges.TryGetValue(targetId, out var cur) && next.Priority < cur.Priority)
            return new JsonObject { ["applied"] = false, ["reason"] = "lower priority" };
        badges[targetId] = next;
        return new JsonObject { ["applied"] = true };
    }

    public BadgeState? BadgeOf(string targetId) { lock (gate) return badges.GetValueOrDefault(targetId); }
}
