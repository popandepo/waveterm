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
