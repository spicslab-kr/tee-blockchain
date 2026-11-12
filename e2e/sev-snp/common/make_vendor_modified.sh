#!/bin/bash
# 공급자 변조 시험용: 응답기를 복사해 모든 답 끝에 " [vendor-modified]" 를 붙인 바이너리를 만든다.
# 서명은 모두 정상으로 나오지만 initrd 해시가 달라져서 User 검증이 거부해야 한다.
# 사용법 (저장소 루트에서): e2e/sev-snp/common/make_vendor_modified.sh gcp|aws
set -e
case "$1" in gcp) OUT=snp-aproxy-evil ;; aws) OUT=snp-aproxy-aws-evil ;; *) echo "사용법: $0 gcp|aws"; exit 1 ;; esac
cd "$(dirname "$0")/../$1"
rm -rf modified && mkdir modified
sed 's/return out.Response, nil/return out.Response + " [vendor-modified]", nil/' responder/main.go > modified/main.go
cp responder/status.go modified/
grep -q 'vendor-modified' modified/main.go
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT" ./modified
rm -rf modified
echo "만듦: e2e/sev-snp/$1/$OUT"
