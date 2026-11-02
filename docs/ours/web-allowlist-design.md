# Allow-list for agent access to web tiles: a design (not built)

Status: design from reading the code, 12 Oct 2026. Nothing here is built or run. It answers four questions: who sets it, where it is kept so a
session cannot set it for itself, how a tile shows that an agent may act on it, and how the owner takes it away.

## What it is for

Today (PRs 63 to 67) the commands that read a web tile (`wsh web get`, `text`, `shot`) are refused on a tile that uses Wave's shared default
store, and allowed on a tile with a `web:partition` of its own. That is a stopgap. The allow-list is the owner's way to say "this tile may be
read, and later acted on, by agents", by name, for a while, and to see and withdraw that.

## What a session can and cannot do today (read from the code)

- A session can write any block's meta with `wsh setmeta`: `SetMetaCommand` takes any object reference and has no check of who asks. So **anything
  stored in block meta, tab meta or client meta is not a wall**, including `web:partition`. Consequence for the stopgap: a session can set
  `web:partition` on a tile. To a new name that gives an empty store (harmless). To the name of a partition the owner has signed in to, it
  would give that tile the owner's login, and the rule would allow reading it. The allow-list must therefore be keyed on more than the partition name.
- The settings writer (`SetBaseConfigValue`) is guarded for some owner keys (PR 58), but a session with file tools can edit any file in the
  profile folder: **a file on disk is not a wall against an agent that can write files**. The wall holds against what goes through Wave's own
  commands (`wsh`), not against a session that edits `%LOCALAPPDATA%\WaveFork` directly.
- The router stamps every call with the route of the link it came in on; a leaf link (the `wsh` of a block) cannot claim another route, and the
  server already uses this (`WebNavAuthorize`: the source route of the call decides, never a value in the request). The route `electron` is
  the Electron main process.

## Who sets it

Only the owner, through the Electron main process, never through `wsh`.

- A right-click item on a web tile (and on its stack tab): **Allow agents on this tile...**. It opens a native dialog from the main process (a
  session cannot click it) that names the tile, the address host it shows now, its partition, and a length of time (default 1 hour, at most 8).
- The main process then calls a new server RPC `WebAccessSet`. The server accepts it only when the stamped source route of the call is
  `electron`. Every other source gets the one refusal text. There is no `wsh` command that sets it, and no setting or meta key.

## Where it is kept

In the memory of the server process, in a small table with no generic write path: no block meta, no config key, no file. Each row is
`{blockId, partition, host, grantedAt, expiresAt}`.

- A session cannot reach it with `setmeta`, `setconfig` or a file edit, because it does not exist outside the process.
- It is gone at every Wave start. After a restart the owner grants again. That is the point: a grant that survives a restart is a grant
  nobody remembers. A persisted list can be added later if the owner asks, with the weakness above stated next to it.
- Reading it is an ordinary RPC for the checks (`WebAccessCheck`), answered from the table; the table is also published to the pages as a
  state event so the tile can draw it (below).

A row only counts while ALL of these hold, checked at every command and again right before the action (the second ask that PR 47 already
makes): the block id is the one granted; the tile's current `web:partition` is the one granted (a session that changes it ends the grant); the
page shown now has the granted host (an agent that navigates the tile to another site ends the grant until the owner grants again); the time
has not run out; the caller is in the tile's own workspace (the rule of PR 47 stays).

## Which commands it allows

- Reading commands (`get`, `text`, `shot`, `info`): allowed on a tile that has a partition of its own OR a grant. Until the grant exists they stay
  as they are in PRs 63 to 67.
- Acting commands (scroll, key, type, click; eval last or never): allowed **only with a grant, on every tile**, including a tile with its own
  partition, because acting changes what the owner sees and can submit forms. Each action is counted and logged.

## How a tile shows that an agent may act on it

The picture is drawn from the server's table, not from anything on the block, so a session can neither fake it nor hide it.

- An **agent badge** (the robot icon) in the tile's header and in its stack tab, with the time left on hover ("agents may act, 42 min").
- A **dashed outline** in one fixed colour that is not the attention orange and not an agent colour, for as long as the grant lasts.
- A short **pulse** of the badge and a line in the tile's activity list each time an agent command runs on it ("read page text, from
  block ab12cd34").
- The same badge appears in the top row on the tab that holds a granted tile, so a grant in a hidden tab is not invisible.

## How the owner takes it away

- Click the badge, or right-click the tile: **Take away agent access**. One click, no dialog, same electron-only RPC `WebAccessClear`.
- **Revoke all** in Wave's menu and as a key chord, for the day something looks wrong.
- It also ends by itself: when the time runs out, when Wave restarts, when the tile is closed or moved to another workspace, when the
  partition changes, when the tile shows another host.
- A session cannot extend or renew a grant. Only the owner's dialog can.
- Every grant, clear and agent action is a line in the server log with the time, the caller's block and the command, and the last ten are
  shown in the tile's context menu, so the owner can see what happened while he was away.

## Pieces, in the order I would build them

1. **Store and checks (about 250 lines Go + tests):** the table, `WebAccessSet`/`Clear`/`Check` (set and clear only from the `electron`
   source), the check wired into `text`, `shot`, `get`, `info` in place of the partition-only rule. Testable without a window.
2. **Dialog, badge, outline, menu items (about 300 lines):** the main-process dialog, the state event, header and stack-tab badge, outline,
   Take away, Revoke all. Needs a window check.
3. **Activity list and the log lines (about 120 lines).**
4. Only then, and only with the owner's word on each: acting commands (about 350 lines for scroll, key, type, click).

## Open points for the owner

- Default and maximum length of a grant (proposed 1 hour and 8 hours).
- Whether a tile with a partition of its own should be readable without any grant (proposed: yes for reading, no for acting).
- Whether a persisted list is wanted (proposed: not at first).
- The workspace-walls decision stays separate: `SetMeta`, `BlockInfo` and the block list still cross workspaces, and until that is closed a
  session can read the tab and address of tiles in other workspaces and set their `url`.

## Known weaknesses, stated plainly

- The list protects the commands of Wave. It does not protect the disk: a session with file access can read the cookie files of Wave's
  profile directly, whatever the list says. Only the operating system's file rights can close that.
- Until step 1 is built the stopgap rule can be walked around by naming an existing signed-in partition (see above).
- A grant covers a host, not a page: an agent on a granted tile can read whatever that host shows, signed in or not.

## Workspace walls: the manager sees everything, a session sees its own (design, not built)

The owner's rule, in his words: "You can see everything, any agents or subsessions etc can only see their stuff or others if specifically
handed it, then you can hand them things if they need it or just summarize if better." In the fork: the manager's session sees every
workspace, tab and tile. Every other session sees only its own, plus what has been handed to it. The manager can hand a tile or a tab to a
session, take it back, or just tell the session what it needs.

### How the server knows who the manager is

- The server holds one value, `managerblock`: the block id of the tile the owner named as the manager. Memory only, not meta, config or file.
  It is empty after a restart, and the owner names it again (one menu item on the tile, see below).
- Only the owner can set it: the call is accepted only from the stamped source route `electron` (the same rule as the allow-list), and Electron
  sends it only from a menu item the owner clicks: "Make this tile the manager". An environment variable is never read for it. A session can copy
  every variable it has, and none of them names the manager.
- A request is the manager's when the identity the server already derived from its token (the block the JWT was minted for) equals
  `managerblock`. A session cannot claim it: it cannot mint a token, and a copied token is still its own block's token.
- Known limit: anything that runs inside the manager's tile is the manager (a program started in that tile, or a second session stacked on it).
  The manager tile should stay one session only. The tile shows a badge so the owner sees which tile it is.
- The owner's own windows (the UI, source route `electron` or the frontend) are not sessions and are not limited by the walls.

### What "handed" is

- A hand-over is an entry `{ to: <block id of the session>, what: <tile id or tab id>, until: <time> }` in a table in server memory.
  No meta, no file: it does not survive a restart, unless the owner later asks for that (open point below).
- Commands, all manager only (refused for anyone else as "not found", see below):
  - `wsh share <tile-or-tab> --with <session-block> [--for 1h]` creates the entry. A tab means every tile in it now and later.
  - `wsh share --list` shows the table. `wsh unshare <tile-or-tab> --with <session-block>` takes one back, `wsh unshare --all` clears it.
- A hand-over gives the same reach the session has on its own tiles and no more: it does not carry an allow-list grant for a web tile. Acting on
  a web tile still needs its own grant from the owner.
- A hand-over ends by its time, by an unshare, by the receiving block being closed, and by a restart. Default one hour, maximum eight, the same
  numbers as the allow-list.
- How a shared tile shows it: a small "shared with <name>" chip in the tile header, drawn from the server's table (the same push the allow-list
  badge uses), with a click to take it back. The name is the session's tile title, never a block id. The receiving session's own tile shows
  "has <n> shared".
- The manager can also just summarize: nothing needs building for that.

### What a session's commands answer for things outside its own

- The check lives in the server, in one place that every id-taking call goes through (`BlockInfo`, `SetMeta`, `GetMeta`, the block and tab
  lists, the web commands, session commands), not in each command. It runs after the identity is derived and before the lookup.
- Outside the session's own set and not handed: the answer is exactly the one for an id that does not exist, "not found", with the same text and
  the same timing as a real miss. Never "refused", and never a message that names the workspace, tab or tile that was asked about, so a
  session cannot learn that something exists by being told it may not touch it.
- Lists (`wsh session list`, block and tab lists, the layout) only contain the session's own and the handed.
- The session's own set is: the tile it runs in, every tile in the same tab, and nothing in other tabs or workspaces. (Open point: whether
  "its own" should be the tab or only the tile. The tab is what the stack and `wsh session start` already work on.)

### What breaks today that relies on seeing across

From what I know of these tools, not re-read or run for this page. Each of these needs the manager's identity, or a hand-over, before the walls are turned on:

- Attention marks: the status tiles and the "needs you" marks are set from other sessions' tiles with `SetMeta` on a block outside the caller's own.
  The manager, which does this, is fine. A session that marks another session's tile is not, and would then use a hand-over or ask the manager.
- The status map and its claim line: it reads tile titles and meta across tabs through the block list. Fine if it runs as the manager. As
  a session it would see only itself.
- `wsh session start`, `go-to` and `STATUS_TAB` (the MalfestioMaster tools): they open or find a tile in another tab. With the walls they work for
  the manager only, or on a tile that was handed over.
- `Open-WaveSession.ps1` (private tool) and `wsh session start --stack`: a session that starts another one creates it in its own tab (fine) or
  stacks it on a target (the target must be its own or handed).
- The news and status web tiles, and `wsh web get/text/shot/info` from a session: they must be handed or be in the session's own tab.
- Tools I run from a worktree session (the dev window, `wsh session list`) see only my own tile and tab afterwards.

### Order of building (about 400 lines, nothing started)

1. `managerblock` plus the owner-only menu item and the badge, and the check function with the "not found" answer, behind a switch that is off.
2. The hand-over table, `share` and `unshare`, and the chip.
3. Turn the switch on only after the manager's own tools (attention marks, status map, go-to) have been run once as the manager and once as a
   session, to see which ones answer "not found".

### Open points

- Whether hand-overs should be saved across restarts (proposed: no).
- Own set = the tab or only the tile (proposed: the tab).
- Whether a second "manager" is ever wanted, for example a work tile. The design allows one value only.
