#!/bin/bash
# 조작 · Vendor — 자기가 빌드해 등록한 이미지 대신 다른 이미지를 띄운다 (설계 흐름 2 ②의 시작: "A 여야 하는데 B 가 돌고 있다").
# 인스턴스 안에서 실제로 도는 내용 — 직접 실행하지 말고 4-Vendor-tamper-swap-enclave 로 실행할 것 (1-Vendor-build-source 까지 끝난 뒤)
# 작은 이미지 하나를 직접 만들어(= 아무도 검증하지 않은 값) 그것으로 엔클레이브를 바꿔 띄운다. 그 다음 내 컴퓨터에서
#   Vendor:  5-Vendor-tamper-self-register-swapped   → 바꿔 띄운 값을 자기 등록부에 올린다
#   Auditor: 6-Auditor-tamper-audit-check-vendor     → ✗ the Vendor's enclave runs something else
#   User:    7-User-tamper-verify-both-registries    → not trusted — only 1 of 2 registries vouch
# 되돌리기: ./tb-vendor launch --eif out/alpine-slim.eif --measurements out/measurements.json
set -euo pipefail
umask 022
mkdir -p tampered
cat > tampered/Dockerfile <<'DOCKER'
FROM alpine:3.20
RUN echo "not the audited image" > /etc/tb-tampered
CMD ["sleep", "1000000"]
DOCKER
echo "== building my own image (nobody has audited this one)"
sudo docker build -q -t tampered:latest tampered
echo "== packing and launching it in place of the image I built and registered"
./tb-vendor pack --image tampered:latest --name tampered --out out-tampered
# 한 인스턴스의 엔클레이브 자원(2 cpu · 512 MiB)은 하나 분량이라, 먼저 내리고 바꿔 띄운다
sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x tb-vendor || true
./tb-vendor launch --eif out-tampered/tampered.eif
echo
echo "the enclave on :5000 now runs my own image, not what I built from the audited source."
echo "built value:     $(grep PCR0 out/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the built one back:  ./tb-vendor launch --eif out/alpine-slim.eif --measurements out/measurements.json"
