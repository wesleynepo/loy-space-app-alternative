// BLE
// ===
// JS twins of ble.c, through bun:ffi on the same libloyble. A call blocks the JS thread until it returns.

// Copies of crypto_words and crypto_words_octets in bend-kit's crypto/effs/crypto.js; keep them in step.
function ble_words(b, n) {
  const w = Math.ceil(n / 4);
  let size = 1;
  while (size < w) {
    size *= 2;
  }
  const a = Array(size).fill(0);
  for (let i = 0; i < n; i += 1) {
    a[i >> 2] = (a[i >> 2] | (b[i] << (8 * (i & 3)))) >>> 0;
  }
  return { $: CID(Tuple), fst: n, snd: a };
}

function ble_words_octets(n, a) {
  n = Number(n);
  if (n > 4 * a.length) {
    return null;
  }
  const b = new Uint8Array(n);
  for (let i = 0; i < n; i += 1) {
    b[i] = (a[i >> 2] >>> (8 * (i & 3))) & 255;
  }
  return b;
}

function ble_fail(code, why) {
  return { $: CID(Fail), error: io_tup(code, why) };
}

const BLE_MISSING = "BLE needs libloyble (bend/ble/loyble.m); set BEND_LIBLOYBLE to its path";

// libloyble from BEND_LIBLOYBLE, else ./libloyble.dylib or ./ble/libloyble.dylib; null if none loads.
function ble_lib() {
  if (globalThis.BEND_BLE !== undefined) {
    return globalThis.BEND_BLE;
  }
  globalThis.BEND_BLE = null;
  const ffi = require("bun:ffi");
  const syms = {
    ble_open: { args: ["ptr", "i32", "i32"], returns: "i32" },
    ble_write: { args: ["ptr", "i32"], returns: "i32" },
    ble_recv: { args: ["ptr", "i32", "i32"], returns: "i32" },
    ble_drain: { args: [], returns: "void" },
    ble_close: { args: [], returns: "void" },
    ble_error: { args: ["i32"], returns: "cstring" },
  };
  for (const path of [process.env.BEND_LIBLOYBLE, "./libloyble.dylib", "./ble/libloyble.dylib"]) {
    if (!path) {
      continue;
    }
    try {
      globalThis.BEND_BLE = { s: ffi.dlopen(path, syms).symbols, ffi };
      break;
    } catch {
      continue;
    }
  }
  return globalThis.BEND_BLE;
}

// A pointer to b; ffi.ptr refuses an empty array.
function ble_ptr(ffi, b) {
  return ffi.ptr(b.length ? b : new Uint8Array(1));
}

// The library's code is negative on failure; 0 or more is success.
function ble_status(c, rc) {
  return rc < 0 ? ble_fail(-rc, String(c.s.ble_error(rc))) : io_done(0);
}

function ble_open(name, scan_ms, connect_ms) {
  const c = ble_lib();
  if (c === null) {
    return ble_fail(2, BLE_MISSING);
  }
  if (name.includes("\0")) {
    return io_fail(84);
  }
  const cname = new TextEncoder().encode(name + "\0");
  return ble_status(c, c.s.ble_open(ble_ptr(c.ffi, cname), scan_ms, connect_ms));
}

function ble_write_words(n, words) {
  const b = ble_words_octets(n, words);
  if (b === null || b.length === 0) {
    return io_fail(22);
  }
  const c = ble_lib();
  if (c === null) {
    return ble_fail(2, BLE_MISSING);
  }
  return ble_status(c, c.s.ble_write(ble_ptr(c.ffi, b), b.length));
}

function ble_recv_words(timeout_ms) {
  const c = ble_lib();
  if (c === null) {
    return ble_fail(2, BLE_MISSING);
  }
  const buf = new Uint8Array(512);
  const n = c.s.ble_recv(c.ffi.ptr(buf), 512, timeout_ms);
  return n < 0 ? ble_fail(-n, String(c.s.ble_error(n))) : io_done(ble_words(buf, n));
}

function ble_drain() {
  const c = ble_lib();
  if (c === null) {
    return ble_fail(2, BLE_MISSING);
  }
  c.s.ble_drain();
  return io_done(0);
}

function ble_close() {
  const c = ble_lib();
  if (c === null) {
    return ble_fail(2, BLE_MISSING);
  }
  c.s.ble_close();
  return io_done(0);
}

io_eff(CID(BLE.open), ble_open);
io_eff(CID(BLE.write.words), ble_write_words);
io_eff(CID(BLE.recv.words), ble_recv_words);
io_eff(CID(BLE.drain), ble_drain);
io_eff(CID(BLE.close), ble_close);
