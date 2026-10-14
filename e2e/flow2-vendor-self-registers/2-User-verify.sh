#!/usr/bin/env bash
# Flow 2, User - verifies what the Vendor registered in its own registry (address in e2e-work/registry-flow2.txt).
#    A pass means the Vendor vouched for its own build, because the Vendor is this registry's approver. The Auditor joins in
#    the tamper scripts 3 to 5.
#    Run from the repository root:  bash "e2e/flow2-vendor-self-registers/2-User-verify.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST
REGISTRY=$(cat e2e-work/registry-flow2.txt)
# att-user verify - requests the attestation document, looks up that value on the registry, and decides whether to trust.
step "VERIFYING" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$REGISTRY"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = not trusted)"
exit $rc
