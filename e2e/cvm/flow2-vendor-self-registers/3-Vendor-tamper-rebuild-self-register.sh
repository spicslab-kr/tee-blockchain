#!/usr/bin/env bash
# CVM flow 2 tamper, Vendor - the CVM form of e2e/flow2-vendor-self-registers/3-Vendor-tamper-swap-self-register.sh. On
#    the Vendor's guest: change one byte of the application (hello -> Hello), build, install, reboot, check the boot and
#    start the responder. Then, on this machine: register the changed value with self-register, on a new registry of
#    the Vendor's own. No Auditor ever built this value; the Vendor vouches for it alone.
#
#    Why a new registry, as Nitro's step 3 does. The contract does not require it: the record key is keccak256(approver,
#    platform, measurement) (contracts/evm/Registry.sol), so the changed value has its own key and step 1's registry
#    would take it too. The choice is the same on a guest, for three reasons:
#    - Step 1's registry keeps vouching only for the build it was deployed for, as on Nitro ("the first registry keeps
#      the honest value"). Step 2's verify line, typed again now, says not trusted, as it should for a changed guest.
#    - The guest build is reproducible: the same changed application gives the same value on every run. On a reused
#      registry a second run of this step would stop at exit 5 (AlreadyRegistered); a fresh one per run never does,
#      like 2b and 4b of flow 1.
#    - It is what the hand runs of 2026-11-16 did (docs/results/sessions-2026-11-16/intel-tdx.md, "T6").
#    No verify line is printed here: against this registry alone verify says trusted (the approver is the Vendor), which
#    is the point step 4 and the line it prints make, with the Auditor's registry next to it.
#
#    Run from the repository root, after step 1 (and the verify of step 2):
#      bash e2e/cvm/flow2-vendor-self-registers/3-Vendor-tamper-rebuild-self-register.sh <format> [option] [-c 0xREGISTRY]
#      -c  use an existing registry whose approver is the Vendor (it must not hold the changed value yet)
#    Writes flow2-vendor-registry-tampered.txt and flow2-vendor-tampered-measurements.json in e2e-work/cvm/<format>/.
#    Step 1 puts the original back (it writes the application again).
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../../.."
. "$here/../../env.sh"
REGISTRY=""
args=()
while [ $# -gt 0 ]; do case "$1" in -c) [ $# -ge 2 ] || { echo "-c needs a registry address" >&2; exit 2; }; REGISTRY=$2; shift 2 ;; *) args+=("$1"); shift ;; esac; done
. "$here/../env-cvm.sh" "${args[@]}"
[ -f "$CVM_WORK/flow2-vendor-registry.txt" ] && [ -f "$CVM_WORK/flow2-vendor-measurements.json" ] \
  || { echo "no self-registration from step 1 in $CVM_WORK: run 1-Vendor-build-launch-self-register.sh ${args[*]} first" >&2; exit 1; }
need_addr VENDOR_ADDR
need_bins

tstep "VENDOR: CHANGING ONE BYTE OF THE APPLICATION" guest_step "$VENDOR_VM" vendor app tampered || exit 1
tstep "VENDOR: BUILDING THE CHANGED APPLICATION" guest_step "$VENDOR_VM" vendor build || exit 1
tstep "VENDOR: INSTALLING AND REBOOTING" install_and_reboot "$VENDOR_VM" vendor || exit 1
tstep "VENDOR: CHECKING THE BOOT AND STARTING THE RESPONDER" guest_step "$VENDOR_VM" vendor check-serve || exit 1

step_start "FETCHING MY OWN VALUE"
guest_get "$VENDOR_VM" out/measurements.json "$CVM_WORK/flow2-vendor-tampered-measurements.json" || exit 1
VENDOR_IP=$(guest_ip "$VENDOR_VM")
[ -n "$VENDOR_IP" ] || { echo "no public address for $VENDOR_VM" >&2; exit 1; }
step_stop
echo "self-registered in step 1:"; cat "$CVM_WORK/flow2-vendor-measurements.json"
echo "running now:";               cat "$CVM_WORK/flow2-vendor-tampered-measurements.json"

if [ -z "$REGISTRY" ]; then
  # att-vendor registry-deploy - a second registry of the Vendor's own, for the changed value.
  step_start "DEPLOYING A NEW REGISTRY OF MY OWN ($NET)"
  REGISTRY=$(deploy_registry "$VENDOR_BIN" "$VENDOR_ADDR") || exit 1
  step_stop
fi
echo "$REGISTRY" > "$CVM_WORK/flow2-vendor-registry-tampered.txt"
echo "the Vendor's registry for the changed value: $REGISTRY ($NET)"

# att-vendor self-register - the guest attests what it was built from, so this passes: self-register checks that the
# guest runs what the Vendor built, not that anybody else agrees with it.
tstep "SELF-REGISTERING THE CHANGED VALUE" "$VENDOR_BIN" self-register --tee-addr "$VENDOR_IP:$RA_PORT" --format "$FORMAT" \
  --build-measurements "$CVM_WORK/flow2-vendor-tampered-measurements.json" --rpc "$RPC" --registry "$REGISTRY" \
  --signing-key-file "$VENDOR_SIGNING_KEY" --service "$CVM_SERVICE" --version "$CVM_VERSION" --arch amd64
rc=$?
echo "self-register exit=$rc"
if [ $rc -ne 0 ]; then
  result T6 vendor-self-registers-changed-guest rejected accepted "self-register of the changed guest, exit $rc" "option $OPTION, registry $REGISTRY"
  exit $rc
fi
result T6 vendor-self-registers-changed-guest accepted accepted "the Vendor registered its own guest, running the one-byte-changed application, in its own registry" "option $OPTION, registry $REGISTRY"
echo "next: 4-Auditor-audit-vendor.sh ${args[*]} (after flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh)"
