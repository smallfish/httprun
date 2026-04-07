#!/usr/bin/env bash

set -euo pipefail

BIN_PATH="${1:-./bin/httprun}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Validate example files"
"$BIN_PATH" validate examples/demo.http
"$BIN_PATH" validate examples/assertions.http
"$BIN_PATH" validate examples/assertions_failure.http
"$BIN_PATH" validate --env dev examples/all_methods.http
"$BIN_PATH" validate examples/request_options.http examples/timeout.http
"$BIN_PATH" validate --jobs 2 examples/demo.http examples/request_options.http

echo "Run direct example"
"$BIN_PATH" run --name ping examples/demo.http >"$TMP_DIR/ping.out"
grep -q "200 OK" "$TMP_DIR/ping.out"

echo "Run assertion example"
"$BIN_PATH" run examples/assertions.http >"$TMP_DIR/assert.out"
grep -q "Summary: 4 requests, 4 passed" "$TMP_DIR/assert.out"
grep -q "assertHeaders" "$TMP_DIR/assert.out"

echo "Verify assertion failure behavior"
if "$BIN_PATH" run examples/assertions_failure.http >"$TMP_DIR/assert-fail.out" 2>"$TMP_DIR/assert-fail.err"; then
  echo "expected assertion failure example to fail"
  exit 1
fi
test ! -s "$TMP_DIR/assert-fail.err"
grep -q "Assertion Failures:" "$TMP_DIR/assert-fail.out"
grep -q "Summary: 1/2 executed, 0 passed, 1 failed, 1 skipped" "$TMP_DIR/assert-fail.out"

echo "Run env + external body example"
"$BIN_PATH" run --env dev --name createItem examples/all_methods.http >"$TMP_DIR/create.out"
grep -q "200 OK" "$TMP_DIR/create.out"

echo "Verify redirect behavior"
"$BIN_PATH" run --name followsRedirect examples/request_options.http >"$TMP_DIR/follows-redirect.out"
grep -q "200 OK" "$TMP_DIR/follows-redirect.out"

"$BIN_PATH" run --name noRedirect examples/request_options.http >"$TMP_DIR/no-redirect.out"
grep -q "302 Found" "$TMP_DIR/no-redirect.out"

echo "Verify timeout behavior"
if "$BIN_PATH" run --name slowRequest examples/timeout.http >"$TMP_DIR/timeout.out" 2>"$TMP_DIR/timeout.err"; then
  echo "expected timeout example to fail"
  exit 1
fi

grep -Eqi "timeout|deadline exceeded|Client\\.Timeout" "$TMP_DIR/timeout.err"

echo "Integration cases passed"
