#!/usr/bin/env bash
# 조작 · User — Vendor 의 등록부와 Auditor 의 등록부를 나란히 본다.
#    e2e-work/contract-flow2.txt (Vendor) + e2e-work/contract.txt (Auditor) 둘 다 --contract 로 준다.
#    Vendor 가 바꿔 띄운 값을 자기 등록부에만 올렸다면: Vendor 등록부 ✓ / Auditor 등록부 ✗ nobody → not trusted — only 1 of 2 registries vouch.
#    실행: bash "e2e/flow2-vendor-self-registers/5-User-tamper-verify-both-registries.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
VREG=$(cat "$WORK/contract-flow2.txt"); AREG=$(cat "$WORK/contract.txt")
echo "Vendor's registry  $VREG"
echo "Auditor's registry $AREG"
# ▶ 우리가 만든 tb 명령 — 등록부 두 개를 콤마로 같이 주면, 둘 다 보증해야 trusted 로 판정한다.
"$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$VREG,$AREG"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
