#!/usr/bin/env bash
# 조작 · User — Vendor 의 등록부와 Auditor 의 등록부를 나란히 본다 (설계 흐름 2 ②의 21~23).
#    flow1 의 3-User-verify 에 등록부 둘을 준 것: e2e-work/contract-flow2.txt (Vendor) + e2e-work/contract.txt (Auditor).
#    Vendor 가 바꿔 띄운 값을 자기 등록부에만 올렸다면: Vendor 등록부 ✓ / Auditor 등록부 ✗ nobody → not trusted — only 1 of 2 registries vouch.
#    실행: bash "e2e/flow2-vendor-self-registers/7-User-tamper-verify-both-registries.sh"
. "$(dirname "$0")/../env.sh"
VREG=$(cat "$WORK/contract-flow2.txt"); AREG=$(cat "$WORK/contract.txt")
echo "Vendor's registry  $VREG"; echo "Auditor's registry $AREG"
exec bash "$(dirname "$0")/../flow1-auditor-registers/3-User-verify.sh" "$VREG,$AREG"
