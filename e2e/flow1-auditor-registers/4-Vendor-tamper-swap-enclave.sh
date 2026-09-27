#!/usr/bin/env bash
# 조작 · Vendor — Auditor 가 준 이미지 대신 다른 이미지를 몰래 띄운다. 여기, 내 컴퓨터에서 실행 — 실제 교체는
#    Vendor 인스턴스 안에서 자동으로 접속해 실행된다. (정상 흐름 2-Vendor-launch-check-record 까지 끝난 뒤)
#    다음: 5-User-tamper-verify   → ✗ nobody … not trusted
#    실행: bash "e2e/flow1-auditor-registers/4-Vendor-tamper-swap-enclave.sh"
here="$(dirname "$0")"
. "$here/../env.sh"

echo "== Vendor 인스턴스로 접속 중..."
nitroBanner "Vendor"
V 'bash -s' <<'REMOTE'
set -euo pipefail
umask 022
mkdir -p tampered
# 준비 — 조작 데모용으로 작은 이미지 하나를 직접 만든다. Auditor 는 이 이미지를 본 적이 없다.
cat > tampered/Dockerfile <<'DOCKER'
FROM alpine:3.20
RUN echo "not the audited image" > /etc/tb-tampered
CMD ["sleep", "1000000"]
DOCKER
echo "== building my own image (the Auditor never saw this one)"
sudo docker build -q -t tampered:latest tampered

# ▶ 우리가 만든 tb 명령 — 이 도커 이미지를 엔클레이브 이미지(EIF)로 포장한다.
echo "== tb-vendor pack"
./tb-vendor pack --image tampered:latest --name tampered --out out-tampered

# 준비 — 인스턴스 하나가 가진 엔클레이브 자원은 하나 분량이라, 먼저 내리고 바꿔 띄운다.
sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x tb-vendor || true

# ▶ 우리가 만든 tb 명령 — 그 조작된 이미지를 내 TEE 에 띄운다 (Auditor 가 준 것 대신).
echo "== tb-vendor launch (조작된 이미지)"
./tb-vendor launch --eif out-tampered/tampered.eif

echo
echo "the enclave on :5000 now runs my own image, not what the Auditor registered."
echo "audited value:   $(grep PCR0 from-auditor/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the audited one back:  ./tb-vendor launch --eif from-auditor/alpine-slim.eif --measurements from-auditor/measurements.json"
REMOTE
rc=$?
localBanner
echo "next: 5-User-tamper-verify"
exit $rc
