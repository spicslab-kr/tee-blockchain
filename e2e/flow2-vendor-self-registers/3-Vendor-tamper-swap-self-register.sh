#!/usr/bin/env bash
# 조작 · Vendor — 자기가 빌드해 등록한 이미지 대신 다른 이미지를 띄우고, 그 바뀐 값도 자기 등록부에 올린다
#    (설계 흐름 2 ②의 시작: "A 여야 하는데 B 를 등록"). 여기, 내 컴퓨터에서 실행 — 교체만 Vendor 인스턴스 안에서
#    자동으로 접속해 실행된다. (1-Vendor-build-self-register 까지 끝난 뒤)
#    다음: 4-Auditor-tamper-audit-check-vendor → 5-User-tamper-verify-both-registries
#    실행: bash "e2e/flow2-vendor-self-registers/3-Vendor-tamper-swap-self-register.sh"
here="$(dirname "$0")"
. "$here/../env.sh"

echo "== Vendor 인스턴스로 접속 중..."
nitroBanner "Vendor"
V './_remote-vendor-tamper-swap-enclave-flow2.sh' || exit 1
localBanner

bash "$here/_vendor-self-register.sh" -o out-tampered
rc=$?
echo "next: 4-Auditor-tamper-audit-check-vendor"
exit $rc
