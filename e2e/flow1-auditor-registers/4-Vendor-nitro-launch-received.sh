#!/bin/bash
# Vendor — Auditor 가 빌드해 준 이미지를 그대로 띄운다 (설계 흐름 1 의 10). 나는 빌드하지 않는다.
# 📍 Vendor 인스턴스, ~ 에서 ./4-Vendor-nitro-launch-received.sh     입력: from-auditor/alpine-slim.eif, from-auditor/measurements.json (Auditor 의 3-Auditor-local-handoff 가 올려 둔다)
# launch --measurements: 받은 EIF 를 직접 재서 Auditor 의 measurements.json 과 같을 때만 띄운다 (전송 중 바뀌지 않았나) → TCP 5000 → 자기 attestation
set -euo pipefail
test -f from-auditor/alpine-slim.eif -a -f from-auditor/measurements.json || { echo "from-auditor/alpine-slim.eif and measurements.json are missing — the Auditor runs 3-Auditor-local-handoff first"; exit 1; }

echo "== received from the Auditor"
sha256sum from-auditor/alpine-slim.eif from-auditor/measurements.json

echo "== tb-vendor launch: measure the received EIF → must equal the Auditor's JSON → enclave → bridge → self attestation"
./tb-vendor launch --eif from-auditor/alpine-slim.eif --measurements from-auditor/measurements.json

echo
echo "running the Auditor's image. next (Vendor, my computer): 5-Vendor-local-check-record"
