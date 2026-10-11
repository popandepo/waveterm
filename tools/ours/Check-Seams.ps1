# Checks that the upstream interfaces our changes lean on are still where docs/ours/seams.md says they are.
#
# RED (exit 1): a seam moved or vanished. Our code may no longer work; read the list, fix or re-point.
#   Seams are matched against the working tree with comments removed and whitespace collapsed, and most are full
#   declaration signatures, so a rename that leaves the old name in a comment, or a changed argument list, is red.
# WATCH (exit 0, printed): the upstream range (BaseRef..Ref) now contains something that may overlap a feature of
#   ours. Only upstream content is scanned (git grep on Ref), and only hits that were not already at BaseRef count,
#   so our own tab feature is never reported as upstream's. A human or the update-review agent decides.
#
# ASCII only (Windows PowerShell 5.1 reads BOM-less files as ANSI). Run from anywhere inside the clone.
#   -Ref      upstream commit to scan for watch hits (default upstream/main; the run fails if the ref does not exist)
#   -BaseRef  the previously synced upstream point (default merge-base of origin/Master and Ref)
[CmdletBinding()]
param([string]$Root = '', [string]$Ref = '', [string]$BaseRef = '', [string]$ExpectedFile = '')

$ErrorActionPreference = 'Stop'
if (-not $Root) { $Root = (& git rev-parse --show-toplevel).Trim() }

function GitC { & git -C $Root @args 2>$null }

function RequireRef([string]$name, [string]$why) {
    & git -C $Root rev-parse --verify --quiet "$name^{commit}" 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { Write-Host "FATAL: git ref '$name' does not exist ($why)"; exit 1 }
}
if (-not $Ref) { $Ref = 'upstream/main' }
RequireRef $Ref 'the upstream commit the watch list scans; fetch the upstream remote or pass -Ref'
if (-not $BaseRef) {
    RequireRef 'origin/Master' 'needed to find the previously synced upstream point; pass -BaseRef'
    $BaseRef = (GitC merge-base origin/Master $Ref | Select-Object -First 1)
    if (-not $BaseRef) { Write-Host 'FATAL: no merge base between origin/Master and the upstream ref'; exit 1 }
}
RequireRef $BaseRef 'the base of the watch range'

function Normalize([string]$file, [string]$text) {
    $ext = [IO.Path]::GetExtension($file).ToLower()
    if ($ext -in '.go', '.ts', '.tsx') {
        $text = [regex]::Replace($text, '/\*.*?\*/', ' ', 'Singleline')
        $text = [regex]::Replace($text, '(?m)^\s*//.*$', ' ')
        $text = [regex]::Replace($text, '(?m)\s//[^\r\n]*$', ' ')
    } elseif ($ext -in '.yml', '.yaml') {
        $text = [regex]::Replace($text, '(?m)^\s*#.*$', ' ')
    }
    return ([regex]::Replace($text, '\s+', ' '))
}

# Each seam: id, file (relative), text that must be present in the active code, why we need it.
# Rows for declarations upstream owns are matched in full (through the return type and the opening brace). Rows for
# declarations we edit ourselves (TabBadges) match the name only, so our own change does not turn them red.
$seams = @(
    @('tab-flagcolor-const', 'pkg/waveobj/metaconsts.go', 'MetaKey_TabFlagColor = "tab:flagcolor"', 'tab colour meta key'),
    @('tab-flagcolor-field', 'pkg/waveobj/wtypemeta.go', 'TabFlagColor string `json:"tab:flagcolor,omitempty"`', 'typed meta field'),
    @('tab-flagcolor-read', 'frontend/app/tab/tab.tsx', 'tabData?.meta?.["tab:flagcolor"]', 'tab bar reads the colour from tab meta'),
    @('tab-flagcolor-validate', 'frontend/app/tab/tab.tsx', 'validateCssColor(rawFlagColor)', 'colour is validated before drawing'),
    @('tab-badges-component', 'frontend/app/tab/tabbadges.tsx', 'export function TabBadges(', 'where a tab symbol is drawn'),
    @('tab-badges-flag', 'frontend/app/tab/tabbadges.tsx', ': "flag"', 'flag glyph for tab colour (our tab symbol replaces it when set)'),
    @('tab-badge-atom', 'frontend/app/tab/tab.tsx', 'useAtomValue(getTabBadgeAtom(id, env))', 'per-tab badge source'),
    @('tab-context-menu', 'frontend/app/tab/tabcontextmenu.ts', 'export function buildTabContextMenu( id: string, renameRef: React.RefObject<(() => void) | null>, onClose: (event: React.MouseEvent<HTMLButtonElement, MouseEvent> | null) => void, env: TabEnv ): ContextMenuItem[]', 'where colour and symbol menu entries go'),
    @('tab-update-name-rpc', 'pkg/wshrpc/wshrpctypes.go', 'UpdateTabNameCommand(ctx context.Context, tabId string, newName string) error', 'tab rename'),
    @('tab-get-rpc', 'pkg/wshrpc/wshrpctypes.go', 'GetTabCommand(ctx context.Context, tabId string) (*waveobj.Tab, error)', 'tab read'),
    @('workspace-list-rpc', 'pkg/wshrpc/wshrpctypes.go', 'WorkspaceListCommand(ctx context.Context) ([]WorkspaceInfoData, error)', 'workspace and tab listing'),
    @('workspace-tabids-rpc', 'pkg/wshrpc/wshrpctypes.go', 'UpdateWorkspaceTabIdsCommand(ctx context.Context, workspaceId string, tabIds []string) error', 'tab reorder'),
    @('svc-createtab', 'pkg/service/workspaceservice/workspaceservice.go', 'func (svc *WorkspaceService) CreateTab(workspaceId string, tabName string, activateTab bool) (string, waveobj.UpdatesRtnType, error)', 'tab create'),
    @('svc-closetab', 'pkg/service/workspaceservice/workspaceservice.go', 'func (svc *WorkspaceService) CloseTab(ctx context.Context, workspaceId string, tabId string, fromElectron bool) (*CloseTabRtnType, waveobj.UpdatesRtnType, error)', 'tab close'),
    @('svc-setactivetab', 'pkg/service/workspaceservice/workspaceservice.go', 'func (svc *WorkspaceService) SetActiveTab(workspaceId string, tabId string) (waveobj.UpdatesRtnType, error)', 'tab switch'),
    @('wsh-prerun', 'cmd/wsh/cmd/wshcmd-root.go', 'func preRunSetupRpcClient(cmd *cobra.Command, args []string) error', 'how every wsh command gets its client'),
    @('wsh-resolve-simple-id', 'cmd/wsh/cmd/wshcmd-root.go', 'func resolveSimpleId(id string) (*waveobj.ORef, error)', 'tab:<id>, tab:N, uuid references on the wsh side'),
    @('server-resolve-simple-id', 'pkg/wshrpc/wshserver/resolvers.go', 'func resolveSimpleId(ctx context.Context, data wshrpc.CommandResolveIdsData, simpleId string) (*waveobj.ORef, error)', 'the same on the server side'),
    @('wsh-jwt-var', 'pkg/wavebase/wavebase.go', 'WaveJwtTokenVarName = "WAVETERM_JWT"', 'controller authentication model'),
    @('wsh-swaptoken-var', 'pkg/wavebase/wavebase.go', 'WaveSwapTokenVarName = "WAVETERM_SWAPTOKEN"', 'single-use token model'),
    @('wsh-setmeta', 'cmd/wsh/cmd/wshcmd-setmeta.go', 'var setMetaCmd = &cobra.Command{', 'setmeta command on any oref'),
    @('wsh-setmeta-rpc', 'pkg/wshrpc/wshrpctypes.go', 'SetMetaCommand(ctx context.Context, data CommandSetMetaData) error', 'meta write used by tab style'),
    @('wsh-badge', 'cmd/wsh/cmd/wshcmd-badge.go', 'if oref.OType != waveobj.OType_Block && oref.OType != waveobj.OType_Tab {', 'badge accepts a tab oref'),
    @('stack-keys-hook', 'frontend/app/store/keymodel.ts', 'registerStackKeys(globalKeyMap', 'next and previous tab inside a stack tile'),
    @('wsh-tab-cmd', 'cmd/wsh/cmd/wshcmd-tab.go', 'var tabCommand = &cobra.Command{', 'existing tab command group we extend'),
    @('create-block-focused', 'pkg/wshrpc/wshrpctypes.go', 'Focused bool `json:"focused,omitempty"`', 'wsh --no-focus sets this field on create-block requests'),
    @('controller-run-decision', 'pkg/blockcontroller/shellcontroller.go', 'if shouldRunCommand(runOnce, runOnStart, curStatus.ShellProcStatus, force) {', 'wsh session start --restart depends on the controller starting runonstart blocks at init'),
    @('create-block-target', 'pkg/wshrpc/wshrpctypes.go', 'TargetAction string `json:"targetaction,omitempty"`', 'wsh slot places blocks with splitright/splitleft/splitup/splitdown/replace'),
    @('blocks-list-rpc', 'pkg/wshrpc/wshrpctypes.go', 'BlocksListCommand(ctx context.Context, data BlocksListRequest) ([]BlocksListEntry, error)', 'wsh slot finds the block of a slot by its meta'),
    @('stack-dnd-type-shared', 'frontend/layout/lib/TileLayout.tsx', 'import { tileItemType } from "./dndtypes";', 'tabs of a stack are drag sources of the same type as tile headers'),
    @('stack-reorder-reducer', 'frontend/layout/lib/layoutModel.ts', 'case LayoutTreeActionType.StackReorder:', 'dropping a tab on another tab of its strip'),
    @('delete-block-rpc', 'pkg/wshrpc/wshrpctypes.go', 'DeleteBlockCommand(ctx context.Context, data CommandDeleteBlockData) error', 'wsh slot close'),
    @('telemetry-default-off', 'pkg/wconfig/defaultconfig/settings.json', '"telemetry:enabled": false,', 'fork rule: telemetry is opt-in; red if an upstream merge flips the default back'),
    @('autoupdate-default-off', 'pkg/wconfig/defaultconfig/settings.json', '"autoupdate:enabled": false,', 'fork rule: no update check against upstream releases unless the user turns it on'),
    @('ping-gated-on-telemetry', 'cmd/server/main-server.go', 'if !wcloud.DiagnosticPingAllowed(usageTelemetry) {', 'the daily diagnostic ping is only sent after the user opted in'),
    @('updater-fallback-off', 'emain/updater.ts', 'settings["autoupdate:enabled"] ?? false', 'an absent autoupdate:enabled reads as off'),
    @('stack-ui-geometry-hook', 'frontend/layout/lib/layoutModel.ts', 'applyStackLayout(node, stackRect, additionalPropsMap);', 'stack members are laid out by our geometry, not the row and column maths'),
    @('stack-ui-strip-hook', 'frontend/layout/lib/TileLayout.tsx', '<StackStrips layoutModel={layoutModel} />', 'the tab strip of each stack is drawn next to the tiles'),
    @('stack-ui-label-hook', 'frontend/app/tab/tabcontent.tsx', 'renderStackTabLabel,', 'tab labels come from block meta via the tab content'),
    @('stack-ui-backend-actions', 'frontend/layout/lib/layoutModel.ts', 'case LayoutTreeActionType.StackInsert: {', 'the page applies stack actions queued by the back end'),
    @('stack-create-block-hook', 'pkg/wshrpc/wshserver/wshserver.go', 'case wshrpc.CreateBlockAction_Stack:', 'create-block with a stack target queues stackinsert'),
    @('stack-rpc-interface', 'pkg/wshrpc/wshrpctypes.go', 'StackBlockCommand(ctx context.Context, data CommandStackBlockData) error', 'wsh group add, remove, activate and stack move'),
    @('stack-layout-queue', 'pkg/wcore/layout.go', 'LayoutActionDataType_StackInsert', 'back-end layout actions for stacks'),
    @('stack-move-layout-action', 'pkg/wcore/layout.go', 'LayoutActionDataType_StackMove', 'wsh stack move queues this action'),
    @('stack-menu-hook', 'frontend/app/block/blockframe-header.tsx', 'label: "Turn into a stack"', 'the block menu offers stack mode'),
    @('stack-add-hook', 'frontend/app/tab/tabcontent.tsx', 'onStackAdd,', 'the + on a strip opens the widget-bar type menu'),
    @('stack-create-in-stack-helper', 'frontend/app/store/global.ts', 'async function createBlockInStack(', 'creates a new block straight into a stack'),
    @('api-focus-window-rpc', 'pkg/wshrpc/wshrpctypes.go', 'FocusWindowCommand(ctx context.Context, windowId string) error', 'local API goto with window=true'),
    @('api-set-active-tab-rpc', 'pkg/wshrpc/wshrpctypes.go', 'ElectronSetActiveTabCommand(ctx context.Context, data CommandElectronTabData) error', 'local API focus tab and goto'),
    @('api-set-block-focus-rpc', 'pkg/wshrpc/wshrpctypes.go', 'SetBlockFocusCommand(ctx context.Context, blockId string) error', 'local API focus block and goto'),
    @('api-path-rpc', 'pkg/wshrpc/wshrpctypes.go', 'PathCommand(ctx context.Context, data PathCommandData) (string, error)', 'wsh api finds the data directory of the running Wave'),
    @('api-list-all-workspaces', 'pkg/wcore/workspace.go', 'func ListAllWorkspaces(ctx context.Context) (waveobj.WorkspaceList, error) {', 'local API list reads every workspace'),
    @('api-main-rpc-client', 'pkg/wshrpc/wshserver/wshserverutil.go', 'func GetMainRpcClient() *wshutil.WshRpc {', 'local API backend sends RPC calls through the main client'),
    @('api-startup-hook', 'cmd/server/main-server.go', 'go wshutil.RunWshRpcOverListener(unixListener, nil)', 'wavesrvapi.Start() is added on the next line of wavesrv startup'),
    @('close-block-ux', 'frontend/app/store/keymodel.ts', 'function uxCloseBlock(blockId: string) {', 'the header menu and the close key of one block; ours asks first when a program is running'),
    @('stack-overlay-strip-hook', 'frontend/layout/lib/TileLayout.tsx', '{overlayNodes} {overlayStrips}', 'the drop target of each stack strip lives in the overlay layer, next to the tile drop targets'),
    @('block-runtime-status', 'pkg/blockcontroller/blockcontroller.go', 'func GetBlockControllerRuntimeStatus(blockId string) *BlockControllerRuntimeStatus {', 'status of a block program in wsh api list and the state record'),
    @('block-status-values', 'pkg/blockcontroller/blockcontroller.go', 'Status_Running = "running" Status_Done = "done" Status_Init = "init"', 'package blockstatus maps these three words to running, exited and notstarted'),
    @('badge-store-memory', 'pkg/wcore/badge.go', 'transient map[string]baseds.Badge', 'badges are transient, which is why we add a persistent symbol'),
    @('electron-bell-rpc', 'pkg/wshrpc/wshrpctypes.go', 'ElectronSystemBellCommand(ctx context.Context) error', 'pattern for electron-route commands (tab new, switch, close)'),
    @('state-record-event-hook', 'pkg/wps/wps.go', 'notifyObservers(event)', 'the state record reacts to every published object update'),
    @('state-record-start-hook', 'cmd/server/main-server.go', 'wavesrvstate.Start()', 'wavesrv starts the state record writer'),
    @('electron-wsh-client', 'emain/emain-wsh.ts', 'async handle_electronsystembell(rh: RpcResponseHelper): Promise<void>', 'where electron-route handlers live'),
    @('window-set-active-tab', 'emain/emain-window.ts', 'async setActiveTab(tabId: string, setInBackend: boolean, primaryStartupTab = false)', 'electron tab switch'),
    @('window-queue-close-tab', 'emain/emain-window.ts', 'async queueCloseTab(tabId: string', 'electron tab close'),
    @('close-tab-handler', 'emain/emain-window.ts', 'closeTabHandler(tabId, confirmClose, ww)', 'every UI route that closes a tab goes through one handler that asks first'),
    @('layout-rootnode-any', 'pkg/waveobj/wtype.go', 'RootNode any `json:"rootnode,omitempty"`', 'layout tree is opaque to the back end (stack node spike)'),
    @('layout-node-type', 'frontend/layout/lib/types.ts', 'export interface LayoutNode { id: string; data?: TabLayoutData; children?: LayoutNode[]; flexDirection: FlexDirection; size: number; kind?: "stack"; activeChildId?: string; }', 'node shape'),
    @('layout-focus-reducer', 'frontend/layout/lib/layoutTree.ts', 'export function focusNode(layoutState: LayoutTreeState, action: LayoutTreeFocusNodeAction) {', 'single place that must activate a stack member'),
    @('layout-update-helper', 'frontend/layout/lib/layoutModel.ts', 'private updateTreeHelper( node: LayoutNode, additionalPropsMap: Record<string, LayoutNodeAdditionalProps>, leafs: LayoutNode[], resizeHandleSizePx: number, magnifiedNodeSizePct: number, boundingRect: Dimensions, resizeAction?: LayoutTreeResizeNodeAction ) {', 'rectangle maths for the stack node'),
    @('layout-balance', 'frontend/layout/lib/layoutNode.ts', 'export function balanceNode( node: LayoutNode, beforeWalkCallback?: (node: LayoutNode) => void, afterWalkCallback?: (node: LayoutNode) => void ): LayoutNode {', 'silently rewrites the tree; guard for stacks'),
    @('layout-addchildat', 'frontend/layout/lib/layoutNode.ts', 'export function addChildAt(node: LayoutNode, idx: number, ...children: LayoutNode[]) {', 'silently rewrites the tree; guard for stacks'),
    @('task-build-backend', 'Taskfile.yml', 'build:backend: desc: Build the wavesrv and wsh components. cmds: - task: build:server - task: build:wsh', 'update script builds with this task'),
    @('task-rmrf-var', 'Taskfile.yml', 'RMRF: ''{{if eq OS "windows"}}powershell -NoProfile -NonInteractive Remove-Item -Force -Recurse -ErrorAction SilentlyContinue{{else}}rm -rf{{end}}''', 'update script overrides this recursive-delete variable'),
    @('task-generate', 'Taskfile.yml', 'generate: desc: Generate Typescript bindings for the Go backend. cmds: - go run cmd/generatets/main-generatets.go - go run cmd/generatego/main-generatego.go', 'update script relies on generate being part of build'),
    @('dev-config-home', 'emain/emain-platform.ts', 'const WaveConfigHomeVarName = "WAVETERM_CONFIG_HOME";', 'dev app isolation'),
    @('dev-data-home', 'emain/emain-platform.ts', 'const WaveDataHomeVarName = "WAVETERM_DATA_HOME";', 'dev app isolation'),
    @('theme-accent-scss', 'frontend/app/theme.scss', '--accent-color: rgb(88, 193, 66);', 'the default accent our accent colour overrides at runtime'),
    @('theme-accent-tailwind', 'frontend/tailwindsetup.css', '--color-accent: rgb(88, 193, 66);', 'the tailwind accent variable our accent colour overrides'),
    @('conn-config-atom', 'frontend/app/store/global.ts', 'function getConnConfigKeyAtom<T extends keyof ConnKeywords>(connName: string, key: T): Atom<ConnKeywords[T]> {', 'reads per-connection config such as display:accent'),
    @('workspace-atom', 'frontend/app/store/global-atoms.ts', 'const workspaceAtom: Atom<Workspace> = atom((get) => {', 'workspace object for the accent fallback'),
    @('conn-keywords', 'pkg/wconfig/settingsconfig.go', 'type ConnKeywords struct {', 'where display:accent is declared'),
    @('telemetry-setting', 'pkg/wconfig/settingsconfig.go', 'TelemetryEnabled bool `json:"telemetry:enabled,omitempty"`', 'dev app runs with telemetry off')
)

# Watch list. type grep: id, 'grep', pathspec, regex, meaning (hits new in BaseRef..Ref). type newfile: id, 'newfile', path regex, meaning.
$watch = @(
    @('upstream-tab-icon', 'grep', 'frontend/app/tab', 'tab:icon|tab:symbol', 'upstream has its own tab icon or symbol key; ours may be redundant or collide'),
    @('upstream-tab-color', 'grep', 'pkg/waveobj', '"tab:(color|accent|symbol|icon)"', 'upstream added another tab colour or symbol key'),
    @('upstream-tab-commands', 'grep', 'cmd/wsh/cmd', 'tabCommand\.AddCommand|Use:\s+"(rename|new|create|close|switch|select)\b', 'upstream added wsh tab management; our commands may duplicate'),
    @('upstream-tab-files', 'newfile', '^cmd/wsh/cmd/wshcmd-(tab|workspace)[^/]*\.go$', 'upstream added a wsh tab or workspace command file'),
    @('upstream-electron-tab-rpc', 'grep', 'pkg/wshrpc', 'Electron(Create|SetActive|Close)Tab|Tab(Create|Close|Switch|Rename)Command', 'upstream added tab RPCs; ours may collide by name or meaning'),
    @('upstream-tab-group', 'grep', 'frontend', 'tabgroup|tab-group|TabGroup|activeChildId', 'upstream added tab grouping or a stack concept'),
    @('upstream-layout-kind', 'grep', 'frontend/layout/lib/types.ts', 'kind\??:|nodeType', 'layout nodes gained a kind; our stack node may collide'),
    @('upstream-confirm-close-tab', 'grep', 'frontend/app', 'confirmclosetab', 'upstream added its own close-tab confirmation (open PR 2678); ours asks always when blocks exist and re-checks the count in the back end'),
    @('upstream-newtablayout', 'grep', 'pkg/wconfig', 'newtablayout', 'upstream added a layout for new tabs (open PR 3117); check that a template cannot make a node our balance guard dislikes'),
    @('upstream-tab-bar-icons', 'grep', 'frontend/app/tab/tabbar-model.ts', 'icon|symbol|emoji', 'tab bar model changed around icons')
)

$red = 0
Write-Host '== expected ids (tools/ours/seams-expected.txt, kept apart from the list below)'
$expectedFile = if ($ExpectedFile) { $ExpectedFile } else { Join-Path $PSScriptRoot 'seams-expected.txt' }
if (-not (Test-Path -LiteralPath $expectedFile)) { Write-Host "RED   expected-ids : file missing: $expectedFile"; $red++ }
else {
    try { $lines = @(Get-Content -LiteralPath $expectedFile -ErrorAction Stop) }
    catch { Write-Host "FATAL: the expected ids could not be read ($expectedFile): $($_.Exception.Message)"; exit 1 }
    $expected = @($lines | Where-Object { $_ -and -not $_.StartsWith('#') } | ForEach-Object { $_.Trim() })
    if ($expected.Count -eq 0) { Write-Host "FATAL: $expectedFile holds no ids, so it protects nothing"; exit 1 }
    $have = @($seams | ForEach-Object { $_[0] }) + @($watch | ForEach-Object { $_[0] })
    foreach ($e in $expected) { if ($have -notcontains $e) { Write-Host "RED   $e : in seams-expected.txt but missing from the seam and watch lists (row dropped?)"; $red++ } }
    foreach ($h in $have) { if ($expected -notcontains $h) { Write-Host "NOTE  $h : in the seam list but not in seams-expected.txt (not protected against being dropped)" } }
    Write-Host "expected ids: $($expected.Count)"
}
Write-Host '== seams (must all be present, comments ignored)'
foreach ($s in $seams) {
    $id, $file, $text, $why = $s
    $path = Join-Path $Root $file
    if (-not (Test-Path $path)) { Write-Host "RED   $id : file missing: $file  ($why)"; $red++; continue }
    $content = Normalize $file ([IO.File]::ReadAllText($path))
    $needle = [regex]::Replace($text, '\s+', ' ')
    if ($content.Contains($needle)) { Write-Host "ok    $id" }
    else { Write-Host "RED   $id : not found in active code of $file : $text  ($why)"; $red++ }
}

function GrepSet([string]$ref, [string]$spec, [string]$rx) {
    $out = & git -C $Root grep -n -I -i -E $rx $ref -- $spec 2>$null
    if ($LASTEXITCODE -gt 1) { Write-Host "FATAL: git grep failed (exit $LASTEXITCODE) for $ref $spec"; exit 1 }
    $set = @{}
    foreach ($l in @($out)) {
        if (-not $l) { continue }
        $m = [regex]::Match($l, '^[^:]*:([^:]+):(\d+):(.*)$')
        if ($m.Success) { $set["$($m.Groups[1].Value)`t$($m.Groups[3].Value.Trim())"] = "$($m.Groups[1].Value):$($m.Groups[2].Value)" }
    }
    return $set
}

Write-Host "== watch (upstream content new in $BaseRef..$Ref; hits need a human decision)"
$watchHits = 0
foreach ($w in $watch) {
    $id = $w[0]; $type = $w[1]
    $hits = @()
    if ($type -eq 'grep') {
        $head = GrepSet $Ref $w[2] $w[3]
        $base = GrepSet $BaseRef $w[2] $w[3]
        foreach ($k in $head.Keys) { if (-not $base.ContainsKey($k)) { $hits += $head[$k] } }
        $why = $w[4]
    } else {
        $added = @(& git -C $Root diff --name-only --diff-filter=A $BaseRef $Ref 2>$null)
        if ($LASTEXITCODE -ne 0) { Write-Host 'FATAL: git diff failed for the watch range'; exit 1 }
        $hits = @($added | Where-Object { $_ -match $w[2] })
        $why = $w[3]
    }
    if ($hits.Count -gt 0) { Write-Host "WATCH $id : $why"; $hits | Select-Object -First 6 | ForEach-Object { Write-Host "        $_" }; $watchHits++ }
    else { Write-Host "ok    $id" }
}

Write-Host "seams red: $red, watch hits: $watchHits, rows: $($seams.Count)"
if ($red -gt 0) { exit 1 }
exit 0
