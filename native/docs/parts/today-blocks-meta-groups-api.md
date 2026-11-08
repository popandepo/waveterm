# CLI contract spec, part B (today's wsh behaviour)

Root for all paths below: R = C:\Projects\Personal\wave\worktrees\feat-native-cli-spec-def31e7a. Citations are file:line relative to R unless they start with C:\.
Read-only extraction. Nothing was run. "not read" marks what I did not open.

## 0. Common behaviour (all verbs)

- Process env read: WAVETERM_JWT (cmd/wsh/cmd/wshcmd-root.go:86-89, const wshutil.WaveJwtTokenVarName), WAVETERM_BLOCKID (wshcmd-root.go:187, 209), WAVETERM_TABID (wshcmd-root.go:228; focusblock wshcmd-focusblock.go:32; createblock via wshcmd-tabflag.go:65), WAVETERM_WORKSPACEID (wshcmd-api.go:154, only api goto).
- No JWT: error text `wsh must be run inside a Wave-managed SSH session (WAVETERM_JWT not found)` (wshcmd-root.go:88). Verbs with PreRunE preRunSetupRpcClient fail with it. `api` verbs call it only when --dir is not given (wshcmd-api.go:86-91).
- Transport: unix domain socket named inside the JWT, then AuthenticateCommand with the JWT (wshcmd-root.go:168-190). If WAVETERM_BLOCKID is set it sends SetPeerInfo "domain:block:<id>" (wshcmd-root.go:187-191).
- Global flag on every verb: `-b/--block` string, default "" "for commands which require a block id" (wshcmd-root.go:260). Only verbs that call resolveBlockArg/resolveBlockArgWithOverride read it (setmeta, getmeta, deleteblock, group add/remove/activate). `blocks list`, `stack move`, `api *`, `createblock` accept it and ignore it silently (quirk).
- Exit codes: any RunE error -> stderr message, process exit 1 (wshcmd-root.go:261-264: Execute -> DoShutdown("",1,true)). Success exit 0 (WshExitCode is 0 unless a verb sets it; none of these verbs do). Cobra's own error print format ("Error: <msg>") and flag-parse errors are library behaviour, not in the read sources (not read). Usage is suppressed (SilenceUsage true on root; blocks list also sets it).
- Telemetry: setmeta, getmeta, deleteblock send a `sendActivity(<verb>)` RPC after the run (wshcmd-root.go:236-246). Others do not. Quirk for a redesign: drop it.
- stdout: WriteStdout via WrappedWriter; in "term wsh mode" every \n becomes \r\n (wshcmd-root.go:43-69, UsingTermWshMode). Where UsingTermWshMode is set: not read. `createblock` uses fmt.Printf directly (not wrapped) (wshcmd-createblock.go:66).

### Object model and ids

- Object types (ORef OType): block, tab, workspace, client, window, layout, temp ... exact list ValidOTypes: pkg/waveobj (var not read; waveobj.go:89 uses it). Names seen in code: block, tab, workspace, client (resolvers.go:87-117), "temp" (resolvers.go:117).
- ORef text form: `<otype>:<uuid>`, split on ":" into exactly 2 fields, otype must match otypeRe and be in ValidOTypes, oid must parse as a uuid (pkg/waveobj/waveobj.go:80-97). uuid.Parse accepts several spellings (urn, braces, no dashes) - quirk: ids are not forced lowercase here, but waveapi requires lowercase (see 7).
- Hierarchy (read from code): workspace.TabIds (ordered) and workspace.ActiveTabId (wshserver.go:1044; wavesrvapi.go:59,62); tab.BlockIds, tab.LayoutState (layout id) (wshserver.go:1049; wcore/layout.go:84); block.ParentORef is the tab or, for sub blocks, another block, followed up to 5 hops (wstore_dbops.go:364-391). Window -> workspace via WorkspaceId (wshserver.go:1012-1016); workspace -> window via DBFindWindowForWorkspaceId (wshserver.go:1039). Client.WindowIds, first = "current" window (wshserver.go:1019-1030).
- Layout tree: LayoutState.RootNode (decoded JSON map with "kind", "id", "children", "data":{"blockId"}); a node with kind=="stack" is a tab group; stack id = node "id" (stackblock.go:117-138). LayoutState.LeafOrder is a list of entries with BlockId, 1-indexed for block numbers (resolvers.go:187-197). LayoutState.PendingBackendActions queue is how the back end asks the page to change layout (wcore/layout.go:98-106). The stored tree can lag a change by 100 ms (stackblock.go:115-116). Full layout node type not read (pkg/waveobj/wtype.go).
- Id forms accepted by the SERVER resolver (ResolveIds, used by resolveSimpleId in the CLI, wshcmd-root.go:201-225) parse order (pkg/wshrpc/wshserver/resolvers.go:37-79):
  1. contains "@" -> discriminator before "@", value after (SplitN 2). Then the switch (resolvers.go:263-278) only knows this/oref/tabnum/blocknum/view/uuid/uuid8, so any other discriminator gives `unknown discriminator: X` (quirk: "@" form is dead except literal "this@..", "oref@..", etc.).
  2. keywords this, block, tab, ws, workspace, client, global, temp -> resolveThis (resolvers.go:82-119). this/block = the calling block (data.BlockId = WAVETERM_BLOCKID); tab = tab of the calling block; ws/workspace = workspace of the calling block; client/global = client singleton; temp = ORef{"temp", client.TempOID}.
  3. full ORef `type:uuid` -> as is, no existence check (resolveORef, resolvers.go:122-128).
  4. `tab:N` (1 to 3 digits) -> N-th tab (1-indexed) of the CALLER's workspace; out of range error `tab num out of range, workspace has %d tabs` (resolvers.go:130-164).
  5. `[a-z]+` or `[a-z]+:N` (view name, e.g. `term`, `web:2`) -> N-th block (default 1) in layout LeafOrder whose meta view equals the name; error `could not find block %d of type %s (found %d)` (resolvers.go:200-251). NOTE: checked before plain number so plain numbers never match this (regex needs letters).
  6. plain integer -> block number: N-th (1-indexed) leaf in the caller's tab LeafOrder (resolvers.go:166-198). Errors: `could not resolve block num %v, leaf order is empty`, `could not find a node in the layout matching blockNum %v`.
  7. full uuid or 8 hex chars -> DBResolveEasyOID: scans every object table; 8-char form uses `oid LIKE prefix%`, first hit of any type wins (wstore_dbops.go:240-260). Quirk: an 8-char prefix is ambiguous across types and picks the first row; the CLI --tab flag refuses it for exactly that reason (wshcmd-tabflag.go:20-22), blocks verbs do not.
  Failure: no match -> `invalid simple id format: <id>` (resolvers.go:78). Single id that errors server-side returns the error; with a CLI call the CLI then prints `resolving blockid: error resolving ids: <err>` (wshcmd-root.go:121,217-218). If the id resolves to nothing without error: `id not found: "<id>"` (wshcmd-root.go:222).
- CLI-side shortcut: if the arg already parses as a full ORef (`type:uuid`) the CLI never calls the server (wshcmd-root.go:202-208). A bare uuid is NOT a full ORef so it goes through ResolveIds. Therefore `-b <uuid>` needs WAVETERM_BLOCKID set (`no WAVETERM_BLOCKID env var set`, wshcmd-root.go:210-212) and a running Wave: a script outside a Wave block cannot address anything but `block:<uuid>` (quirk).
- Default when no -b: "this" (wshcmd-root.go:116-119). Result type is not checked by setmeta/getmeta (any ORef type works: block, tab, workspace, client...). deleteblock and group/stack check type (see per verb).
- Where "block id" in output is a plain lowercase uuid (blocks list JSON `blockid`).

## 1. blocks list  (alias: ls, get)

Source: cmd/wsh/cmd/wshcmd-blocks.go.
- Synopsis: `wsh blocks list [--window ID] [--workspace ID] [--tab ID] [--view V] [--json] [--timeout MS]` (wshcmd-blocks.go:74-79).
- Flags:
  - --window string, "" : window id. Resolved to a workspace by scanning WorkspaceListAll for ws.WindowId == value; none -> `window %s not found` (:136-138).
  - --workspace string, "" : workspace id used as is, no check it exists on the CLI side (:124-125); the server fails with its own error, which is only printed as a Warning (:155-157).
  - --window and --workspace together -> `--workspace and --window are mutually exclusive; specify only one` (:121-123).
  - --tab string, "" : filter after the fact: keep blocks where b.TabId == value (exact string equality) (:163-165). No id resolution: `tab:2`, `this`, 8-char prefix do NOT work (quirk). A nonexistent tab just yields "No blocks found".
  - --view string, "" : filter. Validation: if non-empty after trim it must be in the known list, else `unknown --view %q; try one of: term, web, preview, edit, sysinfo, waveai` (:101-105; list at :283-294: term terminal shell console web browser url preview edit sysinfo sys system waveai ai assistant, case-insensitive). Matching aliases (:259-280): preview/edit match either of preview or edit; terminal/shell/console -> term; browser/url -> web; ai/assistant -> waveai; sys/system -> sysinfo. Quirk: help text says "restrict to view type (term/terminal, web/browser, preview/edit, sysinfo, waveai)"; views like launcher, help, tips, cpuplot, processviewer, tsunami, vdom, aifilediff, waveconfig exist (blockstatus.go:39-43) but cannot be filtered. Also the --view value is compared untrimmed in matchesViewType (:171 uses blocksView, not the trimmed v) so `--view=" web"` passes validation then matches nothing.
  - --json bool, false.
  - --timeout int ms, 5000: RPC timeout for the list calls (:79,109,154).
  - -b/--block ignored.
- Env read: WAVETERM_JWT only (via PreRunE). Does not need WAVETERM_BLOCKID.
- Server calls: WorkspaceListAll (RPC WorkspaceListAllCommand, wshserver.go:1079-1081, includes unsaved scratch workspaces) then BlocksList per workspace (wshserver.go:1002-1065).
- BlocksList server semantics: with WorkspaceId, only it; else with WindowId, that window's workspace; else (never from this CLI, which always passes a WorkspaceId) first window of client. Iterates workspace.TabIds in order, tab.BlockIds in order; any DB read error aborts the whole workspace (`return nil, err`). Only top-level blocks of tab.BlockIds are listed, no sub-blocks.
- stdout on success:
  - JSON (--json): `json.MarshalIndent(array, "  ")` plus "\n". Array element (BlockDetails, :30-36): `{"blockid": string(uuid), "workspaceid": string, "tabid": string, "view": string, "meta": object}`. `view` = meta["view"] string or "". `meta` is the block's full meta map as stored (every key: cmd, cmd:args, cmd:env, url, session:*, frame:title, web:partition, ... values typed as stored JSON: strings, numbers (float64), bools, arrays, objects). NOTE no `windowid` field (BlocksListEntry has it, BlockDetails drops it, :177-183). Quirk: meta can hold secrets (cmd:env, ai:apitoken, urls with tokens); the whole map is printed (waveapi, by contrast, whitelists keys: service.go:98-100).
  - Order: stable sort by workspaceid, then tabid, then blockid (all string compare, by uuid text) (:197-205). So order is NOT tab order or layout order (quirk: loses tab order).
  - Empty result (no blocks after filtering, at least one workspace call succeeded): even with --json prints the text `No blocks found\n` and exits 0 (:188-194). Quirk: --json callers get non-JSON on empty; Show-InWave.ps1:41 and HomeTab.Lib.ps1:7 guard with `if (-not $Text.Trim())` but "No blocks found" is non-empty and would make ConvertFrom-Json throw (not handled by them; they rely on the Home tab always having web blocks). Redesign: always emit `[]`.
  - Table (no --json): tabwriter, minwidth 0, tabwidth 0, padding 2, header `BLOCK ID\tWORKSPACE\tTAB ID\tVIEW\tCONTENT` then rows; ids longer than 36 chars cut to 34 + ".." (uuids are 36 so unchanged); view lowercased or `<unknown>`; CONTENT: preview/edit -> meta file (default `<no file>`), web -> url (`<no url>`), term -> cmd:cwd (`<no cwd>`), else empty (:216-253). Table is meant for humans; scripts here use --json.
- Failures (stderr message, exit 1):
  - `failed to list workspaces: %v` (:111); `no workspaces found` (:115); mutual exclusion; `window %s not found`; unknown --view; `failed to list blocks from all %d workspace(s)` when no block collected AND every workspace call failed (:190).
  - Partial failure: `Warning: couldn't list blocks for workspace %s: %v\n` on stderr, exit 0 if some other workspace succeeded (:155-157). Quirk: stderr noise on success; Health-Check discards stderr with 2>$null.
  - No JWT: see section 0.
- Side effects: none. Focus: none.
- Authorization: none beyond the JWT leaf identity; any block in any workspace is listed (no scoping to the caller's workspace; callers filter with `workspaceid -eq $env:WAVETERM_WORKSPACEID` themselves, Wave-Sessions.ps1:242).
- Quirks to fix: --json empty output; --tab accepts only a full tab uuid; sort order is not tab order; no windowid; no way to filter by workspace name; secrets in meta; `blocks` group is hidden behind alias names ls/get; default is ALL workspaces (docs of BlocksList say "current workspace by default", server comment :1000-1001, but the CLI default is all).

## 2. deleteblock

Source: cmd/wsh/cmd/wshcmd-deleteblock.go; server wshserver.go:585-614; ours_deleteblock.go; pkg/blockstatus/blockstatus.go.
- Synopsis: `wsh deleteblock [block-ref] [-b block] [--force]` (:15,26). Args: at most 1 positional (cobra.MaximumNArgs(1), :18).
- Ref resolution: positional wins over -b, else -b, else "this" (resolveBlockArgWithOverride, wshcmd-root.go:127-140). Any id form from section 0.
- Flags: --force bool false: "delete even if a program is running in the block (or its state cannot be read)" (:26).
- Env: WAVETERM_JWT; WAVETERM_BLOCKID unless the ref is `block:<uuid>`.
- Validation (CLI): resolved ORef type must be "block" else `object reference is not a block` (:49-51). Quirk: `deleteblock` with no args and no -b deletes the CALLING block (default "this"), i.e. the block the script runs in, with --force even a live shell: dangerous default.
- RPC: DeleteBlock {blockid, refuserunning: !force}, 2000 ms timeout (:52; types wshrpctypes.go:375-379). Quirk: 2 s timeout is short for a delete that stops a program.
- Server checks (wshserver.go:585-614), in order: blockid non-empty (`blockid is required`); tab found via DBFindTabForBlockId (`error finding tab for block: ...`, `no tab found for block`); if RefuseRunning: deleteRefusalForBlock -> error `not deleted: <reason>`; then wcore.DeleteBlock(ctx, id, true); then queue a layout action "delete" for that tab; send update events.
- Refusal rules (blockstatus.go:36-63, ours_deleteblock.go:25-33): view in noProgramViews {preview, web, waveai, cpuplot, sysinfo, tips, help, launcher, aifilediff, waveconfig, processviewer} -> allowed. View in programViews {term, tsunami, vdom}: allowed only when controller ShellProcStatus == "done"; otherwise refused with `a program may still be running in the block (state "<status>"); deleting it stops the program and cannot be undone: use --force to delete it anyway`; no status (haveStatus false or controller missing: status "" so the state string is empty -> refused, deleteStatusOf returns "", true when no controller, ours_deleteblock.go:16-22, so message shows state ""). Unknown view -> `the view "<v>" is not known to be free of programs; deleting it may stop a program and cannot be undone: use --force to delete it anyway`. Unreadable block -> `the block could not be read before it was deleted; use --force to delete it anyway`. A block that does not exist: DBGet returns nil -> same unreadable message under RefuseRunning; with --force the earlier DBFindTabForBlockId error arrives first (`error finding tab for block: bad block parent oref: ...`) (quirk: message is mechanical, no "block not found").
- Side effects: block row deleted (filestore zone deleted async, wstore_dbops.go:285-297); block controller teardown is inside wcore.DeleteBlock (not read: pkg/wcore/block.go); layout "delete" action queued for the page of the tab (wshserver.go:607-610) so the tile vanishes only when the page applies it (if the tab is not loaded it is applied later).  Sub-blocks of the block: not read.
- stdout: `block deleted\n` (:56). stderr failure: `delete block failed: <server message>` exit 1 (:54). Also earlier: `resolving blockid: ...`.
- Focus: not changed by the call itself; after the page applies the delete the layout may move focus to another tile (page behaviour, frontend not read).
- Authorization: none beyond JWT leaf identity; any block of any workspace can be deleted (e.g. Wave-Sessions.ps1:250 relies on its own workspace filter and "never own block" check).
- Quirks: default target "this"; text-only output with no id echo; --force name differs from waveapi which has no force; RefuseRunning is a client-set flag (a direct RPC caller can omit it - the guard is advisory, not authorization); message says "use --force" even for RPC callers.

## 3. setmeta

Source: cmd/wsh/cmd/wshcmd-setmeta.go; server wshserver.go:265-274; pkg/wstore/wstore.go:57-75; pkg/waveobj/wtypemeta.go:189-229.
- Synopsis: `wsh setmeta [-b {blockid|blocknum|this}] [--json file.json|-] key=value ...` (:20). Args: any number (MinimumNArgs(0)).
- Flags: --json string "" : path of a JSON file, or `-` for stdin; must be an object (:34-60). Errors `reading from stdin: %v`, `reading JSON file: %v`, `parsing JSON file: %v`, `JSON file must contain an object, not null`.
- Positional `key=value`: split at first "=", missing "=" -> `invalid meta set: "<arg>"` (:131-134). KEY is split on "/" into a nested path (so `a/b=1` sets meta["a"]["b"]) (:142-143, :97-126; a non-map value on the path is silently replaced by a map). Quirk: meta keys that contain "/" cannot be set (e.g. a URL-like key). Keys with ":" are normal flat keys (cmd:runonstart).
- Value parsing (parseMetaValue :62-95), in order: "" or "null" -> JSON null (means DELETE the key); "true"/"false" -> bool; first char `[` `{` or `"` -> json.Unmarshal (error `invalid json value: %v`); else ParseInt(base 0, so 0x10, 0o7, 010 octal!, 1_000 underscore) -> int64; else ParseFloat -> float64; else the literal string. Quirks to fix: "010" becomes 8, "0x1F" becomes 31, "1e3" becomes float 1000, "inf"/"nan"/"Infinity" parse as floats, a string like "123456" cannot be stored as string (session:id numeric-looking names, frame:title=2026 becomes a number), a string starting with `[`, `{` or `"` must be valid JSON, `frame:title=true` becomes bool, empty string cannot be set (it deletes).
- Merge of --json and args: if the JSON file had keys, args override on same key via simpleMergeMeta (null in args deletes the key from the file map, then the file key is gone) (:148-157, :178-183). If nothing: `no metadata keys specified` (:184-186).
- Ref: only -b / default "this" (resolveBlockArg :115-125). Positional args are ALL treated as key=value, no positional ref. ORef type not checked (block, tab, workspace, client all work).
- RPC: SetMeta {oref, meta}, 2000 ms (:192-199).
- Server (wshserver.go:265-274): logs `SetMetaCommand: <oref> | <meta>` (log leaks values incl. url tokens, quirk), UpdateObjectMeta(ctx, oref, meta, mergeSpecial=false) in one DB transaction (wstore.go:57-75): empty oref -> `empty object reference`; object missing -> ErrNotFound (surfaced as `error updating object meta: <ErrNotFound text>`, text not read). MergeMeta (wtypemeta.go:189-229): first, any update key ending ":*" with a true bool value deletes the base key and every key with prefix "base:" (so `cmd:*=true` clears all cmd keys); then, regular keys: keys starting "display:" are SKIPPED silently (mergeSpecial false, :216-218); keys ending ":*" skipped; value nil -> delete; else set. No type check, no key allow-list, no value size limit on the server: any key and any JSON value is stored. Then SendWaveObjUpdate (pushes the object update to pages/subscribers).
- stdout: `metadata set\n` (:200). Failures: `setting metadata: <err>` exit 1; `resolving blockid: ...`; parse errors above.
- Side effects: meta changes are live: a web block reloads when `url` changes; changing cmd/cmd:runonstart affects the next controller start; frame:title changes the tile title. Per key behaviour: see section 8.
- Focus / shown tab: none changed by the call. (Setting meta on a block does not activate it.)
- Authorization: none per key or per object; a block can write meta of ANY object by `block:`/`tab:`/`workspace:` ORef (full ORef bypasses WAVETERM_BLOCKID entirely). Callers constrain themselves (Test-SetHomeTile.ps1:30-43 allow-list).
- Quirks: not atomic across keys when called repeatedly (callers issue one setmeta per key, e.g. Set-HomeTile.ps1:91-93: after a failure the tile is half set, test at Test-SetHomeTile.ps1:103); `--json -` consumes stdin; "/" nesting; numeric string coercion; delete by empty value; silent skip of display:* keys; no echo of the stored value; ":*" clearing keys documented nowhere in help.

## 4. getmeta

Source: cmd/wsh/cmd/wshcmd-getmeta.go; server wshserver.go:148-157.
- Synopsis: `wsh getmeta [-b block] [-v] [--raw] [--clear-prefix] [key...]` (:18-34). Args arbitrary.
- Flags: -v/--verbose bool false: prints `resolved-id: <oref>` to STDERR (:83-85); --raw bool false: if the output is a single string, print it without JSON quotes (:106-111); --clear-prefix bool false: for each `prefix:*` key adds `"prefix:*": true` to the result object (:47-49).
- Ref: -b or "this"; any ORef type.
- Server: GetMeta {oref}, 2000 ms; `error getting object: %w`; `object not found: <oref>`; returns the full meta map (waveobj.GetMeta(obj)) (:148-157).
- stdout (always JSON via MarshalIndent 2 spaces + "\n", except --raw string):
  - no keys: the whole meta object.
  - exactly one key not ending ":*": the VALUE alone (JSON of the value: `"text"` quoted, `true`, `12`, an object, or `null` if missing) (:93-96). Missing key = `null` and exit 0 (quirk: cannot tell missing from null; no exit code).
  - 2+ keys, or any key ending ":*": an object. Exact key present -> value, absent -> null (`result[key]=nil`) (:63-69). Pattern `a:*` -> includes key `a` (if exists) plus all keys with prefix `a:` (:44-60).
  - --raw with a non-string output (number, bool, null, object) still prints JSON. --raw of a missing key prints `null`, not empty.
  - A numeric meta value comes out as float formatting from encoding/json (e.g. 1700000000 prints as 1700000000, large ints may print in exponent form for float64 > 1e21).
- Failure: `getting metadata: <err>` exit 1; `resolving blockid: ...`; `formatting metadata: ...`.
- Side effects/focus: none. Authorization: none; reads any object including other workspaces and client meta (api tokens in `ai:apitoken` are readable).
- Callers: none found in MalfestioMaster\tools or tools\ours (grep `getmeta`: only Check-Seams style mentions of setmeta; no getmeta hit). Quirk: no consumer, so safe to redesign (e.g. `meta get KEY` with exit code 1 when missing).

## 5. group add / group remove / group activate  and  stack move

Source: cmd/wsh/cmd/wshcmd-stack.go; server pkg/wshrpc/wshserver/stackblock.go; actions in pkg/wcore/layout.go:17-31; consts pkg/wshrpc/wshrpctypes_const.go:54-57.
- Terminology: "group" = "stack" = a tile that shows one member block at a time with a tab strip (wshcmd-stack.go:77-79). Members are normal blocks in the same tab.
- Synopses: `wsh group add -b <block> --to <block> [--focus]` (:82-88, 107-109); `wsh group remove -b <block> [--focus]` (:90-96); `wsh group activate -b <block>` (:98-104; NO --focus flag, quirk: Test-ShowInWave.ps1:54 asserts that a caller passing --focus to activate is refused by its allow-list, and cobra would reject unknown flag --focus); `wsh stack move --from <block|group> --to <block|group> [--focus]` (:186-203).
- Flags: --to string "" (add): "the block whose group to join (block id or number)" - required: error `--to is required: the block whose group to join` (:140-142). --focus bool false (add, remove, stack move). stack move: --from string "", --to string "", both required: `both --from and --to are required` (:208-210); same block: `--from and --to are the same block` (:219-221). Note groupFocus var is shared between add and remove flags (one process runs one verb, harmless).
- Ref resolution: the subject of group add/remove/activate is -b or "this" (groupBlockId -> resolveBlockArg, :128-137; `.OID` taken WITHOUT checking the type: a `tab:<uuid>` passed as -b becomes a blockid that is a tab id, the server's DBFindTabForBlockId then fails with `block <id>: bad block parent oref...` (quirk)). Targets (--to, --from, stack move --to) go through resolveBlockRef -> resolveSimpleId then blockIdOfORef: must be type block else `<otype>:<oid> is not a block; groups work between blocks of the same tab (a tab as target is not supported yet)` (:34-47). Error prefix `resolving --to: ...` / `resolving --from: ...`.
- Env: JWT; WAVETERM_BLOCKID unless full `block:<uuid>` ORefs.
- RPC: StackBlock {blockid, targetblockid, action, focused}, default opts (no explicit timeout, :29). action values: "stack" (add), "unstack" (remove), "activate", "move" (stack move) (wshrpctypes_const.go:54-57).
- Server planStackBlock (stackblock.go:20-87), checks in order:
  1. blockid non-empty: `a block id is required`.
  2. find tab of block: `block <id>: <err>`.
  3. stack/move: target required (`a target block is required to stack`), target != block (`a block cannot be stacked with itself`), target's tab found (`target block <id>: <err>`), same tab (`both blocks must be in the same tab to stack them`), not already in the same stack (reads the STORED layout tree, may lag 100 ms): `both blocks are already in the same group`.
  4. unstack/activate: block must be a member of a stack in the stored layout tree, else `block <id> is not in a group`. So `group remove` and `group activate` fail on a block alone in its tile. A just-created block not yet saved to the tree fails too (race, quirk).
  5. else `unknown stack action %q (stack, unstack or activate)`.
  Then wcore.QueueLayoutActionForTab(tab, action) with actions stack, stackmove, unstack, activate; focused passed for stack, stackmove, unstack, NOT for activate (stackblock.go:56-84; activate has no Focused). Then sends object update events.
- Effect semantics from help text (execution is in the page, frontend not read): stack: block joins the group of the target (or a new group of the two is made if target is a plain block); unstack: block leaves the group and is placed next to the group; activate: makes that member the shown tab of the group; stack move: moves ALL members of FROM's group (in order) to the end of TO's group (new group if TO is a plain block) (wshcmd-stack.go:189-193).
- stdout on success (all four): `queued: <action> <blockid> (applied when the tab is open; not confirmed)\n` where action is the RPC action word (stack, unstack, activate, move) and blockid is the SUBJECT block (for stack move it is the --from block id) (:116-125). Parse: `^queued: (stack|unstack|activate|move) ([0-9a-f-]{36})`. Success only means "queued" - the CLI cannot learn whether the page applied it (quirk to fix: a native app can apply synchronously and return the result and the resulting layout).
- Failures: stderr message from the list above, exit 1.
- Focus / shown tab: Does NOT change the active workspace tab. `group activate` changes which member is shown in the group; callers found that it CAN move keyboard focus into the web view when that tile's tab is the one he is typing in (Show-InWave.ps1:5-11, 89-90: "group activate moves keyboard focus into the web view"). `--focus` explicitly requests keyboard focus; without it a layout change "never takes keyboard focus unless --focus is given" (wshcmd-stack.go:78-79, wshrpctypes.go:328-329) but activate does anyway according to the caller note (the contradiction is a quirk; frontend not read to resolve it).
- Authorization: none; any block can restructure any tab by blockid (needs only ids from the same tab).
- Quirks: group add to a block in another tab refused with message; stack target "tab as target not supported yet"; CLI error for non-block type differs from server error; success message says "queued" so scripts cannot detect failure (Restore-Sessions.ps1:124-125 only checks exit code); `-b` global flag ignored by stack move; `group add` does not accept both blocks by `tab:N`.

## 6. createblock (hidden verb)

Source: cmd/wsh/cmd/wshcmd-createblock.go; helper flags wshcmd-tabflag.go, wshcmd-sizeflag.go, wshcmd-nofocus.go, wshcmd-stack.go:49-72; server wshserver.go:312-408; ours_nodesize.go; stackblock.go:89-112.
- Synopsis: `wsh createblock <viewname> [key=value ...] [--tab REF] [--size N] [--stack BLOCK] [-m] [--no-focus]` (Hidden: true, :16-23). Args: MinimumNArgs(1).
- Positional: viewname -> meta["view"] (set AFTER the key=values so it overrides a `view=` given there, :47); the rest parsed exactly like setmeta key=value (parseMetaSets, same value coercion, "/" nesting, null means key set to nil in the block def) (:43).
- Flags:
  - --tab string, "": reference of the tab to create in: allowed forms `tab` (the caller's own tab), `tab:<n>` (1-3 digits, n-th tab of caller's workspace), a full tab uuid with or without a `tab:` prefix (case-insensitive hex); an empty value is an error; anything else (8-char prefix) refused `--tab "<ref>" is not accepted: use a full tab id, tab:<n> or tab (an id prefix can match more than one object)` (wshcmd-tabflag.go:22-61). Empty: `--tab needs a tab: a full tab id, tab:<n> or tab`. Resolution via resolveTabId (the seam resolveTabFn; function body not read: it is in a file I did not open, expected to call ResolveIds). Without --tab: WAVETERM_TABID, or error `no WAVETERM_TABID env var set` (:44-47). Resolve error prefix `resolving --tab: ...`.
  - --size uint, 0 (=not given, tracked by Changed): weight 2 to 50; else `--size must be between 2 and 50, got N` (wshcmd-sizeflag.go:29-37). Has no effect with --replace/--stack/ephemeral (help text :24). Server re-checks: `nodesize must be between 2 and 50 (leave it out for the default), got N` and refuses nodesize with ephemeral (ours_nodesize.go:20-37).
  - --stack string "": block id or number whose group the new block joins (wshcmd-stack.go:49-72): not with --magnified (`--stack cannot be combined with --magnified`); resolved to a block (non-block type error as in section 5); sets TargetBlockId and TargetAction "stack". `--replace` is mentioned in the code but no --replace flag exists on this verb (data.TargetBlockId is never set before applyStackTarget, so the "--stack cannot be combined with --replace" check can never fire here).
  - -m/--magnified bool false: create magnified.
  - --no-focus bool false: Focused = !noFocus, so the DEFAULT is Focused=true: the new block takes focus (wshcmd-nofocus.go:29-32; wshcmd-createblock.go:54).
  - -b ignored.
- With --tab given: if a target (--stack) exists, its tab (BlockInfo RPC) must equal the chosen tab: `the target block is in tab %s, not in tab %s: --tab needs the stack or the block to replace to be in that tab` (wshcmd-tabflag.go:70-101). Without --tab that check is skipped on the CLI; the server still checks (below).
- RPC: CreateBlock {tabid, blockdef{meta}, magnified, focused, targetblockid, targetaction, nodesize}, default opts (wshrpctypes.go:314-326). Fields never set by this CLI: rtopts, ephemeral, other target actions (replace, splitleft/right/up/down) are server-supported but unreachable here.
- Server (wshserver.go:312-408): checkCreateNodeSize; if action stack or (replace with target): target must be in the same tab (`target block <id>: ...`, `the target block must be in the same tab as the new block`, `a target block is required to create a block in a stack`); wcore.CreateBlock(tab, blockdef, rtopts) creates the DB block (errors `error creating block: ...`); then builds a layout action: no target -> "insert" (with Magnified, Ephemeral, Focused, NodeSize); stack -> "stackinsert" with Focused; queues it for the tab (`error queuing layout action: ...`), sends updates, and calls requestTabPageLoad(tabId, blockId) in the background (ours_preload.go:233-235) which asks Electron to load the tab's page so the block starts without being shown (see ours_preload.go; the mechanics not read in full). Because the block is created in the DB BEFORE the layout action is validated, a failure after CreateBlock (e.g. `invalid target action`) leaves an orphan block: quirk (waveapi open has code to take it back, handler.go:45-48 `blockid names a block a failed call made and could not take back`).
- Does the block start its program? Controller start for term/cmd blocks is by the page when it loads the block (requestTabPageLoad + `cmd:runonstart`); exact condition not read (blockcontroller). 
- stdout: `created block <uuid>\n` via fmt.Printf (not through the wrapped writer) (:66). Parse: `^created block ([0-9a-f-]{36})$`. Failure: `create block failed: <err>` (:64) exit 1, plus the validation messages above.
- Focus / shown tab: with default Focused=true the new block takes focus in its tab; creating in a non-active tab with --tab does not change the active workspace tab per the comment in CreateTabCommand style (not stated for CreateBlock; frontend not read). --no-focus avoids taking focus.
- Authorization: none; any tab by full uuid, any workspace/window (`--tab` help: "any workspace or window").
- Callers: none call `createblock` directly (grep). Show-InWave.ps1:73 uses `wsh web open ... --no-focus --stack ... --tab` which is not in my slice (not read) and presumably goes through the same CreateBlock RPC. Restore/Open-WaveSession use `session start` (not read). Quirk: output format differs from every other verb (plain text, not JSON); redesign should return JSON {blockid, tabid}.

## 7. api list / api goto / api focus  and the local API (pkg/waveapi)

Sources: cmd/wsh/cmd/wshcmd-api.go; pkg/waveapi/{handler,service,client,server,token}.go; pkg/waveapi/wavesrvapi/wavesrvapi.go. Not read: open.go, send.go, acl_*.go, *_test.go.
- Transport: HTTP/1.1 over a unix socket `<wave data dir>/api/wave-api.sock` (SocketFileName token.go:16; dir name "api" wavesrvapi.go:31; wshcmd-api.go:19). Auth: header `Authorization: Bearer <token>`; token = 32 random bytes hex (64 chars) read from file `<dir>/api-token` (token.go:15-27, 59-65); a NEW token is written at every wavesrv start (server.go:49-52), so a token cached across a Wave restart is invalid. Token compared as sha256 digests in constant time (handler.go:59-67). Missing/wrong -> 401 `{"error":"unauthorized"}`; non-POST -> 405 `use POST` with Allow header (handler.go:252-262). Auth check comes BEFORE method check and routing. The folder and socket are locked to the current OS user (ACL) and checked at start (server.go:52-79; acl_windows.go not read). Socket path max 100 characters: `socket path is too long (%d characters, the limit is %d)` (server.go:18,57-59). Server timeouts: read header 5 s, read 15 s, write 15 s, idle 30 s, max header 64 KB (server.go:19-24). Client timeout 10 s per call, new connection each call (client.go:41-49). Body limit 1 MiB: 413 `body too large` (handler.go:21,104-116). Client reads at most 8 MiB response (client.go:60).
- Request strictness (strictDecode, handler.go:122-159): body must be exactly one JSON object, unknown keys rejected, duplicate keys rejected, case-exact keys. Errors are 400 `{"error": "..."}` e.g. `body is not a json object`, `unknown key "x"`, `key "x" appears twice`, `body has more than one json value`, `body does not fit the call: ...`. List accepts an empty body or `{}`.
- Endpoints in my slice (all POST, all JSON responses with Content-Type application/json):
  - /v1/list body none. 200 `{"workspaces":[Workspace...]}` (handler.go:175-193).
  - /v1/focus body {workspaceid?, tabid?, blockid?}, 200 `{"ok":true}` (handler.go:194-204).
  - /v1/goto body {workspaceid, tabid, blockid?, window?:bool}, 200 `{"ok":true}` (handler.go:205-215).
  - Others in the same server (outside my slice, listed only): /v1/screen {blockid} -> {"lines":[..]}, /v1/send {blockid, expectsession, text}, /v1/open (openKeys in open.go not read), handler.go:216-251.
- Error mapping (handler.go:75-88, errors service.go:19-26): ErrBadId -> 400; ErrNotFound -> 404; ErrRefusedText/ErrBadUrl/ErrBadTitle -> 400; ErrMismatch/ErrNotActive/ErrWrongSession/ErrNotRunning/ErrBusy/ErrOnPrompt/ErrNoInputBox/ErrNotSent/ErrNotLoaded -> 409; anything else -> 500 with the text hidden as `{"error":"internal error"}` and logged. The CLI turns any non-200 into the error `wave api <status>: <error text>` (client.go:64-68). Error texts: `bad id: missing`, `bad id: "<id up to 64 chars>..." is not a lowercase uuid`, `bad id: give a block id or a tab id`, `not found: block <id>`, `not found: tab <id>`, `not found: workspace <id>`, `address parts do not belong together` (plain ErrMismatch for focus), `address parts do not belong together: tab <t> is not in workspace <w>`, `...: block <b> is not in tab <t>`, `the tab of this block is not the active tab; use goto` (ErrNotActive).
- JSON shapes (service.go:32-65), all keys lowercase:
  - Workspace: {"workspaceid": string, "windowid": string, "name": string, "activetabid": string, "tabs": [Tab]} (no omitempty; tabs is [] never null, wavesrvapi.go:60).
  - Tab: {"tabid": string, "name": string, "blocks": [Block]} (blocks [] never null, wavesrvapi.go:67).
  - Block: {"blockid": string, "view"?: string, "slot"?: string, "controller"?: string, "connection"?: string, "title"?: string, "url"?: string, "status"?: string, "exitcode"?: int, "sessionid"?: string} all but blockid omitted when empty. Sources: view<-meta view; slot<-meta slot; controller<-meta controller; connection<-meta connection; title<-meta frame:title; sessionid<-meta session:id only if it matches lowercase uuid `^[0-9a-f]{8}-...$`; url<-meta url cut to scheme://host[:port] (path, query dropped; no host (about:blank, file:) -> omitted) (service.go:101-119); status/exitcode from the block controller: running | exited (exitcode set) | notstarted; omitted when the block has no controller or unknown state (blockstatus.go:21-34; wavesrvapi.go:85-91). exitcode is a pointer so 0 is distinct from absent.
  - Workspace order: wcore.ListAllWorkspaces order (includes unsaved); tab order = workspace.TabIds; block order = tab.BlockIds (creation order, not layout order) (wavesrvapi.go:45-77). Any read failure aborts the whole list with 500.
  - Whole tree is built in one call, no pagination.
- Note what is NOT in the API JSON: tab ids are uuids; no layout, no stack/group ids, no stackid, no tile position, no cwd, no meta, no windows list.

### 7a. wsh api list
- Synopsis: `wsh api list [--json] [--dir D]` (:39-44, 73-74). Flags: --json bool false; --dir string "" (persistent on `api`): api folder; default `<wave data dir>/api` where data dir comes from RPC Path {pathtype:"data"} (needs the JWT + a live wavesrv) (:85-97). With --dir the JWT is NOT needed (a script can talk to the API from outside Wave if it knows the folder).
- Args: none (cobra.NoArgs).
- stdout JSON (--json): MarshalIndent 2 spaces of the array `[Workspace...]` + "\n" (note: the CLI prints the bare array, while the HTTP body is {"workspaces":[...]} - the CLI unwraps, client.go:75-81). Empty workspace list prints `null`? tree is `resp.Workspaces` nil slice -> JSON `null` if wavesrv returned none (a nil slice marshals as null in the CLI; server side `rtn` is a nil slice when zero workspaces, so `{"workspaces":null}`) (quirk, derived from wavesrvapi.go:49 `var rtn`; low likelihood).
- stdout text (default): per workspace `workspace <wsid> "<name>" window <windowid>\n`, per tab `  tab <tabid> "<name>"[ (active)]\n`, per block `    block <blockid> <view> slot=<slot> "<title>"[ <status>[(<exit>)]]\n` (%q quoting of name/title: Go-escaped, quotes added) (:124-136, 169-178). Not meant to be parsed.
- Failures: stderr message, exit 1: JWT missing (no --dir); `finding the wave data directory: %w`; `reading token file: ...`; `calling wave api: ...` (socket down); `wave api 401: unauthorized`; etc.
- Side effects/focus: none. Authorization: token holder sees ALL workspaces (no per-workspace scoping).
- Callers: Wave-Sessions.ps1:130 (`api list --json`, reads `workspaceid`, `activetabid`).

### 7b. wsh api focus
- Synopsis: `wsh api focus (--block ID | --tab ID)` (:46-51). Flags (local, NOT the global -b; this `--block` shadows the persistent root `-b/--block`? the root flag is `block`/`b` and this local flag is `block` without shorthand: both named --block; cobra resolves the local one first, quirk: ambiguity not verified): --block string "", --tab string "". Neither -> `give --block or --tab` (:141-143). Both allowed (they must agree).
- Client calls /v1/focus with {tabid, blockid} (workspaceid never sent).
- Service.Focus (service.go:177-220): ids must be LOWERCASE full uuids (`checkId`, regexp service.go:30); with a block: block must exist in the tree (`not found: block <id>`); if tabid given it must equal the block's tab, else ErrMismatch; the block's tab must be the ACTIVE tab of its workspace else ErrNotActive (409 `the tab of this block is not the active tab; use goto`); then SetBlockFocus RPC routed to the tab page (`tab:<tabid>` route, 3 s) (wavesrvapi.go:106-109). With a tab only: tab must exist, workspace must match if given; if it is already the active tab -> success, nothing done; else SetActiveTab (Electron RPC ElectronSetActiveTab) (wavesrvapi.go:101-104).
- stdout: nothing on success, exit 0. stderr on failure, exit 1.
- Focus / shown tab: `--tab` CHANGES the shown (active) tab of that workspace, never the window or workspace; `--block` moves focus within the already-active tab only. Comment: "Focus never switches workspace or window" (service.go:177-178).
- Authorization: token only.
- No callers found.

### 7c. wsh api goto
- Synopsis: `wsh api goto --tab ID [--workspace ID] [--block ID] [--window] [--dir D]` (:53-61). Flags: --workspace string "" (default env WAVETERM_WORKSPACEID); --tab string "" (required); --block string ""; --window bool false: also raise the OS window.
- CLI validation: workspace empty and env empty -> `no workspace: give --workspace (WAVETERM_WORKSPACEID is not set)` (:156-158); tab empty -> `no tab: give --tab` (:159-161). Order: workspace check first.
- Service.Goto (service.go:224-262): workspaceid and tabid required lowercase uuids (`bad id: missing`), blockid optional; whole address checked against one tree snapshot BEFORE any change (workspace exists `not found: workspace`, tab in workspace `address parts do not belong together: tab .. is not in workspace ..`, block in tab); then moveFocus: if window: FocusWindow(windowid) (Electron RPC); if the active tab differs: SetActiveTab (Electron RPC); if blockid: SetBlockFocus via `tab:<tabid>` route. Errors from the Electron RPCs return as 500 `internal error` (text hidden), partial progress is possible (window raised, tab not switched).
- stdout nothing on success, exit 0.
- Focus / shown tab: CHANGES the active tab of the workspace (and the focused block if --block); raises the OS window ONLY with --window. Race: not atomic (check, then 2-3 separate RPCs).
- Authorization: token only. Callers gate it themselves with a "he is idle" probe and a manager check (Wave-Sessions.ps1:165-178).
- Callers: Wave-Sessions.ps1:270 (`api goto --tab <TabId>` with no --workspace, no --window; relies on env WAVETERM_WORKSPACEID).
- Quirks: ids must be lowercase uuids (an uppercase uuid is 400); `api focus` and `api goto` do different things on the same flags; wsh `focusblock` (not in slice, wshcmd-focusblock.go) is a third path (routes to tab:<WAVETERM_TABID>, no active tab check); text list is not parseable; JSON list is a bare array while HTTP is wrapped; no group/stack info in the API so callers use `blocks list` + `session list` (not read) for that.

## 8. Meta keys written/read by our tools

Type column is the JSON type stored. "w" = who writes, "r" = who reads, as found in grep. Keys beyond these in a block meta exist (cmd, cmd:args, cmd:cwd, cmd:env, controller, connection, view, file, term:*, ...).

| key | type | meaning | evidence |
|---|---|---|---|
| view | string | block view name: term, web, preview, sysinfo, waveai, tsunami, vdom, launcher, help, tips, cpuplot, aifilediff, waveconfig, processviewer. createblock forces it to the first arg | wshcmd-createblock.go:47; blockstatus.go:39-43 |
| controller | string | block controller kind ("shell" for terminals) | wcore/layout.go:46; waveapi service.go:106 |
| connection | string | connection name (ssh/wsl), empty = local | waveobj/wtypemeta.go:20; service.go:107 |
| url | string | web block address; changing it navigates the block. waveapi shows scheme://host only | wtypemeta.go:18; service.go:113-117; Set-HomeTile.ps1:92 |
| frame:title | string | tile title text. Written by Show-InWave.ps1:80, Set-HomeTile.ps1:76,93; read by waveapi Title and staterecord | wtypemeta.go:35; service.go:108; staterecord/record.go:311 |
| frame:icon | string | tile icon (Font Awesome name or short text); preferred over `icon` by the state record | wtypemeta.go:36; staterecord/record.go:287 |
| icon | string | block icon name (Font Awesome) | wtypemeta.go:28; record.go:290 |
| icon:color | string | CSS colour or hex (#RRGGBB[AA]) for the icon; run through ColorValue to validate | wtypemeta.go:29; record.go:305 |
| web:partition | string | Electron session partition of a web block, e.g. `persist:<blockid>`; Set-HomeTile sets `persist:<blockid>`; Wave refuses to read a tile without its own partition (comment, HomeTab.Lib.ps1:36 - the check is in web code, not read) | wtypemeta.go:144; Set-HomeTile.ps1:91 |
| cmd | string | command line for a term block | wtypemeta.go:43; Health-Check.ps1:343 (joins with cmd:args) |
| cmd:args | array of strings | args for cmd when cmd:shell is false | wtypemeta.go:54; Health-Check.ps1:343 |
| cmd:runonstart | bool | the controller starts the block's command whenever Wave starts (needs cmd:runonce false) | wtypemeta.go:47; wshcmd-session.go:241; Rehome-Manager.ps1:53; Health-Check.ps1:349 |
| cmd:runonce | bool | run only once; session start --restart sets false | wtypemeta.go:49; wshcmd-session.go:240 |
| cmd:clearonstart | bool | clear output at start; session --restart sets false | wshcmd-session.go:242 |
| session:id | string | Claude session uuid the block was opened for; waveapi exposes it as sessionid only when a lowercase uuid | service.go:110; Rehome-Manager.ps1:53; Open-WaveSession.ps1:108 |
| session:name | string | Claude session name; used to find a block (`blocks list` filtered by exact, case-insensitive name) | Wave-Sessions.ps1:242; Open-WaveSession.ps1:108 |
| session:model | string | model of the session | Open-WaveSession.ps1:109 |
| session:effort | string | effort level | Open-WaveSession.ps1:110 |
| session:permissionmode | string | permission mode (e.g. auto) | Rehome-Manager.ps1:53; Restore-Sessions.ps1:89 |
| session:* | string | only keys matching `^session:[A-Za-z0-9._-]+$` via `session start --meta`; values reject control chars (wshcmd-session.go:140,213-233). `setmeta` has no such restriction | wshcmd-session.go:140 |
| attention:level | string | "info", "need" or "urgent" ("" none); bubbles up from tile to stack tab, tile, top tab (highest wins) | pkg/attention/attention.go:16-27,43-51 |
| attention:reason | string | why; required for need/urgent, optional for info; max 120 code points, no control or hidden characters, trimmed | attention.go:23,53-56,69-80 |
| tab:flagcolor | string | tab colour flag; CSS name or hex; set by `wsh tab style --color` | wshcmd-tabstyle.go:18,37-38; record.go:334 |
| tab:accent | string | colour of the whole Wave frame while this tab is active | wshcmd-tabstyle.go:20,33-35 |
| tab:symbol | string | tab symbol: FA icon name (max 40 chars) or 1-2 graphemes (max 32 bytes) | wshcmd-tabstyle.go:19,22-24 |
| ws:accent | string | workspace-level accent fallback | wshcmd-tabstyle.go:21 |
| slot | string | named place of a block in its tab (left, middle, right-top, right-bottom by default; layout data via --layout-file) | wshcmd-slotlayout.go:15; wshcmd-slot.go:26-29; service.go:105 |
| display:* | any | NEVER written by setmeta (silently skipped, mergeSpecial=false) | wtypemeta.go:216-218 |
| <prefix>:* = true | bool | pseudo-key: deletes `prefix` and all `prefix:` keys | wtypemeta.go:195-213 |

Notes: types come from JSON round trip, so numbers are float64 in Go/JSON consumers. Meta has no schema enforcement for tool keys (SetMeta stores anything). Object kinds: block keys in the table are block meta; tab:* are tab meta; ws:accent is workspace meta; `client` meta exists (onboarding:*). Not read: where `tab:symbol`'s consumer in the frontend is, and the frontend use of attention:* (frontend/app/store/attention.ts mentioned at attention.go:5).

## 9. Callers table

Searched: C:\Projects\Personal\MalfestioMaster\tools (non-Test files and the test stand-ins) and C:\Projects\Personal\wave\waveterm\tools\ours (note: waveterm\tools\ours was grepped; the worktree copy not separately). Tests (Test-*.ps1) call through stand-ins of wsh and only assert the argument shapes below; I list real callers and add test lines only where they pin a contract. No caller of `getmeta`, `createblock`, `api focus`, `stack move`, `group remove` was found.

| file:line | verb and flags passed | output / result read | notes |
|---|---|---|---|
| MalfestioMaster\tools\Health-Check.ps1:324 | `wsh blocks list --json` (stderr to $null) | ConvertFrom-Json array; per block `$_.meta.cmd`, `$_.meta.'cmd:args'`, `$_.meta.'cmd:runonstart'` (-ne $true), `$_.blockid` (:342-352) | exits quietly if output empty; "No blocks found" text would break ConvertFrom-Json (inside no try) |
| MalfestioMaster\tools\HomeTab.Lib.ps1:16 | `blocks list --view=web --json` (via scriptblock $Ask) | `tabid` (filter equals Home tab id), `blockid`, `meta.url` (:24,28) | Select-HomeTile expects `meta.url` string, matches regex on 127.0.0.1 home |
| MalfestioMaster\tools\Show-InWave.ps1:40 | `blocks list --view=web --json` | `tabid`, `blockid`, `meta.url`; compares new vs old blockids (:69-77) | empty-output guard `if (-not $text.Trim())` (:41) |
| MalfestioMaster\tools\Show-InWave.ps1:80 | `setmeta -b <blockid> frame:title=<Title>` | exit code only | title that looks like number/bool/JSON/begins with [ { " would be coerced (quirk) |
| MalfestioMaster\tools\Show-InWave.ps1:101 | `group activate -b <blockid>` | exit code only (stdout `queued: activate ...` ignored) | runs only when idle; manager only (:84-100) |
| MalfestioMaster\tools\Set-HomeTile.ps1:76,93 | `setmeta -b <blockid> frame:title=Tether` | exit code | |
| MalfestioMaster\tools\Set-HomeTile.ps1:91 | `setmeta -b <blockid> web:partition=persist:<blockid>` (only if differs from `$tile.meta.'web:partition'`) | exit code | reads `meta.'web:partition'` from blocks list |
| MalfestioMaster\tools\Set-HomeTile.ps1:92 | `setmeta -b <blockid> url=http://127.0.0.1:<port>/home/enter?t=<token>` | exit code | url contains an entry token, the server logs the full meta (wshserver.go:266) = token in log (quirk) |
| MalfestioMaster\tools\Set-HomeTile.ps1:51 | (via HomeTab.Lib) `session list` then `blocks list --view=web --json` | see HomeTab.Lib | `session list` not my slice (not read) |
| MalfestioMaster\tools\Check-HomeTile.ps1:19 | `& $Wsh @a` generic ask (blocks list through HomeTab.Lib) | exit code throws `wsh <verb> failed (exit N)` | |
| MalfestioMaster\tools\Wave-Sessions.ps1:130 | `api list --json` (inside try/catch) | `workspaceid` (equals own workspace) -> `[0].activetabid` | JSON bare array, needs lowercase ids |
| MalfestioMaster\tools\Wave-Sessions.ps1:242 | `blocks list --json` | `workspaceid`, `meta.'session:name'` (case-insensitive equals), `blockid`, `tabid` | |
| MalfestioMaster\tools\Wave-Sessions.ps1:250 | `deleteblock -b <blockid> --force` | exit code (stdout `block deleted` discarded) | refuses own block itself (:246) |
| MalfestioMaster\tools\Wave-Sessions.ps1:270 | `api goto --tab <TabId>` | exit code | no --workspace: uses env WAVETERM_WORKSPACEID; no --window |
| MalfestioMaster\tools\Rehome-Manager.ps1:53 | `setmeta -b <new> cmd:runonstart=true session:id=<id> session:name=<name> session:permissionmode=auto` (retry 3x on nonzero) | `$LASTEXITCODE` | `$new` parsed from `wsh run` stdout (`block:<uuid>`; `run` not my slice) |
| MalfestioMaster\tools\Rehome-Manager.ps1:66 | `deleteblock -b <OldBlock>` (NO --force, stderr merged to $null) | `$LASTEXITCODE` -> log text | refusal by RefuseRunning if old block still has a live program: it only logs "could not be deleted... delete it by hand" (:67) |
| waveterm\tools\ours\Restore-Sessions.ps1:124 | `$Wsh group add -b <info.blockid> --to <anchor blockid>` | exit code only; failure leaves block in own tab (:125) | block id comes from `session start` JSON `.blockid`, `.tabid` (:118-119) |
| waveterm\tools\ours\Check-Seams.ps1:71-120 | none executed: greps source files for the seams (StackBlockCommand, wsh-setmeta, deleteblock cmd...) | n/a | seams file tools\ours\seams-expected.txt:101-106 |
| MalfestioMaster\tools\Test-*.ps1 (Test-ShowInWave.ps1:54, Test-WaveSessions.ps1:209, Test-SetHomeTile.ps1:30-43, Test-RehomeManager.ps1:12-24) | stand-in `wsh`; pin: group activate has NO --focus, `deleteblock -b X --force`, `setmeta -b X key=value` one key per call, `api goto --tab T` | n/a | these must change with the CLI |
| waveterm\tools\ours\host\Open-WaveSession.ps1:155 | `wsh session start ...` (not my slice) | n/a | meta keys session:* flow via `--meta` |

Callers that used verbs only in comments or messages (no call): Show-InWave.ps1:5-6; Rehome-Manager.ps1:31-32,57.

Callers with `wsh web open ... --stack <blockid> --tab <tabid> --no-focus` (Show-InWave.ps1:73) depend on the stack target semantics in section 6 (same TargetBlockId/TargetAction mechanics); `web open` source not read.

## 10. Quirks a clean redesign should fix (consolidated)

1. `blocks list --json` prints `No blocks found` (non-JSON) on empty; print `[]`. (wshcmd-blocks.go:188-194)
2. `blocks list` ignores `-b`, `--tab` takes only a full tab uuid, no `tab:N`/`this`; sort order is by uuid, not tab/layout order; no windowid in output; whole meta (secrets, tokens) in output; default scope all workspaces. (blocks.go:163,197-205,177-183)
3. `deleteblock` with no argument deletes the calling block; guard (`refuserunning`) is a client-set flag, not enforced for other RPC callers; 2 s timeout; no JSON output, no id echo. (deleteblock.go:41-52; wshserver.go:597)
4. `setmeta` value coercion (octal, hex, floats, numeric strings, "true", "null", empty = delete), "/" nesting, no way to set a literal string that looks like JSON, `display:*` silently ignored, `:*` pseudo-keys undocumented, one invocation per key leaves half-applied state in callers, server logs all values. (setmeta.go:62-146; wtypemeta.go:189-229; wshserver.go:266)
5. `getmeta` cannot distinguish missing from null; single-key output is a bare JSON value; `--raw` is partial; no exit code for missing; no callers. (getmeta.go:93-111)
6. `group *` and `stack move` only report "queued"; they cannot say whether the layout changed; `group activate` can take keyboard focus although no --focus is given and it has no --focus flag; subject ORef type not checked on `-b`; group membership known only from the page-saved tree (100 ms lag, wrong "not in a group" right after create). (wshcmd-stack.go:114-125; stackblock.go:114-142; Show-InWave.ps1:89-90)
7. `createblock`: orphan block when the layout action is rejected after the block was created; default Focused=true; success output is a plain text sentence (fmt.Printf, unwrapped); `--replace` is referenced in a check but there is no such flag. (wshserver.go:328-390; createblock.go:54,66; stack.go:59-61)
8. id forms: bare uuid needs WAVETERM_BLOCKID and a live server round trip; 8-char prefix hits the first object of ANY type; `x@y` form is dead; a script outside Wave can only use `block:<uuid>`; case: ORef/uuid parse lenient but waveapi requires lowercase. (wshcmd-root.go:201-225; resolvers.go:37-79; wstore_dbops.go:240-260; service.go:30)
9. `api`: text list is not parseable; `--json` list is bare array vs HTTP wrapper; ids must be lowercase; `focus` and `goto` have overlapping, different rules (active-tab requirement); goto is not atomic and hides Electron errors as `internal error`; token rotates every Wave start; no group/stack/layout info; two parallel ways to enumerate (api list vs blocks list vs session list) with different shapes (api: no meta, blocks: all meta). (service.go:177-262; handler.go:96-102)
10. Authorization: none of these verbs checks that the caller owns the target; any block with a JWT can delete, retitle, restructure any block/tab/workspace by id. Scoping to the caller's workspace is done only inside each PowerShell caller. waveapi is the only scoped-by-secret path (token file, user-only ACL), but also not scoped by workspace. Per-command allow-lists in the router: not found in the files read (pkg/wshutil/wshrouter.go beyond link trust lines not read).
11. Errors: all failures exit 1, no distinct codes; messages are free text; stderr "Warning:" on partial success; `sendActivity` telemetry on setmeta/getmeta/deleteblock.
12. The meta keys `session:*` are free form on `setmeta` but restricted to `^session:[A-Za-z0-9._-]+$` and no control characters in `session start --meta`; attention:level/reason are validated only by the `attention` verb (wshcmd-attention.go:58) while `setmeta attention:level=anything` stores anything. A redesign needs one validator.

## 11. Not read (explicit)
pkg/waveobj/wtype.go (object field definitions, ValidOTypes), pkg/wcore block.go DeleteBlock/CreateBlock internals, controller start rules (pkg/blockcontroller), resolveTabId body, wshcmd-session.go (other than lines 205-245), wshcmd-attention.go (other than line 58), `web open`, `run`, `session list/start`, pkg/waveapi/open.go, send.go, acl_*.go, router per-command policy (wshrouter.go), all frontend code (how the page applies layout actions, focus rules, attention rendering, tab:symbol), the unix-socket ACL implementation, and the whole of Wave's Electron RPC handlers (ElectronSetActiveTab, FocusWindow).
