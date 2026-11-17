#!/usr/bin/env bash
# Go toolchain pin check. The bytes of att-ra-responder change with the Go patch version (measured: go1.26.1 and
#    go1.26.2 give different binaries even with -trimpath), and so do the measurements (PCR0/2). The pinned version has
#    one source, the file internal/setup/build/GO_TOOLCHAIN in the source tree; att embeds that file and at the start of
#    build compares it with the toolchain it was built with, refusing if they differ. This script checks that the
#    refusal really happens on a real Nitro host:
#      1. your machine's go version equals the pin file exactly
#      2. an att-vendor-wrongtc cross-built with another patch version (GOTOOLCHAIN=$WRONG) is uploaded to the
#         Vendor host; running `build` gives exit != 0 and the output contains "pins <pinned version>"
#      3. a correctly built att-vendor passes that check (the build may fail later for another reason; only the toolchain message is checked)
#    Step 2 is skipped with a notice when the other toolchain cannot be downloaded (offline).
#    Run from the repository root:  bash "e2e/toolchain-pin-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST
PIN_FILE=internal/setup/build/GO_TOOLCHAIN
WANT="$(tr -d '[:space:]' < "$PIN_FILE")"
WRONG=go1.26.2   # any patch version other than the pin works; go.mod requires go 1.26.1 or newer, so choose a newer one
fail=0

step "1. LOCAL GO VERSION vs $PIN_FILE"
HAVE="$(go version | awk '{print $3}')"
if [ "$HAVE" = "$WANT" ]; then
  echo "✅ local go version = $HAVE = pin file ($PIN_FILE)"
  result - local-go-equals-pin identical identical "local go version = $HAVE = pin file"
else
  echo "❌ local go version is $HAVE - the pin file says $WANT. Fix it with bash e2e/0-Setup-local-install.sh."
  result - local-go-equals-pin different identical "local go version is $HAVE - the pin file says $WANT"
  fail=1
fi

mkdir -p "$WORK/toolchain"
step "2. CROSS-BUILDING att-vendor WITH THE WRONG TOOLCHAIN ($WRONG)"
if GOTOOLCHAIN=$WRONG go version 2>/dev/null | grep -q "$WRONG"; then
  GOTOOLCHAIN=$WRONG CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$WORK/toolchain/att-vendor-wrongtc" ./cmd/vendor
  HAVE_WRONG=1
else
  echo "WARNING: cannot download GOTOOLCHAIN=$WRONG (offline?) - skipping the live refusal check. The unit tests (go test ./internal/setup/build ./cmd -run Toolchain) cover the same path."
  result T2 wrong-toolchain-refused error refused "skipped: GOTOOLCHAIN=$WRONG could not be downloaded"
  HAVE_WRONG=0
fi
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$WORK/toolchain/att-vendor" ./cmd/vendor

# 2b. The fact the refusal protects against (test matrix T2, footnote 2): the same responder source built with another
#     Go patch version gives other bytes, so the image measurement would differ. Checked on the binary, locally.
if [ $HAVE_WRONG = 1 ]; then
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$WORK/toolchain/att-ra-responder-pinned" ./cmd/ra-responder
  GOTOOLCHAIN=$WRONG CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$WORK/toolchain/att-ra-responder-wrongtc" ./cmd/ra-responder
  a=$(sha256 "$WORK/toolchain/att-ra-responder-pinned" | cut -c1-64); b=$(sha256 "$WORK/toolchain/att-ra-responder-wrongtc" | cut -c1-64)
  if [ "$a" != "$b" ]; then
    echo "✅ the responder built with $WRONG differs from the one built with $WANT ($a vs $b)"
    result T2 responder-bytes-differ different different "att-ra-responder built with $WRONG differs from $WANT" "$WANT $a; $WRONG $b"
  else
    echo "❌ the responder is byte-identical under $WRONG and $WANT ($a)"
    result T2 responder-bytes-differ identical different "att-ra-responder is byte-identical under $WRONG and $WANT" "$a"
    fail=1
  fi
fi

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
    result T2 wrong-toolchain-refused refused refused "build refused (exit $ec): built with $WRONG but the project pins $WANT"
  else
    echo "❌ an att-vendor built with another toolchain was not refused (exit ${ec:-?}) - CheckToolchain is not running at the start of build"
    result T2 wrong-toolchain-refused accepted refused "an att-vendor built with $WRONG was not refused (exit ${ec:-?})"
    fail=1
  fi
fi

step "5. build WITH THE CORRECTLY BUILT att-vendor ($WANT) - MUST PASS THE TOOLCHAIN CHECK"
out="$(remote_build att-vendor)"
echo "$out" | sed 's/^/    /'
if echo "$out" | grep -q "pins $WANT"; then
  echo "❌ an att-vendor built with the pinned toolchain was refused by the toolchain check - GO_TOOLCHAIN and the real build toolchain disagree"
  result T2 pinned-toolchain-accepted refused accepted "an att-vendor built with $WANT was refused by the toolchain check"
  fail=1
else
  echo "✅ toolchain check passed (a later failure for another reason is not part of this test)"
  result T2 pinned-toolchain-accepted accepted accepted "an att-vendor built with $WANT passed the toolchain check"
fi
localBanner

echo
echo "===================== toolchain pin ====================="
[ $fail = 0 ] && echo "✅ all passed" || echo "❌ some checks failed (see the ❌ lines above)"
echo "next: none - this script is the end of this test"
exit $fail
