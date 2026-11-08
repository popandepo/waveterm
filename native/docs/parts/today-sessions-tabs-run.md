# CLI contract spec, part A: session, tab, run (as of worktree feat-native-cli-spec-def31e7a)

Paths are relative to C:\Projects\Personal\wave\worktrees\feat-native-cli-spec-def31e7a unless they start with C:\.
Abbreviations: S = cmd/wsh/cmd/wshcmd-session.go, SS = wshcmd-sessionstack.go, TM = wshcmd-tabmanage.go, T = wshcmd-tab.go,
TF = wshcmd-tabflag.go, NF = wshcmd-nofocus.go, SZ = wshcmd-sizeflag.go, ST = wshcmd-stack.go, TS = wshcmd-tabstyle.go,
R = wshcmd-run.go, ROOT = wshcmd-root.go (all in cmd/wsh/cmd/), WS = pkg/wshrpc/wshserver/wshserver.go.
"not read" = I did not read it.

## 0. Common behaviour (every verb here)

- Auth/transport: PreRunE preRunSetupRpcClient (ROOT:85-95). Needs env WAVETERM_JWT; if empty the error is
  `wsh must be run inside a Wave-managed SSH session (WAVETERM_JWT not found)` (ROOT:88). It connects to the domain socket named in the JWT and
  authenticates with that JWT (ROOT:168-186). If WAVETERM_BLOCKID is set it sends SetPeerInfo "domain:block:<id>" (ROOT:187-191).
- Authorization rule: any process holding a valid block JWT may call every verb in this document. I found NO per-verb allow check in the CLI
  or in these server handlers (WS:159-263, WS:312-408). Router side: AuthenticateCommand binds one route+blockid per link
  (pkg/wshutil/wshrouter_controlimpl.go:90-139); whether the router filters individual commands per route: not read.
  The scripts add their own rules (manager-only focus, never list) in PowerShell, not in wsh (Wave-Sessions.ps1:162-168, 267, 278-284).
  The `force` bypass comment in emain says "any local RPC caller ... the check is a guard against mistakes, not a security boundary" (emain/emain-wsh.ts:181-183).
- Errors: every RunE returns an error; cobra prints `Error: <msg>` to stderr (rootCmd has SilenceUsage:true but not SilenceErrors, ROOT:21-26),
  Execute calls DoShutdown("",1,true) so exit code is 1 (ROOT:261-265, pkg/wshutil/wshutil.go:128-135). Every failure is exit 1; there is no other
  exit code in these verbs. (The "Error: " prefix comes from cobra default behaviour, I did not find a line that sets it; treat as inferred.)
- Stdout writer: WrappedStdout (ROOT:30,75-77). When UsingTermWshMode is true each "\n" becomes "\r\n" (ROOT:43-69); where UsingTermWshMode is set: not read.
- Global flag -b/--block (ROOT:260) is a persistent flag on every verb. `session *`, `tab *` verbs and `run` ignore it (none reads blockArg).
- Env read by this slice: WAVETERM_JWT (preRun), WAVETERM_BLOCKID (resolveSimpleId, ROOT:209; also peer info), WAVETERM_TABID (getTabIdFromEnv, ROOT:227-229,
  used only by run/--tab default), WAVETERM_WORKSPACEID (T:76, TM:166, SS:131). Everything else comes from os.Environ for `run` (R:100-105, see run).
- Reference resolution used by tab verbs: resolveTabId(ref) (TM:92-105): trims, empty -> "tab"; resolveSimpleId (ROOT:201-225): if ref parses as
  full ORef `type:uuid` it is used locally with no env needed (waveobj.go:80-98); otherwise REQUIRES WAVETERM_BLOCKID
  (`no WAVETERM_BLOCKID env var set`, ROOT:211) and calls ResolveIds (2000 ms) on the server. Server grammar (resolvers.go:37-79):
  `x@y`, keywords this/block/tab/ws/workspace/client/global/temp, `type:uuid`, `tab:N` (1-based n-th tab of the CALLER's workspace, resolvers.go:130-164),
  `<view>` or `<view>:N`, plain integer (block number in caller's tab), full uuid, 8 lowercase hex (first object whose id starts with it, resolveUUID -> wstore.DBResolveEasyOID, body not read).
  Error texts: `resolving tab: ...`, `id not found: "<id>"`, `"<ref>" is a <otype>, not a tab` (TM:102).
  QUIRK: a bare tab uuid (no `tab:` prefix) is NOT a full ORef, so it needs WAVETERM_BLOCKID and a server round trip; `tab:<uuid>` does not. The help text
  says "first 8 characters" are accepted for tab close/switch/rename (TM:33,43,52) but tabflag.go deliberately refuses 8 chars for --tab (TF:20-22). Inconsistent.

## 1. session start

Synopsis: `wsh session start [flags] [-- command [args...]]` (S:30). Args: no Args validator, so extra positionals are taken as the command after `--` (cobra gives
args after `--`); `splitSessionCommand(args, fileData)` S:123-138.

Flags (S:84-97, SS:23):
| flag | type | default | validation / error text |
| --name | string | "" | tab name; "" lets server pick next "T<n>" (wcore/workspace.go:204-237). With --stack: validated in buildSessionMemberLook SS:62-90: control/bidi/format runes -> `invalid name: control, bidirectional and invisible format characters are not allowed`; >60 UTF-16 units -> `invalid name: at most 60 characters`. Without --stack NO validation here (tab name is not checked client side or server side, WS:242-253). |
| --color | string | "" | buildTabStyleMeta -> validateColor (body not read: validateColor in csscolormap.go/other, not read). Whitespace-only counts as empty (S:430). |
| --symbol | string | "" | validateTabSymbol TS:192-209: hidden/bidi runes -> `invalid symbol: control, bidirectional and invisible format characters are not allowed`; misplaced ZWJ -> `invalid symbol: a zero width joiner is only allowed inside an emoji sequence`; icon name = optional prefix solid@/regular@/brands@/custom@ then [a-z0-9-]+, max 40 -> `invalid symbol: icon names are limited to 40 characters`; else text: <=32 bytes, <=2 graphemes, none of <>&"'`\ -> `invalid symbol %q: use a Font Awesome icon name ... (at most 32 bytes)` |
| --cwd | string | caller's os.Getwd() | not checked for existence; no Abs() (S:302-308). Error `getting current directory: ...` |
| --env | string array (KEY=VALUE, repeatable) | none | `invalid --env %q: want KEY=VALUE` (S:249-259); empty key refused; later duplicate silently overwrites earlier |
| --args-file | string path | "" | read with os.ReadFile: `reading --args-file: <err>`. Content must be a JSON array of strings: `--args-file must be a JSON array of strings: <err>`. Given with `--` command too: `give the command after -- or in --args-file, not both`. No command: `no command given: put it after -- or in --args-file` (also when first element is "") (S:123-138). Note: an empty file (0 bytes) is treated as "no file" -> falls to the "no command" error unless `--` was given. |
| --workspace | string | env WAVETERM_WORKSPACEID | `no workspace id specified (use --workspace or set WAVETERM_WORKSPACEID)` (TM:163-172) |
| --restart | bool | false | see Validation below |
| --meta | string array | none | `session:KEY=VALUE`; regexp `^session:[A-Za-z0-9._-]+$` and "=" required else `invalid --meta %q: want session:KEY=VALUE`; value with control chars/U+2028/U+2029 -> `invalid --meta %q: a value cannot contain control characters (line breaks, tabs, escape sequences)` (message prints key only); duplicate key -> `--meta session:x given twice` (S:215-233) |
| --activate | bool | false | with --stack: `--activate cannot be combined with --stack: it switches to a new tab and a stacked session has none; show the member with: wsh group activate -b <block>` (SS:52-54) |
| --stack | string (block id or number) | "" | given but blank: `--stack needs a block (id or number); leave --stack out to start the session in a new tab` (SS:45-51). "Given" is detected with Flags().Changed. |

Not available on session start: --no-focus, --size, --tab, --force (session start never focuses, see Focus).

Validation order (S:261-327): args-file read -> splitSessionCommand -> validateSessionStart (meta parse, then restart checks) -> validateSessionStackOptions -> style/look
-> parseSessionEnv -> workspace id -> cwd -> create. A refusal leaves nothing behind.
--restart refusals (S:149-167, best effort, literal text only):
- any argument (command or args) containing the substring `--session-id` -> `--restart cannot be used with --session-id in the command: the second start must resume the session (--resume), not create it again; use a starter script that makes that choice, for example tools/ours/Start-ResumableSession.ps1`
- inline code: base name of command (lowercased, dir and .exe stripped) is `wsl` (always refused) or one of pwsh, powershell, cmd, bash, sh, zsh, fish, dash, nu, node, python, python3, py, ruby, perl AND any later argument equals (case-insensitive) one of its inline flags (table S:171-187, e.g. pwsh: -command -c -encodedcommand -ec -e -commandwithargs -cwa; cmd: /c /k; node: -e --eval -p --print; python: -c) -> `--restart cannot be used with an inline shell command (<command> ...): the flags that matter could be hidden inside it; start a script file instead, for example tools/ours/Start-ResumableSession.ps1`.
  Unlisted interpreters (e.g. deno, php) and `-File x.ps1` pass.

Server-side effects, new-tab mode (createSessionTab S:379-403), in this order:
1. CreateTab RPC (15 s) {workspaceid, name, empty:true} -> server wcore.CreateTab(ws, name, activate=false, ...) (WS:242-253): appends tab to END of workspace TabIds
   (createTabObj, workspace.go:269-290), default name "T<max+1>" when --name empty; error `workspaceid is required` if empty; `workspace <id> not found`.
   NOTE: server CreateTab applies the default new-tab layout (ApplyPortableLayout GetNewTabLayout, workspace.go:253) regardless of data.Empty - I read `wcore.CreateTab(ctx, ws, name, false, data.Empty)` (WS:247) where Empty is passed as isInitialLaunch, and the comment at workspace.go:251-253 says the layout is applied always in this fork. So `Empty:true` effect on the layout: ApplyPortableLayout(..., true) argument semantics NOT read. Do not assume the tab is block-free.
   Also sets tab:background meta from config if configured (workspace.go:257-261).
2. if --color/--symbol: SetMeta on tab (2 s): keys `tab:flagcolor` (color), `tab:symbol` (symbol) (TS:18-19, 93-111).
3. CreateBlock RPC (15 s) with Focused:false (S:354-364): block meta (S:407-426): view=term, controller=cmd, cmd=<command>, cmd:args=[...], cmd:shell=false,
   cmd:cwd=<cwd>, cmd:runonce=true, cmd:runonstart=true, cmd:clearonstart=true; cmd:env={..} only if --env; connection=<RpcContext.Conn> only if non-empty
   (inherits the caller's connection, e.g. WSL/ssh). --restart overrides: cmd:runonce=false, cmd:runonstart=true, cmd:clearonstart=false (S:238-247). Each `--meta` pair is then
   merged on the block as `session:KEY` = string (S:244-246). Server CreateBlock queues an Insert layout action for the tab (WS:390-400) and calls requestTabPageLoad (WS:406).
4. ElectronPreloadTab (route electron, 30 s) loads the tab page hidden/parked off screen (emain-window.ts:562-580).
5. if --activate: ElectronSetActiveTab (15 s) -> queues "switchtab" in the window (emain-wsh.ts:168-174, emain-window.ts:419-429).
Order is deliberate: page loads LAST so the orphan-block clean-up does not delete the block (S:375-378).
Failure cleanup: step 2 or 3 failing -> closeLeftoverTab: SessionCloseTab force:true + ElectronTabClosed; error `<cause>; the empty tab was closed again` or `<cause>; the empty tab <id> could not be closed again: <err>` (S:366-373). Step 4 or 5 failing -> error `tab <t> was created with block <b> but its page could not be loaded in the background: <err>` / `tab <t> was created but could not be activated: <err>`; nothing is printed to stdout in that case even though the tab and block exist (QUIRK: the ids are only inside the error text; sessionStartRun prints stdout only on full success, S:321-326).

Server-side effects, --stack mode (SS:142-162, S:309-318): checkStackRefWorkspace (SS:130-138): if workspace != env WAVETERM_WORKSPACEID the ref must be a full lower-case block uuid (optionally `block:`) else `--stack "<ref>" is counted in your own tab, not in workspace <ws>; give the full block id (wsh session list shows it)`.
resolveBlockRef(ref) (ST:41-47): uses resolveSimpleId, must be a block: `<otype>:<id> is not a block; groups work between blocks of the same tab (a tab as target is not supported yet)`; wrapped `resolving --stack: ...`. Then BlocksList(workspace) 5 s to find the target's tab: `block <id> is not in workspace <ws> (give --workspace if it is in another)`. Then CreateBlock {tabid=target's tab, targetblockid, targetaction="stack", focused:false} (SS:104-112). Server (WS:320-327,382-386): re-checks target in same tab (`the target block must be in the same tab as the new block`), queues StackInsert layout action. --name/--color/--symbol go on the BLOCK: frame:title=name, icon:color=color AND frame:bordercolor=color, frame:icon=symbol (SS:62-90; keys metaconsts.go:30,34,36,37). No tab is created, no preload, no activate. The group is only QUEUED for the tab page: if the tab is not open/loaded the layout change waits (SS:140-141, ST:114-115); success = accepted, not applied.

Stdout on success: ONE line of compact JSON + "\n" (S:325, SS:114-122; Go map marshalled, so key order alphabetical):
- tab mode: `{"blockid":"<uuid>","tabid":"<uuid>"}`
- stack mode: `{"blockid":"<uuid>","stackedwith":"<block uuid>","tabid":"<uuid of the tab holding the stack>"}`
All values are strings. (Help text says `{"tabid":..,"blockid":..}`; the real order is alphabetical because it is a map.) Restore-Sessions parses it with ConvertFrom-Json and reads .tabid/.blockid.
Stderr/exit on failure: `Error: <message>` exit 1 (messages above, plus `creating tab: ...`, RPC errors verbatim).
Focus: never takes keyboard focus (Focused:false); changes the shown tab only with --activate (not with --stack). Preload keeps the new tab off screen.
sendActivity("sessionstart") is a telemetry-local activity ping (ROOT:236-246), not output.
QUIRKS to fix: (a) partial success prints no ids; (b) `--cwd` not made absolute and not validated (run does filepath.Abs, session start does not); (c) --restart check is heuristic text matching; (d) --env duplicate keys silently last-wins; (e) --meta restricted to `session:` prefix and string values only; (f) output key order is accidental; (g) tab mode and stack mode have different semantics for --name/--color/--symbol (tab keys vs block keys); (h) a cwd given as a Windows path is passed through as-is.

## 2. session list

Synopsis: `wsh session list` (S:51-57); cobra.NoArgs (extra arg -> `unknown command "x" for "wsh session list"` from cobra, not read in source).
Flags: none of its own (only global -b, ignored).
Reads: WAVETERM_JWT only. It does NOT use WAVETERM_WORKSPACEID: it lists ALL workspaces (WorkspaceListAll, includes unsaved scratch workspaces, WS:1079-1081) so a caller must filter on workspaceid itself (every caller does).
Server calls (collectSessionTabs S:440-490): WorkspaceListAll (5 s); per workspace BlocksList{workspaceid} (5 s); per tab GetTab (2 s); per block with view=="term" ControllerStatus (2 s). Any RPC failure aborts the whole listing with
`listing workspaces: ...` / `listing blocks of workspace <id>: ...` / `reading tab <id>: ...` / `status of block <id>: ...` (all-or-nothing, exit 1).
Workspaces with nil WorkspaceData are skipped (S:447-449).
Stdout: pretty JSON array (json.MarshalIndent two spaces) of tab objects, then "\n" (S:500-504). Empty list prints `null`, NOT `[]`, when there are no tabs at all (Go nil slice `var tabs []sessionTabInfo`, S:445,500). QUIRK.
Tab object (S:112-120): 
- tabid string; name string; workspaceid string; color string (omitted if empty; from tab meta `tab:flagcolor`); symbol string (omitted if empty; from `tab:symbol`);
- running bool (always present; true if any term block status is "running" or "init", S:436-438,480-482);
- blocks array (always present, [] if none) of {blockid string; title string (omitted if empty; block meta `frame:title`); view string (block meta `view`, may be ""); status string (term blocks only, else ""; one of "none","init","running","done" per rpctypes comment wshrpctypes.go:802); exitcode int (always present; 0 for non-term blocks and for status none)}.
Order: workspaces in WorkspaceListAll order, tabs in workspace tab order (tab bar order), blocks in BlocksList order (tab.BlockIds order, NOT layout order).
Not in output: the active tab, windowid, index, block meta (session:*), cmd. (Wave-Sessions gets the active tab from `api list --json`, Wave-Sessions.ps1:130-131.)
Failure/exit: exit 1 on any RPC error. No focus or state change.
Quirks to fix: `null` for empty; no active-tab marker; no `index`; no session:* meta on blocks (callers use `blocks list --json` for session:name); status "none" conflated with "never started since app launch"; all workspaces always; a single failing GetTab fails everything (whereas `tab list` degrades to "<error>").

## 3. session close

Synopsis: `wsh session close <tab> [--force]` (S:59-68). cobra.ExactArgs(1).
Flag: --force bool default false: "close even if a program is still running" (S:97).
Reads: WAVETERM_BLOCKID unless <tab> is a full `tab:<uuid>` (see section 0); WAVETERM_JWT.
Steps (S:508-529): resolveTabAndWorkspace(arg) (resolve tab id, then WorkspaceListAll 5 s, find the workspace holding it: `no workspace found containing tab <id>`);
SessionCloseTab RPC {workspaceid, tabid, force} (15 s) -> on error `not closed: <err>`; then ElectronTabClosed to the electron route (15 s) with newactivetabid/closewindow -> error `tab <id> was closed but the window could not be told: <err>`.
Server SessionCloseTabCommand (WS:197-227): global mutex sessionCloseLock; `tab <id> not found: <err>`; unless force: for each block of the tab with meta view=="term", status = controller runtime status; no controller (nil) counts as unfinished with status "none"; any ShellProcStatus != "done" is unfinished -> error
`block <blockid> in tab <tabid> is not known to be finished (status <s>); use force to close anyway` (note text says "use force", CLI flag is --force). Non-term blocks (web, etc.) are ignored by the check. With force or all done: DestroyBlockController per block, wcore.DeleteTab(ws, tab, recursive=true): removes tab from workspace, deletes its blocks (DeleteBlock), picks new active tab = ws.TabIds[max(0,min(idx-1,len-1))] if the closed tab was active, else keeps active; if no tabs remain the WINDOW IS CLOSED (workspace.go:296-350). Returns {closewindow, newactivetabid}.
Closing the tab the caller itself runs in is NOT refused by wsh (the PowerShell wrapper refuses it, Wave-Sessions.ps1:282-284).
Stdout success: `tab closed\n` (S:527). Failure: stderr `Error: ...`, exit 1.
Focus/shown tab: if the closed tab was the active one the window's active tab changes to the neighbour (newactivetabid sent to electron, tabClosedInBackend emain-wsh.ts:205-211). Otherwise none.
Quirks: output is a fixed English phrase; the "not finished" rule treats "none" (no status) as running, so a tab opened before the app restarted needs --force; error text says "use force" vs flag --force; window can close when last tab closes.

## 4. tab list (alias `ls`)

Synopsis: `wsh tab list [--workspace <id>] [--json]` (T:24-32, aliases T:26). No Args validator (stray args ignored).
Flags: --workspace string default env WAVETERM_WORKSPACEID (T:52, resolveWorkspaceId T:71-81; error `no workspace id specified (use --workspace or set WAVETERM_WORKSPACEID)`); --json bool default false. NOTE: tabWorkspaceId is one shared variable also bound to `tab move --workspace` (T:52,56) - harmless per process.
Server: WorkspaceList (SAVED workspaces only, not unsaved scratch ones, WS:1067-1069, cf. WS:1071-1078) 5 s -> `failed to list workspaces: ...`; then `workspace "<id>" not found` (T:83-90) if the id is not in that list (QUIRK: a scratch/unsaved workspace is "not found" here but visible to `session list`). Per tab GetTab 2 s; on failure prints to STDERR `warning: could not fetch tab <id>: <err>` and uses name `<error>` (exit stays 0).
Stdout, default text (T:142-151): text/tabwriter, minwidth 0, tabwidth 0, padding 2, pad ' ': header line `INDEX  TAB ID  NAME` (columns separated by runs of >=2 spaces after alignment), then one row per tab `<index>  <tabid>  <name>`; tab id truncated to first 34 chars + `..` if longer than 36 (never for a uuid, 36 chars). Name is last column, printed raw (may contain spaces; trailing alignment: tabwriter does not pad last column). Index is 0-based position in the tab bar. No active-tab marker. Script parse used today: Rehome-Manager.ps1:21 matches lines ending in `\s<TabName>` and takes whitespace-split element [1] = tab id. That breaks for names with spaces, for names that are suffixes of other names (regex `\s$HomeTab$` matches "My Home"), and it picks the FIRST match.
Stdout with --json: pretty JSON array (2-space) of {tabid string, name string, workspaceid string, index int} + "\n" (T:64-69,133-139). Zero tabs -> `[]` (make with capacity, non-nil, T:112).
Exit: 1 on RPC errors. No state change, no focus.

## 5. tab new

Synopsis: `wsh tab new [name] [--workspace <id>] [--activate] [--empty]` (TM:18-28). cobra.MaximumNArgs(1): 2+ args -> cobra's `accepts at most 1 arg(s), received 2`.
Flags (TM:66-70): --workspace string (default env WAVETERM_WORKSPACEID, error as above); --activate bool false; --empty bool false (sent as CreateTabData.Empty; see the layout note in section 1 step 1: effect on block content NOT confirmed).
Name: no trimming, no length or character validation (client TM:182-185, server WS:242-253). "" -> server default "T<max+1>".
Steps (createBackgroundTab TM:146-161): CreateTab (15 s) -> ElectronPreloadTab (30 s, hidden) -> if --activate ElectronSetActiveTab (15 s).
Stdout success: tab id (bare uuid) + "\n" (TM:187-189). Parse: trim the line. On a failure AFTER creation (preload/activate) the tab id IS still printed to stdout and the error goes to stderr, exit 1 (TM:186-190): script must check the exit code, not only stdout.
Failures: `creating tab: <err>` (nothing printed); `tab <id> was created but its page could not be loaded in the background: <err>`; `tab <id> was created but could not be activated: <err>`.
Side effects: tab appended at the end of the tab bar; tab:background meta from config; new-tab layout per server (see above). Focus: none unless --activate (switches shown tab, does not raise window per help TM:43).

## 6. tab rename

Synopsis: `wsh tab rename <name> [--tab <ref>]` (TM:30-38). cobra.ExactArgs(1) (a name with spaces must be one quoted arg).
Flag: --tab string default "" = the tab of the calling block (resolves "tab" via WAVETERM_BLOCKID, TM:92-95). Accepts anything resolveSimpleId accepts (section 0), including 8-char prefix and `tab:<n>`.
Validation: name is TrimSpace'd; empty -> `tab name must not be empty` (TM:197-200). No max length (help says tab bar shows at most 14 chars: display only). Server UpdateTabName (WS:159-167, wstore.go:43-55): `tab not found: "<id>"`; no other check; broadcasts object update.
Other tabs with the same name are allowed (duplicates possible).
Stdout: `tab renamed\n`. Failure: `renaming tab: <err>` or resolve errors; exit 1.
Focus: none. Does not need workspace id. Quirk: cannot rename by name, only by ref; success text is a fixed phrase; the new name echoed is trimmed but not reported.

## 7. tab switch

Synopsis: `wsh tab switch <tab>` (TM:40-47). ExactArgs(1). No flags.
Resolve tab+workspace (TM:107-121): resolveTabId (section 0 rules; WAVETERM_BLOCKID unless `tab:<uuid>`), WorkspaceListAll 5 s, owning workspace.
Calls ElectronSetActiveTab {workspaceid, tabid} on the electron route (15 s) (TM:221). Electron: finds window by workspace id (`no window found with workspace <id>` if none) and queues switchtab with setInBackend=true (emain-wsh.ts:168-174), i.e. the workspace's active tab is recorded and the window shows it.
Stdout `tab switched\n`. Failure `switching tab: <err>`; exit 1.
Focus: changes the SHOWN tab of that window (the only verb besides `session start --activate` / `tab new --activate` that does); does not raise the window (help text TM:43; I did not verify window raise behaviour in emain).
Authorization: same as all; scripts reserve it for the manager (Show-WaveTabs.ps1:16-20). Note Wave-Sessions.ps1 focuses with `api goto --tab` (Wave-Sessions.ps1:270), NOT `tab switch`; `tab switch` has no script caller here.

## 8. tab close

Synopsis: `wsh tab close <tab> [--force]` (TM:49-57). ExactArgs(1) (a tab must be named, no default).
Flag --force bool false (TM:74).
Steps (TM:265-283): resolveTabAndWorkspace; tabBlockCount = GetTab (5 s) len(BlockIds); checkTabCloseAllowed (TM:232-247): without --force: if the count could not be read -> `could not read the tab to see whether it holds blocks (<err>); use --force to close it anyway`; count>0 -> `the tab holds N blocks; closing it stops the programs in them and cannot be undone: use --force to close it anyway` (`1 block` singular). ANY block of any view counts (unlike session close, which looks only at term blocks' status). Empty tab closes without --force.
Then ElectronCloseTab {workspaceid, tabid, force} on the electron route (15 s). Electron (emain-wsh.ts:176-203): no window -> `no window found with workspace <id>`; unless force re-reads the tab and refuses via wshCloseRefusal (text not read); then queueCloseTab(tabId, maxBlocks) which re-checks block count in the same call; if not closed: `the tab was not closed (it gained blocks since it was checked, or the window refused)`.
The backend delete path used by electron close (WorkspaceService.CloseTab, controllers destroyed, window closes when last tab): not read in full; see Check-Seams.ps1:64.
Stdout `tab closed\n`; failure `closing tab: <err>`; exit 1.
Focus: if the closed tab was active, the window shows the neighbour (inferred from DeleteTab logic, workspace.go:320-328, same as session close; electron route path not fully read).
Quirks: two different "is it safe" rules (tab close = any block; session close = unfinished terminal); neither refuses closing the caller's own tab; closing the last tab closes the window.

## 9. tab move

Synopsis: `wsh tab move <tabid> --index <n> [--workspace <id>]` (T:34-43). ExactArgs(1).
Flags: --index int default -1, MarkFlagRequired (cobra error `required flag(s) "index" not set`); --workspace string default env WAVETERM_WORKSPACEID.
IMPORTANT: <tabid> is compared literally to workspace TabIds (T:176-185): it must be the FULL tab uuid; no `tab:<n>`, no prefix, no `tab:<uuid>` form, no resolve via RPC. Error `tab "<id>" not found in workspace "<ws>"`.
Workspace via WorkspaceList (saved workspaces only; same `workspace "<id>" not found` quirk). Index must be 0..len-1 else `index <n> out of range [0, <len-1>]` (T:187-190). Same index -> prints `tab <id> is already at index <n>\n`, exit 0 (T:191-194).
Otherwise reorderTabIds (T:210-223): remove then insert at target index (final position = target index) and UpdateWorkspaceTabIds {workspaceid, tabids} (2 s) -> `failed to update tab order: <err>`. Server (WS:255-263, workspace.go:375-383) just overwrites ws.TabIds with the list: NO check that the set of ids is unchanged, despite the help text claiming "the set of tab ids must remain exactly the same" (T:38-39). Race: a tab created between list and update would be dropped from the tab bar. QUIRK.
Stdout success: `moved tab <id> from index <old> to <new>\n`. Exit 1 on failure. Focus: none (does not change the active tab).

## 10. run

Synopsis: `wsh run [flags] -- command [args...]` or `wsh run -c "<shell string>"` (R:19-25; TraverseChildren true).
Flags (R:27-40 plus helpers):
| flag | type | default | notes |
| -m/--magnified | bool | false | block created magnified; with --stack -> `--stack cannot be combined with --magnified` (ST:62-63) |
| -c/--command | string | "" | run as shell string (cmd:shell=true). With a `--` command too: `cannot specify both -c and command arguments` + help on stderr |
| -x/--exit | bool | false | cmd:closeonexit=true when the command exits 0 |
| -X/--forceexit | bool | false | cmd:closeonexitforce=true (wins over -x) |
| --delay | int ms | 2000 | cmd:closeonexitdelay=float64(delay), always written |
| -p/--paused | bool | false | block created without starting: cmd:runonstart=false and cmd:runonce NOT set |
| --cwd | string | caller's cwd | made absolute with filepath.Abs (R:85-96); not checked for existence |
| --stack | string | "" | put the new block into the tab group of this block (id or number); resolved via resolveBlockRef (ST:41-47); sets TargetBlockId + TargetAction "stack" (ST:55-72) |
| --tab | string | "" (= tab of calling block, WAVETERM_TABID) | TF:35-38. Allowed values: `tab`, `tab:<1-3 digits>`, full uuid or `tab:<full uuid>`. 8-char prefix rejected (TF:53-55): `--tab "<ref>" is not accepted: use a full tab id, tab:<n> or tab (an id prefix can match more than one object)`. Given but empty/blank: `--tab needs a tab: a full tab id, tab:<n> or tab`. Not given and WAVETERM_TABID empty: `no WAVETERM_TABID env var set`. Resolve failure: `resolving --tab: <err>`. Note `tab:<n>` counts the CALLER's workspace (needs WAVETERM_BLOCKID); a full uuid works in any workspace/window. Presence of --tab counts, not its value. |
| --size | uint | 0 (not given) | TF:36 via addSizeFlag (SZ:23-25); given and outside 2..50 -> `--size must be between 2 and 50, got <n>` (SZ:29-37). Server repeats the check (`nodesize must be between ... got <n>`, ours_nodesize.go:20-28; its constants MinCreateNodeSize/MaxCreateNodeSize not read) and refuses with ephemeral. No effect with --stack (help SZ:24). Layout weight of the new tile (10 = default). |
| -a/--append | bool | false | cmd:clearonstart=false (default true) |
| --no-focus | bool | false | Focused=false on the create request (NF:14-32); default Focused=true |
| -b/--block | global | | ignored |

Command parsing quirk (R:61-71): the command is found by scanning os.Args for the literal `--`; shellCmd = next arg, cmdArgs = the rest, cmd:shell=false. `--` as last arg -> help + `no command provided after --`. Neither `--` nor -c: help + `command must be specified after -- or with -c`.
BEWARE: the scan looks at os.Args, so flags placed AFTER `--` go to the program, and a value equal to `--` earlier in the flags also triggers it. Also an empty-string command after `--` is treated as not given (shellCmd == "").
Environment: run copies the WHOLE caller environment (os.Environ, R:99-108) into a block file `env` (wavebase.BlockFile_Env, null-separated) - includes WAVETERM_JWT etc. of the caller. This differs from `session start` which passes no env except --env. (How the controller uses the env file to build the process env: not read.)
Block meta (R:109-136): view=term, cmd:cwd=<abs cwd>, controller=cmd, cmd:clearonstart=true (false with -a), cmd=<command or shell string>, cmd:args=<list or nil>, cmd:shell=<true only for -c>; not paused: cmd:runonce=true and cmd:runonstart=true; paused: cmd:runonstart=false; -X: cmd:closeonexitforce=true elif -x: cmd:closeonexit=true; cmd:closeonexitdelay=<ms>; connection=<RpcContext.Conn> if set.
Order of checks (R:138-169): createTabId -> applyStackTarget -> checkCreateTarget (size check, then with --tab: target block of --stack must be in that tab: `the target block is in tab <t>, not in tab <u>: --tab needs the stack or the block to replace to be in that tab`, via BlockInfo RPC; error wrapper `looking up the tab of the target block: ...`) -> CreateBlock (default RPC timeout) -> on error `creating new run block: <err>`.
Server CreateBlock (WS:312-408): queue Insert layout action (or StackInsert for --stack) with Focused/Magnified/NodeSize, then requestTabPageLoad(tab, block) (ours_preload.go:233: asks the tab page to load; body not read). Note: it does NOT preload through ElectronPreloadTab from the CLI and does not activate the tab: a block created with --tab in a not-shown tab appears when the page is loaded (the fork's preload handles it, details not read).
Stdout on success: `run block created: block:<uuid>\n` (R:168; ORef.String() = "block:<uuid>", waveobj.go:36-41). Script parse today (Rehome-Manager.ps1:48): strip everything up to the last `block:` and expect a bare 36-char uuid. The block id is NOT given as JSON (quirk: session start gives JSON, run gives prose).
Stderr/exit on failure: `Error: ...` exit 1; arg-error cases also dump the command help to stderr first.
Focus: takes focus (Focused=true) UNLESS --no-focus; never switches the shown tab. Nothing is returned about the tab id.
Quirks to fix: prose output; command located by scanning os.Args; whole env copied into the block (secrets) ; `--tab tab:<n>` needs the caller block; `-c` runs through a shell, `--` does not; `--paused` leaves cmd:runonce unset.

## 11. Shared-flag summary

| flag | session start | session list | session close | tab list | tab new | tab rename | tab switch | tab close | tab move | run |
| --tab | no | no | no | no | no | yes (any simple id incl. prefix, default own tab) | no | no | no | yes (strict: tab, tab:N, full uuid) |
| --no-focus | no | no | no | no | no | no | no | no | no | yes |
| --stack | yes (block id/number; new semantics: no new tab) | no | no | no | no | no | no | no | no | yes (adds to group) |
| --size | no | no | no | no | no | no | no | no | no | yes (2..50) |
| --restart | yes | no | no | no | no | no | no | no | no | no |
| --meta | yes (session:* only) | no | no | no | no | no | no | no | no | no (use `setmeta` afterwards) |
| --args-file | yes | no | no | no | no | no | no | no | no | no |
| --name | yes | no | no | no | positional [name] | positional <name> | no | no | no | no |
| --cwd | yes (raw) | no | no | no | no | no | no | no | no | yes (made absolute) |
| --color/--symbol | yes | no | no | no | no | no (separate `tab style`, TS:62-69) | no | no | no | no |
| --activate | yes | no | no | no | yes | no | n/a | no | no | no |
| --force | no | no | yes | no | no | no | no | yes | no | no |
| -b/--block | ignored | ignored | ignored | ignored | ignored | ignored | ignored | ignored | ignored | ignored |

## 12. Callers found (grep of C:\Projects\Personal\MalfestioMaster\tools and C:\Projects\Personal\wave\waveterm\tools\ours; Test-*.ps1 files only simulate wsh and are listed at the end)

| file:line | verb and flags passed | what it reads |
| MalfestioMaster\tools\HomeTab.Lib.ps1:13 | `session list` | JSON array; per tab: .workspaceid (filter), .name (case-insensitive equals 'Home'), .tabid. Needs null/empty handled: ConvertFrom-WshList returns @() only if text is empty (HomeTab.Lib.ps1:7), the literal `null` would make ConvertFrom-Json return $null and the foreach emit nothing (works by accident) |
| MalfestioMaster\tools\Wave-Sessions.ps1:127 | `session list` | .workspaceid filter; .tabid, .blocks[].blockid, .blocks[].view=='term', .name, .running (must be a JSON bool: `$t.running -is [bool]`, else "state unknown"); array order = tab order -> Index (Wave-Sessions.ps1:126-149). Relies on the array order being the tab bar order. |
| MalfestioMaster\tools\Wave-Sessions.ps1:300-302 | `session close <tabid> [--force]` (tabid = bare uuid from list) | exit code only (Invoke-Wsh fails on non-zero and prints the output text, Wave-Sessions.ps1:112-117); output discarded. Bare uuid works because env WAVETERM_BLOCKID is set in a block. |
| MalfestioMaster\tools\Rehome-Manager.ps1:21 | `tab list` (text) | text parse: line matching `\s<HomeTab>$`, whitespace-split element [1] = tab id (column 2 = TAB ID). Header line `INDEX  TAB ID  NAME` must not itself match. Uses WAVETERM_WORKSPACEID. |
| MalfestioMaster\tools\Rehome-Manager.ps1:29,47-49 | `run --tab <tabid> --no-focus --cwd <dir> -- pwsh -NoLogo -Command <cmd>` | stdout `run block created: block:<uuid>`; parsed by stripping through the last `block:` and requiring `^[0-9a-f-]{36}$`; stderr merged (2>&1). `--tab` value is the full uuid from `tab list` (accepted by TF:25). |
| MalfestioMaster\tools\Boot.ps1:68 | text only (message mentions `wsh run --tab ... -- <exe>`) | none |
| MalfestioMaster\tools\Test-WaveSessions.ps1:168-303, Test-ShowWaveTabs.ps1:107-111, Test-ShowInWave.ps1:121 | stand-in wsh; assert call strings `session close <tab> [--force]`, no `tab switch`, no `--activate`, no `--focus` | test doubles |
| waveterm\tools\ours\host\Open-WaveSession.ps1:133-156 | `session start --name N --cwd C --args-file F [--color X] [--symbol Y] [--activate] [--stack B] [--restart] [--meta session:id=..] [--meta session:name=..] [--meta session:model=..] [--meta session:effort=..] [--meta session:permissionmode=..] [--meta <caller-given>]...` | exit code; stdout passed through unparsed (`& wsh @wshArgs` prints the JSON to the caller). The caller (Wave-Sessions.ps1:230-232) returns that text. args-file is a UTF-8 JSON array written by PowerShell (Open-WaveSession.ps1:150-152; note `Set-Content -Encoding utf8` in PS 5.1 would write a BOM, which Go json.Unmarshal rejects; PS7 writes none). Needs an arg containing no single quote (Open-WaveSession.ps1:101-104, its own rule). |
| waveterm\tools\ours\Restore-Sessions.ps1:100-119 | `session start --restart --name N --meta session:id=ID [--meta session:name|model|effort=V] [--cwd C] -- <shell> <args...>` | exit code; stdout parsed with ConvertFrom-Json -> .tabid and .blockid (Restore-Sessions.ps1:118-119,124,126); then `group add -b <blockid> --to <anchor>` (not in my slice). |
| waveterm\tools\ours\Start-ResumableSession.ps1:1 | comment only: documents blocks made by `session start --restart --meta session:id=<guid>` | none |
| waveterm\tools\ours\Check-Seams.ps1:59-65,76-77,122-127,158-159 | no wsh call; source-text seam checks that the Go/TS lines these verbs depend on still exist (UpdateTabNameCommand signature, WorkspaceListCommand, CloseTab, SetActiveTab, controller run decision, emain setActiveTab/queueCloseTab, create-block focused field) | none |
| waveterm\tools\ours\Test-RestoreSessions.ps1:50,92,111 | stand-in logs lines starting `session start` | test double |
Not found among callers: any script calling `tab new`, `tab rename`, `tab switch`, `tab move`, `tab close`. (Search patterns: `@('tab'|'session'|'run'`, `wsh tab|session|run`, `session start|list|close`, `tab list|new|rename|switch|close|move`; scripts outside the two folders, e.g. hooks, other repos: not searched. `Show-InWave.ps1`/`Set-HomeTile.ps1` use `web open --stack --tab --no-focus`, `group activate`, `setmeta`, `blocks list` - outside this slice but they depend on the same --tab/--stack/--no-focus parsing.)

## 13. What the new CLI must keep (derived from callers)

1. `session start` stdout = single JSON object with string fields tabid, blockid (and stackedwith in stack mode); exit 0 only on full success; --args-file JSON array; --restart/--meta/--name/--cwd/--color/--symbol/--activate/--stack semantics as above.
2. `session list` = JSON array in tab-bar order, each with tabid, name, workspaceid, running (bool), blocks[].blockid, blocks[].view, (title, status, exitcode, color, symbol); filtered by caller on workspaceid.
3. `session close <bare tab uuid> [--force]`: exit code is the signal; refusal when a term block is unfinished or status unknown unless --force.
4. `run --tab <full uuid> --no-focus --cwd <dir> -- <cmd args>` returns the new block id parsable from `block:<uuid>`.
5. `tab list` column 2 = tab id, last column = name (replace with --json and keep a name filter).

## 14. Quirks a clean redesign should fix (consolidated)

1. `tab list` plain text parsed by column; no active marker; names with spaces break scripts; makes `tab list --json` the only form.
2. `session list` prints `null` when empty; includes all workspaces; no active tab, no index, no block meta (session:*); one failed GetTab aborts all.
3. `tab list`/`tab move` use WorkspaceList (saved workspaces only) while `session list` uses WorkspaceListAll: same tab can be listed by one and "not found" by the other.
4. `run` prints prose `run block created: block:<id>`; `session start` prints JSON; `tab new` prints a bare id; others print fixed phrases (`tab closed`, `tab renamed`, `tab switched`). Use JSON everywhere.
5. Tab reference grammar is inconsistent: tab close/switch/rename accept bare uuid and 8-char prefix (requires WAVETERM_BLOCKID), `--tab` on run rejects prefix, `tab move` needs the full bare uuid with no resolution. One grammar, no env needed for a full id.
6. `session start` partial success (preload/activate failure) hides ids in the error text; `tab new` prints the id on stdout AND fails. One rule: always print the JSON with ids and a status field.
7. `tab move` help claims atomic same-set check, server just overwrites TabIds (race can drop a tab).
8. `session start --cwd` not absolute/validated; `run --cwd` is made absolute.
9. `run` copies the entire caller environment (incl. WAVETERM_JWT) into the block; `session start` passes only --env.
10. `run` finds the command by scanning os.Args for `--`; `session start` takes cobra's post-`--` args.
11. Two different close safety rules (tab close: any block; session close: unfinished terminals, "none" status counts as running); neither protects the caller's own tab; last tab closing closes the window.
12. `--restart` guard is textual best-effort; `--meta` only `session:` string values; `--env` duplicates silently last-wins.
13. Tab name unvalidated for tab new/rename/session start (only checked in --stack mode via frame:title rules: 60 UTF-16 units, no control/bidi chars).
14. Exit code is always 1; no machine-readable error (cobra `Error: ...` text); error text says "use force" where the flag is `--force`.
15. `Empty:true` in CreateTab may not actually produce a block-free tab in this fork (server applies the new-tab layout, workspace.go:251-253); needs a test before a rewrite relies on it.
16. Focus rules live partly in wsh (Focused:false, preload parked) and partly in PowerShell (manager-only focus); the new app should own "who may change shown tab/focus".

## 15. Not read

validateColor body; wcore.CreateBlock and ApplyPortableLayout/GetNewTabLayout; wstore.DBResolveEasyOID; livePreload (ours_preload.go beyond line 233 and 63); WorkspaceService.CloseTab and wshCloseRefusal text; emain window raise behaviour on setActiveTab; router-level per-command authorization (pkg/wshutil/wshrouter.go beyond controlimpl); where UsingTermWshMode is set; MinCreateNodeSize/MaxCreateNodeSize values; cobra's built-in error wording (inferred); callers outside the two tools folders.
