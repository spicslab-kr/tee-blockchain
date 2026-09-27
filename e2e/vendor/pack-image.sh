#!/bin/bash
# Vendor — 내가 만든 docker 이미지를 tb 가 EIF 로 포장해 띄운다 (이미지를 파일로 넘기는 흐름).  📍 Vendor 인스턴스, ~ 에서 ./pack-image.sh
# 결과: out/nginx.eif, out/measurements.json (이 둘을 Auditor 에게 파일로 넘긴다), 엔클레이브 + TCP 5000
set -euo pipefail
SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim
umask 022

# git 은 실행 비트만 저장하고 실제 모드는 체크아웃한 쉘의 umask 를 따른다 (실측: umask 002 → 775, 022 → 755). docker COPY 는 그
# 모드를 그대로 이미지에 넣어 PCR 이 갈린다. 인덱스의 100755/100644 대로 755/644 로 맞춰 어느 쉘에서 받았든 같게 한다.
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
IMAGE=nginx:base

echo "== source $SRC_REPO @ $SRC_COMMIT ($SRC_DIR)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx

echo "== docker build (my way) → $IMAGE"
sudo docker build -q -t $IMAGE src/docker-nginx/$SRC_DIR

echo "== tb-vendor pack: responder layer → normalize → EIF + measurements"
./tb-vendor pack --image $IMAGE --out out

echo "== tb-vendor launch: enclave → TCP 5000 bridge → self attestation"
./tb-vendor launch --eif out/nginx.eif

echo
echo "hand these to the Auditor:  out/nginx.eif  out/measurements.json"
cat out/measurements.json
