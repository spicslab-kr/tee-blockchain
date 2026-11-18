#!/usr/bin/env bash
# CVM flow 1, Auditor, with both guests up - audit --vendor-tee: the Auditor's guest attests its own build, and the
#    Vendor's guest is asked what it runs. Expected: "same: the Vendor's guest runs exactly what I built", exit 0.
#    (A8 of "The Auditor's half" in docs/build/tdx.md and sev-snp.md.)
#
#    audit registers the Auditor's value as part of the run, and the value from step 1 is already registered on step 1's
#    registry, where a second audit would stop at exit 5 ("already registered"). So this step deploys a registry of its
#    own by default; -c 0xREGISTRY uses a given one, which must not hold this value yet. Step 1's registry, the one the
#    verify command names, is not touched.
#
#    Run from the repository root, after step 2:
#      bash e2e/cvm/flow1-auditor-registers/2b-Auditor-audit-vendor.sh <format> [option] [-c 0xREGISTRY]
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
echo "$REGISTRY" > "$CVM_WORK/registry-2b.txt"

tstep "AUDITING MY GUEST AND ASKING THE VENDOR'S" "$AUDITOR_BIN" audit --tee-addr "$AUDITOR_IP:$RA_PORT" --format "$FORMAT" \
  --build-measurements "$CVM_WORK/auditor-measurements.json" --vendor-tee "$VENDOR_IP:$RA_PORT" \
  --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$AUDITOR_SIGNING_KEY" \
  --service "$CVM_SERVICE" --version "$CVM_VERSION" --arch amd64
rc=$?
echo "audit exit=$rc   registry=$REGISTRY"
if [ $rc -eq 0 ]; then
  result T4 audit-vendor-tee-same accepted accepted "audit --vendor-tee: the Vendor's guest runs what the Auditor built (exit 0)" "option $OPTION, registry $REGISTRY"
else
  result T4 audit-vendor-tee-same rejected accepted "audit --vendor-tee exit $rc, want 0" "option $OPTION, registry $REGISTRY"
  exit $rc
fi
echo "next: 4-Vendor-tamper-rebuild-launch.sh ${args[*]}"
