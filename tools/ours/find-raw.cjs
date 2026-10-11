// node find-raw.cjs <file>...   prints the files that contain a raw control, format or separator character (other than tab, LF, CR).
// Written with numbers only, so that no tool between the author and the file can turn an escape into the character itself.
const fs = require("fs");
function hidden(cp) {
    if (cp === 9 || cp === 10 || cp === 13) return false;
    if (cp < 32 || (cp >= 127 && cp <= 159)) return true;
    if (cp >= 0x200b && cp <= 0x200f) return true;
    if (cp >= 0x2028 && cp <= 0x202e) return true;
    if (cp >= 0x2060 && cp <= 0x2064) return true;
    return cp === 0xfeff;
}
let bad = 0;
const files = process.argv.slice(2);
for (const f of files) {
    const t = fs.readFileSync(f, "utf8");
    for (const ch of t) {
        const cp = ch.codePointAt(0);
        if (hidden(cp)) {
            bad++;
            console.log(f + ": raw U+" + cp.toString(16));
            break;
        }
    }
}
console.log(bad + " file(s) with raw hidden characters out of " + files.length);
