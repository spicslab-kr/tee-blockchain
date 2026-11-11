#!/usr/bin/env bash
# Flow 2 tamper, Auditor - registers the value I built and also asks the Vendor's enclave which value it runs.
#    When the Vendor runs a different image, "✗ the Vendor's enclave runs something else" is printed, and my own value is
#    registered as usual. The Auditor instance needs its own build value (out/measurements.json), so
#    flow1-auditor-registers/1-Auditor-build-audit-handoff.sh must have run first.
#    Run from the repository root:  bash "e2e/flow2-vendor-self-registers/4-Auditor-tamper-audit-check-vendor.sh" [-c 0xREGISTRY]
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST; need_host AUDITOR_HOST; need_addr AUDITOR_ADDR
REGISTRY=""
while getopts "c:" o; do case $o in c) REGISTRY=$OPTARG;; esac; done  # Parse options

# Copy the Auditor's own build value from the Auditor instance.
step "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
echo "Auditor's own value:"; cat "$WORK/auditor-measured.json"

if [ -z "$REGISTRY" ]; then
  # att-auditor registry-deploy - deploys a new Registry contract with the Auditor as its approver.
  step "DEPLOYING REGISTRY"
  out=$("$AUDITOR_BIN" registry-deploy --rpc "$RPC" --deployer-key-file "$DEPLOYER_KEY" --approver "$AUDITOR_ADDR" 2>&1); echo "$out"
  REGISTRY=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # address from the 'deployed at' line
  [ -n "$REGISTRY" ] || { echo "no registry address in registry-deploy output"; exit 1; }
fi
echo "$REGISTRY" > "$WORK/registry.txt"  # the later steps read the registry address from this file

# att-auditor audit - registers my value; with --vendor-tee it also requests an attestation document from the Vendor's
#   enclave and compares it with my value. That comparison reports the swap.
step "AUDITING (checking Vendor's enclave)"
#   audit exits 0 either way (it registers the Auditor's own value); the swap shows only in its output, so that is kept.
"$AUDITOR_BIN" audit --tee-addr "$AUDITOR_HOST:5000" --build-measurements "$WORK/auditor-measured.json" --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$AUDITOR_SIGNING_KEY" --service "$SERVICE" --version "$VERSION" --vendor-tee "$VENDOR_HOST:5000" 2>&1 | tee "$WORK/audit-vendor-check.txt"
rc=${PIPESTATUS[0]}
echo "audit exit=$rc   registry=$REGISTRY   next: 5-User-tamper-verify-both-registries"
if grep -q "runs something else" "$WORK/audit-vendor-check.txt"; then
  result T6 auditor-sees-vendor-swap rejected rejected "audit --vendor-tee: the Vendor's enclave runs something else (audit exit $rc)" "registry $REGISTRY"
elif grep -q "runs exactly what I built" "$WORK/audit-vendor-check.txt"; then
  result T6 auditor-sees-vendor-swap accepted rejected "audit --vendor-tee: the Vendor's enclave runs what the Auditor built (audit exit $rc)" "registry $REGISTRY"
else
  result T6 auditor-sees-vendor-swap error rejected "audit --vendor-tee printed no verdict on the Vendor's enclave (audit exit $rc)" "registry $REGISTRY"
fi
exit $rc
