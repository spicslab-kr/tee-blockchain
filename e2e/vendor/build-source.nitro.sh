#!/bin/bash
# Vendor — 소스를 tb 가 빌드해 띄운다 (Auditor 가 같은 소스를 다시 빌드하는 흐름). 같은 명령으로 같은 값이 나와야 한다.
# 📍 Vendor 인스턴스, ~ 에서 ./build-source.nitro.sh     결과: out/alpine-slim.eif, out/measurements.json, 엔클레이브 + TCP 5000
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

echo "== tb-vendor build: docker build → responder layer → normalize → EIF + measurements  (add --verify to build twice)"
./tb-vendor build --app-dir src/docker-nginx/$SRC_DIR --out out

echo "== tb-vendor launch"
./tb-vendor launch --eif out/alpine-slim.eif

echo
echo "tell the Auditor:  $SRC_REPO @ $SRC_COMMIT  $SRC_DIR   — and expect these values from its own build:"
cat out/measurements.json
