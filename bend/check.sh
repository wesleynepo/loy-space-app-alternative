#!/bin/sh
# Every gate, one bend process at a time (a runaway bend can use tens of GB; see bend-kit's AGENTS.md).
# Needs Bend 2 on PATH; the slot-key test also needs OpenSSL 3 (brew install openssl@3).
set -eu
cd "$(dirname "$0")"
export BEND_NO_TELEMETRY=1
echo "== proofs";         bend PROOF.bend
echo "== golden vectors"; bend test.bend
echo "== properties";     bend prop_test.bend
echo "== slot keys (native build)"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
bend key_test.bend -o "$out/key_test"
BEND_LIBCRYPTO="${BEND_LIBCRYPTO:-/opt/homebrew/opt/openssl@3/lib/libcrypto.3.dylib}" "$out/key_test"
