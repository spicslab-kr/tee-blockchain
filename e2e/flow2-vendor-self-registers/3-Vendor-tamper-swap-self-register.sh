#!/usr/bin/env bash
# Flow 2 tamper, Vendor - replaces the enclave with a different image and registers the new value in a new registry
#    of its own. Run on my machine after 1-Vendor-build-self-register.sh; the swap runs inside the Vendor instance, which
#    the script connects to.
#    Next: 4-Auditor-tamper-audit-check-vendor.sh -> 5-User-tamper-verify-both-registries.sh
#    Run from the repository root:  bash "e2e/flow2-vendor-self-registers/3-Vendor-tamper-swap-self-register.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST

nitroBanner "Vendor"
V 'bash -s' <<'REMOTE'
set -euo pipefail
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; shift; "$@"; }
umask 022
mkdir -p tampered
# Build a small image for the tamper demo. No Auditor has verified this value.
cat > tampered/Dockerfile <<'DOCKER'
FROM alpine:3.20
RUN echo "not the audited image" > /etc/att-tampered
CMD ["sleep", "1000000"]
DOCKER
step "BUILDING TAMPERED IMAGE" sudo docker build -q -t tampered:latest tampered

# att-vendor pack - turns this docker image into an enclave image (EIF) and measures it.
step "PACKING IMAGE" ./att-vendor pack --docker-image tampered:latest --image-name tampered --out-dir out-tampered

# The instance has resources for one enclave at a time, so the previous enclave stops before the replacement launches.
step_start "STOPPING PREVIOUS ENCLAVE"
sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x att-vendor || true
step_stop "STOPPING PREVIOUS ENCLAVE"

# att-vendor launch - runs the tampered image in my TEE in place of the one I built and registered.
step "LAUNCHING TAMPERED ENCLAVE" ./att-vendor launch --eif out-tampered/tampered.eif

echo
echo "the enclave on :5000 now runs my own image; the source build produced a different value."
echo "built value:     $(grep PCR0 out/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the built one back:  ./att-vendor launch --eif out/alpine-slim.eif --build-measurements out/measurements.json"
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# Copy the tampered image's measurements (out-tampered/) from the Vendor instance to this machine.
step_start "FETCHING TAMPERED VALUE"
$SCP "$SSH_USER@$VENDOR_HOST:out-tampered/measurements.json" "$WORK/vendor-build.json"
step_stop "FETCHING TAMPERED VALUE"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"

# att-vendor registry-deploy - deploys a second registry, with the Vendor as approver, for the changed value. The first
#   registry keeps the honest value.
step_start "DEPLOYING REGISTRY"
out=$("$VENDOR_BIN" registry-deploy --rpc "$RPC" --deployer-key-file "$DEPLOYER_KEY" --approver "$VENDOR_ADDR" 2>&1)
step_stop "DEPLOYING REGISTRY"
echo "$out"
REGISTRY=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # address from the 'deployed at' line
[ -n "$REGISTRY" ] || { echo "no registry address in registry-deploy output"; exit 1; }
echo "$REGISTRY" > "$WORK/registry-flow2.txt"  # the later steps read the registry address from this file

# att-vendor self-register - signs the attested (changed) value with my signing key and registers it. No third party compares it.
step "SELF-REGISTERING" "$VENDOR_BIN" self-register --tee-addr "$VENDOR_HOST:5000" --build-measurements "$WORK/vendor-build.json" --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$VENDOR_SIGNING_KEY" --service "$SERVICE" --version "$VERSION"
echo "self-register exit=$?"
# att-user verify - with only this registry listed, the User's check passes.
step "VERIFYING" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$REGISTRY"
echo "verify exit=$?"
echo "next: 4-Auditor-tamper-audit-check-vendor"
