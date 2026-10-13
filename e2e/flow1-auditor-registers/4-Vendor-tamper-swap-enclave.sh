#!/usr/bin/env bash
# Tamper, Vendor - secretly launches a different image instead of the one the Auditor handed over. Run here, on my machine -
#    the actual swap runs inside the Vendor instance, connected to automatically. (After the normal flow through 2-Vendor-launch-check-record)
#    Next: 5-User-tamper-verify   -> ✗ nobody ... not trusted
#    Run: bash "e2e/flow1-auditor-registers/4-Vendor-tamper-swap-enclave.sh"
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
# Build a small image of our own for the tamper demo. The Auditor has never seen this image.
cat > tampered/Dockerfile <<'DOCKER'
FROM alpine:3.20
RUN echo "not the audited image" > /etc/att-tampered
CMD ["sleep", "1000000"]
DOCKER
step "BUILDING TAMPERED IMAGE" sudo docker build -q -t tampered:latest tampered

# att command - packs this docker image into an enclave image (EIF).
step "PACKING IMAGE" ./att-vendor pack --docker-image tampered:latest --image-name tampered --out-dir out-tampered

# An instance has resources for only one enclave, so stop the old one first and then launch the replacement.
step_start "STOPPING PREVIOUS ENCLAVE"
sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x att-vendor || true
step_stop "STOPPING PREVIOUS ENCLAVE"

# att command - launches that tampered image in my TEE (instead of the one the Auditor gave).
step "LAUNCHING TAMPERED ENCLAVE" ./att-vendor launch --eif out-tampered/tampered.eif

echo
echo "the enclave on :5000 now runs my own image, not what the Auditor registered."
echo "audited value:   $(grep PCR0 from-auditor/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the audited one back:  ./att-vendor launch --eif from-auditor/alpine-slim.eif --build-measurements from-auditor/measurements.json"
REMOTE
rc=$?
localBanner
echo "next: 5-User-tamper-verify"
exit $rc
