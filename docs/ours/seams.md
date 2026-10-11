# Seams: upstream interfaces our changes lean on

Each row is a place where our code (or our update tooling) depends on something upstream owns. If one moves,
our code can break without a merge conflict. `tools/ours/Check-Seams.ps1` checks every row below against the
working tree and exits 1 when the declaration is no longer in the active code of its file. Comments are removed
and whitespace collapsed before matching, and most rows are full signatures, so a rename that leaves the old
name in a comment, or a changed argument list or return type, turns it red. The list in that script is the source
of truth, this file explains it. Add a row here and in the script in the same commit as any change that starts depending on something new.

The ids that must stay in the list are also kept in `tools/ours/seams-expected.txt`, apart from the list: a row that is dropped from the script goes red. A new row adds its id to that file in the same commit (a row that is only in the script prints a NOTE). A file that cannot be read, or that holds no ids, stops the run with exit 1 (`tools\ours\Test-CheckSeams.ps1` tests that).

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
| block list | `pkg/wshrpc/wshrpctypes.go` | `BlocksListCommand` | `wsh slot` reads each block meta `slot` |
| block delete | `pkg/wshrpc/wshrpctypes.go` | `DeleteBlockCommand` | `wsh slot close` |

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

## Stacks (tabs inside a tile)

| Seam | File | Symbol or text | We use it for |
|---|---|---|---|
| geometry hook | `frontend/layout/lib/layoutModel.ts` | `applyStackLayout(node, stackRect, additionalPropsMap);` | stack members are laid out by our geometry, not the row and column maths |
| strip hook | `frontend/layout/lib/TileLayout.tsx` | `<StackStrips layoutModel={layoutModel} />` | the tab strip of each stack is drawn next to the tiles |
| label hook | `frontend/app/tab/tabcontent.tsx` | `renderStackTabLabel,` | strip labels come from block meta |
| add hook | `frontend/app/tab/tabcontent.tsx` | `onStackAdd,` | the + on a strip opens the widget-bar type menu |
| back-end actions | `frontend/layout/lib/layoutModel.ts` | `case LayoutTreeActionType.StackInsert: {` | the page applies stack actions queued by the back end |
| create-block target | `pkg/wshrpc/wshserver/wshserver.go` | `case wshrpc.CreateBlockAction_Stack:` | create-block with a stack target queues a stack insert |
| stack RPC | `pkg/wshrpc/wshrpctypes.go` | `StackBlockCommand` | wsh group add, remove, activate and stack move |
| layout actions | `pkg/wcore/layout.go` | `LayoutActionDataType_StackInsert`, `LayoutActionDataType_StackMove` | back-end layout actions for stacks |
| block menu | `frontend/app/block/blockframe-header.tsx` | `label: "Turn into a stack"` | the block menu offers stack mode |
| create into stack | `frontend/app/store/global.ts` | `async function createBlockInStack(` | creates a new block straight into a stack |

## Fork rules: telemetry and updates

| Seam | File | Symbol or text | Why it matters |
|---|---|---|---|
| telemetry default | `pkg/wconfig/defaultconfig/settings.json` | `"telemetry:enabled": false,` | telemetry is opt-in; red if an upstream merge flips it back |
| autoupdate default | same | `"autoupdate:enabled": false,` | no update check against upstream releases unless the user turns it on |
| daily ping | `cmd/server/main-server.go` | `if !wcloud.DiagnosticPingAllowed(usageTelemetry) {` | the ping is only sent after the user opted in |
| updater fallback | `emain/updater.ts` | `settings["autoupdate:enabled"] ?? false` | an absent setting reads as off |

## Local API, state record, session restart, close tab, tab drag

| Seam | File | Symbol or text | We use it for |
|---|---|---|---|
| focus window | `pkg/wshrpc/wshrpctypes.go` | `FocusWindowCommand` | local API goto with `window=true` |
| set active tab | same | `ElectronSetActiveTabCommand` | local API focus tab and goto |
| set block focus | same | `SetBlockFocusCommand` | local API focus block and goto |
| path | same | `PathCommand` | `wsh api` finds the data directory of the running Wave |
| list all workspaces | `pkg/wcore/workspace.go` | `ListAllWorkspaces` | local API list, and the state record, read every workspace |
| main RPC client | `pkg/wshrpc/wshserver/wshserverutil.go` | `GetMainRpcClient` | the local API backend sends its RPC calls through it |
| startup line | `cmd/server/main-server.go` | `go wshutil.RunWshRpcOverListener(unixListener, nil)` | `wavesrvapi.Start()` is added on the next line |
| event observer | `pkg/wps/wps.go` | `notifyObservers(event)` | the state record reacts to every published object update |
| state record start | `cmd/server/main-server.go` | `wavesrvstate.Start()` | wavesrv starts the state record writer |
| controller start decision | `pkg/blockcontroller/shellcontroller.go` | `shouldRunCommand(...)` | `wsh session start --restart` depends on `cmd:runonstart` blocks starting again |
| program status | `pkg/blockcontroller/blockcontroller.go` | `GetBlockControllerRuntimeStatus`, `Status_Running`, `Status_Done`, `Status_Init` | the status of a block's program in `wsh api list` and in the state record |
| close handler | `emain/emain-window.ts` | `closeTabHandler(tabId, confirmClose, ww)` | every UI route that closes a tab goes through the one handler that asks first |
| tab drag type | `frontend/layout/lib/TileLayout.tsx` | `import { tileItemType } from "./dndtypes";` | tabs of a stack are drag sources of the same type as tile headers |
| strip drop target | `frontend/layout/lib/TileLayout.tsx` | `{overlayNodes} {overlayStrips}` | the drop target of each stack strip lives in the overlay layer, next to the tile drop targets |
| tab reorder | `frontend/layout/lib/layoutModel.ts` | `case LayoutTreeActionType.StackReorder:` | dropping a tab on another tab of its strip |

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
| upstream-confirm-close-tab | `confirmclosetab` in `frontend/app` | upstream added its own close-tab confirmation (open PR 2678) |
| upstream-newtablayout | `newtablayout` in `pkg/wconfig` | upstream added a layout for new tabs (open PR 3117) |
| upstream-tab-bar-icons | icon, symbol, emoji in `tabbar-model.ts` | tab bar model changed around icons |

Rehearsed by planting a tab command file and a `tab:icon` line in a throwaway commit and scanning that commit
against its parent: both reported.
