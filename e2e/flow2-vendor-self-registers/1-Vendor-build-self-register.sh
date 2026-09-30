#!/usr/bin/env bash
# Vendor — Auditor 없이 나 혼자 소스를 빌드해 띄우고, 내가 만든 값을 내 키로 체인에 등록한다.
#    여기, 내 컴퓨터에서 실행 — 빌드만 Vendor 인스턴스 안에서 자동으로 접속해 실행된다.
#    실행: bash "e2e/flow2-vendor-self-registers/1-Vendor-build-self-register.sh" [-c 0x컨트랙트]
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"

# 준비 — 인스턴스에 올린 tb 바이너리는 이 컴퓨터의 Go 로 크로스 빌드했고(0-Setup 이 고정된 Go 가 아니면 거절한다),
#   그 Go 가 tb-ra-responder 를 컴파일했다. build-config.json 의 build_env.go 에 적을 값은 그래서 이 컴퓨터의 Go 버전이다.
GOVER=$(go env GOVERSION)

nitroBanner "Vendor"
V "SERVICE=$SERVICE VERSION=$VERSION GOVER=$GOVER bash -s" <<'REMOTE'
set -euo pipefail
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; shift; "$@"; }
SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim
umask 022

# 준비 — 테스트용 소스를 받고, 파일 모드를 소스 저장소(git 인덱스)에 적힌 대로 맞춘다 (어느 쉘에서 받았든 같게).
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"

# 준비 — Vendor 가 Auditor 에게 send 로 넘기는 것과 같은 모양의 build-config.json 을 clone 루트에 적는다 (이미 있으면 그대로).
#   아래 self-register 가 --config 로 읽어 service · version · arch 를 다시 타이핑하지 않게. 앱 디렉터리($SRC_DIR) 밖이라
#   측정값에는 들어가지 않는다. tee_info 는 인스턴스 메타데이터에서 (못 읽으면 unknown).
step_start "WRITING BUILD CONFIG"
if [ -f src/docker-nginx/build-config.json ]; then
  echo "build-config.json already in the clone root — keeping it"
else
  case "$(uname -m)" in x86_64) GOARCH=amd64;; aarch64|arm64) GOARCH=arm64;; *) GOARCH=unknown;; esac
  TOK=$(curl -s -m 2 -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' 2>/dev/null || true)
  md() { curl -s -m 2 -H "X-aws-ec2-metadata-token: $TOK" "http://169.254.169.254/latest/meta-data/$1" 2>/dev/null || echo unknown; }
  cat > src/docker-nginx/build-config.json <<EOF
{
  "schema": "tb.build-config/1",
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws",
  "source": { "repo": "$SRC_REPO", "commit": "$SRC_COMMIT", "path": "$SRC_DIR" },
  "tee_info": { "instance_id": "$(md instance-id)", "region": "$(md placement/region)", "ami_id": "$(md ami-id)" },
  "build_env": { "os": "linux", "arch": "$GOARCH", "go": "$GOVER", "trimpath": true }
}
EOF
fi
cat src/docker-nginx/build-config.json
step_stop "WRITING BUILD CONFIG"

# ▶ 우리가 만든 tb 명령 — 소스를 두 번 빌드해서(--verify) 이미지+측정값을 만들고, 같은 값이 나오는지 스스로 확인한다.
#   Vendor 도 Auditor 처럼 자기 빌드가 재현 가능한지 확인하고 등록해야 한다.
step "BUILDING SOURCE (checking reproducibility)" ./tb-vendor build --app-dir src/docker-nginx/$SRC_DIR --out out --verify

# ▶ 우리가 만든 tb 명령 — 그 이미지를 내 TEE 에 띄운다.
step "LAUNCHING ENCLAVE" ./tb-vendor launch --eif out/alpine-slim.eif

echo
cat out/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# 준비 — 옵션 파싱
CONTRACT=""
while getopts "c:" o; do case $o in c) CONTRACT=$OPTARG;; esac; done

# 준비 — Vendor 인스턴스에서 방금 만든 측정값 파일과 clone 루트의 build-config.json 을 내 컴퓨터로 가져온다.
#   self-register 는 service · version · arch 를 build-config.json 에서 읽는다 (같은 소스라도 x86_64 와 arm64 는 PCR 이
#   다르니, 어느 아키텍처의 값인지도 거기 적혀 있다 — 인스턴스에서 uname -m 을 GOARCH 철자로 바꿔 적었다).
step_start "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$VENDOR_HOST:out/measurements.json" "$WORK/vendor-build.json"
$SCP "$SSH_USER@$VENDOR_HOST:src/docker-nginx/build-config.json" "$WORK/vendor-build-config.json"
step_stop "FETCHING BUILD VALUE"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"
echo "build config:"; cat "$WORK/vendor-build-config.json"

if [ -z "$CONTRACT" ]; then
  # ▶ 우리가 만든 tb 명령 — 등록부(레지스트리 컨트랙트)를 체인에 새로 배포한다. 승인자는 Vendor 자기 자신.
  step_start "DEPLOYING REGISTRY"
  out=$("$TBV" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$VENDOR_ADDR" 2>&1)
  step_stop "DEPLOYING REGISTRY"
  echo "$out"
  CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # 준비 — 출력에서 주소 뽑기
  [ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
fi
echo "$CONTRACT" > "$WORK/contract-flow2.txt"  # 준비 — 다음 단계가 읽을 파일에 남기기

# ▶ 우리가 만든 tb 명령 — 내가 빌드한 값을 내 키로 서명해서 그대로 체인에 등록한다 (제3자 대조 없이).
#   service · version · arch 는 --config 의 build-config.json 에서 온다 — 플래그로 다른 값을 주면 그게 이기되 경고가 찍힌다.
step "SELF-REGISTERING" "$TBV" self-register --config "$WORK/vendor-build-config.json" --tee "$VENDOR_HOST:5000" --measurements "$WORK/vendor-build.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$VENDOR_KEY"
echo "self-register exit=$?"
# ▶ 우리가 만든 tb 명령 — User 입장에서 방금 등록한 게 실제로 믿을 만한지 검증해 본다.
step "VERIFYING" "$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
echo "verify exit=$?"
echo "next: 2-User-verify"
