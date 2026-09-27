#!/usr/bin/env bash
# Vendor — Auditor 없이 나 혼자 소스를 빌드해 띄우고, 내가 만든 값을 내 키로 체인에 등록한다.
#    여기, 내 컴퓨터에서 실행 — 빌드만 Vendor 인스턴스 안에서 자동으로 접속해 실행된다.
#    실행: bash "e2e/flow2-vendor-self-registers/1-Vendor-build-self-register.sh" [-c 0x컨트랙트]
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"

echo "== Vendor 인스턴스로 접속 중..."
nitroBanner "Vendor"
V 'bash -s' <<'REMOTE'
set -euo pipefail
SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim
umask 022

# 준비 — 테스트용 소스를 받고, 파일 모드를 소스 저장소(git 인덱스)에 적힌 대로 맞춘다 (어느 쉘에서 받았든 같게).
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
echo "== source $SRC_REPO @ $SRC_COMMIT ($SRC_DIR)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx

# ▶ 우리가 만든 tb 명령 — 소스를 두 번 빌드해서(--verify) 이미지+측정값을 만들고, 같은 값이 나오는지 스스로 확인한다.
#   Vendor 도 Auditor 처럼 자기 빌드가 재현 가능한지 확인하고 등록해야 한다.
echo "== tb-vendor build --verify"
./tb-vendor build --app-dir src/docker-nginx/$SRC_DIR --out out --verify

# ▶ 우리가 만든 tb 명령 — 그 이미지를 내 TEE 에 띄운다.
echo "== tb-vendor launch"
./tb-vendor launch --eif out/alpine-slim.eif

echo
cat out/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# 준비 — 옵션 파싱
CONTRACT=""
while getopts "c:" o; do case $o in c) CONTRACT=$OPTARG;; esac; done

# 준비 — Vendor 인스턴스에서 방금 만든 측정값 파일을 내 컴퓨터로 가져온다.
$SCP "$SSH_USER@$VENDOR_HOST:out/measurements.json" "$WORK/vendor-build.json"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"

if [ -z "$CONTRACT" ]; then
  # ▶ 우리가 만든 tb 명령 — 등록부(레지스트리 컨트랙트)를 체인에 새로 배포한다. 승인자는 Vendor 자기 자신.
  echo "== registry-deploy (approver = Vendor itself $VENDOR_ADDR)"
  out=$("$TBV" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$VENDOR_ADDR" 2>&1); echo "$out"
  CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # 준비 — 출력에서 주소 뽑기
  [ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
fi
echo "$CONTRACT" > "$WORK/contract-flow2.txt"  # 준비 — 다음 단계가 읽을 파일에 남기기

# ▶ 우리가 만든 tb 명령 — 내가 빌드한 값을 내 키로 서명해서 그대로 체인에 등록한다 (제3자 대조 없이).
echo "== self-register (contract $CONTRACT)"
"$TBV" self-register --tee "$VENDOR_HOST:5000" --measurements "$WORK/vendor-build.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$VENDOR_KEY" --service "$SERVICE" --version "$VERSION"
echo "self-register exit=$?"
# ▶ 우리가 만든 tb 명령 — User 입장에서 방금 등록한 게 실제로 믿을 만한지 검증해 본다.
echo "== verify (User) — the signer will be the Vendor itself"
"$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
echo "verify exit=$?"
echo "next: 2-User-verify"
