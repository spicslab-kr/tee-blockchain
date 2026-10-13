#!/usr/bin/env bash
# User (any machine, no key) - RA to the Vendor enclave, look up the chain with that measurement, compare with the registration.
#    Run: bash "e2e/flow1-auditor-registers/3-User-verify.sh" [CONTRACT]      if omitted, e2e-work/contract.txt is used
#    To check several registries side by side, separate them with commas: 3-User-verify.sh 0xVendorRegistry,0xAuditorRegistry - all of them must vouch for it to be trusted
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST
CONTRACT=${1:-$(cat "$WORK/contract.txt")}
# att command - this is the whole script: confirm the value by RA, look up the registration on the chain, compare, decide whether to trust.
step "VERIFYING" "$USER_BIN" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
