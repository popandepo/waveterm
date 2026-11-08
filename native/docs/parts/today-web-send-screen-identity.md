# CLI contract, part C: web, send, screen, blockshot, attention, badge, tab style, workspace style, safetext, identity, focus

Source root: C:\Projects\Personal\wave\waveterm (same tree as the worktree). Paths below are relative to it. "cmd" = cmd/wsh/cmd.
Convention: file:line quotes are what I read. "not read" = I did not read it. "(inferred)" = follows from a library default I did not open.
Only Read/Grep/Glob used. No Wave process or data touched.

## 0. Common to every verb in this slice

Process model
- rootCmd is `wsh` with SilenceUsage true (cmd/wshcmd-root.go:21-26). Persistent flag `-b/--block` (string, default "") on every command: "for commands which require a block id" (wshcmd-root.go:260). It is accepted by every verb, also those that ignore it (`web open`, `tab style`, `workspace style`, `badge` uses it, see below).
- Exit code: a returned error makes Execute call `wshutil.DoShutdown("", 1, true)` (wshcmd-root.go:261-264). Success path calls `DoShutdown("", WshExitCode, false)` (root.go:257); WshExitCode is a global initialised 0 (root.go:37). Body of DoShutdown: not read. So: success 0, any error 1 (the 1 is from the call site; the process exit itself is inside DoShutdown, not read).
- stderr text on error: the script sees cobra's default error print, "Error: <message>" (inferred: SilenceErrors is not set, root.go:21-26; cobra not read). Stdout is empty on error for every verb below except `web text` (stderr note on truncation, see 3.2) and `web shot`/`blockshot` (prune warning, see 3.7, 6).
- WriteStdout/WriteStderr go through WrappedWriter; only when UsingTermWshMode is true does it rewrite "\n" to "\r\n" (root.go:43-69). UsingTermWshMode is a global; who sets it: not read. In the normal tile case treat it as false. `badge` bypasses this and uses fmt.Printf (wshcmd-badge.go:124,126).
- Every verb except `web open`/`web get`... (all use PreRun/PersistentPreRun) calls `preRunSetupRpcClient` first: needs env WAVETERM_JWT, else error `wsh must be run inside a Wave-managed SSH session (WAVETERM_JWT not found)` (root.go:85-95). `web` subtree uses PersistentPreRunE on `web` (wshcmd-web.go:20); `tab style` and `workspace style` use PreRunE (tabstyle.go:51, workspacestyle.go:27).
- Activity telemetry: most verbs call `sendActivity(name, ok)` which sends the verb name to the local Wave (root.go:236-246). `web open`, `send`, `screen`, `badge`, `tab style` ("tabstyle"), `workspace style` ("workspacestyle") do; `web text/shot/info/url/reload/back/forward`, `web get`, `blockshot`, `attention *` do not (checked by reading each Run function). A redesign can drop all of it.
- Block reference grammar (`-b`, default "this"): resolved by `resolveSimpleId` (root.go:201-225): a full oref (`block:<uuid>`, `tab:<uuid>`, ...) is parsed locally with no server call (root.go:202-208); anything else needs env WAVETERM_BLOCKID (else `no WAVETERM_BLOCKID env var set`, root.go:209-212) and goes to server ResolveIds with BlockId = env value, timeout 2000 ms (root.go:213-217). Server vocabulary (pkg/wshrpc/wshserver/resolvers.go:37-79): `x@y` explicit discriminator; keywords this, block, tab, ws, workspace, client, global, temp; `type:uuid` oref; `tab:N` (N 1-3 digits, N-th tab of the CALLER's workspace, resolvers.go:130-164); `view` or `view:N` (lowercase letters, N-th block of that view in the caller's tab by layout leaf order, resolvers.go:200-251); plain integer = N-th block of caller's tab, 1-based (resolvers.go:166-198); full uuid or 8-hex prefix (`DBResolveEasyOID`, not read; the CLI's own comment says the server "takes the first object that starts with it, whatever its type", wshcmd-tabflag.go:20-22). Not found: `id not found: "<id>"` (root.go:222). Quirk: a bare 8-hex id or all-digit id is ambiguous in the grammar (digits are tried as a number first, resolvers.go:66 before uuid8 at :74).
- `resolveBlockRef` (stack.go:41-47) = resolveSimpleId then must be OType block, else `<otype>:<oid> is not a block; groups work between blocks of the same tab (a tab as target is not supported yet)` (stack.go:36). Used by web nav verbs and `web open --replace/--stack`.
- Quirk (identity): "this" and all numeric/view/tab:N references are resolved using env WAVETERM_BLOCKID, which any process can set to anything. The authenticated identity is the block id inside the JWT (section 7). Only the web/blockshot verbs check the authenticated identity; every other verb here trusts the env value (and in practice trusts any block id the caller names, see 7.4).

## 1. `web open`

Synopsis: `wsh web open [-m|--magnified] [-r|--replace <blockref>] [--no-focus] [--stack <blockref>] [--tab <ref>] [--size <2..50>] <url>` (wshcmd-web.go:23-28, 44-50; flags added by addNoFocusFlag, addStackFlag, addTabFlag which also calls addSizeFlag: wshcmd-nofocus.go:24-27, stack.go:49-51, tabflag.go:35-38, sizeflag.go:23-25).

Args: exactly 1 (cobra.ExactArgs(1), web.go:26). Wrong count gives a cobra usage error (exit 1).
Flags
- `-m/--magnified` bool, default false: "open view in magnified mode" (web.go:45).
- `-r/--replace` string, default "": block to replace. Resolved via resolveBlockRef; error `resolving -r blockid: <err>` (web.go:107-111).
- `--replace` with `--magnified`: error `cannot use --replace and --magnified together` (web.go:114-116).
- `--no-focus` bool, default false: "create the block without moving focus to it" (nofocus.go:26). Sets request field Focused = !noFocus (nofocus.go:30-32; web.go:132).
- `--stack <ref>` string, default "": new block becomes a member of the tab group of that block. With --replace: `--stack cannot be combined with --replace`; with --magnified: `--stack cannot be combined with --magnified` (stack.go:55-64). Resolve error: `resolving --stack: <err>` (stack.go:66-68). Note the order in webOpenRun: the -r check at web.go:114 fires before applyStackTarget, and applyStackTarget's replace check only fires if TargetBlockId is already set (it is when --replace given, web.go:134-137).
- `--tab <ref>` string, default "": the tab to create in. PRESENCE counts, not value (`cmd.Flags().Changed`, tabflag.go:65). Empty value: `--tab needs a tab: a full tab id, tab:<n> or tab` (tabflag.go:51). Accepted forms only: literal `tab`, `tab:<1-3 digits>`, full uuid with optional `tab:` prefix (regexes tabflag.go:23-26); anything else: `--tab "<ref>" is not accepted: use a full tab id, tab:<n> or tab (an id prefix can match more than one object)` (tabflag.go:53-55). Resolve failure: `resolving --tab: <err>` (tabflag.go:58). Without --tab: env WAVETERM_TABID, else `no WAVETERM_TABID env var set` (tabflag.go:44-47).
- `--size <uint>` default 0 = not given. Given and outside 2..50: `--size must be between 2 and 50, got N` (sizeflag.go:29-37). Server also re-checks (checkCreateNodeSize, wshserver.go:315; body not read). No effect with --replace/--stack (sizeflag.go:24).
- `--tab` with `--stack`/`--replace`: target block must be in that tab, else error `the target block is in tab X, not in tab Y: --tab needs the stack or the block to replace to be in that tab` (tabflag.go:70-82, 93-101). Without --tab there is no CLI check; server checks stack targets (`createTargetNeedsSameTab`, wshserver.go:320-327; messages not read).
- Quirk: `-b` is accepted (persistent flag) but ignored by `web open`.

Environment read: WAVETERM_JWT (always), WAVETERM_BLOCKID (only when --replace/--stack/--tab uses a non-oref ref), WAVETERM_TABID (only when --tab absent).
URL: NOT validated by the CLI; passed as meta `url` with `view: "web"` (web.go:125-130). Scheme handling happens in the frontend (ensureUrlScheme; the comment at emain/emain-webnav.ts:8-10 says the tile opens file: addresses and turns non-address text into a search); frontend code not read. Quirk: text that is not a URL silently becomes a web search.
Meta set on the new block: only `view` and `url`. No `web:partition` is set, so a block made by `web open` is on the shared default store and `web text/shot/get` on it are REFUSED (section 3.2/3.5) until someone setmeta's `web:partition` to the block id or `persist:<blockid>`. (Show-InWave.ps1 never reads such tiles.)

Stdout on success: one line `created block <oref>\n` where oref prints as `block:<uuid>` (web.go:148; ORef String format `type:id` per root.go use of oref.String(); the String method itself not read, but `badge` sends oref.String() and server parses with ParseORef, so the form is `block:<uuid>`). A script must parse the id from that line or list blocks afterwards: Show-InWave.ps1:72-77 does the latter (diffs `blocks list --view=web --json` before/after) and never reads the stdout line. Quirk: no `--json` output; machine callers cannot get the id reliably.
Failures: `creating block: <server error>` (web.go:146). Other errors listed above. All exit 1.
Server side: CreateBlockCommand (wshserver.go:312-408): creates the block record in tab, builds a layout action (insert / replace / splitright|left|up|down / stack) carrying `Focused`, queues it for the tab's page (`QueueLayoutActionForTab`, :400), publishes updates, and calls `requestTabPageLoad` in the background (:406): asks Electron to preload the tab page so the controller starts even if the tab is not shown (ours_preload.go:21-26, 30-41, 232-235; up to 5 attempts per block, 30 s request timeout). For `replace` the target block is DELETED (wshserver.go:342). No authorization check at all: any authenticated link may create a block in any tab id it names (wshserver.go:312-408 has no caller check).
Focus: default (no --no-focus) the layout action has Focused true, i.e. the new tile takes focus in that tab's layout (Focused field wshserver.go:340-397; how the page applies it: not read). `--no-focus` sends Focused false. Neither switches the SHOWN tab: nothing in this path calls a tab switch (checked: web.go, wshserver.go:312-408); a block created in a hidden tab is queued and the page is preloaded without being shown (ours_preload.go:30-35). `--stack` target: stack member is not activated unless focused (planCreateInStack, not read).
Authorization rule: none beyond holding a valid JWT. The "only the manager changes focus" rule lives in the scripts, not in wsh (Show-InWave.ps1:81-88 via Test-CallerIsManager). `--no-focus` is the callers' only guard and is entirely voluntary.
Redesign flags: (a) print machine-readable id (`--json` or just the bare id); (b) decide whether `--no-focus` should be default; (c) drop ignored `-b`; (d) set a partition so the tile can be read, or document that reading it is refused; (e) validate url scheme in the CLI.

## 2. `web get` (hidden)

Synopsis: `wsh web get [--inner] [--all] [--json] <css-selector>` (wshcmd-web.go:30-36, 51-53). Hidden: true (web.go:34) so it is absent from help.
Flags: `--inner` bool false (innerHTML instead of outerHTML), `--all` bool false (querySelectorAll), `--json` bool false.
Args: exactly 1.
Flow (webGetRun, web.go:58-99): resolveBlockArg (root.go:115-125; error `resolving blockid: ...`; NOTE it accepts any oref type here) -> BlockInfo RPC (`getting block info: ...`) -> must have meta view "web" else `block <id> is not a web block` -> WebSelector RPC to route `electron`, timeout 5000 ms.
Does NOT call webNavAuthorize and does not map errors to the denied text in the CLI; the authorization happens in emain (`handle_webselector` -> readWebTile, emain/emain-wsh.ts:34-37, emain-webnav.ts:232-267). So denial text from the server is whatever the RPC error says, passed through as is (web.go:84-86 returns err). Quirk: a different denial text path than the other web verbs, which hide existence.
Output: default: one item per line, `%s\n` of RAW outerHTML/innerHTML (web.go:94-96). JSON: `json.MarshalIndent(output, "", "  ")` of a JSON array of strings (web.go:88-92). Both are RAW page data written to the terminal: NO safeText, NO byte cap, NO control-character escaping (unlike `web text`). No match gives an empty array `[]` (JSON) or no output (text) - from emain-web.ts:57-72 value is `[]`; but `webGetSelector` returns null when `selector` is empty (emain-web.ts:51-53), which cannot happen with ExactArgs(1) unless "" is passed: then output null -> `null` in JSON mode, nothing in text mode (inferred).
Selector in emain: escaped by hand (backslash, double quote, single quote at least; emain-web.ts:35-39, rest of the function not read) and spliced INTO code text (`document.${queryMethod}("${escapedSelector}")`, emain-web.ts:61). Quirk: code injection surface; no length or control-char check (compare `web text`, which JSON-encodes the selector and caps it at 512 chars, emain-webnav.ts:269-298). Invalid selector error is the page's message passed through (emain-web.ts:69-71).
Failure exit code 1.
Same partition and workspace rules as `web text` apply because the server uses readWebTile (emain-wsh.ts:34-37).
Redesign: cut `web get`, or fold it into `web text --html [--inner] [--all]` with the same caps, safe printing and selector handling. Callers: none found (section 12).

## 3. `web text`

### 3.1 Synopsis and flags
`wsh web text [--selector <css>] [--max-bytes <n>] [-b <block>]` (wshcmd-webtext.go:27-46). Args: none (cobra.NoArgs).
- `--selector` string default "": "css selector: the text of the first match instead of the whole page". Validation (webtext.go:53-67): more than 512 characters (runes) -> `--selector is at most 512 characters`; any unicode control rune -> `--selector cannot contain control characters`. emain re-checks: error `the selector must be text of at most 512 characters without control characters` (emain-webnav.ts:269-278).
- `--max-bytes` int default 262144: "cut the text after this many bytes of UTF-8 (at most 262144)". Outside 1..262144: `--max-bytes must be between 1 and 262144` (webtext.go:54-56).
- `-b` default "this" (webtext.go:90-93).

Environment: WAVETERM_JWT; WAVETERM_BLOCKID only when -b is not a full oref.
Order in webTextRun (webtext.go:86-131): check options -> resolveBlockRef -> webNavAuthorize -> BlockInfo -> makeWebNavRequest -> WebText RPC (route electron, timeout 10000 ms, webtext.go:49-51).
- resolve failure: with `-b` given, error is the single text `that web tile is not available to this caller` (webtext.go:96-98); without `-b`, `resolving blockid: <err>` (webtext.go:99).
- authorize failure (any error): `that web tile is not available to this caller` (webtext.go:102-104).
- BlockInfo failure: `getting block info: <err>` (webtext.go:106).
- makeWebNavRequest failures (wshcmd-webnav.go:85-96): `block <id> could not be read`; `block <id> is not a web block` (view != "web"); `block <id> has no tab or workspace`. Note these three texts distinguish "exists but not web" for a tile in the caller's own workspace only (authorize ran first).
- RPC failure: if the error contains the denied text -> that text; else the error text through safeText cut at 400 runes (`maxPrintedErrorRunes`, webtext.go:119,133).
- nil answer: `no answer from the window` (webtext.go:122).

### 3.2 emain side checks (readWebTile, emain/emain-webnav.ts:232-267), in this order
1. tabid, blockid, workspaceid all required: `tabid, blockid and workspaceid are required`.
2. blockid must match full lower-case uuid regex (FullBlockIdRe, :228): `blockid must be a full block id`.
3. authorize (WebNavAuthorize RPC to server with callerroute = the source route stamped on the message, timeout 3000 ms, AuthorizeTimeoutMs :14; emain-wsh.ts:19-21).
4. BlockInfo; authorizeWebNav (:100-119): errors `the block could not be read`, `the block is not the one named`, `the block is not in the tab named`, `the tab is not in the workspace named`, `the block is not a web tile`.
5. PARTITION RULE (webReadRefusal, :188-198), reads the stored block meta key `web:partition` (never a caller value): if missing or blank: `this web tile uses the shared default sign-in store of Wave, so what it shows is not given out: give the tile its own partition (web:partition) or wait for the allow-list`. If it is not exactly `<blockid>` or `persist:<blockid>`: `this web tile's partition is not its own, so what it shows is not given out: set web:partition to <id> or persist:<id> (a name another tile or a sign-in may use is refused)`.
6. window by workspace id: `no window found with workspace <id>`; tab loaded in window: `the tab <id> is not loaded in its window (show it once, then ask again)`.
7. web contents: `no web contents found for block <id> (is its page ready?)`.
8. authorize AGAIN, then live session check (liveSessionRefusal :204-217): the contents' actual Electron session must be the session object of the partition named in the record: else `this web tile's page is not on its own partition, so what it shows is not given out (the tile may have been loaded before its web:partition was set; reload it)`.
9. read: run script in the page (`executeJavaScript`), `innerText` of body or first match of selector, sliced to maxChars (buildTextScript :282-298). Page answers: no object -> `the page gave no answer`; error -> `the page refused the selector: <printable err, 200 chars>`; no match -> `nothing on the page matches the selector`. Hidden elements are not listed (innerText) unless the selector names them (CLI help, webtext.go:30-31).
Cap: effectiveMaxBytes = min(asked, 262144) (:339-344); text cut by `capUtf8` on whole characters (:321-337); `truncated` true if cut or the page had more (:363).
Side effects: runs page JavaScript (a hostile page can lie, CLI help says so, webtext.go:30-33). No navigation, no click, no focus.

### 3.3 stdout/stderr
- stdout: the text, escaped, then a newline. Escaping = safeTextBlock (webtext.go:71-84): newline and tab kept; every unprintable rune (class below, section 11) written as `<U+XXXX>` (4 or more hex digits, upper-case, `%04X`); includes carriage return. The cut is applied AFTER escaping to `--max-bytes - 1` bytes (the newline counts, webtext.go:124-125) and never splits a character or a `<U+XXXX>` (webtext.go:135-147). So output can be shorter than the page text by up to 8x expansion.
- stderr (success, exit 0) when the text was cut: `the text was cut (at most <max-bytes> bytes)\n` (webtext.go:127-129). Scripts that capture `2>&1` get this line mixed into the text (HomeTab.Lib.ps1:46 does `2>&1`; only matters if the page text exceeds the cap).
- How a script parses: whole stdout, trimmed. HomeTab.Lib.ps1:46-47 trims the output and on exit 0 uses it as the line.
- Empty text prints just "\n".
Exit: 0 success; 1 on any failure.
Quirks: (1) the same denial text for "not found", "other workspace" and (with -b) "unresolvable" but different text for "in my workspace but not web", by design; (2) 3 authorize round trips per call (CLI, emain before, emain after contents); (3) partition rule makes `web open` tiles unreadable without a prior setmeta; (4) `--max-bytes` default equals the cap so the flag only lowers; (5) page-provided error text is `?`-escaped in emain (printableErrorText :307-318) and then `<U+XXXX>`-escaped again in the CLI.
Authorization rule: section 7.3.
Focus: none. "Reads only ... nothing is navigated, clicked or focused" (webtext.go:30-31; webnav.ts header :4).

## 4. `web reload|back|forward|url|info` (cut unless a caller exists)

Caller check result: NO caller found in C:\Projects\Personal\MalfestioMaster\tools or wave\waveterm\tools\ours for `web reload`, `back`, `forward`, `url`, `info` (grep of the verb patterns and every `$Wsh`/`Invoke-Wsh` call, section 12). Only mentions: wave\waveterm\tools\ours\Check-Seams.ps1:95 (`web-nav-handler`, checks that emain still has `async handle_webnav(`, "the window answers wsh web reload, back, forward and url"), and hooks-draft\never-rules\never-rules.ps1:307 which allow-lists `web (shot|text|info|url)` as plain reads. Recommendation: CUT reload, back, forward, url. `info` and `url` have no consumer; if a health check wants "page loaded", keep only `info` (it subsumes url).

Synopsis: `wsh web reload|back|forward [-b block]`, `wsh web url [--json] [-b block]`, `wsh web info [--json] [-b block]` (wshcmd-webnav.go:22-64). No args (cobra.NoArgs).
Flags: `--json` bool default false, on `url` and `info` only (webnav.go:61-62). Other verbs reject `--json` as unknown flag (cobra).
Flow (webNavRun, webnav.go:146-184): resolveBlockRef (denied text if `-b` given and unresolvable, :153-155) -> webNavAuthorize (denied text on any error) -> BlockInfo (`getting block info: ...`) -> makeWebNavRequest (same three errors as 3.1) -> WebNav RPC to electron, timeout 5000 ms (:78-80). RPC error containing the denied text maps to it, else passed through unchanged (NOT safeText'd here, unlike web text) (:171-177).
emain handle_webnav (emain-wsh.ts:39-65): requires tabid/blockid/workspaceid, full block id regex, authorize, window, tab loaded, authorizeWebNav (view web), contents, authorize again, then webNavigate. NO partition rule for nav verbs (they never expose page content beyond title and address).
webNavigate (emain-webnav.ts:124-166):
- reload: refuses unless current page scheme is http or https: `the page shown is not an http or https page (<scheme or "no scheme">); wsh web only reloads and steps through http and https pages` (:70-75, :131); then wc.reload().
- back / forward: error `nothing to go back to` / `nothing to go forward to`; target history entry must be http(s): `the previous page is not an http or https page (...)...` / `the next page ...`; then goBack/goForward.
- url, info: no action.
- Unknown action: `unknown web action "<x>" (reload, back, forward, url or info)`.
- Result is the state BEFORE the navigation takes effect (comment :121-123): "ask again with url".
Server result fields (CommandWebNavRtnData, wshrpctypes.go:503-509): `url` string (credentials stripped via withoutCredentials, capped 2048 chars), `title` string (capped 200 chars), `cangoback` bool, `cangoforward` bool, `state` string only for info ("loading"|"ready"|"failed"; failed when mainFrame url scheme is `chrome-error:`; emain-webnav.ts:43-57).
stdout formats (formatWebNav, webnav.go:116-144; url and title first go through safeText, url 2048 runes, title 200 runes, with `<U+XXXX>` and `...`):
- reload/back/forward: `<action lower-case> requested (the tile showed <url>)` e.g. `reload requested (the tile showed https://x/)`.
- url: line 1 address, line 2 title (`<url>\n<title>`), plus final newline.
- info: five lines `address: <url>`, `title: <title>`, `state: <loading|ready|failed|unknown>`, `back: <yes|no>`, `forward: <yes|no>`. State other than the three words prints `unknown` (webnav.go:99-105).
- --json (url, info): `json.MarshalIndent(res, "", "  ")` of the struct: keys `url`, `title`, `cangoback`, `cangoforward`, and `state` (omitted for url because it is set to "" and has omitempty; present for info) (webnav.go:121-135; wshrpctypes.go:503-509).
- Quirk: `--json` is a pair of globals keyed by action (webnav.go:178), fine in a one-shot process.
Exit: 0 / 1 as 0.
Quirks to fix if kept: scheme rule http/https only (file: pages refused, "the owner says if file: pages are to be added", emain-webnav.ts:8-10), `show-in-wave` opens file: urls (Show-InWave.ps1:50) which `reload` would refuse; text-vs-denied mapping duplicated in 5 files (webNavDenied constant, plus blockShotDenied).
Focus: none ("nothing takes focus", wshrpctypes.go:488-489).

## 5. `web shot` (cut unless a caller exists)

Caller check: no script calls `web shot`. Only the never-rules allowlist (never-rules.ps1:307) and tests of the CLI. Recommendation: cut `web shot`; keep `blockshot` (it replaces it for terminal blocks) only if a caller exists - see 6.
Synopsis: `wsh web shot [-b block]`, no args (wshcmd-webshot.go:32-47).
Flow (webShotRun, :196-250): resolve (denied text on -b failure) -> authorize -> BlockInfo -> makeWebNavRequest -> WebShot RPC (electron, 15000 ms) -> decodeWebShot -> pick folder -> write file -> print -> prune.
emain: readWebTile with the same checks and partition rule as `web text` (emain-wsh.ts:72-75), then `webShot` (emain-webnav.ts:393-417): `capturePage()`; empty image -> `the tile gave an empty picture (is its tab shown and its page painted?)`; PNG over 4 MiB (MaxShotBytes) is scaled once to width min(1280, floor(width/2)) else `the picture is larger than 4194304 bytes and cannot be scaled down`/`... even scaled down`. Returns {png base64, width, height}.
CLI decode (webshot.go:123-151): empty `no picture from the window`; base64 length cap `the picture is larger than 4194304 bytes`; `the picture from the window is not valid base64`; not PNG signature `the picture from the window is not a PNG`; header/size limits: max 8192 per side and 16777216 pixels, `the picture claims WxH pixels, more than 8192 per side or 16777216 in all`; full decode `the picture from the window is not a complete PNG: <err>`.
Output location: caller cannot choose. Folder from `defaultWebShotDir` (AppData\Local\Wave\web-shots per help text, webshot.go:36-37; the function body is in wshcmd-shotfile_*.go: not read), name `<blockid>-<UTC yyyymmddThhmmssZ>-<8 hex>.png` (webshot.go:112-120). Folder creation refuses non-plain folders (ensureShotDir :178-194; checkShotDir not read). File created exclusive, synced, kept on success (:155-171). 50 newest kept; older files named like ours are removed (:68-110). Prune failure goes to stderr `removing older pictures: <err>` with exit 0 (:246-248).
stdout success: `<absolute path> (<w>x<h>, <n> bytes)\n` where w,h come from the window's answer, n from the decoded bytes (:245). A script parses the path as everything before the last ` (`.
Side effects: writes a file under the user profile; deletes old shots. Focus: none; "The tile must be shown" otherwise empty-picture error.
Quirks: the tool deletes files (prune) in a folder it also writes; width/height printed are the window's claim, not the file's.

## 6. `blockshot`

Caller check: no script in the two trees calls it. Only hooks-draft never-rules.ps1:307 and never-rules-wave-quiet.test.ps1:92-95 allow-list it. Recommendation: cut unless a screenshot of a terminal tile is wanted by a future caller (`screen` gives text).
Synopsis: `wsh blockshot [-b block]`, no args (wshcmd-blockshot.go:21-32). Same folder, name, prune and stdout format as `web shot` (blockshot.go:73-91).
Authorization: same `webNavAuthorize` (own workspace only); denied text here is `that block is not available to this caller` (blockshot.go:19, 51, 56).
Flow: resolve -> authorize -> BlockInfo (`getting block info: <err>`; `block <id> has no tab or workspace`) -> BlockShot RPC (electron, 15000 ms) -> decodeWebShot -> write. Note: RPC errors are NOT mapped to the denied text and not safeText'd here (blockshot.go:65-68).
emain blockShot (emain/emain-blockshot.ts:138-249), in order: ids required; full id; authorize; BlockInfo checks (same four texts as authorizeWebNav but without the web-tile rule); view must be in AllowedViews = ["term"]: `wsh blockshot takes terminal blocks only (view "term"); this block is "<view>" (for a web tile use wsh web shot, which has the sign-in rule)`; EVERY block in the same tab must be view term: `the tab also holds a "<view>" block (not a terminal): it could be drawn over the target, so no picture is taken`; window; tab loaded: `the tab <id> is not loaded in its window (it loads in the background after a start or when a block is made in it; ask again in a moment)`.
Then, under a per-tab lock, for a tab that is not on screen it temporarily adds the tab view at the bottom of the window child order, positions it on screen, turns background throttling off, waits 300 ms, polls layout up to 20 x 250 ms until the block rectangle is at least 100x100, takes up to 4 capture attempts (300 ms apart), then parks the view off screen again and restores throttling (emain-blockshot.ts:35-46, 179-247). Overall deadline 12000 ms (:25): `capturing the block took longer than 12000 ms and was given up (its page may be hung)`. Other errors: `the block is not drawn in its tab (is it behind another member of a stack, or is its page not ready?)`, `the block has no size in its tab`, `another block of this tab is magnified or floating over it, so a capture would show what is on top: refused`, `the block has not been laid out in its tab yet (WxH); its page is asleep, ask again in a moment`.
Focus: "Nothing here calls show, focus or raises a window" (emain-blockshot.ts:9). It does briefly re-order a hidden tab's view inside the window while capturing (visual side effect, covered by the active tab's opaque view per the comment :6-9). Redesign: a native app should capture the control directly.
Quirk: the server RPC is named "all views" in wshrpctypes.go:539 but the allow-list is term only.

## 7. Identity and authorization model

### 7.1 Environment variables (what sets them, what uses them)
Set per shell block by `makeSwapToken` (pkg/blockcontroller/blockcontroller.go:561-599), delivered through the swap token's Env map and exported by the shell integration after `wsh token ...`:
- TERM_PROGRAM=waveterm (:567), WAVETERM=1 (:570; note shellutil.go:228 sets WAVETERM to the exe path for the local shell env, the swap value is 1; later one wins: not determined).
- WAVETERM_BLOCKID = block id (:568). USED BY wsh: `resolveSimpleId` (root.go:187-190, 209-225) as the "this" block and as the peer-info label sent at connect (`domain:block:<id>`, root.go:187-191; label only, not authority).
- WAVETERM_VERSION = wavebase.WaveVersion (:569; also shellutil.go:229). No wsh verb in this slice reads it. Consumers: not found (grep showed only main-server.go:425 unsetting it).
- WAVETERM_TABID = tab id of the block (:571-576). USED BY wsh: `--tab` default for block creation (tabflag.go:44-47, root.go:227-229), focusblock (focusblock.go:32).
- WAVETERM_WORKSPACEID = workspace of the tab (:577-584). USED BY wsh: `workspace style` default (workspacestyle.go:70), tab list/new/move, api, session. Not used for any authorization.
- WAVETERM_CLIENTID = wstore.GetClientId() (:585). Not read by any wsh verb in this slice (grep); main-server.go:419 unsets it. Consumers elsewhere: not read.
- WAVETERM_CONN = remote connection name (:586; empty for local). Not read by wsh from env: `badge --pid` uses the `Conn` claim inside the JWT (`RpcContext.Conn`, wshcmd-badge.go:107-110) instead.
- WAVETERM_WSHBINDIR = `<data dir>/bin` (shellutil.go:230); only for PATH setup by the shell integration scripts (bash_bashrc.sh:7-10, pwsh_wavepwsh.sh:2, zsh_zshrc.sh:2-3). The wsh binary does not read it.
- WAVETERM_JWT: a signed token, see 7.2. Present in the shell only when the swap token's Env contained it and was exported by the init script (swapToken.Env[WaveJwtTokenVarName] set at shellcontroller.go:447, 480, 506 when the connection has wsh enabled); or injected directly into the process environment when block meta `cmd:jwt` is true (ForceJwt, shellcontroller.go:744; shellexec.go:640-644). Required by every verb here (root.go:86-89).
- WAVETERM_SWAPTOKEN: base64(JSON{token: uuid, rpccontext}) put in the new shell's environment (shellexec.go:633-639; format tokenswap.go:27-61). Consumed ONCE by the shell integration: `wsh token "$WAVETERM_SWAPTOKEN" <shell>` then unset (bash_bashrc.sh:13-14; pwsh_wavepwsh.sh:5 and Remove-Item at :10; zsh_zshrc.sh:4-5; fish_wavefish.sh:6-7).
- The fork's resumable-session starter forwards these names to a child: `$SwapVariables = @('WAVETERM_JWT','WAVETERM_BLOCKID','WAVETERM_TABID','WAVETERM_WORKSPACEID','WAVETERM_CLIENTID','WAVETERM_CONN','WAVETERM_VERSION','WAVETERM','TERM_PROGRAM')` (tools\ours\Start-ResumableSession.ps1:335) - the complete list a native replacement must keep providing to those scripts.

### 7.2 The tokens
- JWT (WAVETERM_JWT): claims Sock, RouteId, ProcRoute, BlockId, Conn, Router (wshutil.go:228-235); signed (`wavejwt.Sign`), issuer "waveterm", expiry 365 days (wavejwt.go:129-133; signing key handling not read). The client parses it UNVERIFIED only to read the socket name and context (wshutil.go:359-388; root.go:168-176), then the server validates it (`AuthenticateCommand`, wshrouter_controlimpl.go:90-124: validates signature, validates context shape :312-329, then binds the link).
- Local shell tokens: `RpcContext{ProcRoute: true, SockName, BlockId}` (shellcontroller.go:496-500); ssh/wsl add `Conn` (:436-441, :469-474).
- Swap token (`wsh token <token> <shell-type>`, hidden command, wshcmd-token.go:13-45): exactly 2 non-empty args else `wsh token requires exactly 2 arguments, got N` / `wsh token requires non-empty arguments`; unpacks base64 JSON (`error unpacking token: ...`; needs rpccontext and sockname), connects to that socket, calls AuthenticateToken on the control route; server exchanges it: ONE USE (GetAndRemoveTokenSwapEntry, tokenswap.go:88-97), lifetime 5 minutes (blockcontroller.go:565), router contexts refused (`cannot auth router via token`, controlimpl.go:150-152). Output: the env-var export script for the shell type (bash/zsh `export K='v'`, fish `set -x K v`, pwsh `$env:K = 'v'`; shell types accepted: bash, zsh, fish, pwsh, else `unknown or unsupported shell type for env var encoding: <x>`; names that are not valid shell identifiers are skipped, tokenswap.go:99-146) followed by the shell init script text (token.go:38-43). `wsh token` is the only command that does not authenticate with the JWT.
- Each wsh invocation opens a new domain-socket link and authenticates once; a link may authenticate once and a second token must have the same route and block (`this link is already authenticated as another route or block`, controlimpl.go:126-139).
- Which block the server believes the caller is: `bindLeafIdentity` records the BlockId from the JWT/swap context on the link (controlimpl.go:130-139; SetLinkAuthBlockId). `Router.BlockIdForSourceRoute(route)` returns it for a leaf link's source route, "" for anything else (wshrouter.go:829-844).

### 7.3 Same-workspace rule for web commands (ours_webnav_auth.go)
Applies to: `web text`, `web shot`, `web get` (via emain), `web reload|back|forward|url|info`, `blockshot`. Does NOT apply to `web open`, `send`, `screen`, `attention`, `badge`, `tab style`, `workspace style` (none call it).
- Server RPC `WebNavAuthorize{callerroute, blockid}` -> `checkWebNavCaller` (pkg/wshrpc/wshserver/ours_webnav_auth.go:19-35, 64-85):
  - callerRoute is `callerRouteOf(stamped source, claimed)`: when the message source is `electron` the claimed route in the data is used (emain passes the original caller's stamped source: emain-wsh.ts:20, 48, 63, 81); for any other source the stamped source is used and the claimed value ignored (ours_webnav_auth.go:57-62), so a session cannot ask on behalf of another.
  - if callerRoute == "wavesrv" (the server itself, e.g. AI tools) -> allowed (:24-26, :66-68).
  - callerBlockId = router.BlockIdForSourceRoute(route) (the JWT's block id, :41, :69). Empty -> error `the caller of this web command is not a block of this window (its route "<route>" has no block)` (:27-29). (The CLI collapses any authorize error into the denied text, so a script never sees this one.)
  - workspace of caller block and of target block via DB lookup (tab for block, workspace for tab, :45-51). If either is empty or they differ -> `that web tile is not available to this caller` (:31-33). Lookup failures are logged and treated as empty (:75, :81).
- CLI (webnav.go:67, 75-77, 160-162): any error from authorize returns `that web tile is not available to this caller`; for `blockshot` the text is `that block is not available to this caller`.
- Existence is hidden: nonexistent block, other workspace, and an unresolvable `-b` all give the same text (wshcmd-webnav.go:153-155, webtext.go:96-98).
- Limit: the check is on the WORKSPACE, not the tab, so any block of the caller's workspace (any tab) can be read; the tile "may be in any tab the caller can name (-b)" (webnav.go:83-84). The router link identity is local only: a caller from a remote connection behind another router is refused (comment :21-22).
- Quirk: the identity used (JWT block id) differs from what `-b this` resolves (env block id, root.go:209-212). A caller that sets WAVETERM_BLOCKID to another block id and passes `-b this` gets a block id of its choosing in the request, but authorize judges by the JWT, so cross-workspace access is still refused.

### 7.4 What is NOT authorized (verify before trusting a redesign)
Read: wshserver.go lines cited; none of these handlers inspects the caller's route or block.
- `SetMetaCommand` (wshserver.go:265-274): any link can write meta on ANY oref it names (used by `attention`, `tab style`, `workspace style`, and by scripts' `setmeta`). No workspace rule. Same for `CreateBlockCommand` (:312), `ControllerInputCommand` (:431-445, used by `send`), `EventPublishCommand` (:627-637, used by `badge`; only checks a source exists and stamps Sender), `GetMetaCommand`/`BlockInfoCommand` (:148, :926-954, used by `send`, `screen`, `web get`, web verbs: any block's meta and files list readable), `ResolveIdsCommand`.
- Consequence: `send -b <any block id>` can type into any terminal of any workspace in the Wave instance (CLI only checks view == term). The same-workspace rule is therefore only a rule about WEB PAGE CONTENT and screenshots, not about input, meta or badges.
- `screen` frontend side: `TermGetScrollbackLines` is answered by the front-end view of that block via route `MakeFeBlockRouteId(blockId)` (screen.go:136-139); front-end handler term-wsh.tsx:108-187 has no caller check.

## 8. `send`

Synopsis: `wsh send [-b <block>] [--delay <ms>] [--stdin] [--raw] <token>...` (wshcmd-send.go:26-63). Args: none enforced by cobra (no Args set) - validated in code. PreRunE connects (send.go:49).
Flags
- `--delay` int default 40: pause in ms between tokens. NO validation: negative or zero means no pause (deliverChunks only sleeps when delay > 0, send.go:146-148).
- `--stdin` bool default false: read all of stdin, `TrimRight("\r\n")`, append as ONE text token LAST (send.go:166-172). Read error: `reading stdin: <err>`.
- `--raw` bool default false: lift the control-character refusal for text tokens.
Token vocabulary (send.go:21-24, 65-97, 110-134)
- `key:<Name>`: Name trimmed and lower-cased. Table: enter `\r`, return `\r`, escape `\x1b`, esc `\x1b`, tab `\t`, backtab `\x1b[Z`, backspace `\x7f`, delete `\x1b[3~`, space ` `, up `\x1b[A`, down `\x1b[B`, right `\x1b[C`, left `\x1b[D`, home `\x1b[H`, end `\x1b[F`, pageup `\x1b[5~`, pagedown `\x1b[6~`; `ctrl+<a-z>` (single lower-case letter after lower-casing) = byte letter-'a'+1 (ctrl+c = 0x03). Anything else: `unknown key "<Name as given>"`. The prefix `key:` is case-sensitive (`Key:Enter` is literal text; `KEY:` too). No F-keys, no Insert, no Alt, no Shift, no ctrl+punctuation, no modifiers on arrows, no bracketed paste.
- `text:<text>`: literal text with the `text:` prefix stripped. Any other token (no known prefix) is literal text too. A literal text that happens to start with `key:` cannot be sent except via `text:key:...`.
- Text validation (not --raw): refuses any rune that is unicode control (C0, DEL, C1), format (Cf), line sep (Zl) or para sep (Zp): `token <n>: text contains the control or invisible character U+XXXX; use key:<Name> for keys, or --raw to send it as bytes` (send.go:99-107, 125-129). `<n>` is the 1-based token index. Keys and --raw are not checked.
- All tokens are validated and encoded BEFORE connecting? Order in sendRun (send.go:161-203): activity defer; stdin; `nothing to send: give key:<Name> or text tokens, or --stdin` if zero tokens; encodeSendTokens (errors above); resolveBlockArg (error `resolving blockid: ...`); GetMeta RPC with 2000 ms timeout (`block <id>: <err>`); view must be "term" else `block <id> is not a terminal block (view type: <view>)`; deliver.
- Empty chunks are skipped; if ALL are empty: `nothing to send: every token was empty` (send.go:155-157).
- Delivery: each chunk base64-encoded in one ControllerInput RPC with timeout 5000 ms (send.go:191-196); between chunks `time.Sleep(delay)` (not before the first). Failure mid-way: `sending input to block <id> (after <n> chunk(s)): <err>`; chunks already written stay written (not atomic).
- No cap on token count, chunk size or total bytes in the CLI; the RPC message size limit: not read.
Environment: WAVETERM_JWT; WAVETERM_BLOCKID only for non-oref -b.
stdout on success: `wrote <bytes> bytes in <chunks> chunk(s) to the controller of block <blockid>\n` (send.go:201). Scripts ignore it (Start-ResumableSession.ps1:546-547 discards output with `$null =` and `2>$null`).
Exit: 0 success; 1 error. Exit does not say whether the program accepted the keys.
Server: ControllerInputCommand decodes base64 (`error decoding input data: ...`) and `blockcontroller.SendInput(blockId, ...)` (wshserver.go:431-445) - writes to the block's controller (the pty), no window involvement. Works with any tab shown or not; requires the block's controller to exist (error text from SendInput: not read).
Focus: none; help text: "Nothing is typed into the window and the active tab is not changed" (send.go:29-31).
Authorization: none (see 7.4): any block id in the instance.
Quirks: `--delay` unvalidated; key vocabulary small and case rules asymmetric; no way to send Alt/F-keys; not atomic; no echo of whether the program consumed it.

## 9. `screen`

Synopsis: `wsh screen [-b <block>] [--last <N>] [--max-bytes <n>]` (wshcmd-screen.go:18-50). No args (cobra.NoArgs).
Flags
- `--last` int default 0: only the last N lines after trailing blank removal; 0 = everything. NO validation (negative behaves like 0: `last > 0` test, screen.go:77).
- `--max-bytes` int default 200000: when output would exceed it, the OLDEST lines are dropped. NO validation: `maxBytes <= 0` means unlimited (screen.go:94). At least the last line is always kept even if it alone exceeds the cap (screen.go:98-100).
Flow (screenRun, screen.go:119-153): resolveBlockArg (`resolving blockid: ...`) -> GetMeta (2000 ms; `error getting block metadata: <err>`) -> view must be "term" else `block <id> is not a terminal block (view type: <view>)` -> TermGetScrollbackLines RPC routed to the block's front-end view (`MakeFeBlockRouteId(blockId)`), timeout 5000 ms, with an EMPTY request (linestart 0, lineend 0, lastcommand false), error text `could not read block <id> (its terminal view must be loaded; show its tab once): <err>`.
Front end (term-wsh.tsx:108-187): if the terminal object does not exist returns an empty line list (NOT an error: `lines: []`, :112-120), so a not-yet-loaded view can print nothing with exit 0 (quirk: ambiguous with an empty screen); otherwise returns all buffer lines (the whole xterm buffer: scrollback + screen, `lineend 0` means all, :174-179), joined per logical line by bufferLinesToText (not read).
CLI shaping (screen.go:53-106): remove trailing lines that are blank after TrimSpace; keep last N; escape each line: tab kept, runes below 0x100 that are control become `\xNN` (lower-case hex, 2 digits), other control/Cf/Zl/Zp runes become `\uNNNN` (lower-case, 4 digits). NOTE: different escape format from `web text` (`<U+XXXX>`); `unicode.IsControl` covers C0, DEL and C1 (0x80-0x9f) so those print as `\x80`..`\x9f`.
Truncation: if lines were dropped, a first line is added: `[wsh screen: output limited to <maxBytes> bytes, showing the last <shown> of <total> lines]` (screen.go:103). Scripts must treat this line as a marker, not terminal text.
stdout success: lines joined with "\n" plus a final "\n"; if no text at all, prints NOTHING (no newline) (screen.go:149-151). A script reads it as an array of lines: `@(& $wsh screen -b $block --last 25 2>$null)` (Start-ResumableSession.ps1:535).
Exit: 0 / 1.
Environment: WAVETERM_JWT; WAVETERM_BLOCKID only for non-oref -b.
Focus: none; works for hidden tab provided the view was loaded once ("up to 10 tabs are kept loaded", help text screen.go:30-31; the 10 is from the help text, enforcement not read).
Authorization: none (7.4).
Quirks: the view must be loaded but an unloaded view can look like an empty screen; two escape styles; byte cap counts bytes of the ESCAPED lines; `--last` and `--max-bytes` unvalidated; cannot select a line range; wrapped lines are joined by the front end, so line counts are logical lines.

## 10. `attention info|need|urgent|clear`

Synopsis: `wsh attention <level> --reason "<why>" [-b <block>]` with level in `info`, `need`, `urgent`; `wsh attention clear [-b <block>]` (wshcmd-attention.go:18-54). Args none. PreRunE connects.
Flags: `--reason` string default "" (one shared variable `attentionReason` registered on each level command, attention.go:16,47-51); `clear` takes none.
Validation (`attention.Check`, pkg/attention/attention.go:67-89), run BEFORE resolving the block (attention.go:78-85):
- unknown level (cannot happen from the CLI): `unknown attention level "<x>" (info, need or urgent)`.
- reason with any rune in Cc, Cf, U+2028, U+2029 (checked BEFORE trimming, so a trailing newline is refused not dropped): `the reason has control or hidden characters`.
- reason trimmed; empty and level is need or urgent: `attention level "<level>" needs a reason (--reason "..."), it is shown to the owner`; empty with info is allowed (stores no reason).
- more than 120 code points: `the reason is longer than 120 code points (a letter with an accent written as two code points counts as two)`.
Target: resolveBlockArg; must be a block: `<oref> is a <otype>, attention marks a block` (attention.go:86-88, 101-103). Error from resolve: `resolving blockid: ...`.
Write: SetMeta RPC, timeout 2000 ms, on the block: `attention:level` = level, `attention:reason` = cleaned reason or nil (JSON null deletes the key) (attention.go:57-67, 73-75). Clear sets both keys to nil (attention.go:69-71). Errors: `setting the attention mark: <err>` / `clearing the attention mark: <err>`.
stdout success: `attention <level> set\n` or `attention cleared\n`. Exit 0/1. No machine-readable form.
Levels (pkg/attention/attention.go:16-27, 29-56): info rank 1 ("an icon, no motion"), need rank 2 ("an orange dot", needs reason, shown on hover), urgent rank 3 ("flashing orange outline and dot", needs reason). Meta keys `attention:level`, `attention:reason`. Marks bubble from the block to its stack tab, tile and top tab (help text attention.go:21).
Clearing rules: need and urgent clear automatically when the owner really has the tile: window has focus, the tile is the focused block, after 500 ms if focus just moved there, after 3000 ms if it was already there (frontend/app/store/attentionclear.ts:11-26, attentionschedule.ts:32-59). Info is NEVER cleared by focus; only a session's `attention clear` (attentionclear.ts:6-9). Auto clear is a server compare-and-clear on exactly (level, reason): `ClearAttentionIfCommand` (wshrpc/wshserver/ours_attention.go:19-34; wstore/ours_attention.go:26-59): a mark changed meanwhile is not erased and is not an error. `attention clear` from the CLI is an unconditional write (no compare).
Focus: none; the CLI does not touch focus. Reading focus state is a frontend concern.
Authorization: none; any link can mark or clear any block in the instance (SetMeta, 7.4). Scripts that want "only mine" must pass their own block.
Persistence: stored in block meta, so it survives restart (unlike badges).
Quirks: the same marker is also the "owner needed" signal across restarts; `clear` cannot clear only if still equal; two output strings only; `attention` with no subcommand prints help (cobra, `attentionCmd` has no Run).

## 11. `badge`

Synopsis: `wsh badge [<icon>] [--color <c>] [--priority <float>] [--clear] [--beep] [--pid <pid>] [-b <block-or-tab>]` (wshcmd-badge.go:20-43). Args: at most 1 (MaximumNArgs(1)).
Flags
- `--color` string default "": NOT validated (accepted as is; CSS colour handling in the front end, not read).
- `--priority` float default 10; when `--pid > 0` and `--priority` not given, default becomes 5 (badge.go:53-55).
- `--clear` bool: clear instead of set; icon argument ignored.
- `--beep` bool: after publishing, ask the Electron app to play the system bell: `ElectronSystemBell` on route `electron`; error `playing system bell: <err>` (badge.go:99-104).
- `--pid <int>`: watch the process and clear the badge when it exits. On Windows: error `--pid flag is not supported on Windows` (badge.go:50-52). Else sends BadgeWatchPid to the connection route (`conn:<name>` from the JWT's Conn claim, or local) (badge.go:106-121); error `watching pid: <err>`.
Target: `-b` default "this"; resolve error `resolving block: <err>` (note: different prefix from other verbs, badge.go:57-60); must be block OR tab: `badge oref must be a block or tab (got "<otype>")` (badge.go:61-63). (`-b tab` or `-b tab:2` gives a tab badge.) Checked as a seam by tools\ours\Check-Seams.ps1:73.
Icon: first argument, default `circle-small`; NOT validated (Font Awesome name in the front end; not read).
Publish: a `wps.Event_Badge` event with Scopes [oref], Data BadgeEvent{oref, clear?, badge{badgeid (new UUIDv7), icon, color, priority, pidlinked}} via EventPublish with NoResponse true (badge.go:65-97). So errors from the server are not seen (fire and forget): `publishing badge event: <err>` only for local send failure.
Server: wcore badge store (pkg/wcore/badge.go): subscribes to all badge events; accepts only block/tab orefs (:67-70); stored in MEMORY only ("not persisted and are cleared on restart", :17-19); a new badge replaces the existing one only if its priority is higher, or equal priority and larger badge id (`cmpBadge`, :77-91, :122-123) - so a LOWER-priority `badge` call is silently ignored (stdout still says `badge set`); `--clear` deletes whatever badge is there regardless of who set it; `ClearById`/`ClearAll` exist in the event type but the CLI never sets them (baseds.go:24-30).
stdout success: `badge set\n` or `badge cleared\n` via fmt.Printf (badge.go:123-127) (no CRLF wrapper). Exit 0 / 1. `badge set` is printed even when the store ignored it.
Environment: WAVETERM_JWT; WAVETERM_BLOCKID for non-oref -b. Conn from the JWT (not env).
Focus: none. Auto-clear of badges on focus: the attention text mentions "the same rule as for badges" (attentionclear.ts:8-9); the badge clear code itself: not read.
Authorization: none (any link can publish a badge for any block or tab).
Quirks: success text does not reflect store outcome; priority silently loses; memory-only (this is why the fork adds persistent tab symbols, Check-Seams.ps1:121); Windows lacks --pid; error prefix inconsistent. No caller in scripts.

## 12. `tab style`

Synopsis: `wsh tab style [--tab <ref>] [--color <c>] [--accent <c>] [--symbol <s>] [--clear]` (wshcmd-tabstyle.go:28-69). No args.
Flags
- `--tab` string default "": tab to change; default is the caller's tab (`ref = "tab"`, tabstyle.go:219-222). Accepts the full simple-id grammar (tab id, `tab:<n>`, `tab:<uuid>`, `tab`) via resolveSimpleId; help says an 8-char prefix works (tabstyle.go:43-44) while `--tab` of `web open` forbids prefixes (tabflag.go:20-22): inconsistency. Error `resolving tab: <err>`; wrong type: `"<ref>" is a <otype>, not a tab` (tabstyle.go:223-229).
- `--color` string: CSS colour name or `#RRGGBB`/`#RRGGBBAA`; stored in `tab:flagcolor`. Validation `validateColor` (wshcmd-setbg.go:66-89): hex must be 6 or 8 hex digits: `color must be in #RRGGBB or #RRGGBBAA format` / `invalid hex color: <err>`; names checked case-insensitively against the table `CssColorNames` (csscolormap.go, not read): `invalid color "<c>": must be a hex color (#RRGGBB or #RRGGBBAA) or a CSS color name`. The name is stored as TYPED (not lower-cased, tabstyle.go:98).
- `--accent` string: same colour validation; stored in `tab:accent` (whole-window accent while the tab is active; falls back to workspace accent `ws:accent`, then default green: help text tabstyle.go:33-35).
- `--symbol` string: stored in `tab:symbol`. Rules (validateTabSymbol, tabstyle.go:192-209): runes that are control, Cf (except ZWJ U+200D), Zl, Zp refused: `invalid symbol: control, bidirectional and invisible format characters are not allowed`; misplaced ZWJ: `invalid symbol: a zero width joiner is only allowed inside an emoji sequence`; an icon name (optional prefix `solid@`, `regular@`, `brands@`, `custom@`, then only a-z, 0-9, dash) up to 40 chars: `invalid symbol: icon names are limited to 40 characters`; otherwise text of at most 32 bytes and at most 2 graphemes (approximate counter, :163-190) containing none of `<>&"'` and backtick and backslash: `invalid symbol "<s>": use a Font Awesome icon name (lowercase, digits, dashes) or one or two characters such as an emoji (at most 32 bytes)`. Value trimmed first.
- `--clear` bool: removes `tab:flagcolor`, `tab:symbol`, `tab:accent` (sets all three to nil). With any of the others: `--clear cannot be combined with --color, --symbol or --accent`.
- none of the four: `nothing to set: give --color, --symbol, --accent or --clear`.
Write: SetMeta on the tab oref, timeout 2000 ms; error `setting tab style: <err>`. Order: meta built and validated first (nothing sent on a validation error), then target resolved, then written (tabstyle.go:215-233). Only the keys named are changed (partial update merges; nil deletes).
stdout success `tab style set\n`. Exit 0/1.
Environment: WAVETERM_JWT; WAVETERM_BLOCKID to resolve the default tab or a non-oref ref.
Focus: none; shown tab unchanged. Authorization: none (SetMeta any oref, 7.4); a session can restyle any tab of any workspace.
Quirks: help says 8-char prefix works but it is the "first object that starts with it, whatever its type"; colour name stored as typed; symbol rules are duplicated in front end ("matches the frontend parser", tabstyle.go:160-163) so the app must implement the same rules.

## 13. `workspace style`

Synopsis: `wsh workspace style [--workspace <id>] [--accent <c>] [--clear]` (wshcmd-workspacestyle.go:17-41). No args.
Flags
- `--workspace` string default "": workspace id; default env WAVETERM_WORKSPACEID (workspacestyle.go:68-71). Neither: `no workspace id specified (use --workspace or set WAVETERM_WORKSPACEID)`. NOT resolved through resolveSimpleId: the value is used verbatim as the workspace OID, no existence check in the CLI (a bad id fails in SetMeta: `setting workspace accent: <err>`).
- `--accent` string: colour (validateColor, as in 12); stored in meta key `ws:accent`.
- `--clear` bool: sets `ws:accent` to nil; with `--accent`: `--clear cannot be combined with --accent`. Neither: `nothing to set: give --accent or --clear`.
Write: SetMeta on `workspace:<id>`, timeout 2000 ms. stdout `workspace style set\n`. Exit 0/1.
Focus: none. Authorization: none (any workspace id, 7.4).
Used when: the active tab has no accent of its own; without any accent a workspace uses its icon colour (help, workspacestyle.go:20-21).
Quirks: workspace flag takes a raw id only (no `ws`, no ref grammar) unlike `tab style --tab`.

## 14. safetext rules (cmd/wshcmd-safetext.go and its users)

- `isUnprintableRune(r)` (safetext.go:42-45) = unicode Control (Cc) OR Cf OR Zl OR Zp OR U+FFFD OR `Variation_Selector` OR `Other_Default_Ignorable_Code_Point`.
- `safeText(s, max)` (safetext.go:21-37): per rune: if count >= max append `...` and stop; count++; unprintable -> `<U+%04X>` (upper-case hex, at least 4 digits); else the rune. Limits used: title 200 runes, url 2048 runes (safetext.go:13-14), web error text 400 runes (webtext.go:133). Applies to: `web url/info` url and title (webnav.go:122-123), `web text` error text (webtext.go:119). Not applied to: `web get` (raw), `screen` (own escaper), `send` (input only), error strings from other RPCs.
- `safeTextBlock` (webtext.go:71-84): same class but `\n` and `\t` pass through unescaped; used for the page text of `web text`.
- Server-side twin for page-written errors: `printableErrorText` (emain-webnav.ts:300-318): same classes plus lone surrogates and Default_Ignorable, replaced by `?`, cut to max chars.
- `screen` escaper (screen.go:53-68): own rules, `\xNN` / `\uNNNN` lower-case; keeps `\t`, escapes everything else in Cc/Cf/Zl/Zp; does NOT treat U+FFFD, variation selectors or default-ignorables as unprintable (differs from safetext).
- `attention` reason: Cc, Cf, U+2028, U+2029 refused (attention.go:58-65) (no Zp other than 2029, no U+FFFD).
- `send` text: control, Cf, Zl, Zp refused (send.go:99-107).
- `tab style --symbol`: same four classes but ZWJ allowed in emoji sequences (tabstyle.go:134-144).
- Quirk: FIVE slightly different "unsafe character" predicates. A redesign should have one predicate and one escape format.

## 15. Focus rules, verb by verb (what "no focus" means)

| Verb | Changes keyboard focus? | Changes shown tab? | "no focus" means |
|---|---|---|---|
| web open | yes by default: new tile requests focus in its tab layout (Focused = !noFocus, nofocus.go:30-32) | no (no tab switch call in path; hidden tab is only preloaded, ours_preload.go:30-35) | `--no-focus` sends Focused false; stack member not activated by it either |
| web get/text/shot/reload/back/forward/url/info | no (comments: webnav.ts:4, webtext.go:30-31) | no | n/a (always) |
| blockshot | no, but briefly re-parents a hidden tab's view inside the window for the capture (emain-blockshot.ts:6-9) | no (parked again, :233-239) | n/a |
| send | no ("Nothing is typed into the window and the active tab is not changed", send.go:29-31) | no | bytes go to the pty controller |
| screen | no (screen.go:21-22) | no | needs the tab loaded once |
| attention | no | no | auto-clear happens only when the owner has focus on the tile (attentionclear.ts) |
| badge | no (`--beep` plays a bell, not focus) | no | n/a |
| tab style, workspace style | no | no | n/a |
Who may change focus: wsh enforces NOTHING. The only explicit rule is in the scripts: "Only the manager changes focus or the shown tab" (Show-InWave.ps1:81-88; Test-CallerIsManager from ManagerIdentity.ps1, not read), and `group activate` (the focus-moving verb, Show-InWave.ps1:101; also `focusblock`, focusblock.go:27-51, which routes SetBlockFocus to `tab:<WAVETERM_TABID>`) only when he is idle (He-IsActive exit 7, Show-InWave.ps1:89-100). Out of this slice: `group activate` (stack.go:98-104, 167-173: "without taking keyboard focus") and `focusblock`.
Redesign: make the focus rule a property of the native app (per-caller permission), not of well-behaved scripts; keep `--no-focus` semantics as default.

## 16. Callers (grep of C:\Projects\Personal\MalfestioMaster\tools and C:\Projects\Personal\wave\waveterm\tools\ours)

Method: grepped verb names, every `$Wsh`/`$wsh`/`Invoke-Wsh`/`& wsh` use, and WAVETERM_* names. I read each hit's context. Tests excluded from "real callers" but listed.

| File:line | Verb and flags actually passed | Field or text read |
|---|---|---|
| MalfestioMaster\tools\Show-InWave.ps1:73 | `web open <url> --no-focus --stack <status block> --tab <home tab>` (exact order enforced by the test's allow-list, Test-ShowInWave.ps1:43-53) | stdout and exit code only: exit != 0 stops with the message (Show-InWave.ps1:35); the `created block ...` line is NOT parsed. New block id found by listing `blocks list --view=web --json` before and after (:72-77) |
| MalfestioMaster\tools\HomeTab.Lib.ps1:46 | `web text -b <blockid> --selector '#line'` (2>&1, Out-String, Trim) | the whole trimmed stdout is the "line"; on exit != 0 the (trimmed, whitespace-collapsed, 32+ hex runs replaced by `<hidden>`) text becomes the problem message (:47-51). Parsed later by regex `^[1-9][0-9]* modules?$` (Check-HomeTile.ps1:35, Set-HomeTile.ps1:65) |
| MalfestioMaster\tools\Check-HomeTile.ps1:32 (via Get-TileLine) | same as above | line text; relies on the partition rule being satisfied (Set-HomeTile sets web:partition) and on same-workspace |
| MalfestioMaster\tools\Set-HomeTile.ps1:64 (via Get-TileLine) | same as above | line text |
| wave\waveterm\tools\ours\Start-ResumableSession.ps1:535 | `screen -b <block> --last 25` (stderr dropped) | array of lines; matches `Accessing workspace`, `No, exit`, `Yes, I trust this folder` and the folder name with whitespace removed (:537-538). Relies on `--last` and on `screen` returning nothing when empty |
| wave\waveterm\tools\ours\Start-ResumableSession.ps1:546 | `send -b <block> key:Down` (output discarded) | exit ignored |
| wave\waveterm\tools\ours\Start-ResumableSession.ps1:547 | `send -b <block> key:Enter` | exit ignored |
| MalfestioMaster\hooks-draft\never-rules\never-rules.ps1:307 | policy regex that allow-lists `web (shot|text|info|url)`, `blockshot`, `screen` as plain reads (not a caller) | n/a; needs updating with the new CLI's verb names |
| MalfestioMaster\tools\Test-ShowInWave.ps1, Test-SetHomeTile.ps1, Test-CheckHomeTile.ps1 | stand-in `wsh` fakes matching `web open`, `web text -b X --selector #line` | fake output used as above; test seams env: SHOW_IN_WAVE_TEST_WSH, SET_HOME_TILE_TEST_WSH, CHECK_HOME_TILE_TEST_WSH |
| wave\waveterm\tools\ours\Check-Seams.ps1:72-73,95-98,121 | not calls: source-pattern checks that `wshcmd-badge.go` still has the block-or-tab line, emain still has `handle_webnav(`, attention meta keys and `<AttentionAutoClearing />` exist | n/a |

No caller exists for: `web get`, `web reload`, `web back`, `web forward`, `web url`, `web info`, `web shot`, `blockshot`, `attention *`, `badge`, `tab style`, `workspace style`. Not searched: Tether/Atlas/broker repositories and any hook outside the two directories (out of the given scope). `session start --color/--symbol` (Open-WaveSession.ps1:134-135) styles tabs through `wsh session start`, not through `tab style` (session verb not in my slice, not read).
Other wsh verbs the same scripts use, for the other slices: `blocks list`, `setmeta -b <id> key=value`, `group activate -b`, `session list|start`, `api list|goto`, `deleteblock -b --force`.

## 17. Quirk list for the redesign (collected)

1. `web get` is hidden, raw (no safetext, no cap), splices the selector into code, skips CLI authorize and the denied-text mapping, and overlaps `web text`. Cut or merge into `web text --html`.
2. Five different unsafe-character predicates and two escape formats (`<U+XXXX>` vs `\xNN`/`\uNNNN` vs `?`).
3. Same-workspace rule covers only web page reads, screenshots and nav; `send`, `screen`, `setmeta`, `attention`, `badge`, `tab style`, `workspace style`, `blocks`, `create block` are unauthorized across workspaces (7.4). Decide the intended rule once.
4. "this" and numeric refs resolve from env WAVETERM_BLOCKID (forgeable), authority is the JWT block id.
5. Web reads need a per-tile `web:partition` equal to the block id or `persist:<id>`; `web open` never sets it, so freshly opened tiles cannot be read; the planned "allow-list" does not exist (emain-webnav.ts:183-187).
6. Authorize is asked 3 times per web read (CLI, emain, emain again) plus BlockInfo; one check at the right place is enough in a native app.
7. `web open` prints a human sentence, no id in machine form; callers diff block lists.
8. `-b` is accepted and ignored by `web open`, `tab style`, `workspace style`.
9. `--tab` grammar differs between `web open` (no prefixes) and `tab style` (anything resolveSimpleId takes).
10. `send`: `--delay` unvalidated, small key table (no F-keys/Alt/Insert), case-sensitive `key:` prefix, no cap on size, not atomic, success only means bytes written.
11. `screen`: unvalidated numbers, unloaded view prints nothing and exits 0, truncation marker line mixed into output.
12. `badge`: prints `badge set` even when ignored for lower priority; memory only; `--pid` not on Windows; error prefix differs; no validation of icon or color.
13. `attention clear` is unconditional; `attention` states are stored in block meta (persist), badges are not.
14. `web text` writes a truncation note to stderr while exit 0; callers using `2>&1` (HomeTab.Lib.ps1:46) merge it into the text.
15. Page-text `<U+XXXX>` expansion makes `--max-bytes` an output cap, not a page cap.
16. `reload/back/forward` refuse file: pages although `Show-InWave` opens file: urls.
17. Exit codes are only 0 and 1; no distinct code for "denied", "not found", "refused by partition rule" - scripts parse text (HomeTab.Lib.ps1:48-51). A redesign should add stable codes or `--json` errors.
18. Tool deletes files (`web shot`/`blockshot` prune) - against the owner's no-delete-recursion spirit only in that it deletes unrequested files; at most 50 kept.

## 18. Not read (say so)
- `wshutil.DoShutdown` body, cobra's exact error printing, `CssColorNames` table (csscolormap.go), `defaultWebShotDir`/`checkShotDir`/createShotFile (wshcmd-shotfile_*.go), `blockcontroller.SendInput`, `planCreateInStack`, `checkCreateNodeSize`, `DBResolveEasyOID`, frontend `ensureUrlScheme`, frontend badge handling and `--beep` bell implementation, `ManagerIdentity.ps1`/`Test-CallerIsManager`, `He-IsActive.ps1`, wavejwt key management, `ElectronSystemBell` handler, how the page applies `Focused`, `UsingTermWshMode` setter, `wshcmd-session*.go` (other slice), `blocks list`, `setmeta`, `group activate`, Tether and Atlas repos, test files of the CLI (only referenced by name).
