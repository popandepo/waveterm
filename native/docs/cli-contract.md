# Native Wave: the CLI contract (gate 2), version 0

Session: projects-wave-shell. 8 Nov 2026. Status: DRAFT for a blind review, then for the manager.
What this is: the contract of the command tool that replaces the Go `wsh` in the native rewrite (no Go, no Electron). Every caller (PowerShell
tools, the wave-sessions skill, CLAUDE.md text, the never-rules hook, the Test-* stand-ins) is updated in the same piece. No alias, no old form
accepted: "No backwards compatibility" (owner rule, 31 Oct 2026).
How it was made: behaviour today was extracted from the Go source by three read-only passes (appendix: `parts/today-*.md`, every claim there has a
file:line or says "not read"); the new contract keeps what a caller depends on and fixes the quirks those passes listed. Where a decision is not
settled by evidence it is in section 9 as an open item with a default.
Not in this document: the app internals (layout, terminal), the wire format between CLI and app beyond what the callers see (section 1), and anything
no caller uses and nobody asked to keep (section 8, the cut list).

## 0. Principles (each one removes a defect listed in the appendix)
1. One output form: JSON on stdout for success, JSON on stderr for failure. An empty list is `[]`, never `null` or prose. No telemetry call.
2. One id grammar: a full lower-case uuid, or the keyword `this` (the caller's block) or `tab` (the caller's tab). No 8-char prefixes, no numbers,
   no view names, no `type:` prefixes. The verb says what type it wants; a wrong type is exit 4.
3. Authority comes from the token, never from an environment value: `this` and `tab` are read from the token's record, not from WAVE_BLOCK.
4. The app enforces the rules, not the scripts: scope (same workspace), capability (focus), safety (running programs, own tab). The scripts keep
   their own extra rules (idle probe, never-list) but nothing depends on a well-behaved script.
5. No verb changes keyboard focus or the shown tab unless it is a focus verb AND the caller holds the `focus` capability (section 3.3). "No focus" is
   the default of every creating verb; `--focus` is the exception.
6. A change is applied before the call returns, and the answer says what was applied. No "queued".
7. One text-safety predicate and one escape format for any text the app prints that came from a page, a program or a user (section 7).
8. Several keys in one call are one change (atomic).

## 1. Transport, identity, exit codes
- Binary: `wave.exe` (a thin client). It reads the block token from env, sends one request `{verb, args, token}` over the named pipe
  `\\.\pipe\wave-<instance>` (instance = the data folder name; the pipe name is given to every block in env), prints the app's reply, exits.
  The pipe ACL admits only the owner. The app and the CLI are one product: the request format is internal and may change with them.
- Environment given to a program the app starts in a block (and nothing else of ours):
  `WAVE_TOKEN` (opaque, per block, rotated each app start; replaces WAVETERM_JWT and the swap token),
  `WAVE_PIPE` (the pipe name),
  `WAVE_BLOCK`, `WAVE_TAB`, `WAVE_WORKSPACE` (information for scripts; the app ignores them as identity),
  `WAVE_CONN` and `WAVE_VERSION` are dropped (no caller reads them), `WAVE_BIN` (the folder with wave.exe, put on PATH by the app itself).
  The app starts the program directly with this environment: there is no swap-token exchange and no `token` verb (Start-ResumableSession.ps1:335 loses
  its SwapVariables dance and forwards WAVE_TOKEN, WAVE_PIPE, WAVE_BLOCK, WAVE_TAB, WAVE_WORKSPACE).
- The token is bound to one block and one workspace and to that block's capabilities (3.3). A wave.exe outside a block has no token and every verb
  fails with exit 3 (scripts today exit 2 when WAVETERM_* is missing; they keep that check).
- Exit codes (stable, the stderr JSON repeats them):
  0 ok; 1 internal error; 2 usage (bad flag or value, nothing was sent); 3 denied (no token, wrong scope, missing capability);
  4 not found (id unknown, wrong type); 5 refused by a safety rule (running program, own tab, partition, invalid state); 6 app not reachable;
  7 partial success (something was created and something after it failed: stdout still carries the ids).
- stderr on failure: `{"error":{"code":<exit code>,"kind":"<denied|notfound|refused|usage|partial|internal|unreachable>","message":"<text>"}}`.
  The message is safe text (section 7) and never contains a secret or a token.

## 2. Object model the verbs see
workspace > tabs (ordered) > blocks. A tab shows its blocks as tiles; a stack (group) is one tile with several member blocks, one shown.
Views in use: `term` (a program in a pty), `web`. No other view exists in the native app (his layout: 4 tabs, 30 blocks, 25 term, 5 web: reports\wave-native-his-layout.md).
Block meta is a small typed map with a fixed key set (section 6). Ids are server-made lower-case uuids.

## 3. Rules that apply to every verb
### 3.1 Scope
A token may act on blocks and tabs of its own workspace. Cross-workspace access needs the `crossworkspace` capability. Reading or writing another
workspace without it is exit 3 and the message does not say whether the id exists. (Today only web reads obey this; send, screen, setmeta, attention and
the rest are unscoped: appendix part C 7.4.)
### 3.2 Safety
- `block delete` and `session close` refuse a block or tab with a program that is not known to be finished (exit 5) unless `--force`. "Unknown status" counts as running (as today). The refusal is decided by the app, not by a flag the client sets.
- `session close` refuses the caller's own tab (exit 5) even with `--force`; a block may delete itself only with an explicit id and when it has no running program.
- Closing the last tab of the window is refused (exit 5); the window is never closed by a CLI call.
- `send`, `meta set`, `attention`, `badge`, `tab style` act on blocks/tabs of the caller's workspace only (3.1).
### 3.3 Capabilities
A block record carries a set of capabilities, fixed when the block is made and changeable only by a token that holds `grant`:
`focus` (may move the shown tab or give keyboard focus), `crossworkspace`, `grant`. A normal session has none. The manager block has `focus`
and `grant`. Today "only the manager changes focus" lives in Show-InWave.ps1:81-88 and Wave-Sessions.ps1:162-168; native: the app refuses
(exit 3) a focus verb from a token without `focus`. The scripts keep the idle probe (`He-IsActive`), because "he is typing" is outside the app.
How a block gets `focus` (config in the app, or `session start --cap focus` by a `grant` token) is open item D3.
### 3.4 Output of creating verbs
Always `{"tabid":..., "blockid":...}` (and `"stackedwith"` for a stack member). `status` is `"ok"` or, with exit 7, `"partial"` plus `"warning"`.

## 4. Verbs (the keep list). Callers that exist today are marked with the file:line they are updated in.

### 4.1 session start
`wave session start [--name N] [--cwd ABS] [--args-file F | -- CMD ARGS...] [--color C] [--symbol S] [--stack BLOCK] [--activate] [--restart]
[--meta session:KEY=VALUE]... [--env K=V]...`
- `--cwd` must be absolute and exist (exit 2). Default: caller's cwd. (Today session start does not check; run makes it absolute.)
- `--args-file`: UTF-8 JSON array of strings, BOM tolerated (a PowerShell 5.1 BOM breaks the Go parser today). With `--` command too: exit 2.
- `--name`: at most 60 UTF-16 units, no control, bidi or format characters (today only checked in `--stack` mode). Empty: the app names the tab `T<n>`.
- `--color` CSS name or #RRGGBB[AA]; `--symbol` a Font Awesome name (`[a-z0-9-]{1,40}`, optional `solid@ regular@ brands@`) or at most 2 graphemes / 32 bytes without `<>&"'` backtick backslash. Same rules as `tab style`.
- `--stack BLOCK`: no new tab; the block joins the stack of BLOCK (same workspace). `--name/--color/--symbol` then go on the block (title, icon colour, icon); without `--stack` they go on the tab. `--activate` with `--stack` is exit 2. `--activate` needs `focus` (else 3).
- `--restart`: the app starts the same command again at every app start, output kept (sets `cmd:runonstart=true`, `cmd:runonce=false`, `cmd:clearonstart=false`). Refused (exit 5) when the argv contains `--session-id`, or when the command is a shell/interpreter run with an inline-code flag (the list of shells and flags of wshcmd-session.go:171-187 is copied as is: text matching, a best effort, kept because the starter script is the intended path).
- `--meta`: only `session:KEY` with KEY `[A-Za-z0-9._-]+`, values strings without control characters, no key twice. (Part A 1.)
- `--env`: KEY=VALUE, empty key refused, a repeated key is exit 2 (not last wins). The program gets ONLY these plus section 1's variables (and the system environment of the app), never the caller's environment.
- Effects, in this order, all or nothing before step 4: create tab (not shown), set tab style, create the block (the program starts even when the tab is never shown), `--activate`. A failure up to step 3 leaves nothing behind. A failure after the block exists returns exit 7 with the ids.
- stdout: `{"tabid","blockid","status":"ok"}` or, with `--stack`, `{"tabid","blockid","stackedwith","status":"ok"}`.
- Callers: Open-WaveSession.ps1:133-156 (flags unchanged), Restore-Sessions.ps1:100-119 (reads `.tabid`, `.blockid`).

### 4.2 session list
`wave session list [--all]` (needs `crossworkspace` for `--all`). Own workspace by default; today it lists every workspace and every caller filters.
stdout: array in tab-bar order, `[]` when empty. Element:
`{"tabid","name","workspaceid","index":<0-based>,"active":<bool>,"color"?,"symbol"?,"running":<bool>,
"blocks":[{"blockid","view","title"?,"status":"none|running|done","exitcode":<int>,"meta":{"session:id"?,"session:name"?,"session:model"?,"session:effort"?,"session:permissionmode"?}}]}`.
New fields: `index`, `active` (replaces `api list --json` `activetabid`), `blocks[].meta` (the `session:*` keys, replaces `blocks list --json` for `session:name`).
`running` is true when any term block is running. `status` `none` means "no program started in this app run", not "unknown".
- Callers: Wave-Sessions.ps1:127 and :130 (the `api list` call is deleted; `active` replaces it), HomeTab.Lib.ps1:13, Set-HomeTile.ps1, Show-InWave.ps1, Check-HomeTile.ps1.

### 4.3 session close
`wave session close TAB [--force]` -> `{"closed":"<tabid>","blocks":<n>}`. Rules 3.2. Exit 5 text names the first unfinished block. Caller: Wave-Sessions.ps1:300-302 (also keeps its own "never own tab" check, now duplicated by the app).

### 4.4 tab (new in the CLI surface because "tab create from the CLI" is wanted)
- `wave tab new [--name N] [--activate]` -> `{"tabid"}`. The tab starts with no blocks. `--activate` needs `focus`. (Today `CreateTab` may add a default layout block even with "empty": appendix part A, quirk 15, to be tested.)
- `wave tab rename TAB NAME` -> `{"tabid","name"}`; name rules as `--name`.
- `wave tab switch TAB` -> `{"active":"<tabid>"}`. Needs `focus`. It never raises an OS window and never takes keyboard focus away from a tile the owner is typing in (the app decides that; open item D6). Replaces `api goto --tab` (Wave-Sessions.ps1:270). `--window` and `--block` of `api goto` are cut (no caller).
- `wave tab move TAB --index N` -> `{"tabid","index"}`; the app moves, it does not rewrite a stored list (today's server overwrites the id list and can drop a tab).
- `wave tab style [--tab TAB] [--color C] [--accent C] [--symbol S] [--clear]` -> `{"tabid", "color"?, "accent"?, "symbol"?}`. Keys `tab:flagcolor`, `tab:accent`, `tab:symbol`. Rules as part C section 12 (the colour name is stored lower-case, today as typed). `--clear` with another flag is exit 2. No caller today; kept (attention ledger 20261015-C-8a0e4044 via the manager).
- `tab list` is cut: `session list` is the one listing (it includes tabs without programs). Rehome-Manager.ps1:21 finds the Home tab by `name` in `session list` JSON instead of parsing a text column.
- `wave workspace style [--accent C | --clear]` -> `{"workspaceid","accent"?}`; own workspace only (no `--workspace` id). No caller; kept per manager.

### 4.5 block list
`wave block list [--tab TAB] [--view term|web] [--all]` -> array, `[]` when empty, in tab order then layout order:
`{"blockid","workspaceid","tabid","view","stack"?:"<stackid>","meta":{...}}`.
`meta` carries the readable keys of section 6, except: `cmd:env` is never printed; `url` is printed without query and fragment (the home tile's entry token lives in the query and today lands in every listing and every log).
`--tab` accepts a full tab id, `tab` or nothing. Scope 3.1. The unknown-view validation list is just `term` and `web`.
Callers: Health-Check.ps1:324 (reads meta.cmd, meta.cmd:args, meta.cmd:runonstart), HomeTab.Lib.ps1:16 and Show-InWave.ps1:40 (`--view web`, read tabid, blockid, meta.url), Wave-Sessions.ps1:242 (reads meta.session:name), Set-HomeTile.ps1 (meta.web:partition). One change for them all: the verb name (`blocks list --json` -> `block list`; JSON is the only form).

### 4.6 block delete
`wave block delete BLOCK [--force]` -> `{"deleted":"<blockid>"}`. The id is required: today a bare `deleteblock` deletes the calling block. Rules 3.2.
Callers: Wave-Sessions.ps1:250 (`--force`), Rehome-Manager.ps1:66 (no `--force`; exit 5 is logged as today).

### 4.7 meta
- `wave meta get [BLOCK|TAB] [KEY]...` -> object of the asked keys (all readable keys when none), a missing key is simply absent (exit 4 for an unknown id). No caller today. Own block: `url` is returned in full (the app is the owner of its token), for another block redacted as in 4.5.
- `wave meta set TARGET KEY=VALUE...` stores STRING values only. `wave meta set TARGET --json '{"key":value,...}'` stores typed values (bool, number, string, null = delete). Both forms: all keys or none; the keys must be in the section 6 table with the right type and rules, else exit 2 and nothing changes. An empty value is a valid empty string; `meta unset TARGET KEY...` deletes. No `/` nesting, no octal/hex/number coercion, no `prefix:*` pseudo keys, no silently skipped keys. TARGET is a block id, `this`, or a tab id for the tab keys. Stdout `{"id","set":[keys],"unset":[keys]}`. Values are never logged.
- Callers: Show-InWave.ps1:80 (`frame:title`), Set-HomeTile.ps1:76,91-93 (three calls become one `--json` call), Rehome-Manager.ps1:53 (`cmd:runonstart` is a bool: `--json '{"cmd:runonstart":true,"session:id":"...","session:name":"...","session:permissionmode":"auto"}'`, no 3x retry loop needed for half-applied state).

### 4.8 run
`wave run [--tab TAB] [--stack BLOCK] [--cwd ABS] [--env K=V]... [--focus] -- CMD ARGS...` -> `{"tabid","blockid"}`. No focus by default (today the default takes focus and callers pass `--no-focus`); `--focus` needs `focus`. The command is the argv after `--` only (no `-c` shell string, no scan of the process arguments), the program gets no inherited environment. Cut: `-m`, `-x`, `-X`, `--delay`, `-p`, `-a`, `--size` (no caller). The tab must be given or be the caller's (`tab`); a full tab id works in any workspace the token may reach.
Caller: Rehome-Manager.ps1:29,47-49 (`--tab <uuid> --cwd <dir> -- pwsh ...`; reads `.blockid` instead of stripping text up to `block:`).

### 4.9 group (stacks)
- `wave group add BLOCK --to BLOCK` -> `{"stack":"<stackid>","members":[blockids]}`; `wave group remove BLOCK` -> `{"unstacked":"<blockid>"}`; `wave group move --from BLOCK --to BLOCK` -> members; `wave group activate BLOCK` -> `{"active":"<blockid>"}`.
- Same tab only (exit 5 otherwise). All four are applied before returning and fail loudly (exit 5) when the change is not valid ("not in a group", "already in the same group"), without the 100 ms lag of today's stored-layout check. Optional `--focus` on add, remove, move (needs `focus`). `group activate` needs `focus` because it changes what is shown; it never gives keyboard focus (D6).
- Callers: Show-InWave.ps1:101 (`group activate`), Restore-Sessions.ps1:124 (`group add`; its check of the exit code now also sees a failed change).

### 4.10 web
- `wave web open URL [--tab TAB] [--stack BLOCK] [--focus]` -> `{"tabid","blockid"}`. URL must be `http:`, `https:` or `file:`; anything else is exit 2 (no silent web search). The tile is created with its own partition (`persist:<blockid>`), so `web text` can read it and the "shared default store" refusal disappears. Stack and tab rules as `run`. Caller: Show-InWave.ps1:73 (`--stack B --tab T`, no `--no-focus` needed; the before/after listing diff that finds the new block id is deleted).
- `wave web text BLOCK [--selector CSS] [--max-bytes N]` -> `{"text":"<safe text>","truncated":<bool>}`. Selector at most 512 characters without control characters, matched as a CSS selector by the app (never spliced into code). `--max-bytes` 1..262144, default the cap; the cap counts bytes of the escaped text. Scope 3.1; the only denial text is "that web tile is not available to this caller" (existence is not revealed). Reads only; it runs no navigation and takes no focus. One authorization check, in the app. Callers: HomeTab.Lib.ps1:46 (reads `.text`; the `2>&1` merge of today's stderr cut note is gone because the note is the `truncated` field), Check-HomeTile.ps1, Set-HomeTile.ps1.
- Cut: `web get` (hidden, raw, selector spliced into code; no caller), `web reload|back|forward|url|info|shot` (no caller).

### 4.11 send and screen
- `wave send BLOCK TOKEN... [--delay MS] [--stdin] [--raw]` -> `{"bytes":N,"chunks":N}`. Tokens: `key:NAME` (name case-insensitive: enter, return, escape, esc, tab, backtab, backspace, delete, insert, space, up, down, left, right, home, end, pageup, pagedown, f1..f12, `ctrl+<a-z>`, `alt+<letter>`), `text:...` or bare text. The prefix `key:` and `text:` are matched case-insensitively. Text with control or format characters is exit 2 unless `--raw`. `--delay` 0..1000 (default 40), more than 4096 tokens or 64 KiB in one call is exit 2. Delivered in order; on failure the answer says how many chunks were written. Needs the block to be a running terminal (exit 5 otherwise). Never moves focus or the shown tab. Scope 3.1 (today any block of any workspace can be typed into).
  Callers: Start-ResumableSession.ps1:546-547 (`key:Down`, `key:Enter`).
- `wave screen BLOCK [--last N] [--max-bytes N]` -> `{"lines":["..."],"total":<n>,"truncated":<bool>}`. Visible rows and scrollback of a terminal block, trailing blank lines removed, `--last` 1..10000 (0 = all), `--max-bytes` default 200000 (oldest lines dropped, `truncated` true; no marker line mixed into the text). Control characters are escaped per section 7. Works for any terminal block whether its tab is shown or not (the native terminal model is always alive); a block that is not a terminal is exit 5, an unreadable one is exit 4, never a silent empty answer.
  Caller: Start-ResumableSession.ps1:535 (`--last 25`; reads `.lines`).

### 4.12 block shot (kept: brief wave-background-capture; no script calls it today)
`wave block shot BLOCK` -> `{"path":"<png>","width":N,"height":N}`. A picture of one terminal or web block, drawn by the app itself, so it works for a tab never shown and moves nothing. The PNGs go to `<data>\shots`; the app keeps the newest 50 there and deletes only its own files in that folder. Scope 3.1. (Today: term blocks only, with a dance that re-parents a hidden tab view; none of that exists natively.)

### 4.13 attention and badge (kept per the manager's answer of 8 Nov 20:43; no script calls them today)
- `wave attention info|need|urgent --reason TEXT [--block BLOCK]` and `wave attention clear [--block BLOCK] [--only-if-level L --only-if-reason R]` -> `{"block","level","reason"?}`. Rules as today: reason at most 120 code points, no control or hidden characters, required for need and urgent; stored as block meta `attention:level`, `attention:reason` (survives a restart); need and urgent clear themselves when the owner has the tile in focus (500 ms after focus arrives, 3000 ms if it was already there); `--only-if-*` is the compare-and-clear the app uses itself. Scope 3.1.
- `wave badge [ICON] [--color C] [--priority P] [--block BLOCK | --tab TAB]` and `wave badge --clear [...]` -> `{"applied":<bool>,"reason"?:"lower priority"}` (today it prints "badge set" even when the store ignored it). The icon is a Font Awesome name or short text (validated as the tab symbol), the colour as `--color` of tab style. Badges are persistent like attention (today memory only). `--pid` and `--beep` are cut.

## 5. What each caller must change (the list the implementation PR works through)
| Caller (file:line) | Today | Native |
|---|---|---|
| Wave-Sessions.ps1:127 | `wsh session list`, filter workspaceid, array order | `wave session list` (own workspace already), same fields, plus `active`, `index` |
| Wave-Sessions.ps1:130 | `wsh api list --json` for `activetabid` | deleted; `.active` of the list |
| Wave-Sessions.ps1:242, :250 | `blocks list --json`; `deleteblock -b ID --force` | `wave block list`; `wave block delete ID --force` |
| Wave-Sessions.ps1:270 | `api goto --tab T` | `wave tab switch T` (manager token has `focus`) |
| Wave-Sessions.ps1:300 | `session close T [--force]` | `wave session close T [--force]` |
| Open-WaveSession.ps1:133-156 | `session start ...` flags | same flags; reads JSON `.tabid .blockid .status`; `--cwd` absolute |
| Restore-Sessions.ps1:100-126 | `session start --restart ...`, `group add -b B --to A` | `wave session start ...`, `wave group add B --to A` |
| Start-ResumableSession.ps1:335, :535, :546-547 | SwapVariables list, `wsh.exe token`, `screen`, `send` | WAVE_* variables, no token exchange, `wave screen ... .lines`, `wave send` |
| Show-InWave.ps1:40, :73, :80, :101 | `blocks list`, `web open ... --no-focus --stack --tab`, `setmeta frame:title`, `group activate -b` | `block list --view web`, `web open URL --stack B --tab T` (id in the answer), `meta set B frame:title=T`, `group activate B` |
| HomeTab.Lib.ps1:13,16,46 | `session list`, `blocks list --view=web --json`, `web text -b ID --selector '#line'` | `session list`, `block list --view web`, `web text ID --selector '#line'` reads `.text` |
| Set-HomeTile.ps1:76,91-93 | three `setmeta` calls (title, partition, url with token) | one `meta set B --json {...}` (partition is automatic for new tiles) |
| Check-HomeTile.ps1, Health-Check.ps1:324 | via HomeTab.Lib, `blocks list --json` | `block list` (`meta.cmd`, `meta.cmd:args`, `meta.cmd:runonstart` unchanged) |
| Rehome-Manager.ps1:21,29,47-53,66 | `tab list` text column parse, `run --tab --no-focus`, text parse of `block:<uuid>`, three `setmeta` args, `deleteblock` | `session list` by name, `run --tab` JSON `.blockid`, one `meta set --json`, `block delete` |
| hooks-draft never-rules.ps1:307 | allow-list regex of wsh read verbs | allow-list of `wave screen`, `wave web text`, `wave block shot`, `wave block list`, `wave session list` as reads; every other verb as a write |
| Test-WaveSessions.ps1, Test-ShowInWave.ps1, Test-SetHomeTile.ps1, Test-RehomeManager.ps1, Test-CheckHomeTile.ps1, Test-RestoreSessions.ps1 | stand-ins named `wsh`, asserting argument strings | stand-ins for `wave`, asserting the new argument strings (their allow-lists already pin "no `--focus` on activate", "one key per setmeta", which change) |
| wave-sessions skill, CLAUDE.md (about line 460), boot text of Boot.ps1:68 | `wsh send/screen/blocks list/deleteblock`, `wsh run --tab` | the new verbs |

## 6. Meta keys the app accepts (anything else is exit 2)
| key | on | type | rule |
|---|---|---|---|
| `view` | block | string | `term` or `web`; set by the app, not by `meta set` |
| `cmd`, `cmd:args`, `cmd:cwd` | block | string, string[], string | set by `session start`/`run`; `meta set` may change `cmd:*` flags only (below) |
| `cmd:runonstart`, `cmd:runonce`, `cmd:clearonstart` | block | bool | as 4.1 `--restart` |
| `cmd:env` | block | object of strings | write-only: never listed or returned |
| `url` | block (web) | string | `http:` `https:` `file:` only; setting it navigates the live tile |
| `web:partition` | block (web) | string | set by the app at creation; read-only afterwards (today `Set-HomeTile` writes `persist:<blockid>`, which is now the only possible value) |
| `frame:title` | block | string | at most 200 code points, safe text |
| `frame:icon`, `icon` | block | string | Font Awesome name or short text, as tab symbol |
| `icon:color`, `frame:bordercolor` | block | string | CSS name or #RRGGBB[AA] |
| `session:id`, `session:name`, `session:model`, `session:effort`, `session:permissionmode` | block | string | `session:KEY`, KEY `[A-Za-z0-9._-]+`, no control characters, at most 200 code points; further `session:*` keys allowed |
| `attention:level`, `attention:reason` | block | string | by the `attention` verb only |
| `tab:flagcolor`, `tab:accent`, `tab:symbol` | tab | string | by `tab style` only |
| `ws:accent` | workspace | string | by `workspace style` only |
Not accepted: `slot`, `display:*`, `ai:*`, `term:*` and every other key of the old product. `session:*` and `cmd:*` keys on a tab or workspace are exit 2.

## 7. Text safety (one rule)
`Unsafe(r)` = Unicode category Cc, Cf, Zl, Zp, U+FFFD, a variation selector, or Other_Default_Ignorable_Code_Point. Any text the app prints that came from a page,
a program or another session (web text, screen lines, titles, urls, error messages built from such text) has every unsafe rune replaced by `<U+XXXX>`
(upper-case hex, at least 4 digits); `\n` and `\t` pass in `web text` and `screen`. JSON string escaping applies on top. This replaces five predicates and
three escape formats (`<U+XXXX>`, `\xNN`/`\uNNNN`, `?`) of today. Input that must be refused rather than escaped (names, reasons, symbols, send text without `--raw`) uses the same predicate.

## 8. Cut list of the CLI (no caller found, or replaced), with who confirmed it
Cut: `slot *`, `--size`, `web get`, `web reload|back|forward|url|info|shot`, `api focus`, `api goto --window --block --workspace`, `api list` (replaced by `session list`),
`tab list` (replaced), `createblock` (replaced by `run` and `web open`), `getmeta`'s ref grammar, `stack move` (is `group move`), `token`, `termscrollback`, `badge --pid --beep`, `run -m -x -X --delay -p -a -c`, telemetry activity calls, the local HTTP-over-socket API (`wave-api.sock`: no outside caller found), `blocks list` table output.
Confirmed by the manager 8 Nov 20:43: keep attention, badge, tab and workspace style; cut slot, --size, web reload/back/forward/url/info/shot, api focus (unless this gate finds a caller: it found none).

## 9. Open items (each has a default; none blocks the draft)
- D1 Environment names: default rename to `WAVE_*` (section 1), all callers in the same piece. Alternative: keep `WAVETERM_*` to touch fewer lines. Decision needed before the implementation, not before the review.
- D2 Scope: default same workspace for send/screen/meta/web/shot. If the manager and sessions live in different workspaces, the manager needs `crossworkspace` (default: manager block has it). Wave-Sessions.ps1 already filters on its own workspace id, which suggests one workspace, not proven.
- D3 How a block gets `focus`/`grant`/`crossworkspace`: default a field of the app's config naming the manager's session id (`session:id`), applied when that block starts. Not designed; ManagerIdentity.ps1 (how scripts recognise the manager) was not read.
- D4 Closing the last tab: default refuse (exit 5).
- D5 `Restore-Sessions.ps1` reads `wave-state.json` (a sessions index written 300 ms after any change). With restart-at-start native and `session list` available, its purpose is mostly covered; default: drop the state file and the script unless a recovery case is named.
- D6 Focus semantics of `group activate` and `tab switch`: today's code says layout changes never take keyboard focus, but Show-InWave.ps1:5-11,89-90 records that `group activate` moved keyboard focus into the web view while he was typing (appendix part B, section 5; the frontend was not read to resolve it). Native default: `activate` and `switch` never move keyboard focus; they change what is shown only. To be tested in the app, not decided here.
- D7 Is the first tab of a new window empty or does it get a default block? Default: empty (appendix quirk 15).
- D8 `block shot` pruning to the newest 50 deletes only the app's own files; the alternative is no pruning. Default: prune.
- D9 The 8-char id prefix: cut. If a human uses it by hand at a prompt, say so.

## 10. How this is checked (what gate 3 must run before anything is called done)
1. A stub host (the CLI against a fake in-memory app) with one test per verb and per exit code in section 1, each asserting exact stdout JSON and stderr JSON. Red: any caller of section 5 has a verb or field with no test.
2. Each caller of section 5 run against the stub with its Test-* stand-in replaced by the real CLI (the stand-ins' allow-lists updated).
3. A table-driven test of section 6 (every accepted key and one refused value per rule) and of section 7 (each unsafe class, `\n`, `\t`).
4. Against the real app (gate 3 and 4): each rule of section 3 with two tokens, one with and one without the capability or scope, including "no keyboard focus moved" for the focus verbs while a test tile is being typed into.

## Appendix (today's behaviour, from the Go source)
`parts/today-sessions-tabs-run.md`, `parts/today-blocks-meta-groups-api.md`, `parts/today-web-send-screen-identity.md`: the three extraction passes, unedited. Each claim has file:line; "not read" lists are at the end of each file.
