// BLE is native only: build with `bend x.bend -o x`. The JS lane reports that instead of failing obscurely.
function ble_native_only() {
  return io_fail(95, "BLE needs a native build: bend file.bend -o file");
}

io_eff(CID(BLE.open), ble_native_only);
io_eff(CID(BLE.write), ble_native_only);
io_eff(CID(BLE.recv), ble_native_only);
io_eff(CID(BLE.drain), ble_native_only);
io_eff(CID(BLE.close), ble_native_only);
