#!/usr/bin/env bash
# Vendor — 만든 EIF 를 Auditor 에게 파일로 넘긴다: Vendor 의 out/nginx.eif · out/measurements.json → 내 컴퓨터 → Auditor 의 from-vendor/
#    (인스턴스끼리 ssh 키가 없어서 내 컴퓨터를 거친다. 메일·카톡으로 보내도 같다.)
#    실행: bash "e2e/variants/vendor-builds/vendor/(local)/mac/2-handoff.sh"
set -euo pipefail
. "$(dirname "$0")/../../../../../setup/(local)/mac/env.sh"

mkdir -p "$WORK/from-vendor"
$SCP "$SSH_USER@$VENDOR_HOST:out/nginx.eif" "$WORK/from-vendor/nginx.eif"
$SCP "$SSH_USER@$VENDOR_HOST:out/measurements.json" "$WORK/from-vendor/measurements.json"
echo "from Vendor:"; (cd "$WORK/from-vendor" && sha256 nginx.eif measurements.json | sed 's/^/  /')

A 'mkdir -p from-vendor'
$SCP "$WORK/from-vendor/nginx.eif" "$SSH_USER@$AUDITOR_HOST:from-vendor/nginx.eif"
$SCP "$WORK/from-vendor/measurements.json" "$SSH_USER@$AUDITOR_HOST:from-vendor/measurements.json"
echo "handed to the Auditor. next: on the Auditor instance  ./3-check-received-image.sh"
