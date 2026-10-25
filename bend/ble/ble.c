// BLE effects: thin wrappers over libloyble (bend/ble/loyble.m), which owns all CoreBluetooth state.
// The library is loaded at run time from $BEND_LIBLOYBLE or ./libloyble.dylib, like bend-kit's libcrypto.
// Blocking calls run on an IO helper thread via io_work, so the event loop and other computations keep going.
#include <dlfcn.h>

static struct {
  int (*open)(const char*, int, int);
  int (*write)(const unsigned char*, int);
  int (*recv)(unsigned char*, int, int);
  void (*drain)(void);
  void (*close)(void);
  const char* (*error)(int);
} ble_lib;
static int ble_loaded;

static const char* ble_load(void) {
  if (ble_loaded) return NULL;
  const char* path = getenv("BEND_LIBLOYBLE");
  void* h = dlopen(path ? path : "./libloyble.dylib", RTLD_NOW);
  if (!h) return dlerror();
  ble_lib.open = (int (*)(const char*, int, int))dlsym(h, "ble_open");
  ble_lib.write = (int (*)(const unsigned char*, int))dlsym(h, "ble_write");
  ble_lib.recv = (int (*)(unsigned char*, int, int))dlsym(h, "ble_recv");
  ble_lib.drain = (void (*)(void))dlsym(h, "ble_drain");
  ble_lib.close = (void (*)(void))dlsym(h, "ble_close");
  ble_lib.error = (const char* (*)(int))dlsym(h, "ble_error");
  if (!ble_lib.open || !ble_lib.write || !ble_lib.recv || !ble_lib.drain || !ble_lib.close || !ble_lib.error)
    return "libloyble is missing a symbol";
  ble_loaded = 1;
  return NULL;
}

// w->hand carries the library's return code from the helper thread to the pack function.
static Term ble_status(Env e, IoWork* w) {
  int rc = (int)w->hand;
  return rc < 0 ? io_fail(e, (uint32_t)(-rc), ble_lib.error(rc)) : io_done(e, (Term)0);
}

#ifdef CID(BLE.open)

static void ble_open_call(IoWork* w) {
  w->hand = ble_lib.open((const char*)w->data, (int)w->word, (int)w->made);
}

static Term ble_open_pack(Env e, IoWork* w) {
  free(w->data);
  return ble_status(e, w);
}

// BLE.open(name, scan_ms, connect_ms): find the panel by name, connect, subscribe.
Term ble_open_run(Env e, Term* f, IoWork* w) {
  const char* err = ble_load();
  if (err) return io_fail(e, ENOENT, err);
  w->data = io_cstr(e, f[0], &w->size);
  if (io_nul(w->data, w->size)) {
    free(w->data);
    return io_fail(e, EILSEQ, NULL);
  }
  w->word = (uint32_t)f[1];
  w->made = (intptr_t)(uint32_t)f[2];
  return io_work(w, ble_open_call, ble_open_pack);
}

static void __attribute__((constructor)) ble_open_use(void) {
  io_eff(CID(BLE.open), ble_open_run);
}

#endif

#ifdef CID(BLE.write)

static void ble_write_call(IoWork* w) {
  w->hand = ble_lib.write((const unsigned char*)w->data, (int)w->size);
}

static Term ble_write_pack(Env e, IoWork* w) {
  free(w->data);
  return ble_status(e, w);
}

static int ble_hexval(char c) {
  if (c >= '0' && c <= '9') return c - '0';
  if (c >= 'a' && c <= 'f') return c - 'a' + 10;
  if (c >= 'A' && c <= 'F') return c - 'A' + 10;
  return -1;
}

// BLE.write(hex): one frame, as hex text; the library splits it into <=180-byte writes.
Term ble_write_run(Env e, Term* f, IoWork* w) {
  const char* err = ble_load();
  if (err) return io_fail(e, ENOENT, err);
  uint64_t n = 0;
  char* hex = io_cstr(e, f[0], &n);
  unsigned char* bytes = (unsigned char*)malloc(n / 2 + 1);
  int ok = n % 2 == 0 && n > 0;
  for (uint64_t i = 0; ok && i < n; i += 2) {
    int hi = ble_hexval(hex[i]), lo = ble_hexval(hex[i + 1]);
    ok = hi >= 0 && lo >= 0;
    if (ok) bytes[i / 2] = (unsigned char)(hi * 16 + lo);
  }
  free(hex);
  if (!ok) {
    free(bytes);
    return io_fail(e, EINVAL, "BLE.write expects non-empty hex text");
  }
  w->data = (char*)bytes;
  w->size = n / 2;
  return io_work(w, ble_write_call, ble_write_pack);
}

static void __attribute__((constructor)) ble_write_use(void) {
  io_eff(CID(BLE.write), ble_write_run);
}

#endif

#ifdef CID(BLE.recv)

static void ble_recv_call(IoWork* w) {
  w->hand = ble_lib.recv((unsigned char*)w->data, 512, (int)w->word);
}

static Term ble_recv_pack(Env e, IoWork* w) {
  int n = (int)w->hand;
  if (n < 0) {
    free(w->data);
    return io_fail(e, (uint32_t)(-n), ble_lib.error(n));
  }
  static const char digits[] = "0123456789abcdef";
  char* hex = (char*)malloc((size_t)n * 2 + 1);
  for (int i = 0; i < n; i++) {
    hex[2 * i] = digits[((unsigned char*)w->data)[i] >> 4];
    hex[2 * i + 1] = digits[((unsigned char*)w->data)[i] & 15];
  }
  Term out = io_done(e, io_str(e, hex, (uint64_t)n * 2));
  free(hex);
  free(w->data);
  return out;
}

// BLE.recv(timeout_ms): the next notification chunk, as hex text.
Term ble_recv_run(Env e, Term* f, IoWork* w) {
  const char* err = ble_load();
  if (err) return io_fail(e, ENOENT, err);
  w->data = malloc(512);
  w->word = (uint32_t)f[0];
  return io_work(w, ble_recv_call, ble_recv_pack);
}

static void __attribute__((constructor)) ble_recv_use(void) {
  io_eff(CID(BLE.recv), ble_recv_run);
}

#endif

#ifdef CID(BLE.drain)

static void ble_drain_call(IoWork* w) {
  ble_lib.drain();
  w->hand = 0;
}

static Term ble_drain_pack(Env e, IoWork* w) {
  return ble_status(e, w);
}

Term ble_drain_run(Env e, Term* f, IoWork* w) {
  const char* err = ble_load();
  if (err) return io_fail(e, ENOENT, err);
  return io_work(w, ble_drain_call, ble_drain_pack);
}

static void __attribute__((constructor)) ble_drain_use(void) {
  io_eff(CID(BLE.drain), ble_drain_run);
}

#endif

#ifdef CID(BLE.close)

static void ble_close_call(IoWork* w) {
  ble_lib.close();
  w->hand = 0;
}

static Term ble_close_pack(Env e, IoWork* w) {
  return ble_status(e, w);
}

Term ble_close_run(Env e, Term* f, IoWork* w) {
  const char* err = ble_load();
  if (err) return io_fail(e, ENOENT, err);
  return io_work(w, ble_close_call, ble_close_pack);
}

static void __attribute__((constructor)) ble_close_use(void) {
  io_eff(CID(BLE.close), ble_close_run);
}

#endif
