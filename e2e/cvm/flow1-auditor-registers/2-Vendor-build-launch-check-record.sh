#!/usr/bin/env bash
# CVM flow 1, Vendor - the CVM form of e2e/flow1-auditor-registers/2-Vendor-launch-check-record.sh, and steps 5 to 9 of
#    the guides. On the Vendor's guest: build the same application, install it as the boot entry, reboot, check the
#    boot, record the measured values and start the responder. Then, on this machine, the Vendor checks its own guest:
#    a fresh attestation compared with the Auditor's values (compare), and with the Auditor's registration (lookup and
#    compare against the chain). Ends by printing the verify command a User types on another computer.
#
#    Run from the repository root, after 1-Auditor-rebuild-launch-audit.sh with the same arguments:
#      bash e2e/cvm/flow1-auditor-registers/2-Vendor-build-launch-check-record.sh <format> [option]
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../../.."
. "$here/../../env.sh"
. "$here/../env-cvm.sh" "$@"
need_bins
REGISTRY=$(cat "$CVM_WORK/registry.txt" 2>/dev/null)
RECORD_KEY=$(cat "$CVM_WORK/record-key.txt" 2>/dev/null)
[ -n "$REGISTRY" ] && [ -n "$RECORD_KEY" ] && [ -f "$CVM_WORK/auditor-measurements.json" ] \
  || { echo "no registration from step 1 in $CVM_WORK: run 1-Auditor-rebuild-launch-audit.sh $* first" >&2; exit 1; }

step "VENDOR: THE APPLICATION" guest_step "$VENDOR_VM" vendor app original || exit 1
step "VENDOR: BUILDING" guest_step "$VENDOR_VM" vendor build || exit 1
step "VENDOR: INSTALLING AND REBOOTING" install_and_reboot "$VENDOR_VM" vendor || exit 1
step "VENDOR: CHECKING THE BOOT AND STARTING THE RESPONDER" guest_step "$VENDOR_VM" vendor check-serve || exit 1

step_start "RECORDING THE VENDOR'S VALUES"
guest_get "$VENDOR_VM" out/measurements.json "$CVM_WORK/vendor-measurements.json" || exit 1
VENDOR_IP=$(guest_ip "$VENDOR_VM")
[ -n "$VENDOR_IP" ] || { echo "no public address for $VENDOR_VM" >&2; exit 1; }
echo "$VENDOR_IP" > "$CVM_WORK/vendor-ip.txt"
step_stop
cat "$CVM_WORK/vendor-measurements.json"

mkdir -p "$CVM_WORK/vendor-check"
# att-vendor ra-request and ra-parse - a fresh attestation from my guest, verified.
step "REQUESTING ATTESTATION" "$VENDOR_BIN" ra-request --tee-addr "$VENDOR_IP:$RA_PORT" --format "$FORMAT" --out-file "$CVM_WORK/vendor-check/doc.bin" || exit 1
step "PARSING ATTESTATION" "$VENDOR_BIN" ra-parse --doc "$CVM_WORK/vendor-check/doc.bin" --format "$FORMAT" --out-file "$CVM_WORK/vendor-check/parsed.json" || exit 1
# att-vendor compare - what my guest attests against the values the Auditor measured on its own guest.
step "COMPARING TO THE AUDITOR'S VALUES" "$VENDOR_BIN" compare --attested "$CVM_WORK/vendor-check/parsed.json" --expected "$CVM_WORK/auditor-measurements.json"; a=$?
# att-vendor lookup - who registered what my guest attests.
step "LOOKING UP MY VALUE ON THE REGISTRY" "$VENDOR_BIN" lookup --rpc "$RPC" --registry "$REGISTRY" --measurements "$CVM_WORK/vendor-check/parsed.json" --platform "$FORMAT"; l=$?
# att-vendor compare - against the registration itself (value, expiry, approver).
step "COMPARING TO THE CHAIN REGISTRATION" "$VENDOR_BIN" compare --attested "$CVM_WORK/vendor-check/parsed.json" --rpc "$RPC" --registry "$REGISTRY" --record-key "$RECORD_KEY"; b=$?
if [ $a -eq 0 ] && [ $l -eq 0 ] && [ $b -eq 0 ]; then
  echo "OK: my guest runs exactly what the Auditor rebuilt and registered. Users will get 'trusted'."
  result T4 vendor-checks-own-guest accepted accepted "the Vendor's guest equals the Auditor's values and its registration" "registry $REGISTRY"
else
  echo "MISMATCH: my guest runs something other than what the Auditor registered."
  result T4 vendor-checks-own-guest rejected accepted "compare with the Auditor's values exit $a, lookup exit $l, compare with the registration exit $b" "registry $REGISTRY"
  exit 1
fi
print_verify_line "$VENDOR_IP" "$REGISTRY" "trusted, exit 0"
echo "next: the verify above on the other computer, then 2b-Auditor-audit-vendor.sh $*"
