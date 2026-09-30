#!/usr/bin/env bash
# 아키텍처별 측정값 비교. 같은 소스·같은 Dockerfile을 x86_64 두 대(기존 VENDOR/AUDITOR)와
#    arm64 한 대(ARM64_HOST)에서 각자 빌드해 PCR0·1·2 를 실측한다.
#    기대하는 결과: x86_64 두 대는 완전히 같은 값(재현 — 체크아웃 경로에 안 묶인다는 증거), arm64 는 다른 값(정상 —
#    커널·기계어·베이스 이미지 레이어가 아키텍처마다 다르므로 같은 소스라도 PCR 은 갈린다. 결함이 아니다).
#    같으면 안 되는데 같은 경우(버그)와 달라야 하는데 다른 경우(정상)를 스크립트 끝에서 판정해서 찍는다.
#
#    필요: ARM64_HOST 에 arm64(Graviton) Nitro 인스턴스를 새로 띄우고 그 퍼블릭 IP를 e2e/env.sh 에 적어야 한다.
#    스펙(검토 보완서 §4.1·부록 실측 그대로):
#      - 타입    c7g.xlarge  (Graviton, 4 vCPU) — 기존 VENDOR/AUDITOR 와 같은 급의 m6i.xlarge(x86) 짝
#      - OS      Amazon Linux 2023 (arm64 AMI — x86 AMI 와 다른 이미지이니 콘솔에서 아키텍처 필터를 arm64로)
#      - 옵션    Nitro Enclaves enabled 체크, 나머지(리전 eu-north-1·보안그룹·키페어)는 기존 VENDOR/AUDITOR 와 동일하게
#      - 준비    bash e2e/0-Setup-local-install.sh 와 같은 사전설치가 이 인스턴스에도 필요 (git·docker·nitro-cli는
#                tb-vendor build 가 첫 실행에 알아서 깐다 — INSTANCE-INFO.md 참고)
#    실행: TB-live 에서  bash "e2e/arch-measurement-compare.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

if [ -z "$ARM64_HOST" ]; then
  echo "ARM64_HOST 가 비어 있다 — e2e/env.sh 에 arm64 인스턴스 퍼블릭 IP를 적어야 이 테스트를 돌릴 수 있다."
  echo "필요한 인스턴스 스펙은 이 파일 맨 위 주석 참고."
  exit 0
fi

SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim

# ▶ 우리가 만든 tb 명령의 실행 파일 — x86_64(amd64)용과 arm64용을 각자 크로스컴파일한다.
#   -trimpath: 체크아웃 경로가 바이너리에 안 박히게. 모듈러: 아키텍처를 늘리려면 GOARCH_OF 에 한 줄만 추가하면 된다.
step "CROSS-BUILDING PER ARCHITECTURE"
declare -A GOARCH_OF=( [x86_64]=amd64 [arm64]=arm64 )
mkdir -p "$WORK/arch"
for arch in x86_64 arm64; do
  ga=${GOARCH_OF[$arch]}
  GOOS=linux GOARCH=$ga go build -trimpath -o "$WORK/arch/tb-vendor-$arch"       ./cmd/vendor
  GOOS=linux GOARCH=$ga go build -trimpath -o "$WORK/arch/tb-ra-responder-$arch" ./cmd/ra-responder
  echo "  $arch  responder sha256: $(sha256 "$WORK/arch/tb-ra-responder-$arch" | cut -c1-64)"
done

host_of() { case $1 in V) echo "$VENDOR_HOST";; A) echo "$AUDITOR_HOST";; R) echo "$ARM64_HOST";; esac; }

# measure_on <ssh-func> <storage-key> <label> <arch> — storage-key 는 host 마다 따로 줘야 한다
# (x86_64 Vendor·Auditor 둘 다 arch="x86_64" 이지만 결과 파일은 겹치면 안 되므로).
measure_on() {
  sshf=$1; key=$2; label=$3; arch=$4
  nitroBanner "$label"
  $sshf 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-vendor; true'
  $SCP "$WORK/arch/tb-vendor-$arch" "$SSH_USER@$(host_of "$sshf"):~/tb-vendor"
  $SCP "$WORK/arch/tb-ra-responder-$arch" "$SSH_USER@$(host_of "$sshf"):~/tb-ra-responder"
  $sshf 'bash -s' <<REMOTE
set -euo pipefail
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "\$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "\$1"; shift; "\$@"; }
chmod +x tb-vendor tb-ra-responder
umask 022
fix_modes() { (cd "\$1" && git ls-files -s | while read -r mode _ _ path; do case \$mode in 100755) chmod 755 "\$path";; 100644) chmod 644 "\$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
step "BUILDING ($label)" ./tb-vendor build --app-dir src/docker-nginx/$SRC_DIR --out out
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
  echo "❌ 재현성 회귀 — 같은 아키텍처(x86_64)인데 Vendor 와 Auditor 의 PCR 이 다르다. -trimpath 나 재정규화가 깨졌다."
  fail=1
else
  echo "✅ x86_64: Vendor·Auditor 재현됨 — 같은 소스, 같은 값"
fi
if [ "$(triplet vendor-x86)" = "$(triplet arm64-host)" ]; then
  echo "❌ 이상 — x86_64 와 arm64 의 PCR 이 우연히 같다. GOARCH 가 실제로 안 바뀌었을 가능성 (측정값 캐시·빌드 스킵 등을 의심)."
  fail=1
else
  echo "✅ arm64: x86_64 와 다른 값 — 정상 (결함 아님)"
fi
echo "next: 없음 — 이 스크립트가 이 테스트의 끝이다"
exit $fail
