#!/usr/bin/env bash
# Per-architecture measurement comparison. The same source and Dockerfile are built on two x86_64 machines
#    (the existing VENDOR/AUDITOR) and one arm64 machine (ARM64_HOST), and PCR0/1/2 are measured on each.
#    Expected: the two x86_64 machines give identical values (reproducible - evidence it is not tied to the checkout
#    path); arm64 gives different values (normal - kernel, machine code and base image layers differ per architecture,
#    so the PCRs differ even for the same source; this is not a defect). The script judges at the end and prints
#    whether values that must differ are equal (a bug) or values that must match differ.
#
#    Needs: launch an arm64 (Graviton) Nitro instance and put its public IP in ARM64_HOST in e2e/env.local.sh.
#    Spec:
#      - type    c7g.xlarge (Graviton, 4 vCPU) - the arm64 counterpart of the m6i.xlarge (x86) used for VENDOR/AUDITOR
#      - OS      Amazon Linux 2023 (arm64 AMI - a different image from the x86 one, so set the console architecture filter to arm64)
#      - options Nitro Enclaves enabled; the rest (region eu-north-1, security group, key pair) same as VENDOR/AUDITOR
#      - setup   git, docker and nitro-cli are installed by the first run of att-vendor build, so nothing needs to be
#                installed on the instance beforehand
#    Run from TB-live:  bash "e2e/arch-measurement-compare.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

if [ -z "$ARM64_HOST" ]; then
  echo "ARM64_HOST is empty - put the arm64 instance's public IP in e2e/env.local.sh to run this test."
  echo "See the comment at the top of this file for the instance spec."
  exit 0
fi

need_host VENDOR_HOST; need_host AUDITOR_HOST
SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim

# Build the att executables - cross-compiled separately for x86_64 (amd64) and arm64.
#   -trimpath: keeps the checkout path out of the binary. To add an architecture, add one line to GOARCH_OF.
step "CROSS-BUILDING PER ARCHITECTURE"
declare -A GOARCH_OF=( [x86_64]=amd64 [arm64]=arm64 )
mkdir -p "$WORK/arch"
for arch in x86_64 arm64; do
  ga=${GOARCH_OF[$arch]}
  GOOS=linux GOARCH=$ga go build -trimpath -o "$WORK/arch/att-vendor-$arch"       ./cmd/vendor
  GOOS=linux GOARCH=$ga go build -trimpath -o "$WORK/arch/att-ra-responder-$arch" ./cmd/ra-responder
  echo "  $arch  responder sha256: $(sha256 "$WORK/arch/att-ra-responder-$arch" | cut -c1-64)"
done

host_of() { case $1 in V) echo "$VENDOR_HOST";; A) echo "$AUDITOR_HOST";; R) echo "$ARM64_HOST";; esac; }

# measure_on <ssh-func> <storage-key> <label> <arch> - storage-key must be distinct per host
# (Vendor and Auditor are both arch="x86_64" but their result files must not overlap).
measure_on() {
  sshf=$1; key=$2; label=$3; arch=$4
  nitroBanner "$label"
  $sshf 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x att-vendor; true'
  $SCP "$WORK/arch/att-vendor-$arch" "$SSH_USER@$(host_of "$sshf"):~/att-vendor"
  $SCP "$WORK/arch/att-ra-responder-$arch" "$SSH_USER@$(host_of "$sshf"):~/att-ra-responder"
  $sshf 'bash -s' <<REMOTE
set -euo pipefail
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "\$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "\$1"; shift; "\$@"; }
chmod +x att-vendor att-ra-responder
umask 022
fix_modes() { (cd "\$1" && git ls-files -s | while read -r mode _ _ path; do case \$mode in 100755) chmod 755 "\$path";; 100644) chmod 644 "\$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
step "BUILDING ($label)" ./att-vendor build --app-dir src/docker-nginx/$SRC_DIR --out out
cat out/measurements.json
REMOTE
  localBanner
  mkdir -p "$WORK/arch/$key"
  $SCP "$SSH_USER@$(host_of "$sshf"):out/measurements.json" "$WORK/arch/$key/measurements.json"
}

step "MEASURING ON REAL NITRO HARDWARE"
measure_on V vendor-x86  "Vendor (x86_64)"       x86_64
measure_on A auditor-x86 "Auditor (x86_64)"      x86_64
measure_on R arm64-host  "arm64 comparison host" arm64

pcr() { grep -oE "\"$2\": *\"[0-9a-fA-F]+\"" "$WORK/arch/$1/measurements.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/'; }
triplet() { echo "$(pcr "$1" PCR0)|$(pcr "$1" PCR1)|$(pcr "$1" PCR2)"; }

echo
echo "===================== measurement comparison ====================="
printf '%-16s %-16s %-16s %-16s\n' "host" "PCR0" "PCR1" "PCR2"
for row in "Vendor(x86_64):vendor-x86" "Auditor(x86_64):auditor-x86" "arm64-host:arm64-host"; do
  label=${row%%:*}; key=${row##*:}
  printf '%-16s %-16s %-16s %-16s\n' "$label" "$(pcr "$key" PCR0 | cut -c1-14)…" "$(pcr "$key" PCR1 | cut -c1-14)…" "$(pcr "$key" PCR2 | cut -c1-14)…"
done
echo

fail=0
if [ "$(triplet vendor-x86)" != "$(triplet auditor-x86)" ]; then
  echo "❌ reproducibility regression - same architecture (x86_64) but Vendor and Auditor PCRs differ. -trimpath or normalization is broken."
  fail=1
else
  echo "✅ x86_64: Vendor and Auditor reproduce - same source, same values"
fi
if [ "$(triplet vendor-x86)" = "$(triplet arm64-host)" ]; then
  echo "❌ anomaly - x86_64 and arm64 PCRs are identical. GOARCH may not have actually changed (suspect a cached measurement or a skipped build)."
  fail=1
else
  echo "✅ arm64: differs from x86_64 - expected (not a defect)"
fi
echo "next: none - this script is the end of this test"
exit $fail
