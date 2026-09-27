#!/usr/bin/env bash
# Auditor — 소스를 재현빌드해 자기 TEE 에 띄우고, 대조해서 체인에 등록하고, 그 이미지와 기록을 Vendor 에게 넘긴다
#    (설계 흐름 1 의 2~9). 여기, 내 컴퓨터에서 실행 — 빌드만 Auditor 인스턴스 안에서 자동으로 접속해 실행된다.
#    실행: bash "e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh" [-c 0x컨트랙트] [-v]
#      -c  기존 등록부에 등록 (컨트랙트 주소는 e2e-work/contract.txt 에 남는다)
#      -v  audit 할 때 Vendor 엔클레이브에도 물어본다 (흐름 2 의 8·9·15, 조작 감사용)
set -uo pipefail
here="$(dirname "$0")"
. "$here/../env.sh"

echo "== Auditor 인스턴스로 접속 중..."
nitroBanner "Auditor"
A './_remote-auditor-build-source.sh' || exit 1
localBanner

bash "$here/_auditor-audit.sh" "$@" || exit 1
bash "$here/_auditor-handoff.sh" || exit 1
echo "next: 2-Vendor-launch-check-record"
