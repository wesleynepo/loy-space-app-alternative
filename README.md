# LED panel control (YS524A250818, LOY SPACE protocol)

[![CI](https://github.com/wesleynepo/loy-space-app-alternative/actions/workflows/ci.yml/badge.svg)](https://github.com/wesleynepo/loy-space-app-alternative/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/wesleynepo/loy-space-app-alternative/badges/coverage.json)](https://github.com/wesleynepo/loy-space-app-alternative/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/wesleynepo/loy-space-app-alternative)](go.mod)

Local web UI + Go BLE server for the 64x64 panel, as an alternative to the LOY SPACE app. Reverse-engineered from a PacketLogger capture and the app's own JS.

## Run
```
make run                                   # or: make build && ./led-panel; PORT=9000 make run
# http://127.0.0.1:8765 (localhost only); optional port argument
go test ./...                              # checks frame building and the upload sequence against known-good vectors
```
BLE goes through tinygo.org/x/bluetooth. `web/index.html` is served from disk (an embedded copy is the fallback). Run it from the project folder (or set `LED_PANEL_DIR`) so `.uploads/` and `.cache/` are shared.

Close LOY SPACE (and disconnect the phone) first: the panel accepts one BLE connection at a time. Click Connect in the page.

## Files
- `Makefile`: `run`, `build`, `test`, `cover`, `vet`, `lint`, `fmt`, `check`, `clean`.
- `.golangci.yml`, `.github/`: lint config, CI (lint, test with race detector on Linux and macOS, govulncheck). On `main`, CI also writes the coverage percentage to `coverage.json` on the `badges` branch for the README badge.
- `cmd/server/main.go`: wiring only (port, folders, graceful shutdown).
- `internal/server/`: HTTP layer. API handlers, the LOY SPACE slot list and picture cache. Knows nothing about BLE frames.
- `panel/`: panel control, no HTTP. `link.go` is the BLE link (uploads, deletes, state, validated commands), `uploads.go` the on-disk record of our uploads, `protocol.go` frame build/parse and image packing.
- `panel/protocol_test.go`, `panel/testdata_vectors.json`: protocol checks.
- `web/`: `index.html` and the embedded copy.
- `web/index.html`: slot grid with pictures and usage count, upload, delete, power / rotation / brightness, traffic log.
- `NOTES-app-features.md`, `ROADMAP.md`: protocol notes, risk list, status.

## Notes
- Uploads go to the first free slot in 6-9; the page shows how many of the panel's 60 slots are used. Delete works on the same slots (`08 02 00 <slot>`).
- Pictures uploaded this way are **not shown in the LOY SPACE app**. The app lists programs by sending the panel's slot keys to LOY's cloud, which only knows keys the app registered itself. The panel plays them fine.
- Slot pictures for the app's own slots come from the LOY SPACE folder on this Mac; our uploads are kept in `.uploads/` (not in git).
- Brightness above 7 looks the same on this panel.
- BLE captures (`*.pklg`) are not committed: they contain the host's Bluetooth address and the panel's identifiers.
