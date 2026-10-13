#!/usr/bin/env bash
# Tamper, Vendor - secretly launches a different image instead of the one it built and registered, and also puts the changed
#    value in its own registry. Run here, on my machine - only the swap runs inside the Vendor instance, connected to
#    automatically. (After 1-Vendor-build-self-register)
#    Next: 4-Auditor-tamper-audit-check-vendor -> 5-User-tamper-verify-both-registries
#    Run: bash "e2e/flow2-vendor-self-registers/3-Vendor-tamper-swap-self-register.sh"
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
# Build a small image of our own for the tamper demo. Nobody has ever verified this value.
cat > tampered/Dockerfile <<'DOCKER'
FROM alpine:3.20
RUN echo "not the audited image" > /etc/att-tampered
CMD ["sleep", "1000000"]
DOCKER
step "BUILDING TAMPERED IMAGE" sudo docker build -q -t tampered:latest tampered

# att command - packs this docker image into an enclave image (EIF).
step "PACKING IMAGE" ./att-vendor pack --image tampered:latest --name tampered --out out-tampered

step_start "STOPPING PREVIOUS ENCLAVE"
sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x att-vendor || true
step_stop "STOPPING PREVIOUS ENCLAVE"

# att command - launches that tampered image in my TEE (instead of the one I built and registered).
step "LAUNCHING TAMPERED ENCLAVE" ./att-vendor launch --eif out-tampered/tampered.eif

echo
echo "the enclave on :5000 now runs my own image, not what I built from the audited source."
echo "built value:     $(grep PCR0 out/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the built one back:  ./att-vendor launch --eif out/alpine-slim.eif --measurements out/measurements.json"
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# Fetch the changed value (out-tampered/) from the Vendor instance to my machine.
step_start "FETCHING TAMPERED VALUE"
$SCP "$SSH_USER@$VENDOR_HOST:out-tampered/measurements.json" "$WORK/vendor-build.json"
step_stop "FETCHING TAMPERED VALUE"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"

# att command - deploys one more registry. "Registering B where A was expected" is a new event, so it gets a fresh registry
#   instead of being added to the existing one (the existing registry already holds the honest value A).
step_start "DEPLOYING REGISTRY"
out=$("$VENDOR_BIN" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$VENDOR_ADDR" 2>&1)
step_stop "DEPLOYING REGISTRY"
echo "$out"
CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # extract the address from the output
[ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
echo "$CONTRACT" > "$WORK/contract-flow2.txt"  # save it in the file the next step reads

# att command - signs the changed value with my key and registers it as is (to show that it passes without a third-party comparison).
step "SELF-REGISTERING" "$VENDOR_BIN" self-register --tee "$VENDOR_HOST:5000" --measurements "$WORK/vendor-build.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$VENDOR_KEY" --service "$SERVICE" --version "$VERSION"
echo "self-register exit=$?"
# att command - shows that, verified from the User's side, it passes if only this registry is looked at.
step "VERIFYING" "$USER_BIN" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
echo "verify exit=$?"
echo "next: 4-Auditor-tamper-audit-check-vendor"
