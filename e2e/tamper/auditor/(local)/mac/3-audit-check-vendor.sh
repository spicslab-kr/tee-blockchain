#!/usr/bin/env bash
# 조작 · Auditor — 내 값을 등록하면서 Vendor 의 엔클레이브에도 물어본다 (설계 흐름 2 ②의 8·9·15).
#    정상 흐름의 2-audit 에 -v 를 붙인 것. Vendor 가 다른 이미지를 띄웠으면
#    "✗ the Vendor's enclave runs something else" 가 찍히고, 그래도 내가 검증한 값은 등록된다.
#    실행: bash "e2e/tamper/auditor/(local)/mac/3-audit-check-vendor.sh" [-c 0x컨트랙트]
exec bash "$(dirname "$0")/../../../../normal/auditor/(local)/mac/2-audit.sh" -v "$@"
