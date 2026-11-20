#!/usr/bin/env bash
# CVM flow 2 tamper, Auditor - the CVM form of e2e/flow2-vendor-self-registers/4-Auditor-tamper-audit-check-vendor.sh.
#    audit --vendor-tee with the Auditor's own build: the Auditor's guest attests what the Auditor built, so that value
#    is registered on a registry whose approver is the Auditor; the Vendor's guest runs the changed application of step
#    3, so audit says so and exits 12 ("registered, but the Vendor's TEE runs something else"). Exit 12 is this step's
#    pass. Ends by printing the verify command for step 5, typed by hand, with both registries required: the Vendor's
#    (step 3) vouches, the Auditor's does not, so not trusted, exit 1.
#
#    The Auditor's value comes from flow 1: e2e/cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh builds the
#    application on the Auditor's guest, boots it and writes auditor-measurements.json, as Nitro's step 4 needs flow 1
#    step 1. Without that file this step stops and says so.
#
#    audit registers as part of the run, and on flow 1's registry the Auditor's value is already registered, where audit
#    stops at exit 5 before it can end in 12. So this step deploys a registry of its own by default, as 2b and 4b of flow 1
#    do; -c 0xREGISTRY uses a given one with the Auditor as approver, which must not hold this value yet.
#
#    Run from the repository root, after step 3:
#      bash e2e/cvm/flow2-vendor-self-registers/4-Auditor-audit-vendor.sh <format> [option] [-c 0xREGISTRY]
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../../.."
. "$here/../../env.sh"
REGISTRY=""
args=()
while [ $# -gt 0 ]; do case "$1" in -c) REGISTRY=${2:-}; shift 2 ;; *) args+=("$1"); shift ;; esac; done
. "$here/../env-cvm.sh" "${args[@]}"
if [ ! -f "$CVM_WORK/auditor-measurements.json" ]; then
  echo "no Auditor's own build value in $CVM_WORK (auditor-measurements.json). The Auditor audits with the value it" >&2
  echo "built and booted on its own guest; run flow 1's first step, with the same arguments, then this step again:" >&2
  echo "  bash e2e/cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh ${args[*]}" >&2
  exit 1
fi
VREG=$(cat "$CVM_WORK/flow2-vendor-registry-tampered.txt" 2>/dev/null)
[ -n "$VREG" ] || { echo "no registry from step 3 in $CVM_WORK: run 3-Vendor-tamper-rebuild-self-register.sh ${args[*]} first" >&2; exit 1; }
need_addr AUDITOR_ADDR
need_bins
AUDITOR_IP=$(guest_ip "$AUDITOR_VM"); VENDOR_IP=$(guest_ip "$VENDOR_VM")
[ -n "$AUDITOR_IP" ] && [ -n "$VENDOR_IP" ] || { echo "a guest has no public address" >&2; exit 1; }

if [ -z "$REGISTRY" ]; then
  # att-auditor registry-deploy - the Auditor's registry, a contract of its own with the Auditor as its approver.
  step_start "DEPLOYING THE AUDITOR'S REGISTRY ($NET)"
  REGISTRY=$(deploy_registry "$AUDITOR_BIN" "$AUDITOR_ADDR") || exit 1
  step_stop
fi
echo "$REGISTRY" > "$CVM_WORK/flow2-auditor-registry.txt"

tstep "AUDITING MY GUEST AND ASKING THE VENDOR'S" "$AUDITOR_BIN" audit --tee-addr "$AUDITOR_IP:$RA_PORT" --format "$FORMAT" \
  --build-measurements "$CVM_WORK/auditor-measurements.json" --vendor-tee "$VENDOR_IP:$RA_PORT" \
  --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$AUDITOR_SIGNING_KEY" \
  --service "$CVM_SERVICE" --version "$CVM_VERSION" --arch amd64
rc=$?
echo "audit exit=$rc   the Auditor's registry=$REGISTRY"
if [ $rc -eq 0 ]; then
  echo "the Vendor's guest runs what the Auditor built: run step 3 first, so that it runs the changed application" >&2
  result T6 auditor-sees-vendor-swap accepted rejected "audit --vendor-tee exit 0: the Vendor's guest runs what the Auditor built" "option $OPTION, registry $REGISTRY"
  exit 1
elif [ $rc -ne 12 ]; then
  result T6 auditor-sees-vendor-swap error rejected "audit --vendor-tee exit $rc, want 12" "option $OPTION, registry $REGISTRY"
  exit 1
fi
echo "as expected: the Auditor's value is registered, and the Vendor's guest runs something else (exit 12)"
result T6 auditor-sees-vendor-swap rejected rejected "audit --vendor-tee exit 12: registered; the Vendor's guest runs something else" "option $OPTION, registry $REGISTRY"
echo
echo "the Vendor's registry (step 3):   $VREG"
echo "the Auditor's registry (this step): $REGISTRY"
print_verify_line "$VENDOR_IP" "not trusted, exit 1: only 1 of 2 registries vouch" "$VREG" "$REGISTRY"
echo "next: the verify above (step 5). Step 1 of this flow, or step 2 of flow 1, puts the original application back."
