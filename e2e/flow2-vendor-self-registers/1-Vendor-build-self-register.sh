#!/usr/bin/env bash
# Vendor — Auditor 없이 직접 빌드해 띄우고, 그 값을 자기 등록부에 올린다 (흐름 2). 여기, 내 컴퓨터에서 실행 —
#    빌드만 Vendor 인스턴스 안에서 자동으로 접속해 실행된다.
#    실행: bash "e2e/flow2-vendor-self-registers/1-Vendor-build-self-register.sh" [-c 0x컨트랙트]
here="$(dirname "$0")"
. "$here/../env.sh"

echo "== Vendor 인스턴스로 접속 중..."
nitroBanner "Vendor"
V './_remote-vendor-build-source.sh' || exit 1
localBanner

bash "$here/_vendor-self-register.sh" "$@"
rc=$?
echo "next: 2-User-verify"
exit $rc
