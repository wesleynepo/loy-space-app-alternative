// Standalone check of libloyble against the real panel: open, send device_info (read-only), print the reply, close.
#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int main(int argc, char** argv) {
  void* h = dlopen(argc > 1 ? argv[1] : "./libloyble.dylib", RTLD_NOW);
  if (!h) { fprintf(stderr, "dlopen: %s\n", dlerror()); return 2; }
  int (*open_)(const char*, int, int) = dlsym(h, "ble_open");
  int (*write_)(const unsigned char*, int) = dlsym(h, "ble_write");
  int (*recv_)(unsigned char*, int, int) = dlsym(h, "ble_recv");
  void (*close_)(void) = dlsym(h, "ble_close");
  const char* (*err)(int) = dlsym(h, "ble_error");
  int rc = open_("YS524A250818", 10000, 20000);
  printf("open -> %d %s\n", rc, rc ? err(rc) : "");
  if (rc) return 1;
  // aa55ffff 0e00 2900 c1 030a001b0004000600 + checksum: the app's opening query (read-only)
  unsigned char f[] = {0xaa,0x55,0xff,0xff,0x0e,0x00,0x29,0x00,0xc1,0x03,0x0a,0x00,0x1b,0x00,0x04,0x00,0x06,0x00,0x27,0x04};
  printf("write -> %d\n", write_(f, sizeof f));
  unsigned char b[256];
  int n = recv_(b, sizeof b, 3000);
  printf("recv -> %d:", n);
  for (int i = 0; i < n && i < 40; i++) printf("%02x", b[i]);
  printf("\n");
  close_();
  return 0;
}
