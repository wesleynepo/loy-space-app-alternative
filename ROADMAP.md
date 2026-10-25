# Roadmap

Protocol sources: `NOTES-app-features.md` (static analysis of the app, includes the risk list) and `protocol.go` (verified against a real capture).
Every feature below was checked on the real panel: Chrome DevTools drove the web UI, the Mac webcam (`ffmpeg -f avfoundation -i 0`) checked the display.

## Done and verified
| Feature | Command (group 02/03) | How it was verified |
|---|---|---|
| Connect + app handshake | info query `03 0a ...` | reply parsed; model/firmware shown |
| Show slot / current slot | `02 36` / `03 36` | webcam shows the right image; readback matches |
| Slot list with "on panel" check | `03 35` keys vs the app's program list | all slots match |
| Power on/off, rotation 0/90/180/270 | `02 04`, `02 2f` | webcam + readback (`03 04`, `03 2f`) |
| Brightness | `02 06` | readback; webcam shows 1-7 get brighter, 7-15 identical, so the UI is 1-7 |
| Upload 64x64 image to slots 6-9 (new or replace) | `02 08`, `02 09`, `02 35` | encoder reproduces the captured upload byte for byte; webcam shows uploaded F test card and test pattern; auto-shows the slot afterwards |

## Tried and dropped
- **Mirror** (`02 3a 01 V`): panel acks it but nothing visibly changes, so it is not exposed.

## Not done
- **Clock styles** (`ani_num`), **schedules/playlist** (`27 00`, `26 ...`), **text programs**, **GIF upload** (multi-page `YSTP01`): byte layouts not decoded (see gaps in `NOTES-app-features.md`). The capture has no examples; the next step would be a second capture of the app doing each one.
- **Delete a slot**: no safe-to-expose command was verified. Slots 6+ can be overwritten by uploading again.

## Never implemented (NOTES-app-features.md section 3)
Format/erase (`08 01 03`), clear all (`08 02 00 FF`), delete of slots 0-5, password set/open/close, firmware upgrade (group 04), screen-size set (`1B 05 01 ...`), device-id set (`0A 0A ...`).

## Rules used
- Test uploads only went to slots 6+; slots 0-5 are never overwritten (enforced in `server.go`).
- Anything inferred rather than read from code was confirmed on the panel before a UI feature was built on it.
