#!/bin/bash
# Vendor — Auditor 없이 직접 빌드해 띄운다 (흐름 2). 다음은 1-Vendor-build-self-register 가 자동으로 이어서 등록한다.
# 인스턴스 안에서 실제로 도는 내용 — 직접 실행하지 말고 1-Vendor-build-source 로 실행할 것. 결과: out/alpine-slim.eif, out/measurements.json, 엔클레이브 + TCP 5000
set -euo pipefail
SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim
umask 022

# git 은 실행 비트만 저장하고 실제 모드는 체크아웃한 쉘의 umask 를 따른다 (실측: umask 002 → 775, 022 → 755). docker COPY 는 그
# 모드를 그대로 이미지에 넣어 PCR 이 갈린다. 인덱스의 100755/100644 대로 755/644 로 맞춰 어느 쉘에서 받았든 같게 한다.
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }

echo "== source $SRC_REPO @ $SRC_COMMIT ($SRC_DIR)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx

echo "== tb-vendor build: docker build → responder layer → normalize → EIF + measurements, twice, checking both give the same value"
echo "   (내가 이 소스로 등록하려면 내 값도 재현 가능해야 한다 — Auditor 한테만 강요할 게 아니다)"
./tb-vendor build --app-dir src/docker-nginx/$SRC_DIR --out out --verify

echo "== tb-vendor launch"
./tb-vendor launch --eif out/alpine-slim.eif

echo
echo "built and running. next: 2-User-verify"
cat out/measurements.json
