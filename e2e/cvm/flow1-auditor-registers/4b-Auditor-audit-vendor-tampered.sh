#!/usr/bin/env bash
# CVM flow 1 tamper, Auditor - audit --vendor-tee after the Vendor changed its application (step 4). The Auditor's own
#    guest still attests the Auditor's build, so its value is registered; the Vendor's guest runs something else, so audit
#    says so and exits 12 ("registered, but the Vendor's TEE runs something else"). Exit 12 is this step's pass.
#
#    As in 2b, audit registers as part of the run, so this step deploys a registry of its own by default (on step 1's
#    registry, or 2b's, the value is already registered and audit would stop at exit 5 before the comparison could end
#    in 12); -c 0xREGISTRY uses a given one, which must not hold this value yet.
#
#    Run from the repository root, after step 4:
#      bash e2e/cvm/flow1-auditor-registers/4b-Auditor-audit-vendor-tampered.sh <format> [option] [-c 0xREGISTRY]
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../../.."
. "$here/../../env.sh"
REGISTRY=""
args=()
while [ $# -gt 0 ]; do case "$1" in -c) REGISTRY=${2:-}; shift 2 ;; *) args+=("$1"); shift ;; esac; done
. "$here/../env-cvm.sh" "${args[@]}"
need_addr AUDITOR_ADDR
need_bins
[ -f "$CVM_WORK/auditor-measurements.json" ] || { echo "no Auditor values in $CVM_WORK: run step 1 first" >&2; exit 1; }
AUDITOR_IP=$(guest_ip "$AUDITOR_VM"); VENDOR_IP=$(guest_ip "$VENDOR_VM")
[ -n "$AUDITOR_IP" ] && [ -n "$VENDOR_IP" ] || { echo "a guest has no public address" >&2; exit 1; }

if [ -z "$REGISTRY" ]; then
  step_start "DEPLOYING A REGISTRY FOR THIS AUDIT ($NET)"
  REGISTRY=$(deploy_registry) || exit 1
  step_stop
fi
echo "$REGISTRY" > "$CVM_WORK/registry-4b.txt"

step "AUDITING MY GUEST AND ASKING THE VENDOR'S" "$AUDITOR_BIN" audit --tee-addr "$AUDITOR_IP:$RA_PORT" --format "$FORMAT" \
  --build-measurements "$CVM_WORK/auditor-measurements.json" --vendor-tee "$VENDOR_IP:$RA_PORT" \
  --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$AUDITOR_SIGNING_KEY" \
  --service "$CVM_SERVICE" --version "$CVM_VERSION" --arch amd64
rc=$?
echo "audit exit=$rc   registry=$REGISTRY"
if [ $rc -eq 12 ]; then
  echo "as expected: the Auditor's value is registered, and the Vendor's guest runs something else (exit 12)"
  result T5 audit-vendor-tee-after-change different different "audit --vendor-tee exit 12: registered; the Vendor's guest runs something else" "registry $REGISTRY"
  exit 0
fi
result T5 audit-vendor-tee-after-change error different "audit --vendor-tee exit $rc, want 12" "registry $REGISTRY"
exit 1
