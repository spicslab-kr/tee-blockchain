#!/usr/bin/env bash
# 준비 — 바이너리를 리눅스용으로 빌드해 두 인스턴스에 올린다 (누가 하든 한 번). 자기 컴퓨터용 바이너리는 e2e-work/bin 에.
#    스크립트는 안 올린다 — 모든 단계가 SSH 로 그 자리에서 직접 실행되므로, 인스턴스엔 바이너리만 있으면 된다.
#    실행: TB-live 에서  bash "e2e/0-Setup-local-upload.sh"
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

# ▶ 우리가 만든 tb 명령의 실행 파일 그 자체 — 리눅스용(인스턴스에 올릴 것)과 내 컴퓨터용을 둘 다 빌드한다.
step "BUILDING BINARIES"
GOOS=linux GOARCH=amd64 go build -o tb-vendor ./cmd/vendor
GOOS=linux GOARCH=amd64 go build -o tb-auditor ./cmd/auditor
GOOS=linux GOARCH=amd64 go build -o tb-ra-responder ./cmd/ra-responder
mkdir -p "$BIN"
go build -o "$TBV" ./cmd/vendor
go build -o "$TBA" ./cmd/auditor
go build -o "$TBU" ./cmd/user
echo "responder sha256: $(sha256 tb-ra-responder | cut -c1-64)"  # 준비 — 두 인스턴스에 같은 바이너리가 올라갔는지 나중에 대조용

# 준비 — 떠 있던 ra-proxy 가 옛 바이너리를 잡고 있으면 덮어쓸 수 없다. 엔클레이브와 함께 먼저 내린다.
step "STOPPING OLD ENCLAVES"
V 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-vendor; true'
A 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-auditor; true'
step "UPLOADING BINARIES"
$SCP tb-vendor tb-ra-responder "$SSH_USER@$VENDOR_HOST:~/"
$SCP tb-auditor tb-ra-responder "$SSH_USER@$AUDITOR_HOST:~/"
V 'chmod +x tb-vendor tb-ra-responder; sha256sum tb-ra-responder'
A 'chmod +x tb-auditor tb-ra-responder; sha256sum tb-ra-responder'
echo "uploaded. next: bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh   (내 컴퓨터에서. 자동으로 Auditor 인스턴스에 접속해 빌드한다)"
