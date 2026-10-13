#!/usr/bin/env bash
# Tamper, User - verifies the same way after the Vendor swapped the enclave (4-Vendor-tamper-swap-enclave). The measurement
#    received by RA differs from the value the Auditor registered.
#    It is the same command as 3-User-verify in the normal flow (registry e2e-work/registry.txt). Expected: red ✗ nobody - ... never registered the value this enclave runs
#    -> "nobody has registered these measurements on the chain - no basis for trust", exit 1.
#    Run: bash "e2e/flow1-auditor-registers/5-User-tamper-verify.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST
echo "the Vendor's enclave now runs something the Auditor never registered — the same verify as step 3 must turn red:"
REGISTRY=$(cat "$WORK/registry.txt")
# att command - confirm the value by RA, look up the registration on the chain, compare, decide whether to trust.
step "VERIFYING" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$REGISTRY"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
