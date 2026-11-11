#!/usr/bin/env bash
# Debug mode (test matrix T8), Nitro. A Nitro enclave started with --debug-mode signs attestation documents whose PCRs
#    are all zero; tee/parse/aws treats an all-zero PCR0 as debug and refuses a document whose PCRs are all zero, so the
#    User never compares meaningless values. TDX and SEV-SNP prove this from a recording with one bit flipped; a Nitro
#    document is signed over its PCRs, so the debug case has to be recorded from a real debug enclave. This script does
#    that and keeps the document as a future fixture:
#      1. on the Vendor host, build the test application (as the flow scripts do) and launch it normally with att-vendor
#         launch; request one document and check it verifies with debug=false (the control)
#      2. terminate it and start the same EIF with nitro-cli run-enclave --debug-mode on the same CID, so the TCP bridge
#         att-vendor launch left running reaches it
#      3. request one document; ra-parse must refuse it (exit 7, "every PCR is zero")
#      4. terminate the debug enclave and relaunch the normal one, leaving the host as step 1 found it
#    The two documents are saved under e2e-work/debug-mode/ (normal.doc, debug.doc, with their .nonce files). No chain,
#    no key. Run from the repository root:  bash "e2e/debug-mode-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST
[ -x "$USER_BIN" ] || { echo "$USER_BIN is missing - run bash e2e/0-Setup-local-upload.sh first"; exit 1; }
SRC_REPO=https://github.com/nginx/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim
D="$WORK/debug-mode"; rm -rf "$D"; mkdir -p "$D"
fail=0

step "1. BUILDING AND LAUNCHING THE TEST APPLICATION NORMALLY (Vendor)"
nitroBanner "Vendor"
V 'bash -s' <<REMOTE
set -euo pipefail
umask 022
fix_modes() { (cd "\$1" && git ls-files -s | while read -r mode _ _ path; do case \$mode in 100755) chmod 755 "\$path";; 100644) chmod 644 "\$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
[ -f out-debug/alpine-slim.eif ] || ./att-vendor build --app-dir src/docker-nginx/$SRC_DIR --out-dir out-debug
./att-vendor launch --eif out-debug/alpine-slim.eif --out-dir out-debug
REMOTE
rc=$?
localBanner
[ $rc -eq 0 ] || { result T8 normal-launch error refused "building or launching the test application failed (exit $rc)"; exit 1; }
sleep 5
"$USER_BIN" ra-request --tee-addr "$VENDOR_HOST:5000" --format aws-nitro --out-file "$D/normal.doc" || { result T8 normal-request error refused "request to the normal enclave failed"; exit 1; }
"$USER_BIN" ra-parse --doc "$D/normal.doc" --format aws-nitro --out-file "$D/normal.json" > "$D/normal.txt" 2>&1; rc=$?
sed 's/^/    /' "$D/normal.txt"
if [ $rc -eq 0 ] && grep -q "debug      false" "$D/normal.txt"; then
  echo "✅ control: the normal enclave's document verifies, debug=false"
  result - debug-control-normal-accepted accepted accepted "the normal enclave's document verifies with debug=false"
else
  echo "❌ control failed: ra-parse exit $rc on the normal enclave's document"
  result - debug-control-normal-accepted "$(verdict_of $rc)" accepted "the normal enclave's document: ra-parse exit $rc"
  fail=1
fi

step "2. RESTARTING THE SAME EIF WITH --debug-mode (Vendor)"
nitroBanner "Vendor"
V 'bash -s' <<'REMOTE'
set -uo pipefail
mem=$(sudo nitro-cli describe-enclaves | grep -oE '"MemoryMiB": *[0-9]+' | grep -oE '[0-9]+$' | head -1)
cpus=$(sudo nitro-cli describe-enclaves | grep -oE '"NumberOfCPUs": *[0-9]+' | grep -oE '[0-9]+$' | head -1)
sudo nitro-cli terminate-enclave --all >/dev/null
sudo nitro-cli run-enclave --eif-path out-debug/alpine-slim.eif --cpu-count "${cpus:-2}" --memory "${mem:-1024}" \
  --enclave-cid 16 --enclave-name alpine-slim-debug --debug-mode
REMOTE
rc=$?
localBanner
[ $rc -eq 0 ] || { result T8 debug-launch error refused "nitro-cli run-enclave --debug-mode failed (exit $rc)"; exit 1; }
sleep 10

step "3. REQUESTING A DOCUMENT FROM THE DEBUG ENCLAVE - ra-parse MUST REFUSE IT"
if "$USER_BIN" ra-request --tee-addr "$VENDOR_HOST:5000" --format aws-nitro --out-file "$D/debug.doc"; then
  "$USER_BIN" ra-parse --doc "$D/debug.doc" --format aws-nitro --out-file "$D/debug.json" > "$D/debug.txt" 2>&1; rc=$?
  sed 's/^/    /' "$D/debug.txt"
  if [ $rc -eq 7 ] && grep -q "every PCR is zero" "$D/debug.txt"; then
    echo "✅ the debug enclave's document is refused (exit 7, every PCR is zero)"
    result T8 debug-document-refused refused refused "debug-mode document refused by ra-parse: exit 7, every PCR is zero" "saved as e2e-work/debug-mode/debug.doc"
  else
    echo "❌ the debug enclave's document was not refused as expected (exit $rc, want 7)"
    result T8 debug-document-refused "$(verdict_of $rc)" refused "debug-mode document: ra-parse exit $rc, want 7" "saved as e2e-work/debug-mode/debug.doc"
    fail=1
  fi
else
  echo "❌ no answer from the debug enclave (does the bridge still run? is the responder up in debug mode?)"
  result T8 debug-document-refused error refused "no answer from the debug enclave"
  fail=1
fi

step "4. PUTTING THE NORMAL ENCLAVE BACK (Vendor)"
nitroBanner "Vendor"
V 'sudo nitro-cli terminate-enclave --all >/dev/null; ./att-vendor launch --eif out-debug/alpine-slim.eif --out-dir out-debug >/dev/null && echo "normal enclave relaunched"'
localBanner

echo
echo "===================== debug mode check ====================="
[ $fail = 0 ] && echo "✅ all passed - keep e2e-work/debug-mode/debug.doc as the Nitro debug fixture" || echo "❌ some checks failed (see the ❌ lines above)"
exit $fail
