#!/usr/bin/env bash
# Vendor - launches the image the Auditor handed over as is, and checks for itself that my enclave really matches both that
#    value and the registered value. Run here, on my machine - only the launch runs inside the Vendor instance, connected to
#    automatically.
#    Input: from-auditor/alpine-slim.eif, from-auditor/measurements.json (1-Auditor-build-audit-handoff uploads them to that instance)
#    Run: bash "e2e/flow1-auditor-registers/2-Vendor-launch-check-record.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST

nitroBanner "Vendor"
V 'bash -s' <<'REMOTE'
set -euo pipefail
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; shift; "$@"; }
test -f from-auditor/alpine-slim.eif -a -f from-auditor/measurements.json || { echo "from-auditor/alpine-slim.eif and measurements.json are missing - the Auditor runs 1-Auditor-build-audit-handoff first"; exit 1; }

step_start "CHECKING RECEIVED FILES"
sha256sum from-auditor/alpine-slim.eif from-auditor/measurements.json  # hashes to check the received files
step_stop "CHECKING RECEIVED FILES"

# tb command - measures the received image itself and launches it in my TEE only if it equals the measurements the Auditor handed over.
step "LAUNCHING ENCLAVE" ./tb-vendor launch --eif from-auditor/alpine-slim.eif --measurements from-auditor/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# Read the registration coordinates the Auditor left (which contract, which key) from that record file.
REC="$WORK/from-auditor/measurement-record.json"
field() { grep -oE "\"$1\": *\"[^\"]*\"" "$REC" | head -1 | sed -E 's/.*": *"([^"]*)"$/\1/'; }
R_RPC=$(field rpc); R_CONTRACT=$(field contract); R_KEY=$(field key)
mkdir -p "$WORK/vendor-check"

# tb command - sends an RA request to my enclave and receives the attestation document.
step "REQUESTING ATTESTATION" "$TBV" ra-request --tee "$VENDOR_HOST:5000" --out "$WORK/vendor-check/doc.cbor" --format aws-nitro || exit 1
# tb command - verifies that document and extracts the measurements inside.
step "PARSING ATTESTATION" "$TBV" ra-parse --doc "$WORK/vendor-check/doc.cbor" --out "$WORK/vendor-check/parsed.json" || exit 1
# tb command - compares my enclave's value with the value in the record the Auditor gave.
step "COMPARING TO AUDITOR'S RECORD" "$TBV" compare --a "$WORK/vendor-check/parsed.json" --b "$WORK/from-auditor/measurements.json"; a=$?
# tb command - compares my enclave's value with the actual chain registration (value, expiry, signer).
step "COMPARING TO CHAIN REGISTRATION" "$TBV" compare --a "$WORK/vendor-check/parsed.json" --rpc "$R_RPC" --contract "$R_CONTRACT" --key "$R_KEY"; b=$?
if [ $a -eq 0 ] && [ $b -eq 0 ]; then
  echo "OK — what I run is exactly what the Auditor audited and registered. Users will get 'trusted'."
else
  echo "MISMATCH — what I run is not what the Auditor registered. Users will be told not to trust it."
  exit 1
fi
echo "next: 3-User-verify"
