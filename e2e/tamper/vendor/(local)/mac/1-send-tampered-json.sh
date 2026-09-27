#!/usr/bin/env bash
# 조작 데모 — Vendor 가 measurements.json 을 조작해서 보내면 Auditor 가 잡는다.
#    2-handoff 와 같지만 PCR0 의 첫 글자를 바꿔 보낸다. 그리고 Auditor 인스턴스에서 3-check-received-image.sh 를 돌려
#    빨간 ✗ (it does NOT measure to what … says — not running it) 로 거부되는 것을 보인다. 엔클레이브는 뜨지 않는다.
#    실행: bash "e2e/tamper/vendor/(local)/mac/1-send-tampered-json.sh"     (1-pack-image.sh 를 Vendor 인스턴스에서 먼저)
set -uo pipefail
. "$(dirname "$0")/../../../../setup/(local)/mac/env.sh"
mkdir -p "$WORK/tampered"
$SCP "$SSH_USER@$VENDOR_HOST:out/nginx.eif" "$WORK/tampered/nginx.eif"
$SCP "$SSH_USER@$VENDOR_HOST:out/measurements.json" "$WORK/tampered/measurements.json"
real=$(grep -oE '"PCR0": *"[0-9a-fA-F]+"' "$WORK/tampered/measurements.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/')
first=${real:0:1}; [ "$first" = "0" ] && new=1 || new=0
fake="$new${real:1}"
sed -i.bak "s/$real/$fake/" "$WORK/tampered/measurements.json" && rm -f "$WORK/tampered/measurements.json.bak"
echo "the Vendor claims PCR0 = $fake"
echo "the EIF really is   PCR0 = $real"

A 'mkdir -p from-vendor'
$SCP "$WORK/tampered/nginx.eif" "$SSH_USER@$AUDITOR_HOST:from-vendor/nginx.eif"
$SCP "$WORK/tampered/measurements.json" "$SSH_USER@$AUDITOR_HOST:from-vendor/measurements.json"
echo "handed the tampered pair to the Auditor. now the Auditor checks it:"; echo
A './3-check-received-image.sh'
echo "exit=$?  (expected: not 0 — the Auditor measured the file itself and refused to run it)"
