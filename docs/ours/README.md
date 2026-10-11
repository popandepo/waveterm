# Why this fork exists (ledger R-20261009-105)

**Purpose: several terminals (CLI sessions) as tabs inside ONE block, each member still reachable by its own id.**
The owner rejected stock Wave because it has no tabs within the same square; everything else in this fork queues behind that. Investigation: MalfestioMaster `reports/wave-tab-groups-investigation-2026-10-09.md` (stack node in the layout tree).

Other pages here: `what-wave-can-do.md` (features and how each was checked), `seams.md` and `update-review.md` (keeping the fork small on top of upstream).

## Before every push: look for hidden characters

`node tools/ours/find-raw.cjs <files>` lists files that contain a raw control, format or separator character (zero-width, bidirectional, line and paragraph separator, NUL, BOM); `node tools/ours/escape-raw.cjs <files>` rewrites them as escape sequences inside string literals. Run `find-raw` on every file you changed. Why: on 11 Oct 2026 some editing tools turned `\uXXXX` written in a file into the raw character (test strings, a JSON table of cases, one Go line), where it hides in a diff and breaks nothing. Never type such an escape in file content through a tool that might expand it: build the file with a script that uses numbers (`String.fromCodePoint(0x200b)`), or run `escape-raw` afterwards. Both scripts are written with numbers only for the same reason.
