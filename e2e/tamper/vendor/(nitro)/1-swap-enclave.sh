#!/bin/bash
# 조작 데모 — Vendor 가 Auditor 가 준 이미지 대신 다른 이미지를 띄우면 User(와 Auditor) 가 잡는다.
# 📍 Vendor 인스턴스, ~ 에서 ./1-swap-enclave.sh   (정상 흐름 4-launch-received 까지 끝난 뒤)
# 작은 이미지 하나를 직접 만들어(= Auditor 가 모르는 값) 그것으로 엔클레이브를 바꿔 띄운다. 체인에는 Auditor 가 등록한 값만 있으니
#   User:    normal/user/(local)/…/6-verify                       → ✗ nobody … not trusted
#   Auditor: tamper/auditor/(local)/…/3-audit-check-vendor          → ✗ the Vendor's enclave runs something else
# 되돌리기: ./tb-vendor launch --eif from-auditor/alpine-slim.eif --measurements from-auditor/measurements.json
set -euo pipefail
umask 022
mkdir -p tampered
cat > tampered/Dockerfile <<'EOF'
FROM alpine:3.20
RUN echo "not the audited image" > /etc/tb-tampered
CMD ["sleep", "1000000"]
EOF
echo "== building my own image (the Auditor never saw this one)"
sudo docker build -q -t tampered:latest tampered
echo "== packing and launching it in place of the Auditor's image"
./tb-vendor pack --image tampered:latest --name tampered --out out-tampered
# 한 인스턴스의 엔클레이브 자원(2 cpu · 512 MiB)은 하나 분량이라, 먼저 내리고 바꿔 띄운다
sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x tb-vendor || true
./tb-vendor launch --eif out-tampered/tampered.eif
echo
echo "the enclave on :5000 now runs my own image, not what the Auditor registered."
echo "audited value:   $(grep PCR0 from-auditor/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the audited one back:  ./tb-vendor launch --eif from-auditor/alpine-slim.eif --measurements from-auditor/measurements.json"
