#!/usr/bin/env bash
# CVM flow 2, Vendor - the CVM form of e2e/flow2-vendor-self-registers/1-Vendor-build-self-register.sh. On the Vendor's
#    guest: build the application, install it as the boot entry, reboot, check the boot and start the responder (steps
#    5 to 9 of docs/build/tdx.md and sev-snp.md). Then, on this machine: deploy a registry whose approver is the Vendor
#    and run self-register, which registers the value only if a fresh attestation of the guest equals the build. No
#    Auditor takes part. Ends by printing the verify command for step 2, typed by hand: trusted, because the Vendor
#    vouches for itself.
#
#    Run from the repository root, after e2e/cvm/0-Setup-guests.sh with the same arguments:
#      bash e2e/cvm/flow2-vendor-self-registers/1-Vendor-build-launch-self-register.sh <format> [option] [-c 0xREGISTRY]
#      -c  use an existing registry whose approver is the Vendor (it must not hold this value yet, or the step stops at
#          exit 5)
#    Writes e2e-work/cvm/<format>/flow2-vendor-registry.txt and flow2-vendor-measurements.json; flow 1's files there are
#    left as they are.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../../.."
. "$here/../../env.sh"
REGISTRY=""
args=()
while [ $# -gt 0 ]; do case "$1" in -c) [ $# -ge 2 ] || { echo "-c needs a registry address" >&2; exit 2; }; REGISTRY=$2; shift 2 ;; *) args+=("$1"); shift ;; esac; done
. "$here/../env-cvm.sh" "${args[@]}"
need_addr VENDOR_ADDR
need_bins

tstep "VENDOR: THE APPLICATION" guest_step "$VENDOR_VM" vendor app original || exit 1
tstep "VENDOR: BUILDING" guest_step "$VENDOR_VM" vendor build || exit 1
tstep "VENDOR: INSTALLING AND REBOOTING" install_and_reboot "$VENDOR_VM" vendor || exit 1
tstep "VENDOR: CHECKING THE BOOT AND STARTING THE RESPONDER" guest_step "$VENDOR_VM" vendor check-serve || exit 1

step_start "FETCHING MY OWN VALUE"
guest_get "$VENDOR_VM" out/measurements.json "$CVM_WORK/flow2-vendor-measurements.json" || exit 1
VENDOR_IP=$(guest_ip "$VENDOR_VM")
[ -n "$VENDOR_IP" ] || { echo "no public address for $VENDOR_VM" >&2; exit 1; }
step_stop
cat "$CVM_WORK/flow2-vendor-measurements.json"

if [ -z "$REGISTRY" ]; then
  # att-vendor registry-deploy - a registry of the Vendor's own, with the Vendor as its approver.
  step_start "DEPLOYING MY REGISTRY ($NET)"
  REGISTRY=$(deploy_registry "$VENDOR_BIN" "$VENDOR_ADDR") || exit 1
  step_stop
fi
echo "$REGISTRY" > "$CVM_WORK/flow2-vendor-registry.txt"
echo "the Vendor's registry: $REGISTRY ($NET)"

# att-vendor self-register - a fresh attestation from my guest must equal the value its boot measured; only then is it
# registered, signed with my own key. As in flow 1's audit, the format, service, version and arch are given, not taken
# from a build-config.json.
tstep "SELF-REGISTERING" "$VENDOR_BIN" self-register --tee-addr "$VENDOR_IP:$RA_PORT" --format "$FORMAT" \
  --build-measurements "$CVM_WORK/flow2-vendor-measurements.json" --rpc "$RPC" --registry "$REGISTRY" \
  --signing-key-file "$VENDOR_SIGNING_KEY" --service "$CVM_SERVICE" --version "$CVM_VERSION" --arch amd64
rc=$?
echo "self-register exit=$rc"
if [ $rc -ne 0 ]; then
  result T6 vendor-self-registers rejected accepted "self-register exit $rc" "option $OPTION, registry $REGISTRY"
  exit $rc
fi
result T6 vendor-self-registers accepted accepted "self-register: the Vendor's guest attests its own build, registered by the Vendor" "option $OPTION, registry $REGISTRY"
print_verify_line "$VENDOR_IP" "trusted, exit 0" "$REGISTRY"
echo "That trust rests on the Vendor's word alone: the approver of this registry is the Vendor."
echo "next: the verify above (step 2), then 3-Vendor-tamper-rebuild-self-register.sh ${args[*]}"
