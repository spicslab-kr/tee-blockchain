#!/usr/bin/env bash
# Go toolchain pin check. The bytes of att-ra-responder change with the Go patch version (measured: go1.26.2 and
#    go1.26.8 give different binaries even with -trimpath), and so do the measurements (PCR0/2). The pinned version has
#    one source, the file internal/setup/build/GO_TOOLCHAIN in the source tree; att embeds that file and at the start of
#    build compares it with the toolchain it was built with, refusing if they differ. This script checks that the
#    refusal really happens on a real Nitro host:
#      1. your machine's go version equals the pin file exactly
#      2. a att-vendor-wrongtc cross-built on purpose with another patch version (GOTOOLCHAIN=$WRONG) is uploaded to the
#         Vendor host; running `build` gives exit != 0 and stderr contains "pins <pinned version>"
#      3. a correctly built att-vendor passes that check (it may fail later for another reason - only the toolchain message is checked)
#    Step 2 is skipped with a notice if another toolchain cannot be downloaded (offline).
#    Run from TB-live:  bash "e2e/toolchain-pin-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST
PIN_FILE=internal/setup/build/GO_TOOLCHAIN
WANT="$(tr -d '[:space:]' < "$PIN_FILE")"
WRONG=go1.26.2   # any patch version other than the pin works; go.mod requires >= 1.26.1, so it must be newer
fail=0

step "1. LOCAL GO VERSION vs $PIN_FILE"
HAVE="$(go version | awk '{print $3}')"
if [ "$HAVE" = "$WANT" ]; then
  echo "✅ local go version = $HAVE = pin file ($PIN_FILE)"
else
  echo "❌ local go version is $HAVE - the pin file says $WANT. Fix it with bash e2e/0-Setup-local-install.sh."
  fail=1
fi

mkdir -p "$WORK/toolchain"
step "2. CROSS-BUILDING att-vendor WITH THE WRONG TOOLCHAIN ($WRONG)"
if GOTOOLCHAIN=$WRONG go version 2>/dev/null | grep -q "$WRONG"; then
  GOTOOLCHAIN=$WRONG GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/toolchain/att-vendor-wrongtc" ./cmd/vendor
  HAVE_WRONG=1
else
  echo "WARNING: cannot download GOTOOLCHAIN=$WRONG (offline?) - skipping the live refusal check. The unit tests (go test ./internal/setup/build ./cmd -run Toolchain) cover the same path."
  HAVE_WRONG=0
fi
GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/toolchain/att-vendor" ./cmd/vendor

step "3. UPLOADING TO THE VENDOR HOST"
nitroBanner "Vendor"
[ $HAVE_WRONG = 1 ] && $SCP "$WORK/toolchain/att-vendor-wrongtc" "$SSH_USER@$VENDOR_HOST:~/att-vendor-wrongtc"
$SCP "$WORK/toolchain/att-vendor" "$SSH_USER@$VENDOR_HOST:~/att-vendor"

# remote_build <binary> - run `build` on the Vendor host and return stderr+stdout. The exit code is carried in an __EXIT= line.
remote_build() {
  V "chmod +x $1; mkdir -p /tmp/tc-app; printf 'FROM scratch\n' > /tmp/tc-app/Dockerfile; ./$1 build --app-dir /tmp/tc-app --out-dir /tmp/tc-out 2>&1; echo \"__EXIT=\$?\""
}

if [ $HAVE_WRONG = 1 ]; then
  step "4. build WITH att-vendor-wrongtc ($WRONG) - MUST BE REFUSED"
  out="$(remote_build att-vendor-wrongtc)"
  echo "$out" | sed 's/^/    /'
  ec="$(echo "$out" | sed -n 's/^__EXIT=//p' | tail -1)"
  if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "pins $WANT"; then
    echo "✅ refused (exit $ec) - 'built with $WRONG but the project pins $WANT'"
  else
    echo "❌ a att-vendor built with another toolchain was not refused (exit ${ec:-?}) - CheckToolchain is not running at the start of build"
    fail=1
  fi
fi

step "5. build WITH THE CORRECTLY BUILT att-vendor ($WANT) - MUST PASS THE TOOLCHAIN CHECK"
out="$(remote_build att-vendor)"
echo "$out" | sed 's/^/    /'
if echo "$out" | grep -q "pins $WANT"; then
  echo "❌ a att-vendor built with the pinned toolchain was refused by the toolchain check - GO_TOOLCHAIN and the real build toolchain disagree"
  fail=1
else
  echo "✅ toolchain check passed (a later failure for another reason is outside this test)"
fi
localBanner

echo
echo "===================== toolchain pin ====================="
[ $fail = 0 ] && echo "✅ all passed" || echo "❌ some checks failed (see the ❌ lines above)"
echo "next: none - this script is the end of this test"
exit $fail
