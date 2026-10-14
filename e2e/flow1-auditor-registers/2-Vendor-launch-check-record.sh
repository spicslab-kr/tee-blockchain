#!/usr/bin/env bash
# Flow 1, Vendor - launches the image the Auditor handed over and checks that my enclave runs the value the Auditor measured
#    and registered. Run on my machine; only the launch runs inside the Vendor instance, which the script connects to.
#    Input: from-auditor/alpine-slim.eif and from-auditor/measurements.json on the Vendor instance, and
#    e2e-work/from-auditor/measurement-record.json here (1-Auditor-build-audit-handoff.sh puts them there)
#    Run from the repository root:  bash "e2e/flow1-auditor-registers/2-Vendor-launch-check-record.sh"
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
sha256sum from-auditor/alpine-slim.eif from-auditor/measurements.json  # checksums of the received files
step_stop "CHECKING RECEIVED FILES"

# att-vendor launch - measures the received image and launches it in my TEE when the result equals the measurements the Auditor handed over.
step "LAUNCHING ENCLAVE" ./att-vendor launch --eif from-auditor/alpine-slim.eif --build-measurements from-auditor/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# Read the registration coordinates (RPC URL, registry, record key) from the measurement record.
REC="$WORK/from-auditor/measurement-record.json"
field() { grep -oE "\"$1\": *\"[^\"]*\"" "$REC" | head -1 | sed -E 's/.*": *"([^"]*)"$/\1/'; }
R_RPC=$(field rpc); R_REGISTRY=$(field address); R_RECORD_KEY=$(field record_key)
mkdir -p "$WORK/vendor-check"

# att-vendor ra-request - requests an attestation document from my enclave.
step "REQUESTING ATTESTATION" "$VENDOR_BIN" ra-request --tee-addr "$VENDOR_HOST:5000" --out-file "$WORK/vendor-check/doc.cbor" --format aws-nitro || exit 1
# att-vendor ra-parse - verifies the document and extracts its measurements.
step "PARSING ATTESTATION" "$VENDOR_BIN" ra-parse --doc "$WORK/vendor-check/doc.cbor" --out-file "$WORK/vendor-check/parsed.json" || exit 1
# att-vendor compare - matches my enclave's value against the measurements the Auditor handed over.
step "COMPARING TO AUDITOR'S RECORD" "$VENDOR_BIN" compare --attested "$WORK/vendor-check/parsed.json" --expected "$WORK/from-auditor/measurements.json"; a=$?
# att-vendor compare - matches my enclave's value against the registration on the registry (value, expiry, approver).
step "COMPARING TO CHAIN REGISTRATION" "$VENDOR_BIN" compare --attested "$WORK/vendor-check/parsed.json" --rpc "$R_RPC" --registry "$R_REGISTRY" --record-key "$R_RECORD_KEY"; b=$?
if [ $a -eq 0 ] && [ $b -eq 0 ]; then
  echo "OK — what I run is exactly what the Auditor audited and registered. Users will get 'trusted'."
else
  echo "MISMATCH — what I run differs from what the Auditor registered. Users will be told not to trust it."
  exit 1
fi
echo "next: 3-User-verify"
