# LOY SPACE app: features, BLE command catalog, risk flags

Static analysis only. Nothing was run or sent. Source: `~/Library/Containers/com.yskd.loywf/Data/Library/Pandora/apps/__UNI__F2139F6/www/` (app v1.2.21). Offsets are approximate character offsets into `app-service.js` unless noted.

Legend: **[code]** read directly from JS. **[inferred]** derived from arithmetic or naming, not confirmed on a device.

---

## 1. Feature list (pages)

Routes come from `var __uniRoutes` in `app-config-service.js`. Feature gating uses `needConnectDev`: partition, voice, control, linkage.

- **index**: home / connection hub.
- **ble/blelist**: BLE scan and connect.
- **wifi/wifi**: Wi-Fi mode setup page. Not a BLE command path (see section 3).
- **feedback/feedback, feedback/record**: in-app feedback (server API).
- **nav/programlist**: program list. Play, select, delete, and send programs.
- **nav/textpic**: text programs (fonts, speed, text size, animation).
- **nav/material**: material library. Online materials are downloaded and sent.
- **nav/diy**: DIY drawing and image/GIF/video upload. Converts media via `getMaterialBuff`.
- **nav/imagegif**: image and GIF upload and send.
- **nav/offline**: offline (phone-side) program queue.
- **nav/partition**: multi-region layout. Each region has its own rectangle and content (`list_region`, `info_pos`).
- **nav/draft**: draft programs (save, edit, send).
- **nav/clock**: clock styles (`ani_num` command) and time sync.
- **set/setting**: device settings. Brightness, screen on/off, play loop/fixed time, play schedule.
- **set/upgrade**: firmware upgrade (server check, then transfer over BLE). See risk flags.
- **set/language, set/privacy/index, set/appupdate**: UI-only or phone-side.
- **set/devinfo**: device info (`dev_info`, `param_dev` queries).
- **set/password**: panel lock: verify, set, open, close, read.
- **set/config**: accessory pages. Remote-control key pairing ("control"), motorcycle linkage ("linkage"), car sound ("car_voice"), holiday/expression tags.
- **Other BLE features reachable from data templates** [code, `data_parse` / `dataTimeParse` / `dispatch_data` at app-service ~968700-970700]:
  - rotate (0/90/180/270)
  - game mode (key codes)
  - real-time draw (`rt_draw`)
  - flicker (`pgm_flicker`)
  - mirror
  - channel colour
  - global animation
  - sensor
  - bluetooth on/off, delay-hide, power model
  - telecontrol key mapping
  - playlist and alarm (`play_list`, `alarm`)
  - timed program schedule (`ce`, see 2.9)

Nav icon names in `www/static/blackSkin/` (clock, images, control, link, draft, diy, voice, text, partition, rotate, setting, material, playlist) are consistent with the list above.

---

## 2. Command catalog

### 2.0 Frame format [code]

Functions: `ve` (frame wrap, ~1245576), `ne` (packet list builder, ~1237989), `re` (program to packets, ~1238374), `ie` (program builder, ~1236497), `parse_data` (~1256794).

- Packet = `AA 55 FF FF | LEN u16le | SEQ u16le | DIR u8 | GROUP u8 | body | SUM u16le`.
  - `LEN` = bytes after the LEN field (seq 2 + dir 1 + group 1 + body + sum 2).
  - `SEQ` = `sno` (+ index per packet in a multi-packet send).
  - `DIR` = `C1` for app to panel. Bit 7 set means the 2-byte checksum is present.
  - `GROUP` = `02` set, `03` query, `04` upgrade data.
  - `SUM` = sum of all bytes from `AA 55` through the end of body, mod 65536, little-endian.
- The body's first byte is the command or TLV tag. Body is `TLV` items: `tag u8 | len | value`. Length encoding (`z`/`I`): <128 is one byte; 0x81+1 byte; 0x82+u16le.
- Packet list in storage/transfer: each packet is prefixed by u16le length (`lvdata_to_arr`, `get_pkt_by_idx`).

### 2.1 Simple set/query commands [code, dispatcher `ye` at ~1247044]

Each command is a `data_parse` template object. Callers set the field, then `sendbuffer([obj])`. The cmd id is the first body byte unless noted.

| Feature | Group | Body bytes | Notes |
|---|---|---|---|
| Power on/off (`sendSwitch`) | 02 | `04 02 00 T` | T=1 on, 0 off. `T>1` is clamped to 1. |
| Power timed (period list) | 02 | `04 len n 7F ...` | Each entry is 4 bytes (`T(...)`). |
| Power off with `status_def`/`off_rc` | 02 | `04 03 80 status off_rc` | Type 128 branch. |
| Brightness (`setPsackLight`) | 02 | `06 02 00 V` | V=`value_fix`; if V=0 the code uses 15. Observed levels 1-14 in captures. UI range not confirmed. |
| Brightness schedule | 02 | `06 len n (V HH MM)*` | Type 1 branch. |
| Rotate (`setLedRotate`) | 02 | `2F 01 R` | `R = (deg/90) mod 4`, so 0/1/2/3 for 0/90/180/270. Angle mapping in `getRotateValue` not read. |
| Game key (`game`) | 02 | `30 01 K` | K from table: left 37, up 38, right 39, down 40, start 115, pause 112, done 100, exit 101. Otherwise `id+128`. |
| Show device flag (`show_dev`) | 02 | `29 01 V` | |
| Pgm flicker | 02 | `2D 06 type count time(u32le)` | |
| Bluetooth / delay-hide / power-model | 02 | `34 02 type val` | delay_hide uses type 0; power_model uses type 1. |
| Mirror | 02 | `3A 01 V` | |
| Sensor | 02 | `3E 02 type val` | |
| Channel colour | 02 | `39 ...` | Type 16 uses a colour array. |
| Global animation | 02 | `38 04 ...` / `38 01 00` | Type 255 clears. |
| Real-time draw | 02 | `rt_draw` (index 32) | Pixel write, not read. Shape/points passed as data. |
| Clock style (`setClockStyle`) | 02 | `ani_num` (index 33) | Fields `type` 0-3, `fmt`, `fmt_data`. **Byte layout not fully decoded.** |
| Time sync (`timing`) | 02 | `05 09 YY(u16le, year-2000) MM DD hh mm ss ...` | 11 bytes total, 9-byte payload after tag/len. Matches spec. |
| Set device ID (`set_id`) | 02 | `0A 0A <10 ASCII bytes>` | Caller not found. Changes device ID. |
| Dev info query | 03 | `03 0a` | With `dev_key` set: `0A len <key chars>`. |
| Other queries (`ge`, ~1243751) | 03 | `03 04` power, `03 06` light, `03 2F` rotate, `03 34 01 00` bluetooth, `03 34 01 01` power_model, `03 1B` param_dev, `03 0a` dev_info, plus `pgm_play`, `pgm_key`, `pwd`, `ani_num`, `pgm_flicker` | Replies parsed in `dealParseDataBuff`. |
| Playlist (`play_list`) | 02 | `27 00` | Case 21 (`cancel`) uses `27 00` for "play_list" and `26 02 00 01` for "alarm". Mapping inferred from name table. |
| Password | 02 | `37 len 00 VAL` verify, `37 len 01 VAL` set, `37 01 02` close, `37 01 03` open, `37 01 04`/`get` read | `VAL` is string bytes from `Y()`. See risk flags. |
| Param dev (set) | 02 | `1B 05 01 W(u16le) H(u16le)` | Set width/height. Caller not found. See risk flags. |
| Delete (`delProgramIds`, `deleteFormat`, `clear screen`) | 02 | see 2.2 | |
| Upgrade | 04 | see 2.6 | Uses group 04, not 02. |

### 2.2 Delete (case 2, ~1247821) [code]

- Format: `08 01 03`. Used by `deleteFormat` (template `data_parse[26]`).
- Delete all: `08 02 00 FF`. Used by "clear device screen" (template `data_parse[9]`, `del_all:1`). The UI text "clear screen" is at ~262091.
- Delete program IDs: `08 LEN 00 id1-1 id2-1 ...`. Each ID is sent as `id-1`.
- Delete by rect: `08 LEN 01 slot ...`.
- Delete items: `08 LEN 02 slot rect ...`.

### 2.3 Program send (the important part)

Entry points: `sendProgramData` (~512000), `sendbuffer` (~602297), `parse_data` (~1256794), `re` (~1238374).

**Program object** [code, `new_pkts_program` at ~370736]:
- `pkts_program.id_pro`: 1-based program id (e.g. 6).
- `property_pro`: `width`, `height`, `play_fixed_time`, `play_loop`, `time_sync`, `type_pro`, `data_save`, `show_now`.
- `list_region[]` (`info_pos` {x,y,w,h}), each with `list_item[]` (`type_item`, `info_animate`, `isGif`, …).
- `sno`, `id_res` (40 hex chars), `ts_update` (unix seconds).

**`sendbuffer`** [code]: `set_pkt_size(960, GSize_max_pkt_tlv)` where `GSize_max_pkt_tlv` is 997 by default and 128 for older text firmware. `parse_data` then calls `re`.

**Start frame, group 02, tag 08** (`ae`, ~1234971). Built from captured example:

```
02 | 08 02 00 05 | 09 01 01 | 0c 01 05 | 1c 06 00 00 01 00 03 00 | 0d 01 00 | 1d 09 00 00 00 00 40 00 40 00 00
```

- `02` = group.
- `08 02 00 05`: tag 08, len 2, value `00` then slot `05`. If `cover`=1, the tag is `19` instead of `08` [inferred meaning].
- `09 01 (1 - data_save)`: flag tag.
- `0c 01 slot`: slot index.
- `1c 06 type_bg MODE COUNT(u16) TIME_SYNC(u16)`:
  - type_bg = 0.
  - MODE 0 = loop (count = `play_loop`, default 1), 1 = fixed (count = `play_fixed_time`), 2 = play_cmd.
  - TIME_SYNC default 3 (seconds).
- Per region: `0d 01 region_index` then `1d 09 x(u16le) y(u16le) w(u16le) h(u16le) bg(u8)`. Default rect is 0,0,W,H.
- Optional `16 ...` period block (`info_period_pro`).

**Item / chunk frames, group 02, tag 09** (`B`, ~1222866, used for image/GIF data):

```
02 | 09 01 01 | 0c 01 05 | 0d 01 00 | 0e 01 00 | 12 07 TOTAL(u16) INDEX(u16) 960(u16) FLAG(u8) | 13 LEN(TLV) | DATA
```

- `09 01 (1 - data_save)`.
- `0c 01 slot`.
- `0d 01 region`.
- `0e 01 item` (item index within region, 0-based).
- `12 07`: chunk descriptor, 7 bytes.
  - TOTAL = number of chunks = ceil(len / 960).
  - INDEX = chunk index, 0-based, u16le.
  - `960` = nominal chunk size, u16le. It is always 960, even for the last chunk.
  - FLAG = 0 for images. The upgrade path uses 1.
- `13 LEN`: data-length TLV. For 960 it is `82 C0 03`. For 783 it is `82 0F 03`.
- DATA: the chunk bytes. Last chunk is `len % 960` bytes.

Checked against captured example: `020901010c01050d01000e010012070d000000c003001382c003`:
- `0d 00` total = 13.
- `00 00` index 0. Second chunk has `01 00`.
- `c0 03` = 960.
- `00` flag.
- `13 82 c0 03` = data TLV with length 960.

12303 = 12x960 + 783, so 13 chunks. Matches the spec.

**Finish frame, group 02, tag 35** (`re`, ~1238374):

```
02 | 35 19 SLOT | KEY(20 bytes) | TS(u32le)
```

- `35 19`: tag 0x35, len 25.
- SLOT = id_pro-1 (here 05).
- KEY = first 20 bytes of `id_res` as hex bytes [code: `hex_to_bytes(id_res,2)`, 20 bytes].
- TS = `program.ts_update` = unix seconds, u32le. Captured `9a0add6a` gives 0x6add0a9a = 1792871066 (2026-10-24), consistent with today's date.
- `id_res` = 8-char prefix + 32-char MD5. The prefix is `(id_pro + "0000000").substr(0,8)` [code, `getIDres`]. For id_pro 6 this is `60000000`, which matches the first 4 key bytes in the captured frame.
- The 32-char MD5 is computed over `JSON.stringify(pkts_program) + bin_to_hex(buffers)`. Replicating it would require the exact JSON and buffer bytes [inferred from `getIDres`]. Whether the panel checks the key is **unknown**.

**Image data format** (`q`, ~1225579) [code]:
- Header is 15 bytes: `"YSTP01"` (6 bytes), width u16le at 6, height u16le at 8, page count u16le at 10, `len_valid` u16le at 12, bit depth u8 at 14.
- Depth 1-8 has a palette of `3 * 2^depth` bytes. Depth 24 has no palette and stores raw BGR pixels per row.
- Captured 12303 bytes = 15 + 64*64*3, so the image is 64x64 24-bit with no palette [inferred from arithmetic].

**Mapping to a new program (slot 6, id_pro 7, one 64x64 24-bit image of 12303 bytes):**
- Slot byte = `06` in `0c 01 06`, `08 02 00 06`, `35 19 06`, and `0d 01 00 0e 01 00` unchanged.
- `id_pro` = 7. Key prefix = `70000000`. Full key needs MD5 (see above).
- Chunk count = 13: `0d 00`, indices `00 00` to `0c 00`, size `c0 03`, flag `00`.
- Final chunk length 783 = `0f 03`, TLV `82 0f 03`.
- Frames: start, 13 data frames, finish. Each frame gets its own `sno`/`SEQ`.

### 2.3b Text program [code + capture]

Decoded from `trace-2.pklg` (frame at 96.3s, text "Wesley Text") and the app's builders `Q`, `F`, `ie`. The whole program is **one frame**, not start/item/chunk frames, because it is under the 997-byte single-frame limit (`ne` splits above that). Reproduced byte for byte by `textBody` in `panel/text.go`.

```
02 | 08 02 00 SLOT | 09 01 01 | 0c 01 SLOT | 1c 06 00 00 01 00 00 00 | 0d 01 00 | 1d 09 0 0 W H 0     start + region
   | 09 01 01 | 0c 01 SLOT | 0d 01 00 | 0e 01 00
   | 14 03 ANI SPEED STAY                                    animation: 00 09 03 in the capture
   | 11 0a 00 01 00 06 CODE FONT SIZE ROT INTERVAL(u16)     text style: 01 00 10 00, interval = panel width (0x40)
   | 09 01 01 | 0c 01 SLOT | 0d 01 00 | 0e 01 00 | 12 07 01 00 00 00 c0 03 00 | 13 LEN DATA
   | 18 06 02 SLOT 00 MODE COUNT(u16)                       show now: mode 0 = loop COUNT times, 1 = fixed time
   | 35 19 SLOT KEY(20) TS(u32le)                           finish, same as images
```

- `1c 06` here has `time_sync` 0 (images use 3). Not understood.
- `14 03`: animation type (`model_normal`, 0 here), speed (UI speed minus 1), stay time in seconds [code `ee`].
- `11 0a`: play info `00 COUNT(u16)`, then item kind 6 (text; 9 for audio), then the font fields. ROT is 3 when the text is rotated 90 degrees [code `Q`].
- `12 07`: one chunk, nominal size 960, same as images.
- DATA = layout `03 00 PAGE | 03 01 LINE` (vertical and horizontal alignment, 0 top/left, 1 centre, 2 bottom/right; extra `03 02 1` for 270 degrees, `02 N` rotation, `04 00 N` line spacing, `04 01 N` letter spacing), then a colour block `00 FG(3) BG(3)`, a font block `01 CODE FONT SIZE`, then the text in GB encoding. ASCII is identical in GB.
- Captured colours `ff ff ff 00 00 00` are white on black. The text ended in `0a`; whether the app appends it is unknown.
- Only this one example exists. Colour, size, alignment, animation and speed values other than these are read from the code, not confirmed on the panel.
- Clock, timing, temperature and sensor items reuse this frame with placeholder tokens in the text (`{hh}` becomes `06 02 03`, and so on) [code `Z`].

### 2.4 Clock

- `ani_num` (template `dataTimeParse[0..3]`): types 0-3 with `fmt`, `fmt_data`, `h`, `m`, `s`, `score_r`, `score_b`. Byte layout not decoded (case 33 at ~1253331).

### 2.5 Schedule / timers

- `ce` (~1239510), dispatch case 20 `play_list`: entries of 11 bytes each [code]:
  - `slot-1` (id_pro-1), `rect`, `item`
  - buzzer `0x80` or 0
  - play-once (0) or weekday bitmask (`127` = all days)
  - start h, m, s
  - end h, m, s
- Second branch of `ce` builds a 36002-byte table. Not decoded further.

### 2.6 Firmware upgrade

- Caller: `firmwareDateUpgrade` (~94287). Template `data_parse[14]`: `{upgrade:{model, checksum, md5}}`.
- Firmware file comes from server APIs `api/device/get_new_upgrade` and `api/device/get_app_file` (HTTP, not BLE).
- `md5` = MD5 of the firmware file.
- Upgrade is case 14 (~1250610), which calls `se(...)`:
  - First packet is a 0x12 record with FLAG=1. Then 0x13 with the string `XXXX,model,00000000<md5>`, where commas become NUL, `XXXX` is the 16-bit checksum of the firmware as 4 hex chars. The checksum must match the server value or the send aborts.
  - Data chunks follow with the same 0x12 chunking, FLAG=1.
  - Group is **04** (`ne(n,a,193,4)`).
- Panel error code 10 triggers a "data error" dialog (~94000). Not decoded further.

### 2.7 Reply parsing (for section 4)

- `onBLECharacteristicValueChange` (~595775) to `dealParseDataBuff` (~50832) to `parse_ack_ex`.
- Reply fields stored: `light`, `power`, `dev_info`, `param_dev`, `bluetooth`, `pgm_flicker`, `rotate`, `pgm_play`, `telecontrol`, `pgm_key` (ts and key list), `pwd` (type). Error codes: 10 (data error), 14 (password required).

### 2.8 Not found in this build

- Cases 6, 7, 8, 9, 10, 13, 15-19, 22, 25, 26, 29 are missing from the `ye` dispatcher. These names are `param_gprs`, `param_net`, `param_wifi`, `baudrate`, `config_pgm`, `param_svr`, `tab_scan`, `text_part_update`, `sync`, `param_timing`, `expend`, `param_io` (23 is `_e`), `gamma`, and `delay_start`. These return `null` and are never sent. **No BLE Wi-Fi credential command exists in this build.**

---

## 3. Risk flags (do not send)

| Risk | Where | Bytes | Effect |
|---|---|---|---|
| **Format / erase all programs** | `deleteFormat` (`data_parse[26]`) | `08 01 03` (group 02) | Erases stored programs. Closest thing to factory reset found. |
| **Clear screen / delete all** | `data_parse[9]`, `del_all` | `08 02 00 FF` | Deletes all programs. |
| **Delete program IDs / rect / items** | `delProgramIds`, `data_parse[13,19,24,25]` | `08 LEN 00 ids...`, `08 LEN 01 slot ...`, `08 LEN 02 slot rect ...` | Deletes programs or parts. |
| **Password set / close / open** | `setPassword`, `colsePassword`, `openPassword` | `37 LEN 01 VAL`, `37 01 02`, `37 01 03` | Locks or unlocks the panel. Wrong password returns error 14. |
| **Password verify** | `verifyPassword` | `37 LEN 00 VAL` | Verify only. Not destructive by itself. |
| **Firmware upgrade** | `firmwareDateUpgrade`, `se()` | group 04, `12 07 ... FLAG=1`, metadata `XXXX,model,00000000<md5>` | Overwrites firmware. Bad image may brick the panel. |
| **Screen size / param_dev set** | case 3 (`param_dev`) | `1B 05 01 W(u16le) H(u16le)` | Changes geometry. Could break layouts or partitions. Caller not located. |
| **Device ID set** | case 24 (`set_id`) | `0A 0A <10 bytes>` | Changes device identity. Caller not located. |
| **Schedule / playlist changes** | `ce`, cases 20-21 | `27 00`, `26 02 00 01`, play-list tables | Changes what plays and when. |
| **Brightness** | case 5 | `06 02 00 V` | Not destructive, but V=0 means 15. |
| **Wi-Fi / network settings** | none in BLE | n/a | Not found. Wi-Fi page uses phone-side APIs and HTTP. |
| **Dev info with key** | `getDevInitInfoCmd`, `dev_info` query | `0A LEN <ble_password>` | Sends the stored BLE password in plaintext on every connect. Expected by the app. Log it, do not copy it into notes. |

---

## 4. Does the app read pixel data back from the panel?

**No, as far as the code shows.**

Evidence:
- All reply handling goes through `dealParseDataBuff` and `parse_ack_ex`. The ack fields stored are light, power, dev_info, param_dev, bluetooth, pgm_flicker, rotate, pgm_play, telecontrol, pgm_key (key list and timestamp), and pwd type. No branch stores image or pixel buffers.
- The `get`/`ge` queries only return metadata (power, light, dev_info, param_dev, rotate, bluetooth, pgm_key, pwd, and so on).
- `rt_draw` writes pixels to the panel. It is not a read.

Caveat: the full `parse_ack_ex` decoder (~1114715) was not read line by line.

---

## Gaps and uncertainty

- Clock style (`ani_num`) byte layout: not decoded.
- Brightness UI range: not confirmed. Only 1-14 observed in captures, 15 is the code default.
- Rotation angle mapping (`getRotateValue`): not read.
- `cover` tag (`19` vs `08`): meaning inferred.
- `SEQ` mapping to `sno`: partially inferred from `ne`/`ve`.
- MD5 part of `id_res`: not reproducible without the exact JSON and buffers. Panel validation of the key is unknown.
- Flags `12 07 ... 00` and `01`: `00` for images and `01` for upgrade, from code. Panel-side meaning inferred.
- Cases 1-2, 4-5, 14, 21, 24, 42 (as noted above) read directly. Other dispatcher cases (27-44 etc.) only spot-checked. Schedule (`ce`) second branch and `B`/`O`/`se` edge cases not fully decoded.
