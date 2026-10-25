#!/bin/sh
# Builds the native BLE bridge and the demo. macOS only (CoreBluetooth); needs clang and Bend 2.
set -eu
cd "$(dirname "$0")"
clang -dynamiclib -fobjc-arc -framework CoreBluetooth -framework Foundation -o ble/libloyble.dylib ble/loyble.m
clang -o ble/probe ble/probe.c
bend ble_hello.bend -o ble_hello
echo "built. ./check.sh runs the gates that need no hardware. For the panel, close LOY SPACE and disconnect the Python server, then:"
echo "  BEND_LIBLOYBLE=\$PWD/ble/libloyble.dylib ./ble_hello"
