#!/usr/bin/env bash
# Auditor (자기 키가 있는 자기 컴퓨터에서) — Auditor 가 직접 얻은 값(out/measurements.json)을 가져와,
#    새 등록부를 깔고, audit: 자기 TEE 에 RA → 그 값과 대조 → 같을 때만 등록 → 자기확인.
#    실행: bash "e2e/normal/auditor/(local)/mac/4-audit.sh" [-c 0x컨트랙트] [-v]
#      -c  새로 깔지 않고 그 등록부에 등록 (운영: 한 번 깔고 계속 쓴다). 컨트랙트 주소는 e2e-work/contract.txt 에 남는다
#      -v  Vendor 의 엔클레이브에도 물어봐 내 값과 같은지 찍는다 (설계 흐름 2 의 8·9·15). 등록되는 값은 어차피 내 값
set -uo pipefail
. "$(dirname "$0")/../../../../setup/(local)/mac/env.sh"
CONTRACT=""; CHECK_VENDOR=""
while getopts "c:v" o; do case $o in c) CONTRACT=$OPTARG;; v) CHECK_VENDOR=1;; esac; done

$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
echo "Auditor's own value:"; cat "$WORK/auditor-measured.json"

if [ -z "$CONTRACT" ]; then
  echo "== registry-deploy (approver = Auditor $AUDITOR_ADDR)"
  out=$("$TBA" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$AUDITOR_ADDR" 2>&1); echo "$out"
  CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')
  [ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
fi
echo "$CONTRACT" > "$WORK/contract.txt"

extra=(); [ -n "$CHECK_VENDOR" ] && extra=(--vendor-tee "$VENDOR_HOST:5000")
echo "== audit (contract $CONTRACT)"
"$TBA" audit --tee "$AUDITOR_HOST:5000" --measurements "$WORK/auditor-measured.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$AUDITOR_KEY" --service "$SERVICE" --version "$VERSION" "${extra[@]}"
rc=$?
echo "audit exit=$rc   contract=$CONTRACT   next: bash \"e2e/normal/user/(local)/mac/5-verify.sh\""
exit $rc
