#!/usr/bin/env bash
# Tamper, User - looks at the Vendor's registry and the Auditor's registry side by side.
#    e2e-work/registry-flow2.txt (Vendor) + e2e-work/registry.txt (Auditor) are both passed as --registry.
#    If the Vendor put the swapped value only in its own registry: Vendor registry ✓ / Auditor registry ✗ nobody -> not trusted - only 1 of 2 registries vouch.
#    Run: bash "e2e/flow2-vendor-self-registers/5-User-tamper-verify-both-registries.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST
step "READING REGISTRIES"
VREG=$(cat "$WORK/registry-flow2.txt"); AREG=$(cat "$WORK/registry.txt")
echo "Vendor's registry  $VREG"
echo "Auditor's registry $AREG"
# att command - given two registries separated by a comma, it judges trusted only if both vouch.
step "VERIFYING (both registries)" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$VREG,$AREG"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
