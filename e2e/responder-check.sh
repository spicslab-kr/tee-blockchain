#!/usr/bin/env bash
# Responder build-info check. The enclave image measurement covers the att-ra-responder file that sits next to att, and
#    that file's bytes depend on the Go patch version, on -trimpath, on -buildvcs, on CGO_ENABLED and on GOOS/GOARCH. The toolchain-pin
#    check (toolchain-pin-check.sh) covers the Go version that built att itself. At the start of build and pack, att reads
#    the build info Go left inside the responder binary (the output of go version -m) and refuses when the toolchain
#    differs from the pin, -trimpath is missing, version control was stamped in, cgo was linked, the OS is not linux, or the architecture
#    differs from this instance. This script checks that the refusal happens on a real Nitro host:
#      1. cross-build four responders on your machine: a correct one (linux/amd64 -trimpath -buildvcs=false), one without
#         -trimpath, one with -buildvcs=true, one for arm64 (all with CGO_ENABLED=0, so each has exactly one fault; the
#         cgo refusal needs a native Linux build with gcc and is covered by TestReadResponder_NativeBuildLinksCgo instead)
#      2. upload them to the Vendor host under different names and run `build --responder <file>` with each
#      3. the one without -trimpath is refused with "without -trimpath", the stamped one with "built with version control
#         stamping", the arm64 one with "GOARCH=arm64 ... this instance is amd64" (exit != 0), and the correct one passes
#         the responder check and the build log gets one line "responder: att-ra-responder sha256 ... -trimpath" (the build
#         may fail later for another reason; only the responder line is checked)
#    Each verdict is also written as a results line (env.sh, result()).
#    Run from the repository root:  bash "e2e/responder-check.sh"
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
  result - local-go-equals-pin identical identical "local go version = $HAVE = pin file"
else
  echo "❌ local go version is $HAVE - the pin file says $WANT. Fix it with bash e2e/0-Setup-local-install.sh."
  result - local-go-equals-pin different identical "local go version is $HAVE - the pin file says $WANT"
  fail=1
fi

mkdir -p "$WORK/responder"
step "2. CROSS-BUILDING FOUR RESPONDERS (correct, without -trimpath, with VCS stamping, arm64) AND att-vendor"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o "$WORK/responder/att-ra-responder-ok"        ./cmd/ra-responder
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -o "$WORK/responder/att-ra-responder-notrimpath" ./cmd/ra-responder
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=true -o "$WORK/responder/att-ra-responder-vcs"        ./cmd/ra-responder
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -o "$WORK/responder/att-ra-responder-arm64"     ./cmd/ra-responder
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o "$WORK/responder/att-vendor" ./cmd/vendor
OK_SHA="$(sha256 "$WORK/responder/att-ra-responder-ok" | cut -c1-16)"
echo "correct responder sha256 (first 16): $OK_SHA - the host's build log must print the same value"

step "3. UPLOADING TO THE VENDOR HOST"
nitroBanner "Vendor"
$SCP "$WORK/responder/att-vendor" "$WORK/responder/att-ra-responder-ok" "$WORK/responder/att-ra-responder-notrimpath" \
     "$WORK/responder/att-ra-responder-vcs" "$WORK/responder/att-ra-responder-arm64" "$SSH_USER@$VENDOR_HOST:~/"

# remote_build <responder file> - run `build` with that responder on the Vendor host and return stderr+stdout. The exit code is carried in an __EXIT= line.
remote_build() {
  V "chmod +x att-vendor $1; mkdir -p /tmp/rc-app; printf 'FROM scratch\n' > /tmp/rc-app/Dockerfile; ./att-vendor build --app-dir /tmp/rc-app --out-dir /tmp/rc-out --responder $1 2>&1; echo \"__EXIT=\$?\""
}
exit_of() { echo "$1" | sed -n 's/^__EXIT=//p' | tail -1; }

step "4. build WITH att-ra-responder-notrimpath - MUST BE REFUSED, NAMING -trimpath"
out="$(remote_build att-ra-responder-notrimpath)"
echo "$out" | sed 's/^/    /'
ec="$(exit_of "$out")"
if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "without -trimpath"; then
  echo "✅ refused (exit $ec) - 'att-ra-responder was built without -trimpath'"
  result T3 notrimpath-responder-refused refused refused "build refused (exit $ec): att-ra-responder was built without -trimpath"
else
  echo "❌ a responder built without -trimpath was not refused (exit ${ec:-?}) - CheckResponder is not running at the start of build"
  result T3 notrimpath-responder-refused accepted refused "a responder built without -trimpath was not refused (exit ${ec:-?})"
  fail=1
fi

step "5. build WITH att-ra-responder-vcs - MUST BE REFUSED, NAMING -buildvcs=false"
out="$(remote_build att-ra-responder-vcs)"
echo "$out" | sed 's/^/    /'
ec="$(exit_of "$out")"
if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "built with version control stamping"; then
  echo "✅ refused (exit $ec) - 'built with version control stamping'"
  result T3 vcs-stamped-responder-refused refused refused "build refused (exit $ec): att-ra-responder was built with version control stamping"
else
  echo "❌ a responder with VCS stamping was not refused (exit ${ec:-?})"
  result T3 vcs-stamped-responder-refused accepted refused "a responder built with -buildvcs=true was not refused (exit ${ec:-?})"
  fail=1
fi

step "6. build WITH att-ra-responder-arm64 ON AN amd64 HOST - MUST BE REFUSED, NAMING BOTH ARCHITECTURES"
out="$(remote_build att-ra-responder-arm64)"
echo "$out" | sed 's/^/    /'
ec="$(exit_of "$out")"
if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "GOARCH=arm64" && echo "$out" | grep -q "this instance is amd64"; then
  echo "✅ refused (exit $ec) - 'built for GOARCH=arm64 but this instance is amd64'"
  result - arm64-responder-refused-on-amd64 refused refused "build refused (exit $ec): built for GOARCH=arm64 but this instance is amd64"
else
  echo "❌ an arm64 responder was not refused on an amd64 host (exit ${ec:-?})"
  result - arm64-responder-refused-on-amd64 accepted refused "an arm64 responder was not refused on an amd64 host (exit ${ec:-?})"
  fail=1
fi

step "7. build WITH att-ra-responder-ok - MUST PASS THE RESPONDER CHECK"
out="$(remote_build att-ra-responder-ok)"
echo "$out" | sed 's/^/    /'
if echo "$out" | grep -q "responder: att-ra-responder sha256 $OK_SHA .* -trimpath"; then
  echo "✅ responder check passed - the sha256 $OK_SHA in the build log equals the value measured on your machine (a later failure for another reason is not part of this test)"
  result - correct-responder-accepted accepted accepted "the responder check passed and the build log shows sha256 $OK_SHA"
else
  echo "❌ the correctly built responder did not pass the check, or the log has no 'responder: ... -trimpath' line"
  result - correct-responder-accepted refused accepted "the correctly built responder did not pass the check, or the log has no responder line"
  fail=1
fi
localBanner

echo
echo "===================== responder check ====================="
[ $fail = 0 ] && echo "✅ all passed" || echo "❌ some checks failed (see the ❌ lines above)"
echo "next: none - this script is the end of this test"
exit $fail
