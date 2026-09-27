#!/bin/bash
# Auditor — 받은 EIF 를 직접 재서 Vendor 의 JSON 과 대조하고, 같을 때만 내 TEE 에 띄운다.
# 📍 Auditor 인스턴스, ~ 에서 ./check-received-image.nitro.sh     입력: from-vendor/nginx.eif, from-vendor/measurements.json (노트북이 올려 둔다)
# 결과: out/measurements.json (= 내가 직접 잰 값 → audit --measurements 에 쓴다), 엔클레이브 + TCP 5000
set -euo pipefail
test -f from-vendor/nginx.eif -a -f from-vendor/measurements.json || { echo "from-vendor/nginx.eif and measurements.json are missing — the Vendor runs e2e/vendor/handoff.windows.ps1 first"; exit 1; }

echo "== received files"
sha256sum from-vendor/nginx.eif from-vendor/measurements.json

echo "== tb-auditor launch: measure the received EIF → must equal Vendor's JSON → enclave → bridge → self attestation"
./tb-auditor launch --eif from-vendor/nginx.eif --measurements from-vendor/measurements.json

echo
echo "my measured value (for audit --measurements):"
cat out/measurements.json
