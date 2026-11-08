// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
using System.Globalization;
using System.Text;

namespace WaveNative.Contract;

// cli-contract.md section 7: Unsafe (output) and Refuse (input).
public static class TextSafety
{
    static bool InRange(int v, int lo, int hi) => v >= lo && v <= hi;

    public static bool IsVariationSelector(int v) =>
        InRange(v, 0x180B, 0x180D) || v == 0x180F || InRange(v, 0xFE00, 0xFE0F) || InRange(v, 0xE0100, 0xE01EF);

    // Unicode Other_Default_Ignorable_Code_Point (the .NET libraries do not expose the property).
    public static bool IsOtherDefaultIgnorable(int v) =>
        v == 0x034F || InRange(v, 0x115F, 0x1160) || InRange(v, 0x17B4, 0x17B5) || v == 0x2065 || v == 0x3164 || v == 0xFFA0 ||
        InRange(v, 0xFFF0, 0xFFF8) || v == 0xE0000 || InRange(v, 0xE0002, 0xE001F) || InRange(v, 0xE0080, 0xE00FF) || InRange(v, 0xE01F0, 0xE0FFF);

    public static bool IsUnsafe(Rune r)
    {
        var cat = Rune.GetUnicodeCategory(r);
        return cat is UnicodeCategory.Control or UnicodeCategory.Format or UnicodeCategory.LineSeparator or UnicodeCategory.ParagraphSeparator
            || r.Value == 0xFFFD || IsVariationSelector(r.Value) || IsOtherDefaultIgnorable(r.Value);
    }

    public static bool IsBidiControl(int v) => InRange(v, 0x202A, 0x202E) || InRange(v, 0x2066, 0x2069);

    // Approximation of Extended_Pictographic plus the keycap bases: enough to tell "an emoji sequence" from stray format characters.
    public static bool IsEmojiBase(int v) =>
        InRange(v, 0x1F000, 0x1FAFF) || InRange(v, 0x2600, 0x27BF) || InRange(v, 0x2190, 0x21FF) || InRange(v, 0x2B00, 0x2BFF) || InRange(v, 0x231A, 0x23FF) ||
        InRange(v, 0x25AA, 0x25FE) || v is 0xA9 or 0xAE or 0x203C or 0x2049 or 0x2122 or 0x2139 or 0x24C2 or 0x3030 or 0x303D or 0x3297 or 0x3299
        || InRange(v, '0', '9') || v == '#' || v == '*';

    // Refuse(r) of the contract, with the emoji-sequence context: U+200D and U+FE0E/U+FE0F pass when they follow an emoji base (or a ZWJ/VS that did).
    public static bool IsRefused(string s, out int badCodePoint)
    {
        var runes = s.EnumerateRunes().ToList();
        for (int i = 0; i < runes.Count; i++)
        {
            var r = runes[i];
            var v = r.Value;
            var cat = Rune.GetUnicodeCategory(r);
            bool bad = cat is UnicodeCategory.Control or UnicodeCategory.LineSeparator or UnicodeCategory.ParagraphSeparator || IsBidiControl(v);
            if (!bad && cat == UnicodeCategory.Format)
            {
                bool joiner = v is 0x200D or 0xFE0E or 0xFE0F;
                if (!joiner) bad = true;
                else
                {
                    var prev = i > 0 ? runes[i - 1].Value : -1;
                    bool afterEmoji = prev >= 0 && (IsEmojiBase(prev) || prev is 0x200D or 0xFE0E or 0xFE0F);
                    bad = !afterEmoji;
                }
            }
            else if (!bad && (v is 0xFE0E or 0xFE0F))
            {
                var prev = i > 0 ? runes[i - 1].Value : -1;
                bad = !(prev >= 0 && (IsEmojiBase(prev) || prev == 0x200D));
            }
            if (bad)
            {
                badCodePoint = v;
                return true;
            }
        }
        badCodePoint = 0;
        return false;
    }

    public static bool HasRefused(string s) => IsRefused(s, out _);

    // Output rule: every unsafe rune becomes <U+XXXX>; newline and tab pass when asked.
    public static string Escape(string s, bool keepNewlineAndTab = true)
    {
        var sb = new StringBuilder(s.Length);
        foreach (var r in s.EnumerateRunes())
        {
            if (keepNewlineAndTab && (r.Value == '\n' || r.Value == '\t')) { sb.Append((char)r.Value); continue; }
            if (IsUnsafe(r)) sb.Append("<U+").Append(r.Value.ToString("X4")).Append('>');
            else sb.Append(r.ToString());
        }
        return sb.ToString();
    }

    public static int CodePointCount(string s) => s.EnumerateRunes().Count();
}
