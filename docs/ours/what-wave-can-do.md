# What Wave can do for a host of CLI sessions

Fork notes, not upstream documentation. Base: upstream `main` at `9f1c9670` (5 Oct 2026, version 0.14.5),
Windows 10, built and run from this fork on 10 Oct 2026.

Every claim is marked:

- **RAN**: executed on that build and the output seen.
- **READ**: taken from source only, not executed. Source paths are relative to the repo root.

What would have made a RAN claim red: the command failing, or the output not showing the stated field. What
is not covered is listed at the end.

## Vocabulary

| Word | Meaning |
|---|---|
| window | Electron window; holds one workspace |
| workspace | list of tabs (`TabIds`), one active |
| tab | one page of tiled blocks; has `Name`, `Meta`, a layout object |
| block | one view (terminal, files, web, sysinfo...); uuid; `Meta` map |
| oref | `block:<uuid>`, `tab:<uuid>`, `workspace:<uuid>` |

## 1. How a caller gets to the server (the important constraint)

- `wsh` talks to the server over a domain socket and must authenticate with `WAVETERM_JWT`
  (`cmd/wsh/cmd/wshcmd-root.go` `preRunSetupRpcClient`). **RAN**: with no JWT it prints
  "wsh must be run inside a Wave-managed SSH session (WAVETERM_JWT not found)".
- The JWT is minted for a block and handed to that block's shell. The shell gets a single-use swap token
  (`WAVETERM_SWAPTOKEN`) which the shell init script trades for the real environment with the hidden command
  `wsh token` (`cmd/wsh/cmd/wshcmd-token.go`, `pkg/util/shellutil/tokenswap.go`, `GetAndRemoveTokenSwapEntry`
  deletes the entry). **READ**.
- Consequence (stated once): `wsh` always needs a valid token for the caller. In practice that means the
  caller runs inside a Wave block, where it inherits `WAVETERM_JWT`, `WAVETERM_BLOCKID`, `WAVETERM_TABID`
  and `WAVETERM_WORKSPACEID` (**RAN**: the variable names are present in a block). A process outside Wave
  could in principle be handed a token by something that already has one; that was **not tried** (a probe was
  refused) and it is **not our model**: the design here assumes the controller runs inside a block.
- `wsh` commands accept `-b <ref>` where ref is `this`, `tab`, `ws`, a block number (position in the tab's
  leaf order), a view name such as `term`, a uuid or its first 8 characters, or a full oref
  (`pkg/wshrpc/wshserver/resolvers.go`). **READ**. **RAN**: `-b tab`, `-b this`, `-b 2`, `-b 5`.

## 2. Starting a session in a tab or block

| Want | Command | Status |
|---|---|---|
| New terminal block in the current tab | `wsh term` (opens in cwd) | READ (help text only) |
| New block running a command | `wsh run [-x] [-c "..."] [--cwd d] -- cmd args` | **RAN**: created `block:<uuid>`, appeared in `blocks list` with `controller: cmd`, `cmd`, `cmd:args`, `cmd:cwd` |
| Any block type from a widget | `wsh launch` (widget id), `wsh view <file>`, `wsh web open <url>` | READ |
| New **tab** | none in `wsh`. Backend service `workspace.CreateTab(workspaceId, name, activate)` (`frontend/app/store/services.ts`, `pkg/service/workspaceservice/workspaceservice.go`) | READ. Not an RPC command that `wsh` can send; used by the front end only |
| New window / workspace | none in `wsh` (`wsh workspace` has `list` only) | RAN (`workspace list`), READ for absence |

`wsh run` and `wsh term` target the tab of the calling block (`WAVETERM_TABID`). A block cannot be created
in another tab from `wsh` as shipped. READ (`createblock` RPC takes a `TabId`, but the `run`/`term` commands
fill it from the environment).

## 3. Addressing

- Block ids and tab ids are uuids. **RAN**: `wsh blocks list --json` returns `blockid`, `workspaceid`,
  `tabid`, `view`, `meta` for every block; `wsh tab list --json` returns `tabid`, `name`, `workspaceid`,
  `index`; `wsh workspace list` returns `windowId`, `workspaceId`, `name`, `icon`, `color`.
- `wsh blocks list` can filter by `--workspace`, `--window`, `--tab`, `--view`. READ (help).
- Block numbers are positions in the tab layout order, so they change when tiles move
  (`pkg/wshrpc/wshserver/resolvers.go`). Use uuids for anything stored. READ.

## 4. Rename, colour, icon

| Want | What exists | Status |
|---|---|---|
| Rename tab | RPC `UpdateTabNameCommand(tabId, newName)` and the tab context menu. **No `wsh` command.** | READ. **RAN**: `wsh setmeta -b tab name=Renamed` stored a `name` key in tab *meta* but did **not** rename the tab (`tab list` still showed `T1`). Do not use that |
| Tab colour | Meta key `tab:flagcolor` (CSS colour). Right-click tab, "Flag Tab" offers a fixed palette; `wsh setmeta -b tab "tab:flagcolor=#ff8800"` accepts any valid CSS colour. **Another tab can be addressed from inside any block** with `-b tab:<tabuuid>`, a bare uuid, its first 8 characters, or `tab:<n>` (n-th tab of the workspace) | **RAN**: set from a block, tab shows an orange flag icon in the tab bar (screenshot), value persisted across an app restart. **RAN** from tab T2 on tab T1: `setmeta -b tab:<uuid>`, `-b <uuid>`, `-b <8 chars>`, `-b tab:1` all returned "metadata set" and `getmeta` on the other tab showed the change |
| Tab icon / symbol | **Nothing for tabs.** A tab shows only the flag glyph (coloured by `tab:flagcolor`) and transient badges. `wsh setmeta -b tab "tab:icon=crown"` is accepted and stored (meta accepts unknown keys) but nothing draws it | **RAN** (stored, persisted, not drawn). Frontend reading: `frontend/app/tab/tab.tsx`, `tabbadges.tsx` |
| Tab badge (icon plus colour) | `wsh badge [icon] --color c --priority n -b tab`; icon is a Font Awesome name; event `badge`, stored in memory only ("not persisted and cleared on restart", `pkg/wcore/badge.go`). Badges on a tab clear when that tab gets focus (`frontend/app/store/badge.ts`). Higher priority wins over the flag | **RAN**: `wsh badge -b tab star --color gold` on the active tab: the tab kept showing the flag (consistent with clear-on-focus). `wsh badge -b tab:<other tab uuid> star --color gold` from another tab: the inactive tab showed a gold star with a small dot for the flag colour (screenshot). Persistence: READ only |
| Block title and icon in its frame | Meta keys `frame:title`, `frame:icon`, `frame:text`, `frame:bordercolor`, `frame:activebordercolor` (`pkg/waveobj/metaconsts.go`) | **RAN**: set and persisted across restart. **Not observed to change the header of a terminal block** (header kept the `user@host` text; not investigated why) |
| Tab background | `wsh setbg`, meta `tab:background`, `bg:*` | READ |
| Workspace name, icon, colour | Workspace object has them; edited in the UI | RAN (listed), READ for editing |

Emoji: the badge and block icons go through `makeIconClass` (`frontend/util/util.ts`), which maps names to
Font Awesome classes (or `custom@` / `solid@` prefixes). An emoji string is not handled there. READ.

## 5. Focus, close, read

| Want | Command / RPC | Status |
|---|---|---|
| Focus a block | `wsh focusblock -b <ref>`: RPC `setblockfocus`, routed to the tab page, so the tab must be loaded. It does not switch the active tab or raise the window | **RAN**: returned without error. The visual result was not checked. Behaviour limits: READ |
| Close a block | `wsh deleteblock -b <ref>` | **RAN**: "block deleted", block gone from `blocks list` and from the screen layout after restart |
| Switch active tab / close tab | Services `workspace.SetActiveTab`, `CloseTab` (front end) | READ. No `wsh` command |
| Read terminal text | `wsh termscrollback -b <ref> [--start n --end n] [--lastcommand] [-o file]`; needs the block's view mounted (route `feblock:<id>`) | **RAN**: exited 0 but printed nothing for a shell block and for a finished `run` block. Not investigated (possible causes: shell integration absent, short-lived output). Treat as unproven |
| Read raw output | hidden `wsh debugterm`, file store of the block (`term` file) | READ |
| Send keystrokes to a block | RPC `controllerinput` (`inputdata64`), no `wsh` command | READ |
| Screenshot a block | RPC `captureblockscreenshot` to the tab page; block must be visible | READ |
| Get / set any object's meta | `wsh getmeta [-b ref] [keys]`, `wsh setmeta [-b ref] k=v ...`, `--json file` | **RAN** on tab and block |
| Events | `wps` event bus (`pkg/wps`), `badge`, `waveobj:update`; no `wsh` subscribe command found | READ |

## 6. What persists across a restart

**RAN**: after stopping the dev app and starting it again with the same data folder:

- Tab meta (`tab:flagcolor`, the stored `tab:icon`) persisted.
- Block meta (`frame:title`, `frame:icon`) persisted.
- The block set and layout persisted (4 blocks; the one deleted before the restart stayed deleted).
- Shells are new processes after a restart (the block id is the same, the process is not).

**READ**, not tested: badges are in memory only. Terminal scrollback is kept in a per-block file and a
serialized cache and replayed when the view mounts (`frontend/app/view/term/termwrap.ts`).

## 7. What a session inside a block can set about its own tab

All of it through `wsh` using the inherited environment, no extra permission prompt was seen:
**RAN** `setmeta -b tab` (colour, arbitrary keys), `badge -b tab`, `setmeta -b this` (frame keys).
Not available from inside: rename tab (no command), create tab, switch tab.

## 8. Build and run on Windows (how this was done)

**RAN**, in a folder with portable tools (no system install): Go 1.26.8, Zig 0.17.0 (C compiler for the
SQLite driver), Task 3.54.0, Node 24 (already present; upstream asks for 22), npm 11.

1. `npm ci`
2. `task build:backend` (generate, `wavesrv`, `wsh` for 9 targets): 1 min 43 s on this PC.
3. `npm run dev` (electron-vite) with these environment variables set:
   `WAVETERM_CONFIG_HOME` and `WAVETERM_DATA_HOME` pointing at a private folder (keeps the dev app away from
   any installed Wave), `WCLOUD_ENDPOINT`, `WCLOUD_PING_ENDPOINT`, `WCLOUD_WS_ENDPOINT` as in `Taskfile.yml`
   (without them the server exits: "invalid wcloud endpoint"), `WAVETERM_NOCONFIRMQUIT=1`.
4. A `settings.json` in the config folder with `"telemetry:enabled": false` and `"autoupdate:enabled":
   false` before first start.
5. First start shows a welcome dialog, a telemetry choice, a star-on-GitHub prompt and a feature tour; each
   needs one click.

Checks on the unmodified base:

- `npx vitest run`: 16 files, 51 tests, all pass (2 s). **RAN**
- `go test ./cmd/wsh/...`: pass. **RAN**
- `npx tsc --noEmit`: **17 errors on upstream as it stands**, all in `frontend/preview/mock/*` and
  `frontend/preview/previews/processviewer.preview.tsx` (`numthreads` missing from `ProcessInfo` mocks and
  similar). Our checks must compare against this baseline, not against zero. **RAN**
- `task generate` rewrites one tracked file: `frontend/types/gotypes.d.ts` gains `"block:subblock"?:
  boolean;`. Upstream's committed copy is stale by that line. **RAN**
- `task build:schema` (part of `generate`) runs a recursive force delete of `dist/schema` through the `RMRF`
  variable. Overriding the variable on the command line (`task build:backend "RMRF=cmd /c echo skip"`) avoids
  it; the build succeeded that way. **RAN**
- `task package` and `task clean` also use `RMRF`. Not run.

## 9. Findings that change the plan

1. **Tab colour already exists upstream** (`tab:flagcolor`, context menu, `wsh setmeta`). Feature (a) is
   mostly the missing half: a symbol on the tab, a name-or-hex friendly command, and a way to set it from
   from another tab by tab id. The colour part should reuse `tab:flagcolor` as is. Setting another tab's colour or symbol already works with `wsh setmeta -b tab:<uuid>` (RAN, section 4); a new command is only a friendlier front (colour names, one call for colour plus symbol), not a new capability.
2. **Tab symbol does not exist.** Badges are the nearest mechanism but are transient and cleared on focus,
   so a persistent symbol needs a new meta key drawn in `tab.tsx` / `tabbadges.tsx`.
3. **No `wsh` command renames, creates, switches or closes tabs.** A host that "opens, names, colours,
   addresses and closes" sessions needs new commands for these; the RPC and service calls exist underneath.
4. **The controller runs inside a Wave block** (token requirement and the untried outside option: section 1).

## 10. Not covered

- Visual result of `focusblock`; why `termscrollback` was empty; why a terminal
  header ignored `frame:title`.
- Anything beyond meta and badge on a second tab (created with the UI "+" button, since no command does it).
- `controllerinput`, `captureblockscreenshot`, events, remote connections, WSL, Wave AI.
- Memory and CPU cost of the dev app; many-terminal behaviour.
- Linux and macOS.

## 11. Styling another tab by reference is deliberate

`wsh tab style --tab <ref>` accepts any tab reference, including a full `tab:<uuid>` or bare uuid of a tab in
another workspace or window. The command checks that the reference resolves to a tab object. The RPC it calls,
`SetMetaCommand` (`pkg/wshrpc/wshserver/wshserver.go`), checks nothing about ownership: any caller with a valid
token can set meta on any object it can name (**READ**; the same is true of `wsh setmeta -b <anything>` that
shipped upstream). We keep it that way on purpose: there is one user, and the manager session styles the tabs of
the sessions it runs. Wave has no multi-user boundary for this to cross. `tab:<n>` is relative to the caller's
own workspace. If Wave ever gains such a boundary, the check belongs in `SetMetaCommand`, not in the command.

The stored symbol is validated twice: by the command before writing, and by `parseTabSymbol` before drawing,
because any block can write tab meta directly. Both refuse control, bidirectional and invisible format
characters (the zero width joiner is allowed for emoji sequences) and bound the value (icon names 40 characters,
text 32 bytes and two graphemes, in the command as well as the parser; a zero width joiner must sit inside an emoji sequence). The command counts graphemes with a small approximation (combining marks, variation selectors, emoji modifiers, flags, joined sequences) instead of the full Unicode segmentation the browser has.
## 12. Accent colour (fork feature)

Upstream has one global accent: `--accent-color` in `frontend/app/theme.scss` and `--color-accent`, `--color-accent-50..900`,
`--color-accentbg` in `frontend/tailwindsetup.css` (READ). The fork recolours the whole frame of a tab's page at run
time by setting those variables on the root element (`frontend/app/accent.ts`); each tab is its own page, so switching
tabs switches the colour. Order, first valid colour wins:

1. the focused block's connection: `display:accent` in `connections.json` (so the safety does not depend on any session);
2. the tab: `wsh tab style --accent <colour>` (meta `tab:accent`);
3. the workspace: `wsh workspace style --accent <colour>` (meta `ws:accent`);
4. the workspace's own icon colour (`workspace.color`);
5. the default green.

Blocks on a connection with `display:accent` are also framed in that colour (and their header tinted) whether focused or
not (`frontend/app/block/conn-accent.scss`). Colours are validated before use; an invalid value is skipped. The palette
steps are derived with CSS `color-mix`, so no colour maths runs in script.


## 13. Slots and --no-focus (fork feature)

`wsh slot open <slot> <view> [key=value ...]`, `wsh slot close <slot>`, `wsh slot list` give blocks a fixed place
by name. The slot is stored as block meta `slot`, so a closed block frees its slot with no other bookkeeping.
Opening a slot that is filled replaces its block; opening an empty one splits next to the first filled neighbour
listed in the slot layout (data, `--layout-file`; default left, middle, right-top, right-bottom). It uses the
existing backend placement (`TargetBlockId` and `TargetAction`: splitright, splitleft, splitup, splitdown,
replace), so no RPC, frontend or backend change.

`--no-focus` on `wsh createblock`, `run`, `term`, `web open` and `slot open` sends `Focused: false`.

- **RAN:** `go test ./cmd/wsh/cmd` (planning, replace, close and reopen, unknown slot, bad layout data, custom
  names, no-focus passed through). Changing one default anchor turns two tests red.
- **READ:** the backend splits and replaces as the layout action says; closing removes the node from its parent (`deleteNode`);
  whether the siblings then fill the freed space (size weights) is NOT verified; `webview.tsx` loads a new `url` meta into the open web view
  (`metaUrl` effect). Not yet seen in a window: where the blocks really land, whether the right column keeps its
  shape when middle opens and closes, focus staying put.
- **Limit:** opening a slot never splits one block of a two-block column (refused with a message); with a single right block it nests one
  level deeper than a flat row.
- **Concurrent opens are NOT safe.** The backend has no lock, so two callers opening the same empty slot at the
  same instant can both keep a block. `wsh slot open` re-checks after creating (smallest block id wins, the other
  call removes its own block), but a rival that appears after that check is missed. `wsh slot repair` removes the
  duplicates (keeps the smallest id). The real fix is an atomic slot-owner key in the backend (not done).

## 14. Local API: list, focus, goto (fork feature, slice 1)

A small API inside wavesrv for callers that are not wsh inside a block (later: a broker bridge, a dashboard).
Design and the later slices (send, read, move, events) are written up outside this repository; this section is
only what exists.

- **Transport.** A unix socket `wave-api.sock` in the folder `api` of the Wave data directory. No TCP listener, so
  nothing on the LAN. wavesrv writes a new random token (32 bytes, hex) to `api/api-token` at every start; every
  call needs `Authorization: Bearer <token>`. Token compare is constant time.
- **Folder access.** No child process is started. On Windows the `api` folder is created with its final security
  descriptor in one call (owner = current user, protected DACL with exactly one allow entry for that user, full
  control, inherited by children); an existing folder is only taken over when it is a plain folder (no junction)
  owned by the current user, and its DACL is replaced. The owner and DACL of the folder, the socket and the token
  file are then read back, and the server refuses to start unless each is exactly that. Order: lock and verify the
  folder, check no live server owns the socket, listen, verify the socket, then write the token (temp file, verify,
  rename, verify). A second start on a live socket is refused. The socket path must be at most 100 characters.
  On other systems the folder is mode 0700 and owned by the user. Not covered: another process of the SAME user can
  read the token (that is the model; the next slice needs a per-caller grant before `send` exists); a folder owned
  by another user was only tested through the descriptor check, since no second account exists here.
- **Calls** (POST only; the body is one JSON object with known, exactly spelled, non-repeated keys inside 1 MiB, `list` takes none or `{}`; `/v1/...`): `list` (workspaces > tabs > blocks, ids, names, view, slot, controller,
  connection, title, url cut to scheme and host), `focus` (a block in the active tab, or make a tab active; never changes
  workspace or window), `goto` (workspace + tab [+ block]; the whole address is checked before the first change;
  the OS window is raised only with `window=true`). Ids must be lowercase full uuids. Block meta is allow-listed:
  no command lines, environments or working directories leave the process.
- **wsh.** `wsh api list [--json]`, `wsh api focus --block|--tab`, `wsh api goto --tab [--workspace] [--block]
  [--window]`; the workspace defaults to `WAVETERM_WORKSPACEID`; the folder is found with the `path` RPC or `--dir`.
- **RAN** (`go test ./pkg/waveapi/... ./cmd/wsh/cmd/`): service rules with a fake backend, handler and token checks,
  strict JSON, the security descriptor of the folder, token file and a pre-existing folder with broad entries, the
  refusal to serve when the lock, the socket check or the token write fails, and a round trip over a real unix
  socket. Mutation checks: `tools\Mutate-WaveApi.ps1` (private, not in this repository) changes one production line
  at a time; all but one go red (run it to count), the exception (`DisallowUnknownFields` removed) is an equivalent mutant because the key list
  already refuses unknown keys. Timing of the token compare cannot be unit tested; a test only pins that
  `subtle.ConstantTimeCompare` and `crypto/rand` are used.
- **READ, not run:** the wavesrv backend (`pkg/waveapi/wavesrvapi`): it reads the object store and sends
  FocusWindow, ElectronSetActiveTab and SetBlockFocus, the same calls wsh already uses. It has not been run in a
  live app, so whether `goto` leaves the OS foreground window alone is unverified until a window test.
- **Upstream files touched:** one line in `cmd/server/main-server.go` (`wavesrvapi.Start()`); seam rows
  `api-*` in `tools/ours/Check-Seams.ps1`.

## 15. Nothing goes to the network before the user opts in (fork rule)

Found when a fresh test profile showed the first-run screen with telemetry switched on. In the fork the shipped
defaults are `telemetry:enabled: false` and `autoupdate:enabled: false`, and the daily diagnostic ping is only sent
when telemetry is on. The onboarding toggle follows the setting, so it starts off.

| What | When | Where | Default in the fork |
|---|---|---|---|
| Usage events (`tevents`) | 10 s after start, then every 4 h | `<endpoint>/tevents` | off (setting) |
| Old-style activity upload | same loop | `<endpoint>/telemetry` | off (setting) |
| "no-telemetry" notice | when the user switches telemetry off | `<endpoint>/no-telemetry` | sent only on that change |
| Diagnostic ping (client id, version, arch, date) | 5 min after start, then daily | ping endpoint | off unless telemetry is on |
| Auto-update check | at start and every `autoupdate:intervalms` | update feed of the build | off; the menu item "Check for updates" still works |
| Wave AI cloud mode | on use | Wave's AI proxy | needs telemetry on, so off |

Dev builds need `WCLOUD_*` endpoint variables to start; those point at the upstream dev hosts, which is why test
runs of a build with telemetry on reach `api-dev.waveterm.dev`. Setting `WAVETERM_NOPING` also stops the ping.
Checks: `ours_defaults_test.go` (shipped defaults), `ours_ping_test.go`, and three `Check-Seams.ps1` rows
(`telemetry-default-off`, `autoupdate-default-off`, `ping-gated-on-telemetry`).
**READ, not run:** the update check and Wave AI rows come from reading `emain/updater.ts` and the AI panel code, not
from a network capture.

## 16. Stack UI, steps 1 and 2 (fork feature in progress; the purpose of the fork, see README.md)

A stack is one tile with several terminals as tabs. Steps 1-2: the tile lays all members out at one rectangle below a
28 px strip, shows only the active member (the others stay mounted and keep the same size, so a terminal is not
resized when its tab is hidden), and draws a strip with one tab per member: click to switch, x to close the member and
its terminal. Hidden members are not drop targets and are skipped by directional focus.
- **RAN** (`npx vitest run frontend/layout`, `npx tsc --noEmit` at the baseline of 17 errors): the geometry, the props
  map and the strip model, including "hidden members keep the visible size", "unknown active member falls back to the
  first", "a rectangle shorter than the strip never gives a negative body".
- **READ, not run:** the React strip and the wiring in `layoutModel.updateTreeHelper` and `TileLayout`: nothing in
  a window yet, because nothing can create a stack from outside until step 3 (`wsh group`, the `stack` layout
  action). Whether hidden terminals keep their size and process in the live app is the step 4 window check.

## 17. Stack from the command line (step 3)

`wsh group add --to <block>` (the block is `-b`, default the calling block) puts a block into the tab group of another
block or makes a group of the two; `wsh group remove` takes it out and places it next to the group; `wsh group
activate` shows its tab without taking focus. `wsh createblock|term|run|web open --stack <block>` creates a new block
straight into a group (never as a tile first). Blocks must be in the same tab. Nothing takes keyboard focus unless
`--focus` (group) or no `--no-focus` (create), as before. Every member keeps its own block id.
- **RAN** (`go test ./cmd/wsh/cmd ./pkg/wshrpc/...`, `npx vitest run`, tsc at 17): the planning of each request
  (same tab, no self-stack, no unknown action), the payload of each command, the tree functions `insertNodeIntoStack` and
  `activateStackMember` (including "focus follows the tab when the focused member would be hidden").
- **READ, not run:** the back end queueing the layout actions and the page applying them in `handleBackendAction`;
  `pkg/wcore` has two failing tests (`TestListWorkspaces_*`) that fail the same way on Master without this PR.
- **Seen in a window:** this is the first change after which a stack can be made without touching the code: run
  `wsh group add --to` on two terminals in one tab and the tile shows a tab strip. The window check is step 4.

## 18. Stack from the window: menu, one-member stacks, + (step A)

Block menu (the dots / right-click on the header): "Turn into a stack" on a plain tile (it then shows a strip with one tab and a +),
"Take out of stack" on a member of a stack with two or more members, "Turn stack off" on a stack with one member. A
stack of one stays a stack until it is turned off or its last member is closed; the strip is never shown on a plain tile, so a
tile that is not a stack keeps its full height. The + opens the widget bar's list (terminals first, hidden widgets
left out) and adds the chosen block type to the stack as a new tab, so a stack can mix terminals, web and files.
- **RAN:** vitest for turnIntoStack / turnStackOff / one-member balancing / closing members, the menu list
  (`stackaddmenu.test.ts`), stack size for the menu. tsc stays at 17.
- **READ, not run:** the React wiring (menu items, + button, create path), and that a web or preview block does not
  reload or keep playing while hidden: hidden members stay mounted with `visibility: hidden`, which a web view does not
  treat as a reload; media in a hidden tab keeps playing, which is not handled yet and is part of the window check.
- **Seen in a window after this PR:** a tile can be turned into a stack from its menu, the + adds a block of any widget type.

## 19. Drop on the middle makes a stack (step B)

Dragging a tile over the middle of another tile now outlines the target and, on drop, puts the dragged tile into the
target's stack, or makes a stack of the two; dropping on a member means its stack. The upstream swap action and its
code are unchanged but no longer reachable from the mouse (the owner never uses swap). Edge zones still move tiles
as before. Leaving a stack: drag the active member's header to an edge zone (the move takes it out), or "Take out of
stack" in its menu.
- **RAN:** vitest (centre = stack action, member target = its stack, root refused, edge zones still move).
- **READ, not run:** the outline (the Swap placeholder case now also draws for Stack) and the drag itself.
- **Seen in a window after this PR:** drag a tile onto the middle of another: outline, drop, a tab strip with both.

## 20. Move everything into a group: wsh stack move (step C)

`wsh stack move --from <block|group> --to <block|group>`: FROM is a block, or any member of a group (then the whole
group moves, in its order); TO is a block or group; everything from FROM ends up as members of the group at TO
(appended at the end, or a new group when TO is a plain block). Same tab only. It is queued for the page and not
confirmed; keyboard focus moves only with `--focus`.
- **RAN:** vitest (`moveIntoStack`: tile to tile, group to tile, group to group in order, same group is a no-op, unknown
  nodes and the root refused, focus only when asked), Go (request planning, payloads, refusals).
- **READ, not run:** the page applying `stackmove`. **Seen in a window after this PR:** `wsh stack move --from 2 --to 1`
  puts block 2 (or its whole group) into the group of block 1.
- **Not handled, for the window check:** audio or video in a web block of a hidden tab keeps playing.

Review notes on step C: a **tab as `--to` is refused with a message** (only blocks resolve; "a target stack or a target
window" across tabs is not supported yet and the help says so); naming two members of the same group is an error
from the server; a move is validated as a whole before the first member moves, so a failing member changes nothing.

## 20a. `wsh deleteblock` refuses a block with a running program unless `--force`

Deleting a block stops the program in it, and a script or a session that deletes a tile by command could kill a program by accident (the
click on an X asks since the close-one-block question; a command has nobody to ask, so it refuses instead, as `wsh session close` and `wsh tab close`
already do). Without `--force`, `wsh deleteblock` sends the request with `refuserunning` set, and **wavesrv makes the check in the same call that
deletes** (`DeleteBlockCommand`: read the block's view and its program state, refuse or delete), so a program cannot start between a check in
the command and the delete; what is left is the database write itself. The rule is `blockstatus.DeleteRefusal`: silent only when the answer is
known: the view is one of the registered views known to have no program (web, preview, files, ...), or its program has `done`. A running or starting
(`init`) program, a missing or unknown state, a state that cannot be read, an unknown or mistyped or missing view, and a block that cannot be read all
refuse, with a message that names `--force`. `--force` deletes as before. Other callers of the delete call (`wsh slot close`, the block controller) do not set the
field and keep their behaviour. **The check is opt-in per request:** a caller that does not set `refuserunning` (the `wsh slot close` command, and an older `wsh` binary that
predates the field) deletes unchecked, as before; only the new `wsh deleteblock` asks for it.
- **RAN:** Go tests: the rule (the silent cases and every refused case, each refusal names `--force`); the server's check against a real store
  (a running terminal, an ended one, no controller, an unreadable state, a web tile, an unknown view, a block that is not there); that the command
  builds its request with `refuserunning` unless `--force` and does not build its own request that skips it; that the server checks before it deletes
  and only when asked; `task generate` for the type.
- **Also run:** a route test against a real store (a running block is refused and kept, an unchecked request deletes, an ended block deletes, state "none" is refused);
  the `--force` flag exists and defaults to off (renamed flag: red). **Not covered:** the live-controller path of the state read (`ShellProcStatus` from a registered
  controller); a mutation there stays green because a registered controller needs a real shell process.
- **NOT RUN:** against a live Wave. Anything of mine that deletes terminals by command (the repro script) passes `--force`.
## 21. A session block that comes back after Wave restarts (`wsh session start --restart --meta`)

`wsh session start --restart` marks the block so that Wave starts its program again at every start of the app (the
controller runs a block whose `cmd:runonstart` is true and which is not marked run once) and keeps the earlier output.
`--meta session:KEY=VALUE` (repeatable, only `session:*` keys, no line breaks) writes facts on the block for outside
tools, for example `session:id`, `session:model`, `session:effort`; Wave itself never reads them. A running process always
dies with the app: "coming back" means the same command is started again in the same block, so the command must resume
its own work. For a Claude session that is: first start `--session-id <guid>`, every later start `--resume <guid>`.
- **RAN:** Go tests for the meta rules (only `session:*`, no duplicates, no line breaks) and for restart mode (run once off,
  start with Wave on, output kept; a mutation of the run-once line turned the test red); a real throwaway check outside Wave
  with a cheap model: a session created with `--session-id <guid>` and resumed with `--resume <guid>` answered with the
  word it had been given in the first start.
- **READ, not run:** Wave actually starting the block at its next start (needs the restart in a window); the claude TUI
  (the check used print mode).

Review notes on section 21: `--restart` is refused for a command line that contains a bare `--session-id`: Wave starts the
same command again, so the second start must resume instead of create. The supported way is a starter that decides:
`tools/ours/Start-ResumableSession.ps1` (first start `--session-id <guid>`, later starts `--resume <guid>`; self test
`tools/ours/Test-ResumableSession.ps1`). All `--meta` input (keys and values, plain `session:` keys only) and this rule are
checked before the first tab is created. The controller's start decision is a small function with its own test
(`shouldRunCommand`), but **the restart is not proven until it has been seen in a real window**: whether Wave starts the block
at its next start, what the kept output looks like, and what happens to a program that crashes at once are READ, not run.

Second review pass on section 21, what the `--restart` check can and cannot see: it looks at literal text only. It refuses any
argument that contains `--session-id` and any inline shell command (`pwsh -Command`, `-c`, `cmd /c`, `-EncodedCommand`),
because the flag could sit inside it. It cannot see what a variable expands to inside a script, so a starter script must make
the choice itself (`tools/ours/Start-ResumableSession.ps1` does, and passes the config folder it searched to Claude). Values of
`--meta` may not contain control characters (line breaks, tabs, escape sequences, NUL). The controller start decision has a
test through the controller's own meta reader plus a pin on its call site; the full start path (database and process) is still
only seen in a real window.

Third review pass on section 21: the `--restart` check also refuses the common inline-code forms of other interpreters
(`node -e/--eval/-p`, `python -c`, `ruby -e`, `perl -e`, `pwsh -CommandWithArgs`) and `wsl` in any form; this is **best effort**,
not every interpreter is covered and what a script file does cannot be checked. Values of `--meta` refuse all Unicode
control characters (C0 and C1, DEL, line and paragraph separators). The starter's self test now launches a stub that records
its arguments and environment: it proves that the searched config folder is the one the program receives, that the inherited
`CLAUDE*` markers and `NO_COLOR` are gone, and that the resume arguments arrive (this test found that the arguments were
passed as one array instead of splatted: fixed). The source pin on the controller's call site can be bypassed by a rewrite;
the window restart check is what proves the restart.

## 22. Closing a tab asks first when it holds blocks

Closing a tab (tab bar x, the close key, the vertical tab bar) deletes it and stops every program in it; upstream has no
reopen and, with the default setting, no question. In this fork a tab that holds blocks always asks first ("This tab holds
N blocks. Closing it stops the programs running in them, and it cannot be undone."), whatever `tab:confirmclose` says; an
empty tab asks only when the setting does. If the block count cannot be read the question is asked anyway.
- **RAN:** vitest for the rule (`closeTabPrompt`: blocks always ask, singular, empty tab, unknown count). tsc stays at 17.
- **READ, not run:** the dialog in the main process (`close-tab` handler) in a window. `wsh tab close` and
  `wsh session close` are not questions: they are commands with their own rules.
- **Not done:** reopening a closed tab (about 2 working days).

Review notes on section 22. **Which routes ask, which refuse, which do nothing:**
- Tab bar x, vertical tab bar, the close key: all go through the one `close-tab` handler: they ask (asynchronous dialog, the rest
  of the app keeps running). Only the explicit "Close Tab" answer closes; Escape, a dismissed dialog or any other result keeps the
  tab. A second close request for the same tab while its dialog is open is ignored. The count is the tab's block list, which
  includes the members of a stack, hidden ones too. Tested through the handler (`closetabhandler.test.ts`).
- `wsh tab close <tab>`: **refuses** when the tab holds blocks (the error says how many) unless `--force`; no dialog, because the
  callers are scripts and agents. The check is in the wsh command: a direct RPC call of `electronclosetab` is not checked.
- `wsh session close`: has its own rule (a tab whose program is still running needs `--force`).
- Closing a whole window, deleting a workspace and quitting the app are separate flows with their own settings
  (`window:confirmclose`); they are NOT covered here.

Second review pass on section 22: the guard against a second close request is set before the first await; the block count is read
again before the close, so an empty tab that gained a block, or a confirmed tab that grew, asks again, and a tab that keeps
changing is not closed (`closetabhandler.test.ts` has deferred, overlapping reads). The `wsh tab close` check now also happens in the
main process right before the close (new `force` field of the electronclosetab call), so a direct RPC call of it is covered too;
`force` is the deliberate bypass (next paragraph). A source-level route test checks that the UI routes go through the one handler.

Third review pass on section 22: `force` of the electronclosetab call is the **deliberate bypass for every local RPC caller**, not
only for `wsh tab close --force`. A local caller that can reach the call can already delete blocks, so the refusal guards against
mistakes; it is not a security boundary. No UI route can pass it: the IPC close route has no such parameter and no front end file
sets it (both are tests in `closetabhandler.test.ts`). The handler's window argument is now required.

Final pass on section 22: the count that was approved travels with the close. The close from the window (and the `wsh` route when the
tab was empty or had a count) calls a back-end service, `CloseTabChecked`, that compares the tab's block count with the approved
one **in the same Go function that starts the close** (only synchronous database reads in between, no round trip through the
window); a tab that gained blocks is not closed, and the caller is told so. It is not one database transaction: a block created in
the same few microseconds on another goroutine would not be caught. The window's close request now resolves with whether the tab
was really closed, and a close request that was replaced in the window's action queue resolves false.

Fourth pass on section 22: whatever leaves the window's action queue (a destroyed window, an early end) first tells every close that is
still queued "not closed" and empties the queue, so a caller (the window, or `wsh tab close`) gets an answer instead of waiting
(tested). The unchecked back-end `CloseTab` is reached only for `force` (no limit). A close that the owner confirmed without a count (both reads failed, and the dialog said so) goes through the checked call as "count unknown" (-2): the back end reads the tab itself and refuses if it cannot (tested on the window side, and a test pins that the Go constant is the same -2). The check and the delete are not one database transaction, because the delete is upstream code spanning many store calls; a block created in the same few microseconds is the one case not caught.

## 22a. Closing one block that has a running program asks first (found in his window check, 11 Oct 2026)

The X on a stack member closed its block, and so stopped the program in it, with no question (it called `closeNode` directly); the block header
menu and the close key did the same (upstream behaviour). Now all three ask first, but only when a program is running: a native box
"This closes the program running in it. It cannot be undone." with Cancel (Esc) and Close Block (Enter). The state comes from the controller
(`GetControllerStatus`, the same state as `status` in `wsh api list`) for the views that have programs (`term`, `tsunami`, `vdom`): `running` asks;
only `done` (ended) and `init` (not started), or a view that is KNOWN to have no program (the list of the registered views of this build, in
`closeblock.ts`: web, preview, files, sysinfo, help, ...; a view name that is in neither list, such as one added by a later upstream version, asks), close silently. Anything else asks: a controller that
is missing for a terminal, an empty or unknown state, a state that cannot be read, a view that cannot be read. Only one box per block at a time, held from the question until the close has finished (a
second click on the X while the box is open, or while the close is running, does nothing); the block that was asked about is the one that closes, even if focus moves
while the box is open; Cancel touches nothing. The box is shown only to a Wave tab view (a web tile cannot reach it). **Named exception:** a
command (`wsh deleteblock`, the backend delete action, the local API) never asks, because nobody is there to answer; it is a different thing from
a click on an X, and a test pins that the delete case and `closeNode` do not call the question. The last block of a tab still goes through the tab question. A block closed by
a command (`wsh deleteblock`, the backend delete action) is never asked, because there is nobody to answer.
- **RAN:** vitest for which states ask, the answer deciding, a failed read asking, a failed dialog keeping the block, and source pins on the
  four call sites, on the commands not asking, and on the electron side (type, preload, handler with the message capped, preview stub).
- **NOT RUN, window check:** the box itself. Put a `ping -t localhost` in a stack member and click its X (the box appears; Cancel keeps it
  running; Close Block ends it); the same for the header menu "Close Block" and for Alt+W (the close key is `Cmd:w`, and Cmd is Alt on Windows; Alt+Shift+W closes the whole TAB through the tab question; Ctrl+Shift+W is not bound to anything); Ctrl+Shift+X (replace the block by a launcher) and the X or Alt+W with the AI panel open and a non-empty chat ask the same way. A web tile closes with no box. **An idle shell asks too:** the state `running` is the shell process itself, and the PowerShell integration of this build sends no "a command is running" state, so there is no way to tell an idle prompt from a running command (the fork would need the shell-integration command state for PowerShell, or a check for child processes of the shell).
## 23. Drag a tab out of its stack, reorder tabs (step E)

Each tab on a stack's strip is a drag source of the same kind as a tile header. Dropped on the **edge** of a tile it becomes a tile of
its own (the existing move takes it out of the stack; the neighbour becomes active; the node is the same, so a terminal keeps
its process and scrollback); dropped on the **middle** of another tile it joins that tile's stack; dropped on **another tab** of
the same strip it takes that tab's place; dropped on a tab of another strip it joins that stack after the tab. A tab dropped
on itself does nothing. A stack of one that loses its only tab leaves no empty stack behind.
- **RAN:** vitest for the rules (`reorderStackMember`, `tabDropAction`, an edge drop out of a stack of three and out of a stack of one
  with the same node object kept); tsc stays at 17; Check-Seams 0 red.
- **READ, not run:** the react-dnd wiring on the tab (drag source, drop target, outline style `drop-over`): the real drag needs a
  mouse, so it is part of the window check. **Seen in a window after this PR:** drag a tab off the strip onto the edge of another tile.

Review notes on section 23: the drag itself cannot be driven in the unit tests (react-dnd needs a test backend and a DOM that this
project does not have), so what the tests prove is the tree rules; the wiring of the tab as a drag source is **unproven until it is
seen with a mouse**. "The same node object" does not prove React keeps the terminal mounted either. Window check list for this PR:
(1) drag a tab off the strip onto the edge of a tile and check that it becomes a tile; (2) before the drag type a marker in that tab's
terminal and note its process id; after the drag the marker must still be on the screen and the process id must be the same
(`devdata/proof/drag-check.ps1` in the private test folder does the marker and the pid part with `wsh send` and `wsh screen`);
(3) drop a tab on the middle of another tile, on another tab of the same strip, and on a tab of another strip.

## 23a. Dropping a dragged tab on a stack's strip (found in his window check, 11 Oct 2026)

Reordering by dropping a tab on another tab did nothing, and the highlight over a stack's tab bar showed "a new tile above". Cause: during a
drag the overlay layer covers the layout and is the only thing that gets drag events; the tiles' overlay nodes cover only the body of a
stack, so the strip had no drop target at all (the tab's own drop target, which I wrote, never saw the pointer) and the pointer over the strip
kept the last pending action of the body. Now each strip has a drop target in the overlay (`OverlayStrip` in `TileLayout.tsx`): over the
strip the only possible actions are those of a tab drop, chosen by the tab under the pointer (`stripTabAt`): inside the stack it reorders
(dropped on the empty end or the + it goes to the end), from outside it joins the stack after that tab. The tab under the pointer gets the
`drop-over` outline on the side where the tab will land (`drop-before` / `drop-after`); a body hover clears it, and a strip hover where nothing can be dropped clears the pending action of the body.
- **Pinned only by source text (no DOM test environment here):** that the overlay renders the strips, that the strip asks `applyStripHover` and commits on drop, that the tab carries the
  outline classes and the stylesheet has them. The decision itself (`stripHover`, `applyStripHover`) is tested as functions; a stale action left behind is red.
  Known limit: over the dragged tab's own slot the drop is always allowed by the overlay (cursor only; the drop does nothing).
- **RAN:** vitest for `stripTabAt` (inside, boundary, left of the first, the empty end, a gap, no tabs), for the tree rules at the end of a
  strip, and source pins on the wiring (the overlay strip exists, the tab has no drop target of its own, a body move clears the outline).
- **NOT RUN, window check:** the drag itself. Drag a tab over another tab of its strip (outline on the tab under the pointer, no
  placeholder above the tile), drop (the tabs swap places, the terminal is not restarted); over the empty end of the strip (goes last); a
  tab from another tile onto a strip (joins it); then move off the strip into the tile body (the outline goes away and the usual
  tile placeholder returns).

## 24. The live state record (what runs where, readable from outside)

Wave writes `<data dir>/state/wave-state.json` after every change to a workspace, tab, block, layout or window (a 300 ms
coalescing delay, no polling and no timer), by writing a temp file in the same folder, flushing it and renaming it over the
old one: a reader sees the old record or the new one, never half of one. An unchanged state is not rewritten; `sequence` counts
the writes; a failed write leaves the old file in place and is retried at the next change.
Words: a **workspace** has **tabs** (the T1, T2 row), a tab has **tiles**, a tile of kind `stack` has a **group** of block tabs
(in order, one active), a plain tile has a group of one. Per block: view, title, working folder, the program name only (never
its path, arguments, address or environment: those can hold secrets), the controller, and every `session:*` meta value
(`session:name`, `session:id`, `session:model`, `session:effort`, `session:permissionmode` ...: the starter writes them with
`wsh session start --meta`, see section 21). `sessions` is an index of the blocks that carry a session id: workspace, tab,
tile position, stack and position in its group, so that an outside tool can put things back.
A block that the saved layout does not show yet (the page saves its layout 100 ms after a change) is still listed, as a plain
tile at the end of its tab.
- **RAN:** Go tests for the builder (names, order, active tab of a group, secrets never in the file, sessions index, missing
  layout or block), the writer (atomic replacement, a failed rename keeps the old file and leaves no temp file, no rewrite of an
  unchanged state, a burst gives one build, no write without a change, retry after a failure) and the event filter; two
  mutations (skip-unchanged, sequence) turned tests red.
- **READ, not run:** the loader that reads the object store and the hook in wavesrv startup (needs the database), and the
  file as the app really writes it.
- **Private reader:** `Read-WaveState.ps1` (outside the repository) prints the record and the command lines to resume each session.

Review notes on section 24 (the state record), asked by the reviewer:
- **Where:** `<data dir>/state/wave-state.json`; the folder is created for the current user (mode 0700 on systems that have modes; on
  Windows it inherits the access of the data folder, which sits in the user's own profile or test folder). It is not locked down further
  the way the local API folder is: it holds no token.
- **What could be private:** block **titles** (set by the user or by the program running in the block, so they can contain anything a
  program prints into its title) and **working folders** (paths with the user's name and project names) are in the file on purpose,
  because the folder is what puts a session back. Never in the file: urls, environments, command lines or arguments (only the
  program's base name), and any `session:*` value except name, id, model, effort, permissionmode, status-session and status-tab
  (each at most 200 characters).
- Only blocks that belong to a tab are recorded in it, each once; a block that the saved layout still names but the tab no longer holds,
  or names twice, is not shown as a ghost or a duplicate.
- A failed database read keeps the last good file (the record is never published incomplete). A replace that fails because a reader
  holds the file open is retried for about two seconds before it is given up and tried again at the next change. A test with a real
  open reader on Windows fails if the retry is removed.
- A test changes an object through the event stream and checks that the file changes (it fails if the observer is never registered).
- **Look and attention in the record (added 11-Oct-2026, for a status page that is built outside Wave):** each block has `icon` (`frame:icon`, else `icon`), `iconcolor` (`icon:color`), `attentionlevel` (the reason is not recorded: the file has no token and anyone on the machine can read it, while a block's owner writes `attention:reason` freely, so a password or token could be in it; the reason still decides whether a mark is drawn, so a reason with a hidden character records no level)
  (`attention:level`, `attention:reason`), each tab has `flagcolor` (`tab:flagcolor`). Every one of them is written only when it passes the rules that the window applies before it draws it, and is left out otherwise (never cut or repaired):
  the icon by the symbol rule (`pkg/staterecord/look.go` `SymbolValue`, a port of `parseTabSymbol`: a Font Awesome name of at most 40 characters or text of at most two graphemes and 32 bytes, no control, format or bidirectional characters, no stray
  joiner; a joiner is accepted only between two emoji, so letters joined by one are refused: the window's own rule only refuses a joiner at an edge, doubled or beside white space, the record is stricter), the colours by the colour rule (`ColorValue`, a port of `memberColorValue`: a hex colour, a colour function whose arguments are only numbers, percentages, numbers with an angle unit and the word none (stricter than the window, which lets any word through: `rgb(background)` is not recorded), or a CSS colour name, at most 64 characters, no hidden characters), the mark by `pkg/attention` (a
  mark that would draw nothing is not recorded). One difference on purpose: the window asks the browser whether a colour keyword is a colour; the record accepts only the CSS colour names (plus transparent and currentcolor), so it is never looser.
  The record file is written atomically (a temporary file, then a rename; a reader that holds the file open makes the replace retry), so a page that reads it never sees a half written file. No new event is needed: a change to a block or a tab
  already rewrites it. RAN: `go test ./pkg/staterecord/...` with cgo, including the vectors of both rules, the record tests, and a chain test with a real store that sets a mark, changes its reason, then writes markup, style text and a hidden
  character, and reads the FILE each time (the file changes, and none of the hostile text reaches it). NOT RUN: a real Wave writing the new fields. The colour rule's hidden-character check is redundant with its shapes (no shape allows such a character);
  it is kept as the same defence the window has.


## 25. Keyboard: next and previous tab inside a tile

The design: `Ctrl+Shift+PageDown` shows and focuses the next tab of the stack that holds the focused tile, `Ctrl+Shift+PageUp`
the previous one; both wrap around. **This is unproven until the window check** (see the last point). Nothing happens (the key is not taken) when the focused tile is not a stack or has only one tab. The
chords were chosen because neither is bound in Wave's key map (checked against `keymodel.ts`); Wave's own `Alt+[` and `Alt+]`
still switch the tabs of the workspace (the T1, T2 row). The rule is a pure function (`adjacentStackMember`) with tests (order, wrap,
non-stack, stack of one, unknown id, focus lands on the shown tab).
- **Proven by tests:** the rule, and the dispatch (`stackkeys.test.ts`: both chords are registered through one function, next steps
  forward and previous back, the key is taken only when something was switched and passes on when the tile is not a stack or has
  one tab, and no chord collides with one the app binds). Showing the target: `focusNode` makes the member the active tab of its stack
  (tested), so the target is revealed.
- **Not checked, and what is expected:** a focused text field of the app (for example renaming a tab) would let the chord switch tabs
  too, which does no harm; a web tile keeps its own keys inside its page, so the chord reaches the app only when the app, not the
  page, has the keyboard focus; the onboarding screens switch all global keys off. **READ, not run:** that the keys reach the app while a terminal has the keyboard focus (other global chords such as
  `Ctrl+Shift+Arrow` do), and the chord names (`PageDown` is the key name the browser reports): part of the window check.

## 25a. Command history per PowerShell tile (setting `term:perblockhistory`, on in this fork)

PowerShell keeps one history file for the whole machine (`%APPDATA%\Microsoft\Windows\PowerShell\PSReadLine\ConsoleHost_history.txt`); every
new shell loads it, so after a restart pressing Up in any tile showed the last command typed in another tile. Wave does nothing about history
for PowerShell (checked: its start-up script has no history line). Now a local PowerShell block gets its own file, `<data dir>\history\<block id>.txt`
(the block id survives a restart, so a tile gets its own history back; a new tile starts empty, as asked). The path travels in the shell's
environment (`WAVETERM_PWSH_HISTFILE`, only for local PowerShell and only when the setting is not false) and Wave's PowerShell start-up script
sets PSReadLine's `HistorySavePath` to it; if the line editor is already running, its history is replaced with the block's file. The variable
is removed after use. When a block is deleted its history file is removed by name (one file, validated as a uuid so an id can never point
outside the folder). Bash and zsh are left as they are (zsh already keeps Wave's own history file and merges it into `~/.zsh_history`).
- **RAN:** Go tests (the path only for a uuid id, on unless switched off, local PowerShell only, the folder is made, removal takes exactly one file
  and a bad id is ignored, the start-up script text sets the path inside a try); `task generate` for the setting (clean diff).
- **Probed, not in a Wave window:** in a non-interactive PowerShell 7.6 with PSReadLine 2.4.5 the methods `ClearHistory`, `AddToHistory`,
  `GetHistoryItems` exist, but they fail without a running line editor; the start-up script therefore sets the path first and only then tries
  them, inside a try. Whether the path alone is enough before the first prompt, or the replace is needed, is the window check.
  **Loading without doubling, and multi-line commands:** when the line editor is already running, the saved entries are put back with saving switched off
  (and the save style restored afterwards, also on a failure), because with incremental saving every added entry would be appended to the file again and
  every restart would double it. The file is read in PSReadLine's own format: a command of several lines has a backtick at the end of every line but the
  last, and those lines are joined again; empty lines are skipped. Both are run in a real PowerShell by the Go tests (the save style goes off before the
  first entry and back after the last, also when adding fails; with the switch-off removed that test goes red).
  **Limit, on purpose:** the block only acts when the line editor has a `ClearHistory` method (PSReadLine 2.4.5 on pwsh 7.6, the one on his PC, is the version
  this was checked against; Windows PowerShell 5.1 with PSReadLine 2.0.0 also has the method); an older one that had already loaded the shared file would show it anyway while saving to the block's file, so there the
  history is left alone (the shared one). The start-up block is run in a real PowerShell by the Go tests: it sets the path from the environment,
  removes the variable, and a failing or missing line editor neither breaks the start nor changes the path (with the path line removed the test goes red).
- **NOT RUN, window check:** open a new PowerShell tile (Up shows nothing from other tiles); type two commands; quit Wave and start it again
  (Up in the same tile shows those two, not other tiles' commands); delete the tile and look that `<data dir>\history\<id>.txt` is gone.
## 25b. Right click copies or pastes in a terminal (setting `term:rightclickpaste`, on in this fork)

Wave opened its context menu on every right click, and he had to pick Paste from it (found in his window check, 11 Oct 2026). With the setting on, a right click in a terminal
works as in Windows Terminal: it copies the selection (trimmed as the menu's Copy does, then the selection is cleared) or, with nothing selected, pastes
(the same native paste as the menu item). Shift or Ctrl held while right clicking still opens the menu, so the other items (open a link,
magnify, send to Wave AI) stay reachable. The setting is off upstream and on in this fork's defaults; set it to false for the old behaviour.
Two safeguards found in review. The paste goes to THIS terminal: it takes the block focus and the keyboard focus first, because the native paste lands
in whichever element holds the focus (a right click on a terminal that did not hold it would otherwise paste into the one that did). A program that
takes the mouse itself (vim, Claude Code: the terminal's mouse tracking is on) keeps the click: Wave opens no menu and copies and pastes nothing (Shift
+ right click still gives the menu). The paste is sent only when the terminal's input element really holds the keyboard focus: Wave waits for it (up to 300 ms) and
pastes nothing at all if it does not settle, rather than risk the other terminal. The selection is cleared only after the clipboard took the copy. Multi-line text: the native
paste ends in the terminal's own paste handler (xterm `paste`), which wraps the text in bracketed-paste markers when the program asked for them (PowerShell's
line editor does), so a multi-line paste is not run by itself; a shell that does not ask for them (plain `cmd.exe`) receives it as typed, as with Ctrl+V;
`term:allowbracketedpaste` (on by default) is the setting that switches that off.
- **RAN:** vitest for the decision (setting off or unset gives the menu whatever is selected; on: copy with a selection, paste without; Shift or Ctrl, or a
  program that takes the mouse, gives the menu), the whole click run against two terminals with different keyboard focus (a right click on B while A holds the
  focus pastes into B; with the focus step removed that test goes red), and source pins on the wiring and the setting; `task generate` diff is only the setting.
- **NOT RUN, window check:** right click with nothing selected (pastes, also into a terminal that did not have the focus), select text and right click (copies,
  selection goes away), Shift + right click (menu), right click inside vim or Claude Code (the program gets it).

## 26. Is the program in a block still running? (`status` in `wsh api list` and in the state record)

Each block that has a program now shows one of three words: `running`, `exited` (with its exit code, also `0` for a clean exit) or
`notstarted`. A block with no program (a web tile, a file view) or a state this code does not know shows nothing: unknown is never
reported as running. It is read from the block controller's own state (`GetBlockControllerRuntimeStatus`), not from block meta, so a
block cannot claim a status by writing one. `wsh api list` prints it after the title (`running`, `exited(2)`); `wsh api list --json`
has `status` and `exitcode`; the state record has the same two fields per block in a tile's group, and the record is rewritten when a
program starts or exits (the `controllerstatus` event with a block scope now counts as a change).
- **RAN:** Go tests (`pkg/blockstatus`, `pkg/staterecord`, `pkg/waveapi`, `cmd/wsh/cmd`): the mapping, that an exit code of 0 is kept,
  that unknown is empty, that the record changes when a program exits and is rewritten on the event, that the API block still has
  only allow-listed fields and never takes a status from meta. Check-Seams: 2 new rows (`block-runtime-status`, `block-status-values`).
- **READ, not run:** a real program starting and exiting in a window and the file then changing; `wsh api list` against a live Wave.
- **Review of section 26:** the record writer used to push its timer back on every event, so a program that restarts in a loop (events faster than the 300 ms debounce) would have kept the file stale for as long as the loop lasted. It now arms the timer on the first event and lets the later ones ride on it: at most one write per window, and at least one (a test sends an event every 10 ms for 600 ms and needs 5 to 16 builds while they last; the old writer gave 0). An exit event is driven end to end in a test with a real store: the file shows `exited` and the code, and the test goes red when the event is dropped.

## 27. Put sessions back after a restart (`tools/ours/Restore-Sessions.ps1`)

Reads the state record and, for every recorded session that is not running, starts it again in its own background tab with
`wsh session start --restart`, running `Start-ResumableSession.ps1` (first start creates the session under its id, later starts resume
it). A session that was a member of a stack is added to the group of another member with `wsh group add`, when one is known (a
running member, or the first one restored here). **It only prints the plan unless `-Execute` is given.**
- **Never starts a session that is already running:** a session counts as running when a file in the sessions folder
  (`-SessionsDir`, default `~\.claude-personal\sessions`) names its id and the process in it is alive with the same start time (a reused
  pid does not count). The check is made again right before each start. A folder or a file that cannot be read stops the script with
  exit 1 and no wsh call: it cannot prove a session is not running.
- **Never uses a bypass mode:** only `auto`, `default`, `acceptEdits` and `plan` are passed on; any other recorded mode (bypass,
  skip-permissions) is dropped with a NOTE and the session starts in its default mode. The script has no switch for it.
- **RAN:** `tools/ours/Test-RestoreSessions.ps1` (stand-in wsh, made-up record and sessions folder; count the GREEN lines it prints, incl. dry run calls nothing,
  running skipped, dead pid and reused pid not counted, bypass and unknown modes dropped, a session that appears between the look and
  the start is skipped, unreadable folder or file stops it); with the running check removed the test goes red.
- **READ, not run:** against a live Wave with real sessions; where a restored tab ends up in the layout (it opens a new background tab; only
  stack membership is restored, not the tile's place in the tab).

## 36. Web tiles by command: reload, back, forward, url

`wsh web reload|back|forward|url [-b block]` act on a web tile without taking focus. `url` prints the current address and then the title (`--json` adds whether the tile can go back or forward). The block is any
reference that names a web block (`-b`, default the calling block); the command reads the block's own tab and workspace, so the tile may be in a tab other than the caller's, as long as that tab is loaded in its window
(shown once; a tab view stays loaded in the cache, 10 tabs by default). Not a web block, or a block that cannot be read: refused before anything is sent. Back or forward with nowhere to go is an error. The answer
carries the address the tile had when the command was handled; a navigation finishes afterwards, so a script that needs the new address asks again with `url`.
How it works: a new window command `webnav` (`CommandWebNavData`, answered by the Electron side like `webselector`): the window asks the page of the request's tab for the tile's web contents (the old code asked only the
shown tab), then calls reload or the navigation history of those web contents. The block id is checked to be a full id before it is put into the page query.
- **RAN:** Go tests with the block info and the window call replaced: the request carries the block's own tab and workspace, only a web tile is accepted, each of the four commands sends its action, a refusal from the
  window comes back to the caller, the output format; vitest for the web contents side with stub contents (reload, back and forward only when possible, url reads only, an unknown action and no contents are errors); source pins
  on the handler (tab loaded, tab-specific lookup, id check); red when the "nothing to go back to" check is removed.
- **NOT RUN in a window:** the commands against a real web tile, in a shown tab and in a tab that is loaded but not shown; whether `file:///C:/...` and `http://127.0.0.1:<port>/` pages load in a web tile. Reading
  only: the tile accepts `file:`, `http:` and `https:` addresses unchanged (`ensureUrlScheme`), a bare localhost or IP address gets `http://`, and the guest page is not blocked by the tab view's navigation handlers (they
  are on the tab view, not on the guest). A real run was not done because the owner's preview window is open and no window test is run while he is at the PC.

Review fixes for section 36 (Codex on PR 47), each red without it:
- **Authorization in the handler:** none of the three ids is taken on trust. The window asks the block's own record (`BlockInfoCommand`) and refuses unless the block is the one named, in the tab named, the tab in the workspace named, and a web tile (`authorizeWebNav`); this comes before any
- **The CALLER is bound too (round 2 of the review):** the ids in the request come from the caller, so they only say what the caller WANTS. Who the caller IS comes from the router: it stamps the source route on every message from a leaf link, and at authentication it records the block id of the link's token (`SetLinkAuthBlockId`, `BlockIdForSourceRoute`; the session cannot choose either). Before any window, tab or web contents is looked at, emain asks the server (`WebNavAuthorizeCommand`, with `rh.getSource()` as the caller route and the block): the call is refused unless the caller's block is in the SAME workspace as the web tile (a route with no authenticated block, a remote caller behind another router, an unknown tile and a missing workspace are refused too; the server's own route, used by the AI tools, is trusted). READ: no layer above did this (the router stamps the source but has no workspace rule, and any trusted link may address the electron route). A navigation never focuses a window or changes the shown tab (the request only sends an IPC message to the page of the loaded tab and calls reload or a history step on the web contents; READ, not RAN in a window). RAN: go tests (router: the block behind a source route, a route that is not a leaf's own source route gives nothing; server: A to B refused, B to A refused, unknown route refused, same workspace allowed, the server allowed), vitest pins on the order in the handler; mutations red: workspace compare off, caller block not required, the router not recording the block, the source route not checked, the emain call removed, the emain call after the lookup. NOT RUN: a real second workspace in a window.
  **Round 3:** a link authenticates ONCE: a second token on the same link is refused unless it is for the same route and block (`linkAuthIdentity`), the result of binding the route is checked, and the block is recorded only after a successful bind and only once per link (`SetLinkAuthBlockId` refuses a second, different block and a link without a source route). One denial text (`WebNavDenied`) for a tile that does not exist and a tile in another workspace. The question to the server is asked a second time right before the action (a belt). READ, why a tile cannot change workspace under the check: the block list of a tab changes in exactly two places, `CreateBlock` (`pkg/wcore/block.go:149`, append) and block deletion (`block.go:221`, remove), and the tab list of a workspace only by creating and deleting a tab (`workspace.go:286`, `:308`): no code moves a block to another tab or a tab to another workspace. READ, tokens: a block's token (`WAVETERM_JWT`, or the swap token that becomes it) is minted by wavesrv when it starts that block's program and lives in that program's environment and in memory (the swap token is used once). A session in another workspace does not get it through Wave; but any process of the SAME Windows user can read another process's environment (I read one with the PEB reader in tools), so this binds the commands against ids from the wrong workspace, it is not a defence against a hostile process of the same user.
  **Round 4:** wsh asks the server FIRST (`WebNavAuthorizeCommand` called by the CLI itself, so the server judges the route that the router stamped on that call), before the CLI reads anything about the tile: a tile that does not exist, a tile in another workspace and a block reference that cannot be resolved all end in the one text (`that web tile is not available to this caller`, also when the window's answer carries it). The command also closes an oracle: only emain may name another caller's route in the request; from anybody else the stamped route of the call itself is used and the claimed one is ignored (a session cannot ask in the name of another route or of the server, nor find out which blocks of other workspaces exist). **Finding, not fixed here:** `BlockInfoCommand` (any trusted client, so any session's wsh) returns the full record of ANY block id, with its meta and file list, whatever its workspace: it is its own access question (it is also what `wsh` uses to find a tile's tab and workspace), listed for the morning.
  **Round 5 (run-the-code review):** (1) BLOCKER fixed: the JWT path (`AuthenticateCommand`) did not record the block, and every wsh command but `wsh token` authenticates with the JWT, so every real session was refused, on its own workspace too; both ways of authenticating now go through one function (`bindLeafIdentity`) that sets the identity once from the verified claims, and tests authenticate a link the way wsh does (a real JWT through `AuthenticateCommand` on a registered link, then the route leads to the JWT's block; a second JWT for another route or block is refused). (2) `wsh web get` (webselector) asks the same question before it looks at any window (the change of `emain-web.ts` that asks the tab named in the request would otherwise have widened it). (3) Master merged in. (4) **Limit, older than this PR:** a ROUTER link (every websocket, a remote or WSL connserver) is not stamped with a source and may name any: the question holds against sessions on leaf links (the wsh of a block), not against a remote machine that Wave is connected to. (5) The question has a time limit (3 seconds, shorter than wsh's 5), so a late yes cannot act after wsh gave up; workspace lookup errors are logged (and still end in the one denial); the address that `url` returns has no user name or password; a test pins that no handler calls a focus, show or tab switch. Not closed: if the tab unloads between the loaded check and the lookup, the lookup falls back to the shown tab (the second question right before the action still binds the block to the caller's workspace). **Follow-up (after the merge of PR 47, written, NOT RUN):** closed in `emain-web.ts` (a named tab that is not loaded gives null at once, the shown tab is never asked in its place; source pin in `emain-webnav.test.ts`); a workspace lookup that fails is never believed even when it returns a value (the value is dropped, the question fails closed; Go test with a lookup that returns the caller's workspace and an error); a Go test that every way to forge a JWT (changed signature, claims of another block under a good signature, alg none with and without a signature, a cut signature, two parts, junk, empty) leaves the link untrusted with no route and no block, with the real token as the control. The BlockInfo and wall findings of the review are in the one-pager for the morning (a session of workspace A can read the tab, workspace, address and title of a tile in B, complete a short id, set its url, read the page of a tile in the shown tab of any workspace's window, and create a block in B's tab: the workspace-walls decision).
  web contents are looked up. Every caller is a block of the same user's Wave (wsh can already act on any block id), so there is no other owner to check.
- **Schemes:** these commands reload, and step back and forward through, **http and https pages only**, checked on the history entry that the step would land on (`getEntryAtIndex`); a file:, data:, javascript:, devtools:, chrome:, view-source:, about: or unparsable entry
  is refused with a message, nothing is navigated. What the tile itself accepts today (`ensureUrlScheme` in the web view): an address starting with http:, https: or file: is used as it is; localhost and IP addresses get http://; a domain gets https://; anything else becomes a
  search. So a file: page can be opened in a tile (`wsh web open file:///...`) but these commands do not reload it or step back to it; say if file: is to be added to the allowed list (`AllowedNavSchemes`).
- **Printed text:** the address and the title come from the page. The command prints every control, format and separator character as a visible `<U+XXXX>` code (so no terminal sequence reaches the terminal), in text and in JSON, and cuts the address at 2048 and the title at 200 characters
  (the window cuts them too before it answers).
## 28. A member's icon and colour on its strip tab (colour work, step 1)

Each tab of a stack strip draws the member's symbol before its name: `frame:icon` when that is a usable symbol, else `icon` (both validated by
`parseTabSymbol`: a Font Awesome name or at most two graphemes of text, so an emoji works; hidden and bidirectional characters are refused).
The member's colour is `icon:color` (a value that is not a CSS colour counts as no colour): a 2px line in that colour under its tab, and a
tint (22 percent) on the tab of the member that is shown. A member without a colour looks as before. The decision is `stackTabLook`
(`frontend/app/tab/stacktablook.ts`); the label is `stacktablabel.tsx`; the placement is in `stackstrip.scss`.
**What is accepted** (one place each, `stacktablook.ts`, applied when the tab is drawn, so a value written by `wsh setmeta` or any program is checked too):
colour (`memberColorValue`): at most 64 characters; `#rgb`, `#rgba`, `#rrggbb`, `#rrggbbaa`, or rgb/rgba/hsl/hsla/hwb/lab/lch/oklab/oklch with plain arguments (no nested
parentheses, so no `var()`, `url()`, `calc()`, `expression()`), or a bare keyword of 3 to 30 letters; and where a window exists `CSS.supports("color", ...)` must accept it too
(`validateCssColor`), so a keyword that is not a colour is refused there. Symbol (`parseTabSymbol`): a Font Awesome name of at most 40 characters (`[a-z0-9-]` with an optional
`solid@`/`regular@`/`brands@`/`custom@` prefix and `+spin|beat|fade|fw`), or text of at most 2 graphemes and 32 bytes; control, format and bidirectional characters and misplaced
joiners are refused. A string of combining characters counts as one grapheme, so it is bounded by the 32 bytes: a base letter with up to 15 marks passes, 16 do not; the strip also
clips the text symbol to 3em, so nothing a program writes can widen a tab. The label of a member without a title is the last folder of its cwd, with `/` or `\` as separator.
The tint is drawn behind the label and the symbol (the tab isolates its stacking context; the fill has z-index -1).
- **RAN:** vitest for `stackTabLook` (frame:icon before icon, a refused frame:icon falls back, emoji, three graphemes refused, no icon or colour,
  a colour that is not a colour); red when the order is swapped and when the colour check is dropped. Source pins on the label and the stylesheet.
- **NOT RUN, window check:** that an emoji renders in the strip (and in the tile header, which is not changed here), the line and the tint on
  a real strip, a member without a colour looking as before.
- **Later steps (own PRs):** the right-click menu on strip tabs and Set colour / Set icon, tile outlines, top row, window outline, one wsh line.

## 29. Set colour, Set icon and the strip tab menu (colour work, step 2)

The block header menu (gear, right click on the header) has **Set colour** and **Set icon**; a right click on a tab in a stack's strip opens
Take out of stack (only when the stack has more than one member), Close (through the close question, as the X does), Rename (writes `frame:title`;
empty clears it), Set colour and Set icon. Colour: None, the seven tab flag colours, or Other (a CSS colour typed or pasted) and writes `icon:color`.
Icon: None, a short list (terminal, robot, code, bug, rocket, star, flask, wrench), or Other (an emoji or a Font Awesome name typed or pasted) and writes `frame:icon`.
**One place validates:** `stacktablook.ts` (`memberColorValue`, `memberSymbolValue`; see section 28 for what they accept). The strip applies it when
it draws, so a value set by `wsh setmeta` or any program is checked there; the menus apply the same functions before they write (`memberLookMeta`), and a
refused Other text is asked again with the text kept (five tries, a cancel ends it) and nothing is written until one is accepted. A text symbol is
clipped to 3em in the strip, so a long string cannot widen a tab. The question box is `TextPromptModal` (new, `frontend/app/modals`).
- **RAN:** vitest: what each choice writes, what is refused (url(), var(), three emoji, 41 characters, markup), None clears, a cancel writes nothing, a refused text asks
  again, Take out of stack only with more than one member, Close and Rename, and source pins for the wiring (header menu, strip right click, tab content, window deps, modal
  registry). Red when the colour check is removed from `memberLookMeta` and when Take out of stack is always shown. Two seam rows added.
- **NOT RUN, window check:** the menus themselves (a right click on a strip tab, the nested submenus, the question box typing and pasting an emoji, Esc and Enter),
  that Rename shows in the strip and the header, and that the colour and icon appear at once.

Review fixes for section 29: a name (Rename, or any caller of `memberLookMeta`) must have no control, format or line characters and at most 60 characters
(`memberTitleValue`); the strip draws a title through `drawnTitle` whoever wrote it (`wsh setmeta` too): hidden characters mean the title is not used, a long one is cut to 60.
The menus check that the block still exists before the menu is shown and again before each write. The Close of the strip tab menu is built by `makeStripTabMenuInfo`
and goes through the same route as the X (`closeStripTab`); tested with a question that refuses (nothing closed) and with none set up (nothing closed).
Colours: control characters and newlines are refused before any pattern, and the function pattern allows spaces only (section 28).

## 31. Tile outlines in colour (colour work, step 3)

The outline of a tile (the border drawn by `BlockMask`) is computed by `tileOutlineColor` (`frontend/app/block/tileoutline.ts`). The order, exactly:
- **Focused tile:** 1. the block's override `frame:activebordercolor`; 2. the block's own colour `icon:color` (the agent's colour); 3. the tab's colour: first its flag colour `tab:flagcolor`, then
  the border colour of its background (`bg:activebordercolor`, from the tab or its background config); 4. the stylesheet's accent, which is the workspace colour (`ws:accent`).
- **Tile that is not focused:** 1. `frame:bordercolor`; 2. the tab's colour: `tab:flagcolor`, then the background's `bg:bordercolor`; 3. an ephemeral tile with none is white; otherwise no colour.
"Tab colour" includes the tab background's own border colour, after the flag colour. Every candidate, the explicit overrides and the tab background's colours included, goes through
`memberColorValue` (section 28); one that is not accepted counts as not set, so the next one applies. Only the outline is coloured: no header and no tile is filled (pinned: the outline
function is used only in `BlockMask`, assigned only to `style.borderColor`; the header and utility files do not use it). The window outline and the top row are later steps.
- **RAN:** vitest for both orders, for every candidate refused when it is not accepted (url(), var(), a newline, a number, empty), the ephemeral white, and the source pins; red when the
  focused order is swapped, when the unfocused outline takes the block's colour, and when the override is not validated.
- **NOT RUN, window check:** a focused tile in a tab with a flag colour and a block with `icon:color` (outline = the block's colour), a tile without colours (workspace accent as before),
  unfocused tiles in a flagged tab (outline = tab colour), that no header is filled.
## 33. `--tab <ref>` on createblock, term, run and web open

`wsh createblock|term|run|web open --tab <ref>` creates the block in another tab than the calling block's: `<ref>` is a full tab id (with or without `tab:`), `tab:<n>` (the n-th tab of the caller's workspace) or `tab` (the caller's own); an id prefix and every other form is refused, and a `--tab` that is
given but empty is refused (it is never taken as absent). The reference is then resolved by the same resolver as `wsh tab switch`. Without `--tab` nothing changes (the tab of the calling block, `WAVETERM_TABID`). With `--tab` the command also works from a place that has no tab of its own.
`--stack` and `--replace` need their target block to be in that tab: the command looks the target's tab up and refuses with a message that names both tabs (it does not send the request), and the SERVER makes the same check when it handles the request, for stack and for replace (`createTargetNeedsSameTab`), so a target that moved after the command looked is still caught. Tiles cannot be moved between tabs today; a future move between tabs must keep that check in the server.
- **RAN:** Go tests: the choice (no flag uses the environment, a blank flag is not given, the flag wins, the flag works without an environment, an unknown tab is refused and says `--tab`); the target check
  (same tab, other tab, no target, unknown block); the four commands have the flag, default empty, use only the shared choice and check the target after applying it; red when the flag choice, the target
  check and one command's flag are removed.
- **NOT RUN in a window:** a block created in a tab that is not shown, in another workspace or window: whether the page of that tab applies the new block's layout action when it is loaded later (the back end queues
  it per tab; that path is the one the page already uses when a tab is opened, but it is not verified here), and that the tab id resolves from a session of another workspace. The tab is NOT switched to, and the
  new block takes focus only inside its own tab (use `--no-focus` as before).

Review answers for section 33 (Codex, PR 42):
- **Accepted forms of `--tab`:** a full tab id (with or without `tab:`), `tab:<n>` and `tab`. An id prefix of 8 characters is refused: the server resolves it with `LIKE 'prefix%'` over every object type and takes the first hit, without noticing that more than one matches. A flag that is given but empty is refused (the command looks at whether the flag was given, not at its value).
- **Another workspace or window:** a full tab id resolves to any tab, in any workspace or window (the resolver does not look at where the caller is). `tab:<n>` is the n-th tab of the CALLER's workspace and never reaches another. For the manager this is the useful case: a session in tab Home can create a terminal in the tab Reviewers by id. READ in the resolver (`pkg/wshrpc/wshserver/resolvers.go`).
- **A tab that does not exist:** a full id that names no tab fails in the server when the block is created (`CreateBlock` reads the tab); nothing is created. READ.
- **A tab whose page is not loaded** (not shown, another workspace, another window): the block object is created in that tab and the placement is queued as a layout action in that tab's layout state; the tab's page applies the pending actions when its layout loads, and the clean-up of blocks that are in no layout is queued AFTER them (`CleanupOrphanedBlocks` adds its action behind the pending ones), so the new block is laid out when the tab is next shown and is not removed first. READ in `layoutModel.ts` and `blockservice.go`; not run in a window.
- **Focus:** the request names the new block's focus inside ITS tab only (`--no-focus` as before); nothing is sent to the caller's tab, the shown tab is not switched, and the caller's focus does not move. READ.
- **Targets:** `--stack` was already checked by the server (`checkStackTargetInTab`); `--replace` was not and now is (`createTargetNeedsSameTab`, tested against a real store: a cross-tab replace is refused and creates no block); the command checks both as well, for a better message. `web open -r` now resolves its block through the same block resolver as `--stack` (a non-block reference is refused).
- **RAN (new):** the command path of createblock, term, run and web open with `--tab` and a stack or replace target in another tab sends NO create request (red when the target check is removed); a given but empty `--tab` sends nothing (red when it is treated as absent); a target in the chosen tab is sent with that tab; without the flag the caller's tab is used; id prefixes and other forms are refused before any lookup.
## 34. Attention marks, part 1: the meta, the command and the rules

A block can say that it needs the owner. Two block meta keys carry it: `attention:level` (`info`, `need` or `urgent`) and `attention:reason` (text, at most 120 characters, no control, format or line characters;
required for `need` and `urgent`, optional for `info`). The settled look (his words on the mock-up: "that's perfect, ship it"): info is an icon with no motion; need is an orange DOT only (stack tab, tile
header, top tab), no outline or ring; urgent is a flashing orange outline and dot that climbs from the stack tab to the tile outline, the top tab, the taskbar and the workspace button. The orange levels clear when
the owner really focuses the tile or when a session clears them; a pulse stops after five minutes and stays steady; a tile can be muted; reduced motion is respected. **This part builds only the rules, the keys and the
command; nothing is drawn yet** (the drawing is the next parts).
- **Commands:** `wsh attention info|need|urgent --reason "why" [-b block]` and `wsh attention clear [-b block]`. The reason is refused when it is missing for need or urgent, too long, or has hidden characters; the check is the
  same one the window will apply when it draws, so a program that writes the meta without the command gets no mark for a bad value. Clear sets both keys to null. The keys are persisted block meta, so a need survives a
  page rebuild, a tab view rebuild, a workspace switch and a restart of Wave.
- **The rules, in two languages with one table of cases:** `pkg/attention` (Go) and `frontend/app/store/attention.ts` (TypeScript) both read `pkg/attention/testdata/vectors.json`; `Highest` / `highestAttention` is the highest level among
  several (the rule for every container: the stack tab holds one block, a stack tile its members, a top tab its blocks).
- **RAN:** Go tests (vectors, the meta reader, the command's meta and refusals, clear sets exactly two keys null, the command has three levels and clear) and vitest on the same vectors; red when the reason requirement is dropped
  (Go and TypeScript).
- **NOT RUN in a window:** `wsh attention` against a running Wave (the write is the same set-meta call as `wsh setmeta`).

Review answers for section 34 (Codex, PR 43):
- **Hidden characters are looked for before anything is trimmed**, in Go and in TypeScript: "ok" followed by U+FEFF, a trailing line break, a zero-width space or a line separator is refused, never trimmed away (the shared table has each case).
- **The limit is 120 code points**, in both languages and in the command's error text. A letter with an accent written as two code points counts as two (60 such letters pass, 61 do not; tested in both languages from the one table). Counting what the eye sees would need a grapheme library and buys little for one line of text.
- **Set and clear each write both keys in ONE set-meta call** (`attentionSend` with one map holding `attention:level` and `attention:reason`), so no reader sees a level with the old reason or a reason with no level; clear sets both to null in the same map (tested).
- **For the part that draws the hover text:** the reason is text from block meta, which any program can write. The renderer must read it only through `attentionOf` (the validating reader) and must put it in the page as text, never as markup, so a reason like `<img src=x>` is shown as those characters.
## 30. A new ephemeral block replaces the one on screen: the old one is asked about

When a second ephemeral block opens while one is on screen, the old one used to be closed directly (its program stopped, nothing asked). It now goes through the same close
question as the X (`replaceEphemeral`, `ephemeralreplace.ts`): answered yes, the old block closes and the new one shows; answered no, or no question set up, or the question or the close
fails, the old one stays and the NEW block is discarded (there is only one ephemeral place). The new block exists already in the back end, so it is deleted for real through
`ObjectService.DeleteBlock` (`discardEphemeralBlock`: retried once; a second failure and a missing delete callback are logged as errors and reported, never silent; the tab content always
passes `onNodeDelete`, pinned). A discard that cannot be deleted is reported (`replaceEphemeral` returns `discarded: false`), its id is kept (`DiscardLedger`) and tried again when the next
ephemeral block arrives; the tab's orphan clean-up when a layout loads is the last net.
One question at a time: while a replacement is under way every arriving ephemeral block is discarded; the guard is checked before the empty-slot path, because the slot can be empty for a
moment while the old block closes.
- **RAN:** vitest: order ask, close, show; no answer leaves the old block and discards the new; no question set up discards the new; a failing question or close keeps the old and discards
  the new; a failed discard is reported and kept in the ledger and retried; the guard comes before the empty-slot path; source pins that `newEphemeralNode` does not close directly and that both
  discard sites use the remembering discard. Red when the answer is ignored and when the guard order is swapped.
- **NOT RUN, window check:** an ephemeral replace with a running program in the old block (the box appears; Cancel keeps the old block and the new one disappears; Close Block shows the new one).
## 32. Web tiles: what was seen in a window, and what is not known

Seen by the owner in the preview 17760c63 (11 Oct 2026, first look): a web tile with a filled form kept the form when it was moved within its tab, moved into another
stack, and when another top tab was shown and then this one again. Switching to another WORKSPACE reloaded it.
- **Correction of an earlier reading.** The first reading of the code said that moving a web tile makes a new web view and loses the page. That was wrong for the moves above: the
  layout keeps every tile mounted at a place of its own (a tile is moved by changing its position, not by remounting it), and the web contents of a tab that is not shown stay alive in a tab view that is kept
  (`window:maxtabcachesize`, 10 by default). What reloads: leaving a workspace (its tab views are destroyed), and, not tested, a tab whose view was dropped from the cache because more than
  the cache size of tabs were shown since.
- **There is no "move a tile into another top tab" operation in this fork** (no drag onto a top tab, no `wsh` command; `wsh stack move` and `wsh group add` need the same tab). "Switching to another top tab" in
  the first look was a switch of the shown tab, which keeps the web contents; whether he also dragged a tile to another top tab is not known and is not possible with what is built.
- **Window check for later (web tiles, harder):** (1) three or four web tiles at once in one tab, each with a different filled form (a text field, a long page scrolled down, a video paused at a time);
  (2) put two of them into one stack and switch between the members ten times: forms, scroll position, video time; (3) the same in two windows and two workspaces, then switch workspace and back: which reload
  (expected: all of the other workspace); (4) open more tabs than the cache size (10) and come back to the first: does the web tile reload; (5) a hidden member's audio is muted (PR 27) and un-muted when shown;
  (6) close one web tile with a filled form: no question (it has no program), the others untouched; (7) restart Wave: web tiles reload (nothing keeps a form across a restart).

## 40. A session opened in a tile comes back after Wave restarts (the starter, and what opens it)

The chain, with who does what: a script that opens a session (outside this repository: `Open-WaveSession.ps1` of the owner's tools, called by `Wave-Sessions.ps1 open`) calls `wsh session start --restart --meta session:id=<guid>
--meta session:name=... (and model, effort, permissionmode when given)`. Wave then (1) marks the block `cmd:runonstart=true` and `cmd:runonce=false` so it starts the SAME command again at every start of the app, in the same block, keeping the
earlier output (`cmd:clearonstart=false`); (2) keeps the `session:*` keys on the block, which is what puts it into the `sessions` index of the state record (`<data dir>\state\wave-state.json`: only blocks that carry a session id are
listed there; `Restore-Sessions.ps1` reads that index). The command Wave starts again is `tools/ours/Start-ResumableSession.ps1`: its first start creates the session under the id (`--session-id`), every later start resumes it
(`--resume`). `--restart` is refused for a command line with a bare `--session-id` or an inline shell command (section 21), so a plain launcher cannot be used with it.
- **The starter puts the folder of wsh on the PATH** (`WAVETERM_WSHBINDIR`, once). Wave adds that folder to the PATH of the shells it starts, not of a program it starts directly (as this one is), so a
  Claude session in a tile could not run `wsh` by name. The folder goes AFTER the entries that are already there (a `wsh` the user already has wins), only if it is a fully qualified path (`X:\` or `X:/`) to an existing folder, and any error in this step leaves the PATH as it was and never stops the session, and is not added when the same
  folder is already there under another spelling (compared as full paths, case-insensitive, trailing separators and `..` resolved). RAN: `Test-ResumableSession.ps1` (the folder is last on the PATH and the launched stub runs `wsh` and prints what
  it printed; an earlier `wsh` of the same name wins; a missing, unset or relative variable leaves the PATH alone; a folder that is already there, also under another spelling, is not added again; red when the folder is put first, when the
  comparison is textual, and when the absolute-path check is removed).
- **The starter has NO default config folder (incident, 11-Oct-2026).** `CLAUDE_CONFIG_DIR` used to default to `%USERPROFILE%\.claude` when it was not set. That folder belongs to the old single runner on the work side of the machine and a
  personal session must never read or write it. A dev Wave started by the proof script had every `CLAUDE*` variable stripped, so two throwaway sessions fell back to that default and stood at Claude's first-run screen for a few minutes
  before they were stopped by their recorded ids. Rule now: the folder must be fully qualified (`X:\` or `X:/`: no relative path, no `X:name`, no UNC or `\\?\` form), is resolved to its FINAL path (junctions, symbolic links, 8.3 names, `..`)
  and is then judged, with and without `-ThrowawayConfig`: at or below `<profile>\.claude` or `<profile>\.claude-work` (case-insensitive) is refused with exit code 2 before Claude is started; at or below `<profile>\.claude-personal` is accepted;
  any other folder is accepted only with `-ThrowawayConfig` (a test). The folder handed on to Claude is the resolved full path. The opening script (`Open-WaveSession.ps1`), `Restore-Sessions.ps1` and the dev-proof start script carry the same
  rule and write `-ConfigDir <full path>` into the block's stored command, because Wave itself NEVER has `CLAUDE_CONFIG_DIR`: the launcher (`WaveFork.exe`) removes every `CLAUDE*` variable from Wave's environment, so a restart cannot depend on it.
  RAN (`Test-ResumableSession.ps1`, run with `pwsh`, under a made-up profile whose name has spaces and an 8.3 name; no path in any test is the real profile's): 20 refusal cases with exit code 2 and the stub never started (unset, empty, blank,
  `.claude`, below it, capitals, `.claude-work`, `..` into `.claude`, forward slashes, the 8.3 name of the profile, a junction under `.claude-personal` that leads to `.claude`, a junction under a throwaway folder that leads to `.claude-work`,
  a UNC form, an extended length form, relative and drive-relative paths, `.claude-personal-evil`, a throwaway folder without the switch), three more below a profile that has no `.claude` folder at all (the deny list is case-insensitive where no
  on-disk spelling exists), `CLAUDE_CONFIG_DIR` of `.claude` in the environment. Removing only the old default line is not visible to a test, because the refusal catches the folder anyway (the default is gone as a second layer).
  Also here: a launch under Windows PowerShell is refused (exit 2: a prompt with quotes or backslashes does not arrive as one argument there; `-DryRun` still runs there) and a prompt that starts with a dash is refused (it would be read as an
  option); the session id must match to the end of the string (a trailing newline is refused); only a FILE named `<id>.jsonl` counts as a transcript. **Limit:** two starts of the same id at the same instant can both create the session (no lock).
- **The starter exchanges the Wave swap token before claude starts (so a session inside Wave can use wsh).** A program that Wave starts directly gets `WAVETERM_SWAPTOKEN` (single use, valid 5 minutes after the block started) and none of `WAVETERM_JWT`, `WAVETERM_BLOCKID`, `WAVETERM_TABID`, `WAVETERM_WORKSPACEID`; the shells Wave starts exchange it themselves (`wavepwsh.ps1`), a directly started program never does, and a session that is resumed after a restart sits idle past the 5 minutes. The starter runs `wsh.exe token <token> pwsh` once (the wsh.exe in the folder `WAVETERM_WSHBINDIR`, an existing fully qualified folder; time limit 15 seconds, `-SwapTimeoutSeconds` 1 to 30). The token is the PACKED form Wave sets (`shellutil.PackSwapToken`: base64 of JSON `{token, rpccontext}`, a few hundred characters, with `+ / =`; READ from the code, not measured on a real Wave): the starter accepts the base64 alphabet only, 8 to 8192 characters, and requires that it decodes to JSON with exactly the keys `token` and `rpccontext`, the token a lower-case canonical uuid, and `rpccontext` holding only the fields of `wshrpc.RpcContext` with their types, a non-empty `sockname` and a `routeid` (always written by Wave, often empty) (nothing of it is printed), then passes it as ONE argument. An earlier version required a plain UUID shape and so never called wsh in a real Wave. The outcome is ONE fixed code (`ok`, `no-token`, `token-shape`, `wsh-folder`, `wsh-missing`, `start-failed`, `timeout`, `too-large`, `no-lines`, `no-access` (wsh printed usable lines but no `WAVETERM_JWT`, so nothing is set), `exit-code:<n>`), never wsh's own words: it is on the Note line and in one line `<time> <session name> swap=<code>` (on success also `set=<NAMES>`) of `%LOCALAPPDATA%\WaveFork\logs\starter.log` (`-LogPath`), capped at 200 lines. The log's final folder must be at or below `-LogRoot` (default `%LOCALAPPDATA%\WaveFork`) and never at or below `.claude` or `.claude-work` (judged before any folder is made); a log that cannot be written never stops the start; `no-token` is not logged (a shell may have exchanged already); the token must decode to a JSON object. The Note says that restarting the block gives a new token. **RAN:** the self test with a stub wsh.exe and a token made the way Wave makes it. and PARSES what it prints, never runs it: only lines of the exact form `$env:NAME = "value"` with NAME on a fixed list (`WAVETERM_JWT`, `_BLOCKID`, `_TABID`, `_WORKSPACEID`, `_CLIENTID`, `_CONN`, `_VERSION`, `WAVETERM`, `TERM_PROGRAM`) are taken; every other line (a command, a subexpression, a PATH or other assignment, a line with more after the value) is ignored. Inside a value Wave escapes a double quote, a backtick and a dollar sign with a backtick; only that is undone; a value with an escaped newline, any other backtick sequence or a control character is refused. The variables are set AFTER the environment cleanup, `WAVETERM_SWAPTOKEN` is removed in every case, and nothing prints a token or JWT. **Exposure:** the token is on the command line of the one wsh call while it runs (milliseconds; another process of the same user can list it then, as it can read this process's environment all along); it is single use. A failed, slow, refused or impossible exchange never stops the session: it starts without wsh access and one plain line says so. `[CmdletBinding(PositionalBinding = $false)]`: a stray argument is an error and never becomes a model or an effort (found when Wave's Unix-style quoting split a prompt with a single quote). **Round 2 of the review:** the token must be a plain token (Wave mints a UUID: `makeSwapToken`; the shape is letters, digits, dash and underscore, 8 to 64, no leading dash) and goes in as an argument list element; the folder that wsh.exe is run from (and that goes on the PATH) is the only folder a program is ever run from, so it has the gates of the config and plugin folders (fully qualified, a plain-spelling refusal of `.claude` and `.claude-work` before any file system call, final path never at or below them) and its final path must be at or below `-WshRoot` (default `%LOCALAPPDATA%\WaveFork\profile\data\bin`: READ in `shellutil.go`, Wave sets `WAVETERM_WSHBINDIR` to its data folder plus `bin`; the value in his running Wave was not read); one deadline covers the read of the output and the exit (a wsh whose child keeps the pipe open, or one that never ends, is killed at the deadline), the output is read in pieces and refused above 256 KB, a line above 16 KB is ignored; when an exchange was tried and did not succeed the inherited `WAVETERM_JWT`, `BLOCKID`, `TABID` and `WORKSPACEID` are removed (with no token at all they are left alone: a shell already exchanged them); the token variable is removed whether empty or not (an empty one cannot be made from .NET, so that case is READ, not tested). RAN: `Test-ResumableSession.ps1` with a stub `wsh.exe` (compiled by Windows PowerShell): variables set, an escaped quote, dollar and backtick value, ignored names and PATH assignments (both cases), an injected command or subexpression, an escaped newline, the call arguments, a failing stub, a hanging stub (stopped after 1 second), no wsh folder, a folder without wsh.exe, no token, a stray positional argument; mutations red (parser replaced by Invoke-Expression, name list removed, token left in the environment, a failed swap stopping the start, no time limit, exit code unchecked, newline escape allowed, control characters allowed, positional binding on). NOT RUN: a real Wave block (the swap against the real `wsh token`).
- **Starter follow-ups (after the swap): a prompt FILE, tilde refusal, listed names cleared, 8 hex key, where the default wsh root fits.** (1) `-PromptFile <path>` (with `-PromptRoot`, default `%LOCALAPPDATA%\WaveFork\prompts`): the first prompt is read from a file because Wave joins a tile's command with Unix-style quoting (a single quote becomes `'"'"'`), which PowerShell reads as several arguments; a `-Prompt` with a single quote was cut and its pieces bound to `-Model` and `-Effort` (claude exit 1, no transcript; RAN, 11 Oct). The file is only read (the starter deletes nothing), must be a plain UTF-8 file of at most 64 KB (not a link), fully qualified, outside `.claude` and `.claude-work` by plain spelling and by final path, and its folder must be at or below `-PromptRoot`; one trailing line break and a byte order mark are dropped, nothing else; `-Prompt` together with `-PromptFile` is refused (exit 2). `Open-WaveSession.ps1` writes `<PromptDir>\<session id>.txt` and stores `-PromptFile`, and REFUSES any other argument with a single quote (a name, a folder, a model) before anything is made. Tests: eight texts (single and double quotes, backslash before a quote, semicolon, ampersand, pipe, dollar, backtick, trailing full stop, a non-ASCII letter, a tab, a trailing backslash) arrive as one argument byte for byte, and the refusals. NOT RUN through Wave's own quoting layer (the stub is a script; the stored command no longer carries the prompt, so nothing is quoted). (2) A `~` anywhere in a config, plugin, wsh, log or prompt path is refused on its text alone before the path is resolved (an 8.3 short name makes GetFullPath look inside the folder). (3) After a successful exchange every listed name that wsh did not print is cleared, so nothing inherited survives next to the new values. (4) The status key is `<name>-<8 hex>` (4 hex could repeat in a day). (5) The default `-WshRoot` (`%LOCALAPPDATA%\WaveFork\profile\data\bin`) fits the INSTALLED fork only: a dev Wave (its own data folder) needs `-WshRoot` named, as `Start-WaveDevProof.ps1` does; `Restore-Sessions.ps1` passes no `-WshRoot`, so for a restored session the default applies. A missing `LOCALAPPDATA` no longer ends the starter (no exchange, the session starts). **RAN in the installed Wave on 11 Oct 2026 (the swap PR, not these follow-ups):** the starter and `Open-WaveSession.ps1` worked for three sessions (the mover, runner-research, the manager), each with `swap=ok` in the starter log, and a real restart brought the shown tab's session back in four seconds. The follow-ups here are written, NOT RUN yet (hold on test runs).
- **A block opened the OLD way** (`--restart` not given; the keys are `cmd:runonce=true` and no `session:*`) is not started again at a restart (it is marked run once): it comes back as an empty finished block with its old output, and it is not in
  the record. It can be marked from inside Wave with `wsh setmeta -b <block> cmd:runonce=false cmd:clearonstart=false session:id=<guid> session:name=<name>` plus `cmd` and `cmd:args` replaced by the resumable starter's command line (the old command
  would create the session again with its id and be refused as already in use): possible but NOT RUN, and it needs the session's real id (a session that Claude named itself has an id that only the session knows); reopening it with the new script
  is the safe way, and `claude --resume <id>` in a new tile brings the conversation back.
- **RAN (dev Wave on its own profile, top right of the screen, 11-Oct-2026, 16:35 to 16:47):** a session was opened with `Open-WaveSession.ps1` (restart on) through the dev block's wsh; the record showed it with its `session:id`; the dev Wave was
  stopped and started again; the block started BY ITSELF and resumed the SAME id (`--resume <id>`, one transcript for that id, no second session). A block opened the old way came back as an empty finished block and was not started again.
  Two facts from it: a block in a tab that is NOT shown does not start at a restart, it starts when its tab is shown (`wsh tab switch`, or a click); and a cmd block that Wave starts directly gets `WAVETERM_SWAPTOKEN`, not `WAVETERM_JWT`, so a
  script in it must run `wsh token $env:WAVETERM_SWAPTOKEN pwsh` first (the starter itself needs no wsh). Closing the dev Wave through its window (`CloseMainWindow`) ended everything in 0.5 s with the layout intact.
## 37. Attention marks, part 2: the stack tab shows the mark

The attention design is settled (docs section 34): info is an icon with no motion; need is an orange DOT only, with no outline; urgent is a pulsing orange outline and dot. This part draws the first container: the
strip tab of a stack member. A member's tab reads `attention:level` and `attention:reason` of its block (and its badge, `wsh badge`, as a small icon after the symbol):
- **info:** a small "circle-info" icon after the name; the reason, if there is one, is the hover text.
- **need:** an 8 px orange dot (`#ff9500`) at the right of the tab, steady, the reason as hover text. No outline, no ring.
- **urgent:** the same dot and a 2 px orange outline round the whole tab that pulses slowly (1.6 s, opacity 0.25 to 1). With "reduce motion" set in the system the outline does not pulse and stays steady.
- The attention colour is its own and never replaces or tints the member's own colour (the line under the tab, the tint of the shown tab). Nothing is filled.
- A value that does not pass the rules (an orange level without a reason, an unknown level, a reason with hidden characters or over 120 code points) draws nothing: the tab reads it only through `attentionOf` (section 34). The reason is shown as an attribute (text), never as markup.
- **Clearing:** a need or urgent mark on the FOCUSED block clears itself when the owner really has the tile: the window has focus and the block is the focused one, 0.5 s after the focus moved there and 3 s when it was already
  there (the rule of badges, `AttentionAutoClearing` in `app.tsx`, rule in `attentionclear.ts`); it writes null to both keys, so the session sees it gone. Info does not clear by focus. A session clears its own mark with
  `wsh attention clear`. Nothing clears a tile that is only shown in a tab without focus.
- **Review fixes (Codex):** the plan for clearing is made again when the focused block, the window focus, the level OR the reason changes (a program that replaces the reason of a focused block's mark gets the full wait for
  its new mark, it is not erased early); and when the timer fires the plan is checked against the present once more, synchronously right before the write: the window still has focus, the block is still the focused one, and
  the mark is exactly the one that was planned for (level and reason). The logic is `scheduleAttentionClear` (`attentionschedule.ts`), tested with fake timers (focus change, window focus lost, cancel, a replaced reason or level, the
  re-plan); there is no DOM test environment in this repository, so the component's effect is a thin call of it, pinned by source text. **Closed (conditional clear):** the window no longer reads the mark and then writes null. It asks the server (`ClearAttentionIfCommand`: block id, planned level, planned reason) and the server (`wstore.ClearBlockAttentionIf`) compares and clears in ONE transaction: the mark is removed only if it is still exactly (level, reason) when the request arrives, otherwise nothing is written (a program's newer mark survives, also one written during the round trip). **Both planned values must be strings that passed the validation (a mark that draws nothing is never planned), and the server never matches an empty or missing expected value.** **Still open, on purpose:** a program that clears and writes the SAME level and reason again between the plan and the request is indistinguishable from the old mark and is cleared. RAN: go tests with a real store (match clears both keys and only those; any other mark or non-string level writes nothing, the version stays; a missing block is an error; the interleaving plan, program writes y, stale clear arrives; 150 rounds of a clear racing a new mark, the new mark always survives), vitest on the wiring (the planned level and reason reach the server call), mutations red (compare off, reason not compared, only one key removed, no write, scheduler passing nothing, the window still writing meta).
  that a program writes during that round trip is erased.
- **Review fixes (run-the-code review of PR 52):** the info icon is out of the flow like the dot (the label clips its overflow with an ellipsis, so an icon inside it vanished on a long name); the pulse also stops for Wave's own
  setting `window:reducedmotion` (the class `prefers-reduced-motion` on the app), not only for the system media query; the window wiring of the clear (`makeAttentionClearEnv`, `focusSwitchedSince` in `attentionschedule.ts`) is a
  function that reads the present at each call and is tested with fakes, instead of lines of `app.tsx` that only a source pin saw.
- **Stated behaviour (known, on purpose, pinned by tests that say "observation"):** an orange level that draws nothing (no reason, or a reason with a hidden character) is still planned and erased on focus; a mark rewritten with the same
  level and reason while the timer runs is erased by the old plan (the two are indistinguishable); the clear needs the window focus plus the focused block, not the keyboard in it; the CSS-wide colour keywords (`inherit`, `currentcolor`,
  `transparent`) pass the colour check in a browser and give a text-coloured or invisible outline; a badge icon with `+spin` or `+beat` animates on the strip tab. A reviewer's attack file (25 tests: markup in a reason, hidden characters,
  prototype names as levels, 43 bad values at the four colour entrances, the clear under cancel, replace and lost focus, the compiled stylesheets) is part of the tests as `attentionlook.test.tsx`. Merge commit `fc781392` of this line does NOT type-check
  (an unclosed component, fixed by `46f301df`): remember it when bisecting.
- **Not in this part:** the tile outline and header mark, the top tab, the workspace button, the taskbar flash; mute per tile; the five-minute limit of the pulse; the rate rule. The mark of a block that is not the active member of
  its stack is on its strip tab (that is the point of this step); a tile that is not a stack has no strip, so its mark is not drawn yet.
- **RAN:** vitest for the mark of every level and of every value a program could write that must draw nothing, for the clear rule (only orange levels, only with window focus, the two delays), the clear meta (both keys, null), and
  source pins on the label, the stylesheet (the ring pulses, the dot does not, reduced motion stops it, the member's line is untouched) and the app wiring (timer re-checks focus, writes both keys); mutation table: the ring for need too, no validation, info clearing, clearing without window focus, no reduced-motion rule, no hover text: each red.
- **NOT RUN, window check:** `wsh attention need --reason "x"` in one tile of a stack while another member is shown: a dot on the first member's tab with the reason on hover; `urgent`: the pulsing outline; click the tab (it takes the focus) and the mark goes after half a second;
  `wsh attention info` shows the icon and stays; the pulse stops with "reduce motion" on; a long tab name keeps the dot visible.
## 38. The top row shows the tab colour (colour work, step 4)

By the settled scheme (the "SETTLED 11 Oct" row of the colour mock-up): the ACTIVE top tab is filled with its tab colour (`tab:flagcolor`, 20 percent: a tunable, `ActiveFillPercent` in `toptablook.ts`, and the line of the other tabs `InactiveLinePercent` (55); the tests are built from the constants, so changing them changes no test) and has its line under it (the full colour, 2 px); the other tabs
that have a colour show only a dimmer line (55 percent); a tab without a colour looks as before. The top bar itself is not filled, the gaps between the tiles and the headers are not touched, and no edge mark is added. The
rule is `topTabLook` (`frontend/app/tab/toptablook.ts`): the colour goes through `memberColorValue` (section 28), so a value that is not accepted draws nothing; the two small elements (`tab-colour-fill`,
`tab-colour-line`) sit inside the tab behind its name. The existing flag glyph on the tab is unchanged.
- **RAN:** vitest for active, inactive and every value that must draw nothing, named colours and functions, and source pins (the tab draws both elements from the rule, before its name; the stylesheet only adds the two
  small elements); mutation table: fill on the other tabs, full line on the other tabs, no validation, no fill, no line: each red.
- **NOT in this step:** the vertical tab bar (`app:tabbar` left; its tab component is a different one and is not changed), the window outline (step 5), the workspace button.
- **NOT RUN, window check:** a tab with a flag colour: when active it is tinted with a line under it, when not active only a dimmer line; the bar behind the tabs stays unfilled; a tab without colour is as before; the close button and the name stay readable on the tint.
## 39. The window outline in the workspace colour (colour work, step 5)

By the settled scheme ("workspace = outline around the window, top bar not filled"): a 3 px border round the whole window in the colour of the workspace: its accent (`ws:accent`, set with `wsh workspace style`) when
there is a valid one, else the colour of the workspace itself. No colour, no outline. It is a border only (`window-outline` in `app.tsx` and `app.scss`): it fills nothing, takes no mouse events, and sits above the
content and below the modals and typeaheads (z-index 50). The colours go through `memberColorValue` (section 28): a value that is not accepted counts as not set.
- **RAN:** vitest for the accent first, then the workspace colour, none, and every value that must not pass; source pins (one outline, from the workspace's accent and colour, a border without any background, no mouse
  events, z-index 50).
- **NOT RUN, window check:** a workspace with a colour shows the outline round the whole window including the tab bar edge and the tile area, not over the title bar's buttons; the top bar stays unfilled; switching workspace
  changes it; a workspace without colour has none. Whether the Windows frame (title bar overlay, window corners) clips the border at the corners is not known.

## 41. `--size`: the weight of a new tile (a 60 to 40 split without the mouse)

`wsh web open`, `wsh term`, `wsh run` and `wsh createblock` take `--size <2 to 50>`: the weight of the new tile in the layout. Every tile has a weight (10 for one that nobody resized) and the layout shares the space in proportion to the
weights, so `--size 7` next to a tile of weight 10 gives the new tile 7 of 17 parts (about 41 percent), `--size 10` is an equal half. It counts only when the block is added as a NEW tile (a plain insert or a split); with `--replace` or `--stack` the
block takes the place of another and the flag is ignored. The range 2 to 50 is chosen so that a new tile cannot crush a neighbour that nobody resized (weight 10): the smallest share is a new tile of weight 2, 2 of 12 parts = 16.7 percent, and a new tile of weight 50 leaves the neighbour 10 of 60 parts = 16.7 percent; only a neighbour that was itself resized to a small weight can end up with less. A value outside 2 to 50 is refused by wsh before anything is created, and the SERVER checks it too (any RPC client can send the field): only an absent field means the default weight, anything outside 2 to 50 (0 included) is an error, a weight for an ephemeral block is an error (the weight is for tiles in the layout only), and the check runs before the block is made (`ours_nodesize.go`). The request field is `nodesize` on `CommandCreateBlockData` (the layout already honoured it for new nodes; nothing used it
**Found by the review:** the front end honoured `nodesize` for splits, replace and insert-at-index but NOT for a plain insert (a block created without a target), so `--size` did nothing there. The translation from a queued layout action to a tree action is now one function (`treeActionForBackendAction`, `frontend/layout/lib/backendaction.ts`) used by the layout model for the plain insert and both splits; vitest `backendaction.test.ts` sends actions with a nodesize through it and applies the result to a layout tree (split right: weights 10 and 7; split below: 10 and 25; plain insert: 10 and 4; no nodesize: 10 and 10), and pins that the model uses it and that no node is built in the handler without the weight; mutations red: the insert dropping the weight, the splits dropping it, the model building its own node.
from outside).
RAN: `go test ./pkg/wshrpc/wshserver/ ./cmd/wsh/cmd/` with cgo, and vitest `frontend/layout/tests/newtilesize.test.ts` (the layout functions of a split and an insert with a weight of 7 give children of weight 10 and 7, a share of about 41 percent; without a weight an equal half; a vertical split and a plain insert keep the weight): the range (0, 1, 51, 101 and 100000 refused, 2, 7, 10, 50 accepted), the handler called with 1, 51, 100 and a huge weight (refused with the weight's own error before anything else is touched), the flag on all four commands, the size reaching the request, the server passing the weight to the four splits and the plain insert and not to a replace
(a source pin on `wshserver.go`, because the handler needs a database); mutations red: a split without the weight, the range check off, the size not applied, the flag not added, a replace that carries the weight. `task generate` was run (only
`gotypes.d.ts` changed). NOT RUN: a window check that the new tile really gets about 41 percent, and that the layout keeps the weights after a restart (it saves the tree, so it should).

## 42. No welcome screen and no upgrade screen (and what still sends data with telemetry off)

The fork never opens the welcome screen (the "Welcome to Wave Terminal" modal with the usage-data tick box) or the upgrade screens by itself, on a fresh profile or after a version change. Upstream opened them from `modalsrenderer.tsx` when no terms had
been agreed (`tosagreed`) and when `onboarding:lastversion` was older than the current onboarding version. The decision is now `onboardingToOpen` (`frontend/app/onboarding/onboardingpolicy.ts`), which always says "none"; nothing needs a
screen to reach a working window, and `telemetry:enabled` stays at its default of off (pinned by `ours_defaults_test.go`). `tosagreed` stays 0 on a fresh profile; the only things that read it are the telemetry cohort fields, which are not sent
with telemetry off. The modals themselves are still in the code (nothing else opens them: the other places only close them).
RAN: vitest (`onboardingpolicy.test.ts`: fresh profile with and without a first tab, seven older versions, the same version, the renderer's wiring); mutations red: the old "no terms" rule back (2 red), the old "older version" rule back (3), the renderer
opening a screen without the decision (1). NOT RUN: a fresh profile in a dev window (the dev window of 11-Oct-2026 showed the welcome screen on a fresh profile before this change; it has to be looked at again after it).

Every way into a first-run or upgrade screen that the code has (READ, then pinned by tests that scan the frontend and emain sources): (1) the modals renderer, now only through `onboardingToOpen`, which says none; (2) the "Release Notes" entry of the widget menu (a click, `pushModal("UpgradeOnboardingPatch", { isReleaseNotes: true })`): it opens the release notes page, which has no tick box, no terms and no telemetry call, and writes NOTHING when closed (with `isReleaseNotes` the close handler returns before the `onboarding:lastversion` write, `onboarding-upgrade-patch.tsx`; an earlier version of this sentence said it wrote the version, which was wrong); kept, because the owner asks for it; (3) the modal registry, which only makes the names known. There is no menu item, shortcut, command, deep link or AI panel path to the welcome screen or the other upgrade screens. Nothing but the welcome screen itself calls `ClientService.AgreeTos` or `TelemetryUpdate`.
What the skipped welcome used to SET (READ: `onboarding.tsx`, `clientservice.go`, `workspace.go`): `AgreeTos` sets `tosagreed` (a timestamp) and applies the STARTER LAYOUT to the first tab (`BootstrapStarterLayout`: the initial set of tiles); `TelemetryUpdate` writes `telemetry:enabled`; the upgrade screens write `onboarding:lastversion`. Now none of those is written on a fresh profile: `tosagreed` stays 0, `onboarding:lastversion` stays unset (nothing reads it any more), `telemetry:enabled` keeps its default (off), and the first tab gets the ORDINARY new-tab layout (one terminal). Correction: an earlier version of this paragraph said the first tab kept a single terminal; that was wrong. Upstream skips the layout for the first tab on a first launch (`CreateTab` with `isInitialLaunch`) because the welcome screen applied the starter layout, so without the welcome screen a fresh profile started with an EMPTY first tab (RAN by the reviewer on a real store: 0 blocks). `CreateTab` now applies the new-tab layout on the first launch too; the starter layout's web tile (upstream's GitHub page) is not opened. Test: `pkg/wcore/ours_firsttab_test.go` (real store, NOT RUN yet: hold on test runs). Nothing reads `tosagreed` to send or enable anything: the telemetry cohort fields use it (as "now" when it is 0) and are not sent with telemetry off. His installed profile is unaffected (it has its own layout and `tosagreed`).
What still sends data when `telemetry:enabled` is false (READ from the code, none of it RAN):
- **Usage telemetry** (`telemetry.go`): not sent. The daily diagnostic ping (client id, version, architecture): not sent, `DiagnosticPingAllowed` is the telemetry setting. **Sent anyway:** `SendNoTelemetryUpdate` (`wcore.GoSendNoTelemetryUpdate`, `main-server.go`
  `setupTelemetryConfigHandler`): when the setting CHANGES while Wave runs (either direction), the client id and the new value go to `api.waveterm.dev`. It does not run at start and not while the setting stays as it is.
- **Update check:** off (`autoupdate:enabled` false by default and read as false when absent, pinned by `ours_defaults_test.go`).
- **Wave AI cloud mode** (`cfapi.waveterm.dev`): refused while telemetry is off (`usechat.go`), and the AI panel shows a "telemetry required" prompt with a button that would switch it on; it appears only when that cloud mode is chosen, not at first run. Wave AI with a
  provider of your own sends the prompt to that provider when you use it.
- **Web tiles** load whatever address a tile is given; the shown page does its own requests.

## 43. Nothing phones home when the telemetry setting is touched, and the AI panel cannot switch telemetry on

Two things were left (READ in `docs` section 42 of the welcome change, which lists everything that sends with telemetry off): (1) when the telemetry setting CHANGES while Wave runs, upstream sends the client id and the new value to its server (`wcore.GoSendNoTelemetryUpdate` -> `wcloud.SendNoTelemetryUpdate`), in either direction; (2) the AI panel's "Enable Telemetry and Continue" button (and a link in the mode menu) calls `WaveAIEnableTelemetryCommand`, which writes `telemetry:enabled: true`.
Now: `wcloud.NoTelemetryUpdateAllowed()` is false and `SendNoTelemetryUpdate` returns without any request (so every caller is covered); `WaveAIEnableTelemetryCommand` refuses with a plain reason before it touches the config (upstream's body stays below it, unreachable, so a merge shows up as a conflict); the AI panel's message has NO button and says that Wave AI Cloud is not available in this fork because it needs telemetry (a model of your own works without it); the mode menu line says the same, as plain text. Chosen: remove the button and say it plainly, not hide the cloud mode (the cloud entries stay visible in the menu, marked). Review round: the earlier sentence here, that usage telemetry can only be switched on by editing the file by hand, was FALSE: `wsh setconfig telemetry:enabled=true` from any terminal inside the app was accepted by `SetConfigCommand`, written to `settings.json` and applied live (RAN by the reviewer), and `ClientService.TelemetryUpdate` did the same. Now: `wconfig.SetBaseConfigValue`, the one function that writes the settings file, refuses `telemetry:enabled`, `autoupdate:enabled`, `autoupdate:channel` and the clear forms `telemetry:*` and `autoupdate:*` as its first statement (also in a request that carries other keys), and `SetConfigCommand` and `TelemetryUpdate` refuse them first with the same plain reason (`wconfig.OwnerOnlyKeysRefused`). So only the settings file edited by the owner changes them. While telemetry is off `telemetry.RecordTEvent` drops the event before anything is written (nothing to send, nothing to keep; events recorded earlier are still removed after a day as before). The app menu item "Check for Updates" is removed: it asked `dl.waveterm.dev` even with automatic updates off and would have offered upstream's build over the fork (READ). The installed unpacked build has no `app-update.yml` in its resources (RAN: a listing), so the updater has no feed configured there either way. Tests: Go `pkg/wconfig` (keys, first statement of the writer), `pkg/service/clientservice`, `pkg/wshrpc/wshserver` (SetConfigCommand refusals; with a counting transport and an un-uploaded event in a real store: nothing recorded, nothing sent, plus a control that the stub sees a live sender), vitest `emain/emain-menu-noupdate.test.ts`. **NOT RUN yet** (hold on test runs). Seam rows added for the gate, the writer, the command, the service, the recording function and the menu.
RAN: `go test ./pkg/wcloud/ ./pkg/wshrpc/wshserver/` (a counting HTTP transport sees 0 calls from the update in both directions; the AI panel request is refused and the handler's first statement is the refusal), vitest `frontend/app/aipanel/notelemetrybutton.test.ts` (no source file calls the command except the generated client; no button in the message or in the mode menu line); mutations red: the update allowed again, the guard removed from the sender, the enable command restored, a button in the message, a click handler in the menu line. NOT RUN: a window (the look of the new message).

## 44. A session as a member of a stack (`wsh session start --stack`)

`wsh session start --stack <block>` creates no tab. The program block is created in the tab of `<block>` and put into its group, as `wsh group add` does (a create request with target action `stack`; the back end already refuses a target in another tab before it creates anything). `--name`, `--color` and `--symbol` then go on the block, because there is no tab to carry them: `--name` is `frame:title` (the label in the strip), `--color` is `icon:color` (the strip tab) and `frame:bordercolor` (the outline of the tile), `--symbol` is `frame:icon` (the symbol in the strip tab). The same checks as for a tab apply (colour, symbol, no control or format characters), and a name is at most 60 UTF-16 units, as the strip draws it. `--stack` cannot be combined with `--activate`. The group is only queued for the page of the tab (the same as `wsh group add`), so it is applied when that tab is open. The output is the same JSON object with `"stackedwith":"<block>"` added; `tabid` is the tab of the target. `wsh session list` now also prints a block's `frame:title` as `title`, which is what a go-to by name can match.

Why `frame:icon` did not change the header of a terminal tile: `BlockFrame_Header` (`frontend/app/block/blockframe-header.tsx`) draws the icon and the view name only when the view does not use the terminal header (`!useTermHeader`), and a terminal does use it. The key is read (it is applied to `viewIconUnion`) but nothing draws it for a terminal. It is drawn in the strip of a stack (`stacktablabel.tsx`, `stacktablook.ts`), which is where a stacked session shows it. Read from the source; not seen in a window.

- **RAN (12 Oct 2026):** `go vet ./cmd/wsh/cmd/` clean and `go test ./cmd/wsh/cmd/` ok after one fix (the test compared a MetaMapType with a plain map). The nine new tests in `wshcmd-sessionstack_test.go` pass: look keys, refusals, the create request, lookup then create, nothing created when the target is not found. **Not run:** mutation checks of them, and `wsh` against a real Wave.
- **NOT RUN, window check:** a session created with `--stack` appears as a member of the stack in the tab of the target, with its title, symbol and colour in the strip, and restarts with Wave (`--restart`) in the same place; whether a member in a tab that is not shown starts at a Wave start is the open question of section 16.
- **Not done here:** `wsh session close <tab>` closes a whole tab, so it is the wrong tool for a stacked session (close the block); the go-to of the status page lives in MalfestioMaster and still looks a session up by its tab name (`STATUS_TAB`): with `--stack` the name is the strip label and `title` in `wsh session list`, the tab name is that of the host tab.

## 45. A grab strip in the top row (`frontend/app/tab/tabbarlayout.ts`)

Why the right side could not be grabbed: the row (`.tab-bar-wrapper`) is a drag region, but `.tab-bar` is `-webkit-app-region: no-drag` over its whole box, every tab is inside it, and with ten tabs at the minimum width of 100 px the tabs take all the room. What was left at the right end was 139 px on Windows, and that is where the window's own buttons sit (minimise, maximise, close), so nothing could be grabbed; the left end is 10 px. Read from `tabbar.tsx` and `tabbar.scss`.

The change: on Windows the block at the right end is 96 px wider (`WindowDragStripWidth`), left of the window buttons, a drag region. macOS and Linux keep their fixed 12 px edge (not scaled by zoom, as before), since the obstruction is the Windows button area. It is part of the right container that `setSizeAndPosition` already measures, so the tabs get their width from what is left: they shrink to the minimum and then scroll (the scrolling is the existing overlay scrollbar), instead of taking the strip. The empty room right of the tabs when there are few tabs was already a drag region. The tab width and scroll rule is `computeTabLayout` in the same module, so the tests check it as behaviour (the strip stays outside the tab viewport, a full row scrolls and the last tab is reachable) rather than as source text. The tabs box width and the end-of-row scroll target are `tabsContentWidth` and `tabsEndScrollLeft`, used by the bar, and the last-tab test scrolls with those through a browser-style clamp; a content width one tab short fails it.

- **RAN (14 Oct 2026, review fixes):** `npx vitest run frontend/app/tab/`: 67 passed (tabbarlayout 9); `npx tsc --noEmit`: 17 errors, the same baseline as before the change. The new layout tests were red before `computeTabLayout` existed and before the strip was limited to Windows.

- **RAN (12 Oct 2026):** `npx vitest run frontend/app/tab/tabbarlayout.test.ts`: 6 passed; `npx tsc --noEmit`: 17 errors, the known baseline, none in the tab bar files. **Not run:** the window (a person is at the PC, so no window was shown), mutation checks of the tests.
- **NOT RUN, window check:** at 1280 px with ten tabs, a 96 px strip left of the window buttons moves the window when dragged; the tabs scroll and the last tab can still be reached; a double click on the strip maximises (the system's behaviour for a drag region; not seen). The 3 px at the top above the tabs is unchanged and still thin.

## 46. `wsh web info`: the address, title, load state and history of a web tile

`wsh web info [--json] [-b block]` is the read-only sibling of `wsh web url`: it prints the address, the title, the load state and whether back and forward have somewhere to go. The state is one of three words: `loading` (the web contents say they are loading), `failed` (the main frame shows Chromium's error page, address scheme `chrome-error:`), otherwise `ready`. The CLI prints only those three words (anything else, an empty or missing state included, prints as `unknown`, in text and in `--json`), the address is printed without a user name and password, and title and address are cut and made printable as for `wsh web url`. It goes through the same RPC and the same checks as `reload`, `back`, `forward` and `url` (the caller may act only on a web tile of its own workspace, asked before anything about the tile is read), and nothing is navigated or focused.

- **RAN (12 Oct 2026):** `go vet` of `cmd/wsh/cmd` and `pkg/wshrpc` clean; `go test` of `cmd/wsh/cmd`, `pkg/wshrpc`, `pkg/wshrpc/wshserver` ok; `npx vitest run emain/emain-webnav.test.ts` 26 passed (six new for the state); `npx tsc --noEmit` the baseline 17 errors. `task generate` changed only `frontend/types/gotypes.d.ts`.
- **NOT run:** a live tile. Whether Electron's `mainFrame.url` really is `chrome-error://chromewebdata/` after a failed navigation is from memory of Chromium's behaviour, not seen here; if it is not, a failed page reads as `ready`. `state` is the state at the moment of the call.

## 47. `wsh web text`: the text of a web tile (and which tiles it refuses)

`wsh web text [--selector css] [--max-bytes n] [-b block]` prints the text as the page renders it (`innerText`) of its body, or of the first element that matches `--selector` (hidden elements are not listed unless `--selector` names them), cut at 262144 bytes of UTF-8 (`--max-bytes` can only lower that; a cut is said on stderr). The cap is on what is printed: every control or format character is shown as `<U+XXXX>` first and the cut comes after, never inside a character or an escape, and the page script cuts at a whole character, never inside a surrogate pair. An error message the page wrote (a getter that throws) is made printable in emain and again in the CLI. The tool changes nothing itself (nothing is navigated, clicked or focused); a hostile page can run its own code when its text is read, since it can replace `innerText`, with no more power than the page already has. It is a new RPC (`webtext`, handled by emain like `webnav`) and has the same order of checks as the navigation commands: who is asking and whether that caller may act on the tile (own workspace only) comes first, then the record of the block (the tile must be the one named, in the tab and workspace named, a web tile), then the rule below, and only then the page is touched; the caller is asked once more right before the page is read.

**The rule on the shared sign-in store.** A web tile with no `web:partition` of its own shows its pages in Wave's default store, which all web tiles share and which keeps cookies and logins across restarts, so what it shows may be a signed-in page of the owner. `wsh web text` is refused on such a tile ("uses the shared default sign-in store of Wave") and allowed only on a tile whose `web:partition` is exactly its own block id or `persist:<block id>`. Any other name is refused ("partition is not its own"): a session can set `web:partition`, and a name the owner is signed in to elsewhere would hand that login to the tile; a name made of the tile's own id cannot be one the owner uses for another tile. This is the stopgap until the allow-list (designed in PR 68, `docs/ours/web-allowlist-design.md`). The record is checked before the page is fetched, and right before the read the live session of the web contents must be the session of exactly the partition the record names, `<id>` and `persist:<id>` being different stores (`liveSessionRefusal`, Electron session objects compared): a tile keeps the session it was loaded with, so a record changed to the tile's own name after loading on another store is refused. The whole order is one function, `readWebTile`, tested with stand-ins for the window, the store and the sessions. The decision is made by emain from the block's own record, never from a value in the request. The owner's allow-list (a tile allowed by name) does not exist yet; when it does, it is the second way past this rule. Limits that are NOT checked: a `persist:<block id>` partition keeps its logins, so if the owner signs in inside that tile, the tile is readable by a caller of its workspace; `wsh web get` (HTML by selector, older) has the same rule since section 49. Also not checked: two tiles that share `persist:<one tile's id>` share its store, so if the owner signs in inside either, the one whose own id it is can be read; that is the same residual, left for the allow-list.

Output: line breaks and tabs are kept; every other control, format or separator character of the page is printed as `<U+XXXX>`. The selector goes into the page script as a JSON string, is at most 512 characters and has no control characters.

- **RAN (12 Oct 2026):** `go vet` of `cmd/wsh/cmd` and `pkg/wshrpc` clean; `go test` of `cmd/wsh/cmd`, `pkg/wshrpc`, `pkg/wshrpc/wshserver` ok; `npx vitest run emain/emain-webnav.test.ts` 37 passed (11 new for text, incl. source pins on the order of the checks); `npx tsc --noEmit` the baseline 17 errors; `task generate`.
- **NOT run:** a live tile (whether `innerText` of a real page and the partition meta behave as read here), mutation checks. This PR is stacked on `wsh web info` (it needs its generated types).

## 48. `wsh web shot`: a picture of one web tile, saved to a new file

`wsh web shot [-b block]` saves a PNG of the web contents of ONE tile (`capturePage` on the tile's own web contents, never the window). New RPC `webshot`, handled by emain; same order of checks as `wsh web text` (the caller first, own workspace only; the block's own record; the sign-in rule; then the page; the caller asked again right before). The sign-in rule is the one of section 47: a tile with no `web:partition` of its own is refused, because a picture of a signed-in page gives out what the text does.

The file: the caller does not choose it. It always goes into the tool's own folder, `AppData\Local\Wave\web-shots` under the profile folder of the user running wsh, taken from the process token (on other systems `.cache/Wave/web-shots`, macOS `Library/Caches/Wave/web-shots`, under the home folder of the user's passwd entry). `LOCALAPPDATA`, `USERPROFILE`, `HOME` and `XDG_CACHE_HOME` are not used: the caller's environment sets them, and the Known Folder API expands `USERPROFILE` from it as well. The tool makes `Wave` and then `web-shots` one at a time and refuses either if it is not a plain folder (a junction, a symlink or another reparse point; elsewhere also a folder of another user), so nothing is made through a link at either level. The folders above `Wave` (the profile's `AppData\Local`, the home's `.cache`) are trusted: they belong to the user, and a link there is the user's own doing. The 50 newest pictures named like ours are kept, older ones removed after each new one, and the picture just saved is never the one removed (even when other names claim later times); nothing else in the folder is touched. Best effort: two shots at the same moment can leave one more than 50. Known limits: a race by another process of the same account between the folder check and the create is not defended (the same account is not the adversary here); concurrent shots are best effort; the picture inherits the cache folder's permissions; the folder is under the user's profile, whose default ACL is private to the user; an ACL widened by someone else is not checked. with a new name `<blockid>-<utc time>-<random>.png`, created with exclusive create (never an existing file), and the path is printed. Before the file is made the picture must be a complete PNG within the 4 MB cap, and its declared size is checked first (`png.DecodeConfig`: at most 8192 per side and 16M pixels), so a small file that claims a huge picture is refused before anything is allocated for it; then the whole image is decoded. On Windows the new file is delete-pending from the moment it is made, and the mark is cleared only after the whole picture is written and flushed: a failed write or flush removes it through its own handle, and a close after that cannot leave a half picture. Elsewhere a failure removes it by name in the tool's own folder. The page is read through `readWebTile`, as for `wsh web text`: the caller and the record first, then the caller again and the live session of the tile's contents right before `capturePage`.

- **RAN (12 Oct 2026):** `go vet` of `cmd/wsh/cmd` (windows and linux) and `pkg/wshrpc` clean; `go test` of `cmd/wsh/cmd`, `pkg/wshrpc`, `pkg/wshrpc/wshserver` ok (six web shot tests, including the junction case); `npx vitest run emain/emain-webnav.test.ts` 43 passed (the count pin of the authorize calls is now 7); `npx tsc --noEmit` the baseline 17 errors; `task generate`.
- **RAN (14 Oct 2026, the redesign):** `go test ./cmd/wsh/cmd` 133 passed, `go vet` windows, linux and darwin clean. Tests: no `--out` and no argument; one new file with the generated name in the folder, the path printed, a second shot a second file; an existing file never written over; a signature only, a cut PNG, junk and a GIF refused with no file made; a failed write leaves no file. Mutants: no full decode, and a discard that only closes, each fail a test.
- **NOT run:** a live tile (what `capturePage` gives for a tile in a hidden tab or a tile that is partly off screen), mutation checks. Stacked on `wsh web text` (needs its rule and generated types).

## 49. `wsh web get` gets the sign-in rule too (this changes what works today)

`wsh web get` (the older, hidden command that prints the HTML of a selector) is now refused on a tile with no `web:partition` of its own, by the rule of sections 47 and 48: the check goes through `readWebTile` like text and shot: the caller, then the tile's own record and the rule, then the window, the page, the caller again and the live session right before the selector runs. **What stops working:** reading any web tile that shows its pages in Wave's shared default store, for example a news or status tile that was opened with `wsh web open` and never given a partition. To keep reading such a tile, give it the one partition name the rule accepts, made of its own full block id: `wsh setmeta -b <blockid> web:partition=persist:<blockid>` (or `web:partition=<blockid>` for a store in memory that is gone at the next start). Any other name is refused. A tile keeps the store it was loaded with, so the live check refuses it ("not on its own partition") until its page is loaded again on the new store: reopen the tile. A tile that is moved to a new partition starts without the logins of the shared store. The allow-list (a tile allowed by name) does not exist yet.

- **RAN (12 Oct 2026):** `npx vitest run emain/emain-webnav.test.ts` (three new tests: the order caller, record, rule, window, page; a full block id first; no partition from the request); `npx tsc --noEmit` the baseline 17 errors.
- **NOT run:** a live tile, mutation checks. Stacked on `wsh web shot` (it uses the rule function of `wsh web text`).

## 50. A blocked sub-resource no longer covers a web tile

Seen on 12 Oct 2026 in the installed build: a page that had loaded (`wsh web get` read its fields) was covered by a black area with red text "Failed to load https://www.facebook.com/tr/: ERR_BLOCKED_BY_CSP". The web tile (`frontend/app/view/webview/webview.tsx`, `failLoadHandler`) listened to `did-fail-load` and showed the error for every event except `ERR_ABORTED` (-3). That event also fires for a frame inside the page and for a sub-frame's blocked request; the handler never looked at `isMainFrame`. Upstream has the same handler (read from `upstream/main`), so this is not our change.

The fix: `isPageLoadFailure` (`loadfailure.ts`): only a main-frame failure is shown, an aborted navigation never, and an event with no frame field counts as the main frame (a failure is never hidden by a missing field). The tile also no longer calls `onFailLoad` (the caller that falls back to a search) for those.

What `wsh web info` (PR 64) says for such a tile: before the fix `ready`, while the glass showed the error overlay, so the two disagreed. `info` reads the web contents (loading, or the main frame at an error page), not the overlay that the page of the window draws. After the fix the overlay is shown only for a main-frame failure, and Chromium then shows its own error page in the tile (address scheme `chrome-error:`), which is what `info` calls `failed`: the two agree for a main-frame failure. Not covered: a main-frame failure that Chromium does not turn into an error page (not seen), and the overlay is not cleared by a sub-frame navigation of a page that failed (unchanged).

- **RAN (12 Oct 2026):** `npx vitest run frontend/app/view/webview` 3 files passed (six new tests in `loadfailure.test.ts`: main frame shown, sub-frame and blocked sub-resource not shown whatever the code, aborted never, missing frame field, source pins); `npx tsc --noEmit` the baseline 17 errors.
- **NOT run:** a live tile (the page above), mutation checks.

## 51. `wsh blockshot`: a picture of one block, in a tab that is not shown

`wsh blockshot [-b block]` saves a PNG of ONE terminal block (view `term` only; every other view is refused by name, because a view can embed a page with the owner's sign-ins; more views are added one at a time), cut out of the loaded page of its tab, and prints its path. The tab does not have to be shown. The caller check is the one of `wsh web shot` (own workspace only; one denial text for a missing block and a block in another workspace), the file goes into the same folder with the same naming and keep rules; every non-terminal view is refused (use `wsh web shot` for a web tile). A picture that arrives after the deadline is dropped before it is encoded. The whole capture ends after 12 s (an internal deadline: a hung page cannot hold the tab lock or leave the view inside the window), and every cleanup step runs on its own.

How: emain looks up the block's rectangle in the tab's page (`.block[data-blockid=...]`, the frame, not the stack strip tab that carries the same attribute) and calls `capturePage(rect, {stayAwake: true})`. A tab that is not shown is parked at (-15000, -15000), where Chromium does not composite it (empty picture) and its page does not lay out (blocks stay at the layout minimum, 29x61 or 64x64, and `document.visibilityState` is hidden). So, for the capture only, under a lock per tab: the view is moved inside the window at the BOTTOM of the child order (the active tab's opaque view covers it), background throttling is switched off, the rectangle is polled every 250 ms (up to 20 times) until both sides are at least 100 px (a throwaway capture wakes the page at the 3rd and 8th look), the picture is taken (up to 4 tries for a transient `UnknownVizError`), the view is parked again and throttling is back on. A block that never lays out is an error ("not been laid out"), never a tiny picture. Nothing shows, focuses or raises a window.

- **RAN (trial Wave on its own profile, 25 Oct 2026, build 4b7c4197, he was idle):** the block of a never-shown tab, 974x1161, title bar and the program's text visible; `Test-BackgroundCapture.ps1 -Live` GREEN (foreground, caret, all visible windows and the active tab unchanged; result not blank). `npx vitest run emain/emain-blockshot.test.ts` 19 passed; `go test ./cmd/wsh/cmd -run BlockShot` ok; `npx tsc --noEmit` the baseline 17 errors.
- **Found only by the live run, each fixed with a test:** the command had no RPC client set up; a parked view gives an empty picture; the first rectangle was the strip tab's; blocks of a never-shown tab are at the layout minimum.
- **NOT run:** a non-web block whose rectangle is overlapped by a web tile (a magnified or ephemeral block on top: the crop would hold the pixels of what is above it); whether the move behind the active view is invisible to the owner (the check compares only before and after); a stacked block that is not the front member (expected: an error); a block in a tab of a window that is minimised; a second capture at the same moment from two callers on real Wave (the lock is tested with fakes); mutation checks; the live Wave 82eec4b4 (this needs a new build there).
