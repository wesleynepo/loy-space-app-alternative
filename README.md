# LED panel control (YS524A250818, LOY SPACE protocol)

Local web UI + Python BLE server for the 64x64 panel, as an alternative to the LOY SPACE app. Reverse-engineered from a PacketLogger capture and the app's own JS.

## Run
```
python3 -m venv .venv && .venv/bin/pip install bleak
.venv/bin/python server.py          # http://127.0.0.1:8765 (localhost only)
```
Close LOY SPACE (and disconnect the phone) first: the panel accepts one BLE connection at a time. Click Connect in the page.

## Files
- `server.py`: HTTP API + persistent BLE link; serves `web/index.html`.
- `protocol.py`: frame build/parse, commands, image packing and upload frames.
- `web/index.html`: slot grid with pictures and usage count, upload, delete, power / rotation / brightness, traffic log.
- `scan.py`, `gatt.py`, `probe.py` (read-only queries), `panel.py` (one-off command), `parse_pklg.py`, `rebuild.py`: investigation tools.
- `NOTES-app-features.md`, `ROADMAP.md`: protocol notes, risk list, status.

## Notes
- Uploads go to the first free slot in 6-9; the page shows how many of the panel's 60 slots are used. Delete works on the same slots (`08 02 00 <slot>`).
- Pictures uploaded this way are **not shown in the LOY SPACE app**. The app lists programs by sending the panel's slot keys to LOY's cloud, which only knows keys the app registered itself. The panel plays them fine.
- Slot pictures for the app's own slots come from the LOY SPACE folder on this Mac; our uploads are kept in `.uploads/` (not in git).
- Brightness above 7 looks the same on this panel.
- BLE captures (`*.pklg`) are not committed: they contain the host's Bluetooth address and the panel's identifiers.
