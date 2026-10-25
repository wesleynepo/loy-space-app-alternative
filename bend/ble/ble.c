// BLE effects: thin wrappers over libloyble (bend/ble/loyble.m), which owns all CoreBluetooth state.
// The library is loaded at run time from $BEND_LIBLOYBLE, like bend-kit's libcrypto. Blocking calls run on an IO
// helper thread via io_work, so the event loop and other computations keep going.
#include <dlfcn.h>
#include <string.h>

// Copies of crypto_words and crypto_words_octets in bend-kit's crypto/effs/crypto.c (and wire_words in wire.c):
// bytes cross the boundary as (len, words), four octets to a U32 word, low byte first. Keep them in step.
static Term ble_words(Env e, const unsigned char* p, u64 n) {
  u64  w    = (n + 3) / 4;
  u64  d    = 0;
  Term zero = 0;
  while ((1ull << d) < w) {
    d += 1;
  }
  Term a = blk_new(e, false, d, 0, 1, &zero);
  u64  l = blk_loc(e.mem, a);
  for (u64 k = 0; k < w; k += 1) {
    u32 x = 0;
    for (u64 j = 0; j < 4 && 4 * k + j < n; j += 1) {
      x |= (u32)p[4 * k + j] << (8 * j);
    }
    blk_write(e.mem, false, l, (u32)k, x);
  }
  return io_tup(e, (Term)n, a);
}

// Consumes the array term a; bad is set when a is not an array that holds n octets.
static unsigned char* ble_words_octets(Env e, Term a, u64 n, u64* len, bool* bad) {
  u64* H = e.mem;
  bool b = term_tag(a) != TAG_BUF || n > (4ull << blk_cls(a));
  *bad   = *bad || b;
  *len   = b ? 0 : n;
  unsigned char* buf = io_mem(malloc(*len + 1));
  u64            l   = b ? 0 : blk_loc(H, a);
  for (u64 i = 0; i < *len; i += 1) {
    buf[i] = (unsigned char)(blk_read(H, false, l, (u32)(i / 4)) >> (8 * (i % 4)));
  }
  term_drop(e, a);
  return buf;
}

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
  void* h = path ? dlopen(path, RTLD_NOW) : dlopen("./libloyble.dylib", RTLD_NOW);
  if (!h && !path) h = dlopen("./ble/libloyble.dylib", RTLD_NOW);
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

#ifdef CID(BLE.write.words)

static void ble_write_call(IoWork* w) {
  w->hand = ble_lib.write((const unsigned char*)w->data, (int)w->size);
}

static Term ble_write_pack(Env e, IoWork* w) {
  free(w->data);
  return ble_status(e, w);
}

// BLE.write.words(len, words): one frame; the library splits it into <=180-byte writes.
Term ble_write_words_run(Env e, Term* f, IoWork* w) {
  u64            n   = 0;
  bool           bad = false;
  unsigned char* in  = ble_words_octets(e, f[1], (u64)(u32)f[0], &n, &bad);
  if (bad || n == 0) {
    free(in);
    return io_fail(e, EINVAL, "BLE.write.words needs len > 0 and an array that holds len octets");
  }
  const char* err = ble_load();
  if (err) {
    free(in);
    return io_fail(e, ENOENT, err);
  }
  w->data = (char*)in;
  w->size = n;
  return io_work(w, ble_write_call, ble_write_pack);
}

static void __attribute__((constructor)) ble_write_words_use(void) {
  io_eff(CID(BLE.write.words), ble_write_words_run);
}

#endif

#ifdef CID(BLE.recv.words)

static void ble_recv_call(IoWork* w) {
  w->hand = ble_lib.recv((unsigned char*)w->data, 512, (int)w->word);
}

static Term ble_recv_pack(Env e, IoWork* w) {
  int n = (int)w->hand;
  if (n < 0) {
    free(w->data);
    return io_fail(e, (uint32_t)(-n), ble_lib.error(n));
  }
  Term out = io_done(e, ble_words(e, (const unsigned char*)w->data, (u64)n));
  free(w->data);
  return out;
}

// BLE.recv.words(timeout_ms): the next notification chunk.
Term ble_recv_words_run(Env e, Term* f, IoWork* w) {
  const char* err = ble_load();
  if (err) return io_fail(e, ENOENT, err);
  w->data = malloc(512);
  w->word = (uint32_t)f[0];
  return io_work(w, ble_recv_call, ble_recv_pack);
}

static void __attribute__((constructor)) ble_recv_words_use(void) {
  io_eff(CID(BLE.recv.words), ble_recv_words_run);
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
