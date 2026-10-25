// Blocking CoreBluetooth central for the LOY panel: find by name, connect, subscribe, write, recv.
// One connection per process. Every call blocks and takes a timeout; see BLE-BRIDGE-SPEC.md.
#import <CoreBluetooth/CoreBluetooth.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

enum { OK = 0, E_RADIO = -1, E_NOTFOUND = -2, E_CONNECT = -3, E_CHARS = -4, E_NOCONN = -5, E_WRITE = -6, E_RECV = -7 };
#define WRITE_MAX 180

@interface Loy : NSObject <CBCentralManagerDelegate, CBPeripheralDelegate>
@end

static CBCentralManager* cm;
static dispatch_queue_t q;
static Loy* dl;
static CBPeripheral* per;
static CBCharacteristic *wch, *n1, *n2;
static volatile int connected, pending, ready_state;
static NSString* want_name;
static dispatch_semaphore_t sReady, sFound, sConn, sWrite, sRx, sDisc;
static NSMutableArray<NSData*>* rx;
static NSLock* rxLock;

static BOOL is(CBCharacteristic* c, NSString* uuid) { return [c.UUID isEqual:[CBUUID UUIDWithString:uuid]]; }

@implementation Loy
- (void)centralManagerDidUpdateState:(CBCentralManager*)c {
  ready_state = (int)c.state;
  if (c.state == CBManagerStatePoweredOn) dispatch_semaphore_signal(sReady);
}
- (void)centralManager:(CBCentralManager*)c didDiscoverPeripheral:(CBPeripheral*)p
     advertisementData:(NSDictionary*)ad RSSI:(NSNumber*)rssi {
  NSString* n = ad[CBAdvertisementDataLocalNameKey] ?: p.name;
  if (want_name && [n isEqualToString:want_name] && !per) {
    per = p; p.delegate = self;
    [c stopScan];
    dispatch_semaphore_signal(sFound);
  }
}
- (void)centralManager:(CBCentralManager*)c didConnectPeripheral:(CBPeripheral*)p { [p discoverServices:nil]; }
- (void)centralManager:(CBCentralManager*)c didFailToConnectPeripheral:(CBPeripheral*)p error:(NSError*)e {
  dispatch_semaphore_signal(sConn);
}
- (void)centralManager:(CBCentralManager*)c didDisconnectPeripheral:(CBPeripheral*)p error:(NSError*)e {
  connected = 0;
  dispatch_semaphore_signal(sDisc); dispatch_semaphore_signal(sConn);
  dispatch_semaphore_signal(sRx); dispatch_semaphore_signal(sWrite);
}
- (void)peripheral:(CBPeripheral*)p didDiscoverServices:(NSError*)e {
  if (e || p.services.count == 0) { dispatch_semaphore_signal(sConn); return; }
  pending = (int)p.services.count;
  for (CBService* s in p.services) [p discoverCharacteristics:nil forService:s];
}
- (void)peripheral:(CBPeripheral*)p didDiscoverCharacteristicsForService:(CBService*)s error:(NSError*)e {
  for (CBCharacteristic* c in s.characteristics) {
    if (is(c, @"FFF2")) wch = c;
    else if (is(c, @"FFF1")) n1 = c;
    else if (is(c, @"5833FF03-9B8B-5191-6142-22A4536EF123")) n2 = c;
  }
  if (--pending > 0) return;
  if (!wch || !n1) { dispatch_semaphore_signal(sConn); return; }
  pending = n2 ? 2 : 1;                       // the app subscribes to both notify characteristics
  [p setNotifyValue:YES forCharacteristic:n1];
  if (n2) [p setNotifyValue:YES forCharacteristic:n2];
}
- (void)peripheral:(CBPeripheral*)p didUpdateNotificationStateForCharacteristic:(CBCharacteristic*)c error:(NSError*)e {
  if (e) { dispatch_semaphore_signal(sConn); return; }
  if (--pending == 0) { connected = 1; dispatch_semaphore_signal(sConn); }
}
- (void)peripheral:(CBPeripheral*)p didUpdateValueForCharacteristic:(CBCharacteristic*)c error:(NSError*)e {
  if (e || !c.value) return;
  [rxLock lock]; [rx addObject:[c.value copy]]; [rxLock unlock];
  dispatch_semaphore_signal(sRx);
}
- (void)peripheralIsReadyToSendWriteWithoutResponse:(CBPeripheral*)p { dispatch_semaphore_signal(sWrite); }
@end

static int await_sem(dispatch_semaphore_t s, int ms) {
  return dispatch_semaphore_wait(s, dispatch_time(DISPATCH_TIME_NOW, (int64_t)ms * NSEC_PER_MSEC)) == 0;
}

void ble_close(void);

// Find the panel by advertised name, connect, discover and subscribe. 0 on success, else a negative code.
int ble_open(const char* name, int scan_ms, int connect_ms) {
  @autoreleasepool {
    if (connected) return OK;
    if (!cm) {
      sReady = dispatch_semaphore_create(0); sFound = dispatch_semaphore_create(0);
      sConn = dispatch_semaphore_create(0); sWrite = dispatch_semaphore_create(0);
      sRx = dispatch_semaphore_create(0); sDisc = dispatch_semaphore_create(0);
      rx = [NSMutableArray new]; rxLock = [NSLock new];
      q = dispatch_queue_create("loyble", DISPATCH_QUEUE_SERIAL);
      dl = [Loy new];
      cm = [[CBCentralManager alloc] initWithDelegate:dl queue:q];
      atexit(ble_close);  // the panel takes one connection; leave it free when the process ends
    }
    if (!await_sem(sReady, 5000)) return E_RADIO;
    per = nil; wch = n1 = n2 = nil; want_name = [NSString stringWithUTF8String:name];
    dispatch_sync(q, ^{ [cm scanForPeripheralsWithServices:nil options:nil]; });
    if (!await_sem(sFound, scan_ms)) { dispatch_sync(q, ^{ [cm stopScan]; }); return E_NOTFOUND; }
    dispatch_sync(q, ^{ [cm connectPeripheral:per options:nil]; });
    await_sem(sConn, connect_ms);
    if (connected) return OK;
    dispatch_sync(q, ^{ if (per) [cm cancelPeripheralConnection:per]; });
    return wch && n1 ? E_CONNECT : (per.services.count ? E_CHARS : E_CONNECT);
  }
}

// Write without response in pieces of at most 180 bytes, honouring the radio's flow control.
int ble_write(const uint8_t* buf, int len) {
  @autoreleasepool {
    for (int off = 0; off < len; off += WRITE_MAX) {
      if (!connected) return E_NOCONN;
      NSData* d = [NSData dataWithBytes:buf + off length:(len - off < WRITE_MAX ? len - off : WRITE_MAX)];
      __block BOOL can = NO;
      for (int tries = 0; tries < 100; tries++) {
        dispatch_sync(q, ^{ can = per.canSendWriteWithoutResponse; });
        if (can) break;
        await_sem(sWrite, 20);
      }
      if (!can) return E_WRITE;
      dispatch_sync(q, ^{ [per writeValue:d forCharacteristic:wch type:CBCharacteristicWriteWithoutResponse]; });
      if (len > WRITE_MAX) usleep(20000);
    }
    return OK;
  }
}

// One notification chunk into out; its length, or E_RECV on timeout / E_NOCONN once disconnected and empty.
int ble_recv(uint8_t* out, int cap, int timeout_ms) {
  for (;;) {
    [rxLock lock];
    NSData* d = rx.count ? rx[0] : nil;
    if (d) [rx removeObjectAtIndex:0];
    [rxLock unlock];
    if (d) { int n = (int)d.length < cap ? (int)d.length : cap; memcpy(out, d.bytes, n); return n; }
    if (!connected) return E_NOCONN;
    if (!await_sem(sRx, timeout_ms)) return E_RECV;
  }
}

void ble_drain(void) {
  [rxLock lock]; [rx removeAllObjects]; [rxLock unlock];
  while (dispatch_semaphore_wait(sRx, DISPATCH_TIME_NOW) == 0) {}
}

int ble_is_connected(void) { return connected; }

void ble_close(void) {
  if (!cm || !per) return;
  if (connected) {
    dispatch_sync(q, ^{ [cm cancelPeripheralConnection:per]; });
    await_sem(sDisc, 2000);
  }
  connected = 0;
}

const char* ble_error(int code) {
  switch (code) {
    case E_RADIO: return "Bluetooth is off or this app has no Bluetooth permission";
    case E_NOTFOUND: return "panel not found (close LOY SPACE / power-cycle the panel)";
    case E_CONNECT: return "connect failed or timed out";
    case E_CHARS: return "panel is missing the expected characteristics";
    case E_NOCONN: return "not connected";
    case E_WRITE: return "write timed out (radio not ready)";
    case E_RECV: return "no reply from panel";
    default: return "unknown BLE error";
  }
}
