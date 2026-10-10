# Seams: upstream interfaces our changes lean on

Each row is a place where our code (or our update tooling) depends on something upstream owns. If one moves,
our code can break without a merge conflict. `tools/ours/Check-Seams.ps1` checks every row below against the
working tree and exits 1 when the declaration is no longer in the active code of its file. Comments are removed
and whitespace collapsed before matching, and most rows are full signatures, so a rename that leaves the old
name in a comment, or a changed argument list or return type, turns it red. The list in that script is the source
of truth, this file explains it. Add a row here and in the script in the same commit as any change that starts depending on something new.

Check: `tools\ours\Check-Seams.ps1` (exit 0 ok, exit 1 a seam moved; WATCH lines never fail the run).
What would have made it red: a rename, removal or signature change of the declaration in the listed file (tested by
renaming one function while leaving the old declaration in a comment, and by adding an argument to another).
Not covered: a seam whose declaration is unchanged but whose behaviour changed. That is what the update review is for
(`docs/ours/update-review.md`).

## Tab colour and symbol (feature a)

| Seam | File | Symbol or text | We use it for |
|---|---|---|---|
| tab colour key | `pkg/waveobj/metaconsts.go`, `wtypemeta.go` | `MetaKey_TabFlagColor`, `tab:flagcolor` | reuse as the tab colour |
| colour read and validation | `frontend/app/tab/tab.tsx` | reads `tab:flagcolor`, `validateCssColor` | colour is drawn from here |
| symbol drawing | `frontend/app/tab/tabbadges.tsx` | `TabBadges`, `icon: "flag"` | where the symbol is drawn |
| badge source | `frontend/app/tab/tab.tsx` | `getTabBadgeAtom` | badges stay transient; ours is persistent |
| badge store | `pkg/wcore/badge.go` | `transient map[string]baseds.Badge` | explains why a persistent symbol is new |
| tab menu | `frontend/app/tab/tabcontextmenu.ts` | `buildTabContextMenu` | menu entry for colour and symbol |

## wsh commands (features a and b)

| Seam | File | Symbol or text | We use it for |
|---|---|---|---|
| client setup | `cmd/wsh/cmd/wshcmd-root.go` | `preRunSetupRpcClient`, `rootCmd` | every new command uses it |
| token model | `pkg/wavebase/wavebase.go` | `WAVETERM_JWT`, `WAVETERM_SWAPTOKEN` | controllers run inside a block |
| caller tab | `cmd/wsh/cmd/wshcmd-tabindicator.go` | `WAVETERM_TABID` | "the tab the caller runs in" |
| reference resolution | `pkg/wshrpc/wshserver/resolvers.go` | `resolveSimpleId` | `tab:<id>`, `tab:N`, uuid, 8 chars |
| setmeta and badge | `cmd/wsh/cmd/wshcmd-setmeta.go`, `wshcmd-badge.go` | `setMetaCmd`, tab oref accepted | colour and symbol from outside |
| tab command group | `cmd/wsh/cmd/wshcmd-tab.go` | `tabCommand` | rename, new, switch, close join it |
| create-block focus | `pkg/wshrpc/wshrpctypes.go` | `CommandCreateBlockData.Focused` | `--no-focus` on createblock, run, term, web open |
| create-block target | `pkg/wshrpc/wshrpctypes.go` | `TargetBlockId`, `TargetAction` | `wsh slot` placement (splitright, splitleft, splitup, splitdown, replace) |

## RPC and services under tab management (feature b)

| Seam | File | Symbol | We use it for |
|---|---|---|---|
| rename | `pkg/wshrpc/wshrpctypes.go` | `UpdateTabNameCommand` | tab rename |
| read | same | `GetTabCommand` | tab listing |
| list | same | `WorkspaceListCommand` | workspaces and their tabs |
| reorder | same | `UpdateWorkspaceTabIdsCommand` | tab move |
| create, close, switch | `pkg/service/workspaceservice/workspaceservice.go` | `CreateTab`, `CloseTab`, `SetActiveTab` | the calls under the new commands |
| electron-route pattern | `pkg/wshrpc/wshrpctypes.go`, `emain/emain-wsh.ts` | `ElectronSystemBellCommand`, `handle_electronsystembell` | tab new, switch and close are electron-route RPCs written after this pattern |
| electron tab switch and close | `emain/emain-window.ts` | `setActiveTab`, `queueCloseTab` | what those handlers call |
| meta write | `pkg/wshrpc/wshrpctypes.go` | `SetMetaCommand` | `wsh tab style` |

The generated clients (`pkg/wshrpc/wshclient/wshclient.go`, `frontend/app/store/wshclientapi.ts`,
`frontend/types/gotypes.d.ts`) conflict on almost every change to the RPC list. They are regenerated with
`task generate`, never merged by hand.

## Layout, for the stack node spike

| Seam | File | Symbol | Why it matters |
|---|---|---|---|
| opaque tree | `pkg/waveobj/wtype.go` | `RootNode any` | new node fields persist with no Go change |
| node shape | `frontend/layout/lib/types.ts` | `LayoutNode` | we add optional fields |
| focus reducer | `frontend/layout/lib/layoutTree.ts` | `focusNode` | the one place that must activate a stack member |
| rectangle maths | `frontend/layout/lib/layoutModel.ts` | `updateTreeHelper` | stack members share one rectangle |
| silent rewriters | `frontend/layout/lib/layoutNode.ts` | `balanceNode`, `addChildAt` | rewrite the tree on every update; need guards |

## Accent colour

| Seam | File | Symbol | We use it for |
|---|---|---|---|
| default accent | `frontend/app/theme.scss`, `frontend/tailwindsetup.css` | `--accent-color`, `--color-accent` | the variables our accent colour overrides at run time |
| connection config | `frontend/app/store/global.ts`, `pkg/wconfig/settingsconfig.go` | `getConnConfigKeyAtom`, `ConnKeywords` | `display:accent` per connection |
| workspace | `frontend/app/store/global-atoms.ts` | `workspaceAtom` | workspace accent and colour as the fallback |

## Build and dev tooling (update script)

| Seam | File | Symbol | Used by |
|---|---|---|---|
| backend build | `Taskfile.yml` | `build:backend`, `generate` | `Update-FromUpstream.ps1` |
| recursive delete variable | `Taskfile.yml` | `RMRF` | overridden on the command line; if it is renamed the override stops working and the build may delete folders |
| dev isolation | `emain/emain-platform.ts` | `WAVETERM_CONFIG_HOME`, `WAVETERM_DATA_HOME` | dev app keeps its own data |
| telemetry | `pkg/wconfig/settingsconfig.go` | `telemetry:enabled` | dev app runs with it off |

## Watch list (collision with upstream features)

These are not seams but tripwires. They look only at upstream content: `git grep` on the new upstream head
(`-Ref`, default `upstream/main`), and only for hits that were not already present at the previously synced point
(`-BaseRef`, default the merge-base of `origin/Master` and `-Ref`). Our own commits are never scanned, so our own
tab feature is never reported as upstream's. A hit means upstream may now ship something that overlaps one of our
features. The run still exits 0; the update review decides, and an unresolved overlap blocks the merge.

| Id | Looks for (new in the upstream range) | Would mean |
|---|---|---|
| upstream-tab-icon | `tab:icon`, `tab:symbol` in `frontend/app/tab` | upstream has its own tab icon or symbol key |
| upstream-tab-color | `"tab:color"`, `"tab:accent"`, `"tab:symbol"`, `"tab:icon"` in `pkg/waveobj` | another tab colour or symbol key |
| upstream-tab-commands | `tabCommand.AddCommand` or a `Use:` of rename, new, create, close, switch, select in `cmd/wsh/cmd` | wsh tab management |
| upstream-tab-files | new files `cmd/wsh/cmd/wshcmd-tab*.go`, `wshcmd-workspace*.go` | wsh tab or workspace commands, however registered |
| upstream-electron-tab-rpc | `ElectronCreateTab`, `ElectronSetActiveTab`, `ElectronCloseTab`, `Tab(Create|Close|Switch|Rename)Command` in `pkg/wshrpc` | tab RPCs that collide with ours |
| upstream-tab-group | `tabgroup`, `TabGroup`, `activeChildId` in `frontend` | tab grouping or a stack concept |
| upstream-layout-kind | `kind`, `nodeType` in layout `types.ts` | layout nodes gained a kind |
| upstream-tab-bar-icons | icon, symbol, emoji in `tabbar-model.ts` | tab bar model changed around icons |

Rehearsed by planting a tab command file and a `tab:icon` line in a throwaway commit and scanning that commit
against its parent: both reported.
