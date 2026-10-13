#!/usr/bin/env bash
# Tamper, Auditor - registers the value I built, and also asks the Vendor's enclave directly to see whether it matches.
#    If the Vendor launched a different image, "✗ the Vendor's enclave runs something else" is printed, and the value I
#    verified is still registered normally. The Auditor instance must already have its own build value
#    (out/measurements.json), i.e. flow 1's 1-Auditor-build-audit-handoff must have been run first.
#    Run: bash "e2e/flow2-vendor-self-registers/4-Auditor-tamper-audit-check-vendor.sh" [-c 0xCONTRACT]
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST; need_host AUDITOR_HOST
REGISTRY=""
while getopts "c:" o; do case $o in c) REGISTRY=$OPTARG;; esac; done  # Parse options

# Fetch the Auditor's own build value from the Auditor instance.
step "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
echo "Auditor's own value:"; cat "$WORK/auditor-measured.json"

if [ -z "$REGISTRY" ]; then
  # att command - deploys a new registry (the Registry contract) on the chain.
  step "DEPLOYING REGISTRY"
  out=$("$AUDITOR_BIN" registry-deploy --rpc "$RPC" --deployer-key-file "$DEPLOYER_KEY" --approver "$AUDITOR_ADDR" 2>&1); echo "$out"
  REGISTRY=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # extract the address from the output
  [ -n "$REGISTRY" ] || { echo "no registry address in registry-deploy output"; exit 1; }
fi
echo "$REGISTRY" > "$WORK/registry.txt"  # save it in the file the next step reads

# att command - registers my value and, with --vendor-tee, also sends an RA request to the Vendor enclave to compare directly.
#   This is the key command that catches the swap by measurement.
step "AUDITING (checking Vendor's enclave)"
"$AUDITOR_BIN" audit --tee-addr "$AUDITOR_HOST:5000" --build-measurements "$WORK/auditor-measured.json" --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$AUDITOR_SIGNING_KEY" --service "$SERVICE" --version "$VERSION" --vendor-tee "$VENDOR_HOST:5000"
rc=$?
echo "audit exit=$rc   registry=$REGISTRY   next: 5-User-tamper-verify-both-registries"
exit $rc
