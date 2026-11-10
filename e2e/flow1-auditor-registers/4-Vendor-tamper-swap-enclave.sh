#!/usr/bin/env bash
# Flow 1 tamper, Vendor - replaces the audited enclave with a different image. Run on my machine after
#    2-Vendor-launch-check-record.sh; the swap itself runs inside the Vendor instance, which the script connects to.
#    Next: 5-User-tamper-verify.sh   -> ✗ nobody ... not trusted
#    Run from the repository root:  bash "e2e/flow1-auditor-registers/4-Vendor-tamper-swap-enclave.sh"
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
# Build a small image for the tamper demo. The Auditor has not seen this image.
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

# att-vendor launch - runs the tampered image in my TEE in place of the Auditor's image.
step "LAUNCHING TAMPERED ENCLAVE" ./att-vendor launch --eif out-tampered/tampered.eif

echo
echo "the enclave on :5000 now runs my own image; the Auditor registered a different value."
echo "audited value:   $(grep PCR0 from-auditor/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the audited one back:  ./att-vendor launch --eif from-auditor/alpine-slim.eif --build-measurements from-auditor/measurements.json"
REMOTE
rc=$?
localBanner
echo "next: 5-User-tamper-verify"
exit $rc
