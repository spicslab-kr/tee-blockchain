#!/usr/bin/env bash
# CVM flow 1 tamper, Vendor - the CVM form of e2e/flow1-auditor-registers/4-Vendor-tamper-swap-enclave.sh. On the
#    Vendor's guest: change one byte of the application (hello -> Hello), build, install, reboot and check the boot, and
#    start the responder. The Auditor registered the original. Ends by printing the verify command for the other
#    computer, which must now say not trusted. Step 2 puts the original back (it writes the application again).
#
#    Run from the repository root, after step 2 (and 2b):
#      bash e2e/cvm/flow1-auditor-registers/4-Vendor-tamper-rebuild-launch.sh <format> [option]
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../../.."
. "$here/../../env.sh"
. "$here/../env-cvm.sh" "$@"
REGISTRY=$(cat "$CVM_WORK/registry.txt" 2>/dev/null)
[ -n "$REGISTRY" ] || { echo "no registry from step 1 in $CVM_WORK: run steps 1 and 2 first" >&2; exit 1; }

tstep "VENDOR: CHANGING ONE BYTE OF THE APPLICATION" guest_step "$VENDOR_VM" vendor app tampered || exit 1
tstep "VENDOR: BUILDING THE CHANGED APPLICATION" guest_step "$VENDOR_VM" vendor build || exit 1
tstep "VENDOR: INSTALLING AND REBOOTING" install_and_reboot "$VENDOR_VM" vendor || exit 1
tstep "VENDOR: CHECKING THE BOOT AND STARTING THE RESPONDER" guest_step "$VENDOR_VM" vendor check-serve || exit 1

guest_get "$VENDOR_VM" out/measurements.json "$CVM_WORK/vendor-tampered-measurements.json" || exit 1
VENDOR_IP=$(guest_ip "$VENDOR_VM")
[ -n "$VENDOR_IP" ] || { echo "no public address for $VENDOR_VM" >&2; exit 1; }
echo "registered (step 1):"; cat "$CVM_WORK/auditor-measurements.json"
echo "running now:";         cat "$CVM_WORK/vendor-tampered-measurements.json"
result T5 vendor-runs-changed-app different different "the Vendor's guest now runs a one-byte-changed application" "option $OPTION, registry $REGISTRY"
print_verify_line "$VENDOR_IP" "$REGISTRY" "not trusted, exit 1"
echo "next: the verify above on the other computer, then 4b-Auditor-audit-vendor-tampered.sh $*"
