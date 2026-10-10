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
- **Limit:** opening `middle` while only the right column exists splits the column's top block, which nests one
  level deeper than a flat row.
