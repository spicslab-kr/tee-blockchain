#!/usr/bin/env bash
# 조작 · Vendor — 바꿔 띄운 이미지(4-Vendor-nitro-tamper-swap-enclave)의 값을 자기 등록부에 등록한다 (설계 흐름 2 ②의 "A 여야 하는데 B 를 등록").
#    flow2 의 2-self-register 를 out-tampered 값으로 돌리는 것 (Vendor 인스턴스에서 4-Vendor-nitro-tamper-swap-enclave 뒤). 컨트랙트는 e2e-work/contract-flow2.txt 에 남는다.
#    실행: bash "e2e/flow2-vendor-self-registers/5-Vendor-local-tamper-self-register-swapped.sh"
exec bash "$(dirname "$0")/2-Vendor-local-self-register.sh" -o out-tampered
