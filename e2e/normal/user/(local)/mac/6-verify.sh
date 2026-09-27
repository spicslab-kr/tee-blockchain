#!/usr/bin/env bash
# User (아무 컴퓨터, 키 없음) — Vendor 엔클레이브에 RA → 그 측정값으로 체인을 찾아 → 등록과 대조.
#    실행: bash "e2e/normal/user/(local)/mac/6-verify.sh" [컨트랙트]      생략하면 e2e-work/contract.txt
#    등록부 여러 개를 나란히 보려면 쉼표로: 6-verify.sh 0xVendor등록부,0xAuditor등록부 — 전부가 보증해야 trusted
. "$(dirname "$0")/../../../../setup/(local)/mac/env.sh"
CONTRACT=${1:-$(cat "$WORK/contract.txt")}
"$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
