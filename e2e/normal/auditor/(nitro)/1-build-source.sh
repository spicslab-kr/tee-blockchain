#!/bin/bash
# Auditor — Vendor 가 알려 준 소스(레포·커밋)를 내가 빌드해서 이미지와 측정값을 만들고, 내 TEE 에 띄운다 (설계 흐름 1 의 2).
# 📍 Auditor 인스턴스, ~ 에서 ./1-build-source.sh
# 결과: out/alpine-slim.eif, out/measurements.json (이 둘이 정본 — 등록도, Vendor 에게 주는 것도 이것), 엔클레이브 + TCP 5000
# 재현빌드가 필요 없다: 이미지는 하나뿐이고 그걸 만든 게 나다. "소스 = 이미지" 를 내가 보증한다.
set -euo pipefail
SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim
umask 022
# git 은 실행 비트만 저장하고 실제 모드는 체크아웃한 쉘의 umask 를 따른다. docker COPY 는 그 모드를 이미지에 넣으니 인덱스대로 맞춘다.
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }

echo "== source from the Vendor: $SRC_REPO @ $SRC_COMMIT ($SRC_DIR)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx

echo "== tb-auditor build: docker build → responder layer → repack → EIF + measurements   (this image is the one that gets registered and handed to the Vendor)"
./tb-auditor build --app-dir src/docker-nginx/$SRC_DIR --out out

echo "== tb-auditor launch: my own enclave, to attest what I built"
./tb-auditor launch --eif out/alpine-slim.eif

echo
echo "built and running. next (Auditor, my computer): 2-audit  then  3-handoff"
cat out/measurements.json
