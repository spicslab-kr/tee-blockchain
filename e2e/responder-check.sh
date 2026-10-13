#!/usr/bin/env bash
# Responder build-info check. What gets measured inside the enclave image is not tb but the tb-ra-responder file next
#    to it, and that file's bytes are determined by the Go patch version it was built with, whether -trimpath was used,
#    and GOOS/GOARCH. The toolchain-pin check (toolchain-pin-check.sh) only looks at which Go built tb itself, so a
#    responder file produced by some other go build passes it. So at the start of build/pack, tb reads the build info Go
#    left inside the responder binary (the same thing go version -m shows) and refuses if: it is not the pinned
#    toolchain, -trimpath is missing, it is not linux, or the architecture differs from this instance. This script checks
#    that the refusal really happens on a real Nitro host:
#      1. cross-build three responders on your machine - a correct one (linux/amd64 -trimpath), one without -trimpath, one for arm64
#      2. upload the three to the Vendor host under different names and run `build --responder <file>` with each
#      3. the one without -trimpath is refused with "without -trimpath", the arm64 one with "GOARCH=arm64 ... this instance is amd64"
#         (exit != 0), and the correct one passes the responder check and the build log gets one line
#         "responder: tb-ra-responder sha256 ... -trimpath" (it may fail later for another reason - only the responder line is checked)
#    Run from TB-live:  bash "e2e/responder-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST
PIN_FILE=internal/setup/build/GO_TOOLCHAIN
WANT="$(tr -d '[:space:]' < "$PIN_FILE")"
fail=0

step "1. LOCAL GO VERSION vs $PIN_FILE"
HAVE="$(go version | awk '{print $3}')"
if [ "$HAVE" = "$WANT" ]; then
  echo "✅ local go version = $HAVE = pin file ($PIN_FILE)"
else
  echo "❌ local go version is $HAVE - the pin file says $WANT. Fix it with bash e2e/0-Setup-local-install.sh."
  fail=1
fi

mkdir -p "$WORK/responder"
step "2. CROSS-BUILDING THREE RESPONDERS (correct, without -trimpath, arm64) AND tb-vendor"
GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/responder/tb-ra-responder-ok"        ./cmd/ra-responder
GOOS=linux GOARCH=amd64 go build           -o "$WORK/responder/tb-ra-responder-notrimpath" ./cmd/ra-responder
GOOS=linux GOARCH=arm64 go build -trimpath -o "$WORK/responder/tb-ra-responder-arm64"     ./cmd/ra-responder
GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/responder/tb-vendor" ./cmd/vendor
OK_SHA="$(sha256 "$WORK/responder/tb-ra-responder-ok" | cut -c1-16)"
echo "correct responder sha256 (first 16): $OK_SHA - the host's build log must print the same value"

step "3. UPLOADING TO THE VENDOR HOST"
nitroBanner "Vendor"
$SCP "$WORK/responder/tb-vendor" "$WORK/responder/tb-ra-responder-ok" "$WORK/responder/tb-ra-responder-notrimpath" \
     "$WORK/responder/tb-ra-responder-arm64" "$SSH_USER@$VENDOR_HOST:~/"

# remote_build <responder file> - run `build` with that responder on the Vendor host and return stderr+stdout. The exit code is carried in an __EXIT= line.
remote_build() {
  V "chmod +x tb-vendor $1; mkdir -p /tmp/rc-app; printf 'FROM scratch\n' > /tmp/rc-app/Dockerfile; ./tb-vendor build --app-dir /tmp/rc-app --out /tmp/rc-out --responder $1 2>&1; echo \"__EXIT=\$?\""
}
exit_of() { echo "$1" | sed -n 's/^__EXIT=//p' | tail -1; }

step "4. build WITH tb-ra-responder-notrimpath - MUST BE REFUSED, NAMING -trimpath"
out="$(remote_build tb-ra-responder-notrimpath)"
echo "$out" | sed 's/^/    /'
ec="$(exit_of "$out")"
if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "without -trimpath"; then
  echo "✅ refused (exit $ec) - 'tb-ra-responder was built without -trimpath'"
else
  echo "❌ a responder built without -trimpath was not refused (exit ${ec:-?}) - CheckResponder is not running at the start of build"
  fail=1
fi

step "5. build WITH tb-ra-responder-arm64 ON AN amd64 HOST - MUST BE REFUSED, NAMING BOTH ARCHITECTURES"
out="$(remote_build tb-ra-responder-arm64)"
echo "$out" | sed 's/^/    /'
ec="$(exit_of "$out")"
if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "GOARCH=arm64" && echo "$out" | grep -q "this instance is amd64"; then
  echo "✅ refused (exit $ec) - 'built for GOARCH=arm64 but this instance is amd64'"
else
  echo "❌ an arm64 responder was not refused on an amd64 host (exit ${ec:-?})"
  fail=1
fi

step "6. build WITH tb-ra-responder-ok - MUST PASS THE RESPONDER CHECK"
out="$(remote_build tb-ra-responder-ok)"
echo "$out" | sed 's/^/    /'
if echo "$out" | grep -q "responder: tb-ra-responder sha256 $OK_SHA .* -trimpath"; then
  echo "✅ responder check passed - the sha256 $OK_SHA in the build log equals the value measured on your machine (a later failure for another reason is outside this test)"
else
  echo "❌ the correctly built responder did not pass the check, or the log has no 'responder: ... -trimpath' line"
  fail=1
fi
localBanner

echo
echo "===================== responder check ====================="
[ $fail = 0 ] && echo "✅ all passed" || echo "❌ some checks failed (see the ❌ lines above)"
echo "next: none - this script is the end of this test"
exit $fail
