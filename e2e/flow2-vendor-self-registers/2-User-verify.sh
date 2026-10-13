#!/usr/bin/env bash
# Flow 2, User - verifies what the Vendor put in its own registry. The registry is e2e-work/contract-flow2.txt.
#    Even if it passes, the signer is the Vendor itself - only "the Vendor vouched for its own build". The Auditor comes in
#    with the tamper scripts (3 to 5), which are the story after this.
#    Run: bash "e2e/flow2-vendor-self-registers/2-User-verify.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST
CONTRACT=$(cat e2e-work/contract-flow2.txt)
# att command - confirm the value by RA, look up the registration on the chain, compare, decide whether to trust.
step "VERIFYING" "$USER_BIN" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
