#!/usr/bin/env bash
# 흐름 2 · User — Vendor 가 자기 등록부에 올린 것을 검증한다. 등록부는 e2e-work/contract-flow2.txt.
#    통과해도 서명자가 Vendor 자신이다 — "Vendor 가 자기 것을 보증했다" 까지. Auditor 가 들어오는 3~5 가 그 뒤 이야기.
#    실행: bash "e2e/flow2-vendor-self-registers/2-User-verify.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
CONTRACT=$(cat e2e-work/contract-flow2.txt)
# ▶ 우리가 만든 tb 명령 — RA 로 값 확인 → 체인에서 등록 조회 → 대조 → 믿을지 판단.
step "VERIFYING" "$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
rc=$?
echo "verify exit=$rc  (0 = trusted, 1 = nothing registered or nothing matches)"
exit $rc
