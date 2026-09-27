#!/usr/bin/env bash
# 조작 · Vendor — 바꿔 띄운 이미지(2-swap-enclave)의 값을 자기 등록부에 등록한다 (설계 흐름 2 ②의 "A 여야 하는데 B 를 등록").
#    정상 흐름의 8-self-register 를 out-tampered 값으로 돌리는 것. 컨트랙트는 e2e-work/contract-flow2.txt 에 남는다.
#    실행: bash "e2e/tamper/vendor/(local)/mac/3-self-register-swapped.sh"
exec bash "$(dirname "$0")/../../../../normal/vendor/(local)/mac/8-self-register.sh" -o out-tampered
