#!/usr/bin/env bash
# User (아무 컴퓨터, 키 없음) — Vendor 엔클레이브에 RA → 그 측정값으로 체인을 찾아 → 등록과 대조.
#    실행: bash "e2e/flow1-auditor-registers/3-User-verify.sh" [컨트랙트]      생략하면 e2e-work/contract.txt
#    등록부 여러 개를 나란히 보려면 쉼표로: 3-User-verify.sh 0xVendor등록부,0xAuditor등록부 — 전부가 보증해야 trusted
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
CONTRACT=${1:-$(cat "$WORK/contract.txt")}
# ▶ 우리가 만든 tb 명령 — 이게 이 스크립트의 전부다: RA 로 값 확인 → 체인에서 등록 조회 → 대조 → 믿을지 판단.
step "VERIFYING"
"$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
