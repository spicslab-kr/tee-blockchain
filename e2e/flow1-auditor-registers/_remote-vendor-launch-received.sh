#!/bin/bash
# Vendor — Auditor 가 빌드해 준 이미지를 그대로 띄운다 (설계 흐름 1 의 10). 나는 빌드하지 않는다.
# 인스턴스 안에서 실제로 도는 내용 — 직접 실행하지 말고 2-Vendor-launch-check-record 로 실행할 것. 입력: from-auditor/alpine-slim.eif, from-auditor/measurements.json (Auditor 의 1-Auditor-build-audit-handoff 가 올려 둔다)
# launch --measurements: 받은 EIF 를 직접 재서 Auditor 의 measurements.json 과 같을 때만 띄운다 (전송 중 바뀌지 않았나) → TCP 5000 → 자기 attestation
set -euo pipefail
test -f from-auditor/alpine-slim.eif -a -f from-auditor/measurements.json || { echo "from-auditor/alpine-slim.eif and measurements.json are missing — the Auditor runs 1-Auditor-build-audit-handoff first"; exit 1; }

echo "== received from the Auditor"
sha256sum from-auditor/alpine-slim.eif from-auditor/measurements.json

echo "== tb-vendor launch: measure the received EIF → must equal the Auditor's JSON → enclave → bridge → self attestation"
./tb-vendor launch --eif from-auditor/alpine-slim.eif --measurements from-auditor/measurements.json

echo
echo "running the Auditor's image. next: 자동으로 이어서 check-record 까지 함 (2-Vendor-launch-check-record)"
