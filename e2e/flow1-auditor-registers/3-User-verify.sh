#!/usr/bin/env bash
# Flow 1, User (any machine, no key) - requests an attestation document from the Vendor's enclave, looks up that
#    measurement on the registry, and compares it with the registration.
#    Run from the repository root:  bash "e2e/flow1-auditor-registers/3-User-verify.sh" [REGISTRY]
#      REGISTRY defaults to the address in e2e-work/registry.txt.
#    Several registries are given as a comma-separated list: 3-User-verify.sh 0xVendorRegistry,0xAuditorRegistry
#    The enclave is trusted when every listed registry vouches for it.
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST
REGISTRY=${1:-$(cat "$WORK/registry.txt")}
# att-user verify - requests the attestation document, finds the registrations of that value on the registry, checks each one, and decides whether to trust.
step "VERIFYING" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$REGISTRY"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = not trusted)"
exit $rc
