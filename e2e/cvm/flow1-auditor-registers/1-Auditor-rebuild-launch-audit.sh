#!/usr/bin/env bash
# CVM flow 1, Auditor - the CVM form of e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh, and A4 to A8 of
#    "The Auditor's half" in docs/build/tdx.md and sev-snp.md. On the Auditor's own guest: build the application,
#    install the build as the boot entry, reboot, and check that the boot measured exactly that build. Then, on this
#    machine: deploy a registry and audit the guest, which registers the value only if the guest attests what was built.
#    The application is the one 0-Setup-guests.sh (and step 2, for the Vendor) writes, so both sides build the same files.
#
#    Run from the repository root, after e2e/cvm/0-Setup-guests.sh with the same arguments:
#      bash e2e/cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh <format> [option] [-c 0xREGISTRY]
#      -c  use an existing registry instead of deploying one (it must not hold this value yet, or audit stops at exit 5)
#    The chain is NET (env.sh): Sepolia by default, so the other computer can reach it and the transactions show on
#    Etherscan; NET=local for development. Writes e2e-work/cvm/<format>/registry.txt and auditor-measurements.json.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../../.."
. "$here/../../env.sh"
REGISTRY=""
args=()
while [ $# -gt 0 ]; do case "$1" in -c) [ $# -ge 2 ] || { echo "-c needs a registry address" >&2; exit 2; }; REGISTRY=$2; shift 2 ;; *) args+=("$1"); shift ;; esac; done
. "$here/../env-cvm.sh" "${args[@]}"
need_addr AUDITOR_ADDR
need_bins

tstep "AUDITOR: THE APPLICATION" guest_step "$AUDITOR_VM" auditor app original || exit 1
tstep "AUDITOR: BUILDING" guest_step "$AUDITOR_VM" auditor build || exit 1
tstep "AUDITOR: INSTALLING AND REBOOTING" install_and_reboot "$AUDITOR_VM" auditor || exit 1
tstep "AUDITOR: CHECKING THE BOOT AND STARTING THE RESPONDER" guest_step "$AUDITOR_VM" auditor check-serve || exit 1

step_start "FETCHING THE AUDITOR'S OWN VALUE"
guest_get "$AUDITOR_VM" out/measurements.json "$CVM_WORK/auditor-measurements.json" || exit 1
guest_get "$AUDITOR_VM" out/build-value.json "$CVM_WORK/auditor-build-value.json" || exit 1
AUDITOR_IP=$(guest_ip "$AUDITOR_VM")
[ -n "$AUDITOR_IP" ] || { echo "no public address for $AUDITOR_VM" >&2; exit 1; }
step_stop
cat "$CVM_WORK/auditor-measurements.json"

if [ -z "$REGISTRY" ]; then
  step_start "DEPLOYING THE REGISTRY ($NET)"
  REGISTRY=$(deploy_registry "$AUDITOR_BIN" "$AUDITOR_ADDR") || exit 1
  step_stop
fi
echo "$REGISTRY" > "$CVM_WORK/registry.txt"
echo "registry: $REGISTRY ($NET)"

# att-auditor audit - a fresh attestation from my guest must equal the value it measured after my own build; only then
# is it registered. The format is given, not taken from a build-config.json: on a guest the Vendor hands over the
# application folder and the base initrd (0-Setup-guests.sh), not a config.
tstep "AUDITING MY GUEST" "$AUDITOR_BIN" audit --tee-addr "$AUDITOR_IP:$RA_PORT" --format "$FORMAT" \
  --build-measurements "$CVM_WORK/auditor-measurements.json" --rpc "$RPC" --registry "$REGISTRY" \
  --signing-key-file "$AUDITOR_SIGNING_KEY" --service "$CVM_SERVICE" --version "$CVM_VERSION" --arch amd64
rc=$?
if [ $rc -eq 0 ]; then
  result T4 auditor-audit-registers accepted accepted "audit: the Auditor's guest attests its own build, registered" "option $OPTION, registry $REGISTRY"
else
  result T4 auditor-audit-registers rejected accepted "audit exit $rc" "option $OPTION, registry $REGISTRY"
  exit $rc
fi

# The record key, for the Vendor's comparison in step 2.
out=$("$AUDITOR_BIN" lookup --rpc "$RPC" --registry "$REGISTRY" --measurements "$CVM_WORK/auditor-measurements.json" --platform "$FORMAT" 2>&1)
echo "$out"
RECORD_KEY=$(echo "$out" | grep -oE 'record key +0x[0-9a-fA-F]{64}' | awk '{print $3}' | head -1)
[ -n "$RECORD_KEY" ] || { echo "the registration was not found on $REGISTRY" >&2; exit 1; }
echo "$RECORD_KEY" > "$CVM_WORK/record-key.txt"
echo "registered under record key $RECORD_KEY. next: 2-Vendor-build-launch-check-record.sh ${args[*]}"
