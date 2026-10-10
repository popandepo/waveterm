# Checks that the upstream interfaces our changes lean on are still where docs/ours/seams.md says they are.
#
# RED (exit 1): a seam moved or vanished. Our code may no longer work; read the list, fix or re-point.
# WATCH (exit 0, printed): upstream now contains something that may overlap with a feature of ours. A human or the
# update-review agent must decide. Both lists are data below; keep them in step with seams.md.
#
# ASCII only (Windows PowerShell 5.1 reads BOM-less files as ANSI). Run from anywhere inside the clone.
[CmdletBinding()]
param([string]$Root = '')

$ErrorActionPreference = 'Stop'
if (-not $Root) { $Root = (& git rev-parse --show-toplevel).Trim() }

# Each seam: id, file (relative), literal text that must be present, why we need it.
$seams = @(
    @('tab-flagcolor-const', 'pkg/waveobj/metaconsts.go', 'MetaKey_TabFlagColor', 'tab colour meta key'),
    @('tab-flagcolor-key', 'pkg/waveobj/metaconsts.go', '"tab:flagcolor"', 'tab colour meta key string'),
    @('tab-flagcolor-field', 'pkg/waveobj/wtypemeta.go', 'json:"tab:flagcolor,omitempty"', 'typed meta field'),
    @('tab-flagcolor-read', 'frontend/app/tab/tab.tsx', 'tab:flagcolor', 'tab bar reads the colour from tab meta'),
    @('tab-flagcolor-validate', 'frontend/app/tab/tab.tsx', 'validateCssColor', 'colour is validated before drawing'),
    @('tab-badges-component', 'frontend/app/tab/tabbadges.tsx', 'export function TabBadges', 'where a tab symbol is drawn'),
    @('tab-badges-flag', 'frontend/app/tab/tabbadges.tsx', 'icon: "flag"', 'flag glyph for tab colour'),
    @('tab-badge-atom', 'frontend/app/tab/tab.tsx', 'getTabBadgeAtom', 'per-tab badge source'),
    @('tab-context-menu', 'frontend/app/tab/tabcontextmenu.ts', 'export function buildTabContextMenu', 'where colour and symbol menu entries go'),
    @('tab-update-name-rpc', 'pkg/wshrpc/wshrpctypes.go', 'UpdateTabNameCommand(ctx context.Context, tabId string, newName string) error', 'tab rename'),
    @('tab-get-rpc', 'pkg/wshrpc/wshrpctypes.go', 'GetTabCommand(ctx context.Context, tabId string) (*waveobj.Tab, error)', 'tab read'),
    @('workspace-list-rpc', 'pkg/wshrpc/wshrpctypes.go', 'WorkspaceListCommand(ctx context.Context) ([]WorkspaceInfoData, error)', 'workspace and tab listing'),
    @('workspace-tabids-rpc', 'pkg/wshrpc/wshrpctypes.go', 'UpdateWorkspaceTabIdsCommand(ctx context.Context, workspaceId string, tabIds []string) error', 'tab reorder'),
    @('svc-createtab', 'pkg/service/workspaceservice/workspaceservice.go', 'func (svc *WorkspaceService) CreateTab(workspaceId string, tabName string, activateTab bool) (string, waveobj.UpdatesRtnType, error)', 'tab create'),
    @('svc-closetab', 'pkg/service/workspaceservice/workspaceservice.go', 'func (svc *WorkspaceService) CloseTab(', 'tab close'),
    @('svc-setactivetab', 'pkg/service/workspaceservice/workspaceservice.go', 'func (svc *WorkspaceService) SetActiveTab(', 'tab switch'),
    @('wsh-prerun', 'cmd/wsh/cmd/wshcmd-root.go', 'func preRunSetupRpcClient', 'how every wsh command gets its client'),
    @('wsh-rootcmd', 'cmd/wsh/cmd/wshcmd-root.go', 'rootCmd', 'where new wsh commands register'),
    @('wsh-jwt-var', 'pkg/wavebase/wavebase.go', 'WaveJwtTokenVarName  = "WAVETERM_JWT"', 'controller authentication model'),
    @('wsh-swaptoken-var', 'pkg/wavebase/wavebase.go', 'WaveSwapTokenVarName = "WAVETERM_SWAPTOKEN"', 'single-use token model'),
    @('wsh-tabid-env', 'cmd/wsh/cmd/wshcmd-tabindicator.go', 'WAVETERM_TABID', 'caller tab from the environment'),
    @('wsh-tab-resolver', 'pkg/wshrpc/wshserver/resolvers.go', 'func resolveSimpleId', 'tab:<id>, tab:N and uuid resolution for -b'),
    @('wsh-setmeta', 'cmd/wsh/cmd/wshcmd-setmeta.go', 'var setMetaCmd = &cobra.Command', 'setmeta command on any oref'),
    @('wsh-badge', 'cmd/wsh/cmd/wshcmd-badge.go', 'oref.OType != waveobj.OType_Tab', 'badge accepts a tab oref'),
    @('wsh-tab-cmd', 'cmd/wsh/cmd/wshcmd-tab.go', 'var tabCommand = &cobra.Command', 'existing tab command group we extend'),
    @('badge-store-memory', 'pkg/wcore/badge.go', 'BadgeStore is an in-memory store', 'badges are transient, which is why we add a persistent symbol'),
    @('layout-rootnode-any', 'pkg/waveobj/wtype.go', 'RootNode', 'layout tree is opaque to the back end (stack node spike)'),
    @('layout-node-type', 'frontend/layout/lib/types.ts', 'export interface LayoutNode', 'node shape'),
    @('layout-focus-reducer', 'frontend/layout/lib/layoutTree.ts', 'export function focusNode', 'single place that must activate a stack member'),
    @('layout-update-helper', 'frontend/layout/lib/layoutModel.ts', 'updateTreeHelper', 'rectangle maths for the stack node'),
    @('layout-balance', 'frontend/layout/lib/layoutNode.ts', 'export function balanceNode', 'silently rewrites the tree; guard for stacks'),
    @('layout-addchildat', 'frontend/layout/lib/layoutNode.ts', 'export function addChildAt', 'silently rewrites the tree; guard for stacks'),
    @('task-build-backend', 'Taskfile.yml', 'build:backend:', 'update script builds with this task'),
    @('task-rmrf-var', 'Taskfile.yml', 'RMRF:', 'update script overrides this recursive-delete variable'),
    @('task-generate', 'Taskfile.yml', 'generate:', 'update script relies on generate being part of build'),
    @('dev-config-home', 'emain/emain-platform.ts', 'WAVETERM_CONFIG_HOME', 'dev app isolation'),
    @('dev-data-home', 'emain/emain-platform.ts', 'WAVETERM_DATA_HOME', 'dev app isolation'),
    @('telemetry-setting', 'pkg/wconfig/settingsconfig.go', 'json:"telemetry:enabled,omitempty"', 'dev app runs with telemetry off')
)

# Watch list: if the pattern is FOUND (regex, case-insensitive) in the file set, upstream may have added an overlapping
# feature. Each entry: id, glob of files (git pathspec), regex, what it would mean.
$watch = @(
    @('upstream-tab-icon', 'frontend/app/tab/*', '"tab:icon"|tab:icon', 'upstream added its own tab icon key; ours may be redundant or collide'),
    @('upstream-tab-color', 'pkg/waveobj/*.go', '"tab:color"|tab:accent', 'upstream added another tab colour key'),
    @('upstream-tab-rename-cmd', 'cmd/wsh/cmd/*.go', 'Use:\s+"(rename|new|create|close|switch|select)\b', 'upstream added wsh tab management; our commands may duplicate'),
    @('upstream-tab-group', 'frontend/*', 'tabgroup|tab-group|TabGroup|"stack"', 'upstream added tab grouping or a stack concept'),
    @('upstream-layout-kind', 'frontend/layout/lib/types.ts', 'kind\??:|nodeType|activeChildId', 'layout nodes gained a kind or active child; our stack node may collide'),
    @('upstream-tab-bar-model', 'frontend/app/tab/tabbar-model.ts', 'icon|symbol|emoji', 'tab bar model changed shape around icons')
)

$red = 0
Write-Host '== seams (must all be present)'
foreach ($s in $seams) {
    $id, $file, $text, $why = $s
    $path = Join-Path $Root $file
    if (-not (Test-Path $path)) { Write-Host "RED   $id : file missing: $file  ($why)"; $red++; continue }
    $content = [IO.File]::ReadAllText($path)
    if ($content.Contains($text)) { Write-Host "ok    $id" }
    else { Write-Host "RED   $id : text not found in $file : $text  ($why)"; $red++ }
}

Write-Host '== watch (hits need a human decision)'
$watchHits = 0
foreach ($w in $watch) {
    $id, $glob, $rx, $why = $w
    $files = @(& git -C $Root ls-files -- $glob)
    $hits = @()
    foreach ($f in $files) {
        $p = Join-Path $Root $f
        if (-not (Test-Path $p)) { continue }
        $m = Select-String -Path $p -Pattern $rx -CaseSensitive:$false -ErrorAction SilentlyContinue
        if ($m) { $hits += $m | Select-Object -First 3 | ForEach-Object { "$($f):$($_.LineNumber)" } }
    }
    if ($hits.Count -gt 0) { Write-Host "WATCH $id : $why"; $hits | Select-Object -First 6 | ForEach-Object { Write-Host "        $_" }; $watchHits++ }
    else { Write-Host "ok    $id" }
}

Write-Host "seams red: $red, watch hits: $watchHits"
if ($red -gt 0) { exit 1 }
exit 0
