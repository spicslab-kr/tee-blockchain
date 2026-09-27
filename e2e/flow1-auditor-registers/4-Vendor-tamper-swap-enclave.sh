#!/usr/bin/env bash
# 조작 · Vendor — Auditor 가 준 이미지 대신 다른 이미지를 띄운다. 여기, 내 컴퓨터에서 실행 — 실제 교체는
#    Vendor 인스턴스 안에서 일어나고, 여기서 자동으로 그 인스턴스에 접속해 대신 실행해 준다. (정상 흐름 2-Vendor-launch-check-record 까지 끝난 뒤)
#    다음: 5-User-tamper-verify   → ✗ nobody … not trusted
#    실행: bash "e2e/flow1-auditor-registers/4-Vendor-tamper-swap-enclave.sh"
. "$(dirname "$0")/../env.sh"
echo "== Vendor 인스턴스로 접속 중..."
nitroBanner "Vendor"
V './_remote-vendor-tamper-swap-enclave-flow1.sh'
rc=$?
localBanner
echo "next: 5-User-tamper-verify"
exit $rc
