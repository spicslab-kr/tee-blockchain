#!/usr/bin/env bash
# Per-architecture measurement comparison (test matrix T1, T3). The same source and Dockerfile are built on two x86_64
#    machines (VENDOR_HOST and AUDITOR_HOST) and, when ARM64_HOST is set, on one arm64 machine, and PCR0/1/2 are measured
#    on each. The Auditor checks the source out under a different path from the Vendor's, so the x86_64 pair differs in
#    machine and in path:
#      - expected: the two x86_64 machines give identical values (T1: two machines; T3: two build paths). If they
#        differ, this script cannot tell which of the two caused it.
#      - expected: arm64 gives different values, because kernel, machine code and base image layers differ per
#        architecture. Without ARM64_HOST this part is skipped and reported as skipped.
#    The script ends with a verdict and writes one results line per verdict (env.sh, result()).
#
#    arm64 host: bash e2e/cloud/aws-nitro.sh create-arm64 (c7g.xlarge, Graviton, Amazon Linux 2023 arm64, Nitro
#    Enclaves on), then put its address in ARM64_HOST in e2e/env.local.sh.
#    Run from the repository root:  bash "e2e/arch-measurement-compare.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST; need_host AUDITOR_HOST
SRC_REPO=https://github.com/nginx/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim

ARCHES="x86_64"
[ -n "$ARM64_HOST" ] && ARCHES="x86_64 arm64"

# Cross-compile att-vendor and att-ra-responder per architecture.
#   -trimpath keeps the checkout path out of the binary. GOARCH_OF maps uname -m names to GOARCH names; add one entry per architecture.
step "CROSS-BUILDING PER ARCHITECTURE"
declare -A GOARCH_OF=( [x86_64]=amd64 [arm64]=arm64 )
mkdir -p "$WORK/arch"
for arch in $ARCHES; do
  ga=${GOARCH_OF[$arch]}
  GOOS=linux GOARCH=$ga go build -trimpath -buildvcs=false -o "$WORK/arch/att-vendor-$arch"       ./cmd/vendor
  GOOS=linux GOARCH=$ga go build -trimpath -buildvcs=false -o "$WORK/arch/att-ra-responder-$arch" ./cmd/ra-responder
  echo "  $arch  responder sha256: $(sha256 "$WORK/arch/att-ra-responder-$arch" | cut -c1-64)"
done

host_of() { case $1 in V) echo "$VENDOR_HOST";; A) echo "$AUDITOR_HOST";; R) echo "$ARM64_HOST";; esac; }

# measure_on <ssh-func> <storage-key> <label> <arch> <checkout dir> - uploads the binaries, builds on that host from the
# given checkout folder, and copies the measurements to $WORK/arch/<storage-key>/.
measure_on() {
  sshf=$1; key=$2; label=$3; arch=$4; src=$5
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
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT) INTO $src"
[ -d $src ] || git clone -q $SRC_REPO $src
git -C $src checkout -q $SRC_COMMIT
fix_modes $src
step_stop "FETCHING SOURCE"
step "BUILDING ($label, from \$PWD/$src)" ./att-vendor build --app-dir $src/$SRC_DIR --out-dir out-arch
cat out-arch/measurements.json
REMOTE
  rc=$?
  localBanner
  mkdir -p "$WORK/arch/$key"
  rm -f "$WORK/arch/$key/measurements.json"
  [ $rc -eq 0 ] && $SCP "$SSH_USER@$(host_of "$sshf"):out-arch/measurements.json" "$WORK/arch/$key/measurements.json"
  return $rc
}

step "MEASURING ON REAL NITRO HARDWARE"
measure_on V vendor-x86  "Vendor (x86_64)"  x86_64 src/docker-nginx
measure_on A auditor-x86 "Auditor (x86_64)" x86_64 src-other-path/checkout/docker-nginx
[ -n "$ARM64_HOST" ] && measure_on R arm64-host "arm64 comparison host" arm64 src/docker-nginx

pcr() { [ -f "$WORK/arch/$1/measurements.json" ] && grep -oE "\"$2\": *\"[0-9a-fA-F]+\"" "$WORK/arch/$1/measurements.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/'; }
triplet() { echo "$(pcr "$1" PCR0)|$(pcr "$1" PCR1)|$(pcr "$1" PCR2)"; }
values() { echo "PCR0=$(pcr "$1" PCR0) PCR1=$(pcr "$1" PCR1) PCR2=$(pcr "$1" PCR2)"; }

echo
echo "===================== measurement comparison ====================="
printf '%-16s %-16s %-16s %-16s\n' "host" "PCR0" "PCR1" "PCR2"
for row in "Vendor(x86_64):vendor-x86" "Auditor(x86_64):auditor-x86" "arm64-host:arm64-host"; do
  label=${row%%:*}; key=${row##*:}
  [ -f "$WORK/arch/$key/measurements.json" ] || continue
  printf '%-16s %-16s %-16s %-16s\n' "$label" "$(pcr "$key" PCR0 | cut -c1-14)…" "$(pcr "$key" PCR1 | cut -c1-14)…" "$(pcr "$key" PCR2 | cut -c1-14)…"
done
echo

fail=0
if [ ! -f "$WORK/arch/vendor-x86/measurements.json" ] || [ ! -f "$WORK/arch/auditor-x86/measurements.json" ]; then
  echo "❌ a build on an x86_64 host failed; nothing to compare"
  result T1 x86-two-machines error identical "a build on an x86_64 host failed"
  fail=1
elif [ "$(triplet vendor-x86)" != "$(triplet auditor-x86)" ]; then
  echo "❌ reproducibility regression - same architecture (x86_64) but Vendor and Auditor PCRs differ. Check -trimpath and the file-mode normalization."
  result T1 x86-two-machines-two-paths different identical "Vendor and Auditor PCRs differ on x86_64 (two machines and two checkout paths)" "vendor $(values vendor-x86); auditor $(values auditor-x86)"
  fail=1
else
  echo "✅ x86_64: Vendor and Auditor reproduce - two machines, two checkout paths, same values"
  result T1 x86-two-machines-two-paths identical identical "x86_64: Vendor and Auditor reproduce - two machines, two checkout paths, same values" "$(values vendor-x86)"
  result T3 x86-two-checkout-paths identical identical "the Auditor built from a different checkout path on another machine and got the same values" "$(values vendor-x86)"
fi
if [ -z "$ARM64_HOST" ]; then
  echo "ARM64_HOST is empty - the arm64 comparison was skipped."
  result T1 arm64-differs error different "skipped: ARM64_HOST is empty"
elif [ ! -f "$WORK/arch/arm64-host/measurements.json" ]; then
  echo "❌ the build on the arm64 host failed"
  result T1 arm64-differs error different "the build on the arm64 host failed"
  fail=1
elif [ "$(triplet vendor-x86)" = "$(triplet arm64-host)" ]; then
  echo "❌ anomaly - x86_64 and arm64 PCRs are identical. GOARCH may be unchanged (check for a cached measurement or a skipped build)."
  result T1 arm64-differs identical different "x86_64 and arm64 PCRs are identical" "$(values arm64-host)"
  fail=1
else
  echo "✅ arm64: differs from x86_64 as expected"
  result T1 arm64-differs different different "arm64: differs from x86_64 as expected" "$(values arm64-host)"
fi
echo "next: none - this script is the end of this test"
exit $fail
