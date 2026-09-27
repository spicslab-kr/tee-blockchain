#!/usr/bin/env bash
# 조작 · User — Vendor 가 바꿔 띄운 뒤(4-Vendor-tamper-swap-enclave) 똑같이 검증한다. RA 로 받은 측정값이 Auditor 가 등록한 값과 다르다.
#    정상 흐름의 3-User-verify 그대로 (등록부 e2e-work/contract.txt). 기대: 빨간 ✗ nobody — … never registered the value this enclave runs
#    → "nobody has registered these measurements on the chain — no basis for trust", exit 1.
#    실행: bash "e2e/flow1-auditor-registers/5-User-tamper-verify.sh"
echo "the Vendor's enclave now runs something the Auditor never registered — the same verify as step 6 must turn red:"
exec bash "$(dirname "$0")/3-User-verify.sh"
