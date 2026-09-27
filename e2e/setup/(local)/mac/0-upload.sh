#!/usr/bin/env bash
# 준비 — 바이너리를 리눅스용으로 빌드해 두 인스턴스에 올린다 (누가 하든 한 번). 자기 컴퓨터용 바이너리는 e2e-work/bin 에.
#    실행: TB-live 에서  bash "e2e/setup/(local)/mac/0-upload.sh"
set -euo pipefail
. "$(dirname "$0")/env.sh"

GOOS=linux GOARCH=amd64 go build -o tb-vendor ./cmd/vendor
GOOS=linux GOARCH=amd64 go build -o tb-auditor ./cmd/auditor
GOOS=linux GOARCH=amd64 go build -o tb-ra-responder ./cmd/ra-responder
mkdir -p "$BIN"
go build -o "$TBV" ./cmd/vendor
go build -o "$TBA" ./cmd/auditor
go build -o "$TBU" ./cmd/user
echo "responder sha256: $(sha256 tb-ra-responder | cut -c1-64)"

# 떠 있던 ra-proxy 가 옛 바이너리를 잡고 있으면 덮어쓸 수 없다 — 엔클레이브와 함께 먼저 내린다
V 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-vendor; true'
A 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-auditor; true'
$SCP tb-vendor tb-ra-responder "e2e/normal/vendor/(nitro)/1-pack-image.sh" "e2e/normal/vendor/(nitro)/1-build-source.sh" "e2e/tamper/vendor/(nitro)/2-swap-enclave.sh" "$SSH_USER@$VENDOR_HOST:~/"
$SCP tb-auditor tb-ra-responder "e2e/normal/auditor/(nitro)/3-check-received-image.sh" "e2e/normal/auditor/(nitro)/3-rebuild-source.sh" "$SSH_USER@$AUDITOR_HOST:~/"
V 'chmod +x tb-vendor tb-ra-responder *.sh; sed -i "s/\r$//" *.sh; sha256sum tb-ra-responder'
A 'chmod +x tb-auditor tb-ra-responder *.sh; sed -i "s/\r$//" *.sh; sha256sum tb-ra-responder'
echo "uploaded. next: on the Vendor instance  ./1-pack-image.sh  (your own docker image)  or  ./1-build-source.sh  (tb builds the source)"
