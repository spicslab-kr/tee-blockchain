#!/bin/bash
# 데모 — Vendor 가 감사받은 이미지 대신 다른 이미지를 띄우면 User(와 Auditor) 가 잡는다.
# 📍 Vendor 인스턴스, ~ 에서 ./2-swap-enclave.sh   (audit 까지 끝난 뒤)
# nginx:base 에 파일 하나를 더한 이미지(= 다른 PCR)를 pack 해서 그것으로 엔클레이브를 바꿔 띄운다. 체인에는 원래 값만 있으니
#   User:    .\e2e\normal\user\(local)\windows\5-verify.ps1  (또는 tamper\user\(local)\windows\5-verify-both-registries) → ✗ nobody … not trusted
#   Auditor: .\e2e\tamper\auditor\(local)\windows\4-audit-check-vendor.ps1     → ✗ the Vendor's enclave runs something else
# 되돌리기: ./tb-vendor launch --eif out/nginx.eif
set -euo pipefail
umask 022
mkdir -p tampered
cat > tampered/Dockerfile <<'EOF'
FROM nginx:base
RUN echo "not the audited image" > /etc/tb-tampered
EOF
echo "== building a different image on top of nginx:base (one extra file → different PCR0/PCR2)"
sudo docker build -q -t nginx:tampered tampered
echo "== packing and launching it in place of the audited enclave"
./tb-vendor pack --image nginx:tampered --name nginx-tampered --out out-tampered
# 한 인스턴스의 엔클레이브 자원(2 cpu · 512 MiB)은 하나 분량이라, 감사받은 엔클레이브를 먼저 내리고 바꿔 띄운다
sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x tb-vendor || true
./tb-vendor launch --eif out-tampered/nginx-tampered.eif
echo
echo "the enclave on :5000 now runs nginx:tampered, not what the Auditor registered."
echo "audited value:   $(grep PCR0 out/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the audited one back:  ./tb-vendor launch --eif out/nginx.eif"
