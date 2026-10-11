// node escape-raw.cjs <file>...   replaces every raw control/format/separator character (see find-raw.cjs) in a JS/TS/Go source file by its escape sequence
// (backslash, u, four hex digits), which is valid inside a string literal in all three. Numbers only in this source.
const fs = require("fs");
const BS = String.fromCharCode(92);
function hidden(cp) {
    if (cp === 9 || cp === 10 || cp === 13) return false;
    if (cp < 32 || (cp >= 127 && cp <= 159)) return true;
    if (cp >= 0x200b && cp <= 0x200f) return true;
    if (cp >= 0x2028 && cp <= 0x202e) return true;
    if (cp >= 0x2060 && cp <= 0x2064) return true;
    return cp === 0xfeff;
}
for (const f of process.argv.slice(2)) {
    const t = fs.readFileSync(f, "utf8");
    let out = "";
    let n = 0;
    for (const ch of t) {
        const cp = ch.codePointAt(0);
        if (hidden(cp)) {
            out += BS + "u" + cp.toString(16).padStart(4, "0");
            n++;
        } else {
            out += ch;
        }
    }
    if (n > 0) {
        fs.writeFileSync(f, out);
    }
    console.log(f + ": " + n + " escaped");
}
