#!/usr/bin/env bash
# Replay against a live enclave (test matrix T9). Uses no chain and no key: two fresh attestation documents are
#    requested from the running enclave, each is checked against its own nonce, and then document 1 is presented
#    against the nonce of request 2. That replay must be refused with exit code 6 (nonce), and the two documents must
#    differ (fresh evidence each time, not a cached one).
#
#    Needs an enclave answering on VENDOR_HOST:5000, for example after flow1-auditor-registers/2-Vendor-launch-check-
#    record.sh, or after `att-vendor launch` on the Vendor instance. 0-Setup-local-upload.sh builds the local att-user.
#    Run from the repository root:  bash "e2e/replay-check.sh"
#    Another TEE: TEE_ADDR=<host:port> RESULTS_PLATFORM=<format> bash e2e/replay-check.sh
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

FMT=$RESULTS_PLATFORM
if [ -z "${TEE_ADDR:-}" ]; then need_host VENDOR_HOST; TEE_ADDR="$VENDOR_HOST:5000"; fi
[ -x "$USER_BIN" ] || { echo "$USER_BIN is missing - run bash e2e/0-Setup-local-upload.sh first"; exit 1; }
D="$WORK/replay"; rm -rf "$D"; mkdir -p "$D"
fail=0

step "1. TWO FRESH REQUESTS TO $TEE_ADDR"
"$USER_BIN" ra-request --tee-addr "$TEE_ADDR" --format "$FMT" --out-file "$D/a.doc" || { result T9 request-answered error refused "request 1 failed"; exit 1; }
"$USER_BIN" ra-request --tee-addr "$TEE_ADDR" --format "$FMT" --out-file "$D/b.doc" || { result T9 request-answered error refused "request 2 failed"; exit 1; }

step "2. EACH DOCUMENT VERIFIES AGAINST ITS OWN NONCE"
for d in a b; do
  "$USER_BIN" ra-parse --doc "$D/$d.doc" --format "$FMT" --out-file "$D/$d.json" > "$D/parse-$d.txt" 2>&1; rc=$?
  cat "$D/parse-$d.txt" | sed 's/^/    /'
  if [ $rc -eq 0 ] && grep -q "nonce      ok" "$D/parse-$d.txt"; then
    echo "✅ document $d: verifies, nonce checked"
    result T9 "own-nonce-$d" accepted accepted "document $d verifies and carries the nonce sent with it"
  else
    echo "❌ document $d: exit $rc, or the nonce check did not run"
    result T9 "own-nonce-$d" rejected accepted "document $d: ra-parse exit $rc, or no 'nonce ok' line"
    fail=1
  fi
done

step "3. THE TWO DOCUMENTS DIFFER"
if cmp -s "$D/a.doc" "$D/b.doc"; then
  echo "❌ the two documents are byte-identical - the enclave did not make fresh evidence"
  result T9 two-documents-differ identical different "two requests returned byte-identical documents"
  fail=1
else
  echo "✅ the two documents differ"
  result T9 two-documents-differ different different "two requests returned different documents"
fi

step "4. DOCUMENT 1 PRESENTED AGAINST REQUEST 2'S NONCE - MUST BE REFUSED (exit 6)"
"$USER_BIN" ra-parse --doc "$D/a.doc" --format "$FMT" --nonce "$D/b.doc.nonce" --out-file "$D/replay.json" > "$D/replay.txt" 2>&1; rc=$?
sed 's/^/    /' "$D/replay.txt"
if [ $rc -eq 6 ]; then
  echo "✅ replay refused (exit 6, nonce)"
  result T9 replay-refused refused refused "document 1 against request 2's nonce: refused, exit 6"
else
  echo "❌ replay not refused as a nonce mismatch (exit $rc, want 6)"
  result T9 replay-refused "$(verdict_of $rc)" refused "document 1 against request 2's nonce: exit $rc, want 6"
  fail=1
fi

echo
echo "===================== replay check ====================="
[ $fail = 0 ] && echo "✅ all passed" || echo "❌ some checks failed (see the ❌ lines above)"
exit $fail
