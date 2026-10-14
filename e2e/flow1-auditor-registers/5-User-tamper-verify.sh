#!/usr/bin/env bash
# Flow 1 tamper, User - runs the same verify as 3-User-verify.sh (registry from e2e-work/registry.txt) after the Vendor
#    swapped the enclave in 4-Vendor-tamper-swap-enclave.sh. The attested measurement now differs from the registered value.
#    Expected: red ✗ nobody - ... approver never registered the value this enclave runs, then
#    "nobody has registered these measurements on the chain - no basis for trust", exit 1.
#    Run from the repository root:  bash "e2e/flow1-auditor-registers/5-User-tamper-verify.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST
echo "the Vendor's enclave now runs a value the Auditor did not register — the same verify as 3-User-verify must turn red:"
REGISTRY=$(cat "$WORK/registry.txt")
# att-user verify - requests the attestation document, looks up that value on the registry, and decides whether to trust.
step "VERIFYING" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$REGISTRY"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = not trusted)"
exit $rc
