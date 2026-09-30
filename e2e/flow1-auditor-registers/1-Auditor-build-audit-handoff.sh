#!/usr/bin/env bash
# Auditor — 소스를 빌드해서 내 엔클레이브에 띄우고, 빌드값과 실제로 돌아가는 값이 같은지 확인한 뒤 체인에 등록하고,
#    그 이미지와 기록을 Vendor 에게 넘긴다. 여기, 내 컴퓨터에서 실행 — 빌드만 Auditor 인스턴스 안에서 자동으로 접속해 실행된다.
#    실행: bash "e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh" [-c 0x컨트랙트] [-v] [0x키]
#      -c  기존 등록부에 등록 (컨트랙트 주소는 e2e-work/contract.txt 에 남는다)
#      -v  audit 할 때 Vendor 엔클레이브에도 물어본다 (Vendor 가 다른 이미지를 띄웠는지 조작 감사용)
#      0x키  handoff 가 쓸 등록 key. 생략하면 체인에서 measurement 로 찾는다
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"

# 준비 — 인스턴스에 올린 tb 바이너리는 이 컴퓨터의 Go 로 크로스 빌드했고(0-Setup 이 고정된 Go 가 아니면 거절한다),
#   그 Go 가 tb-ra-responder 를 컴파일했다. build-config.json 과 기록의 build_env.go 에 적을 값은 그래서 이 컴퓨터의 Go 버전이다.
GOVER=$(go env GOVERSION)

nitroBanner "Auditor"
A "SERVICE=$SERVICE VERSION=$VERSION GOVER=$GOVER bash -s" <<'REMOTE'
set -euo pipefail
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; shift; "$@"; }
SRC_REPO=https://github.com/qixiangme/docker-nginx
SRC_COMMIT=ef5a25a6314e652a2dcfbce6074bef729f4d9639
SRC_DIR=mainline/alpine-slim
umask 022

# 준비 — 테스트용 소스를 받는다. git 은 실행 비트만 저장해서, 체크아웃한 쉘의 umask 에 따라 파일 모드가 달라진다.
# 그러면 도커 이미지 안 파일 모드도 달라져 측정값이 흔들리니, 소스 저장소(git 인덱스)에 적힌 모드로 강제로 맞춘다.
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"

# 준비 — Vendor 가 send 로 넘겼다면 clone 루트에 build-config.json 이 있다 (receive 가 그걸 읽는다). 이 테스트 소스에는
#   없으니 같은 모양으로 하나 적는다 — 아래 audit 가 --config 로 읽어 service · version · arch 를 다시 타이핑하지 않게.
#   앱 디렉터리($SRC_DIR) 밖이라 측정값에는 들어가지 않는다. tee_info 는 인스턴스 메타데이터에서 (못 읽으면 unknown).
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

# ▶ 우리가 만든 tb 명령 — 소스를 도커로 빌드해서 엔클레이브 이미지(EIF)와 측정값(PCR0/1/2)을 만든다.
step "BUILDING SOURCE" ./tb-auditor build --app-dir src/docker-nginx/$SRC_DIR --out out

# ▶ 우리가 만든 tb 명령 — 그 이미지를 내 TEE 에 실제로 띄운다.
step "LAUNCHING ENCLAVE" ./tb-auditor launch --eif out/alpine-slim.eif

echo
cat out/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# 준비 — 옵션 파싱
CONTRACT=""; CHECK_VENDOR=""
while getopts "c:v" o; do case $o in c) CONTRACT=$OPTARG;; v) CHECK_VENDOR=1;; esac; done
shift $((OPTIND - 1))
KEY=${1:-}

# 준비 — Auditor 인스턴스에서 방금 만든 측정값 파일과 clone 루트의 build-config.json 을 내 컴퓨터로 가져온다.
#   audit 는 service · version · arch 를 build-config.json 에서 읽는다 (Vendor 가 이미 넘긴 값을 다시 타이핑하지 않는다).
#   아래 handoff 의 손으로 짜는 기록에는 아키텍처가 따로 필요하니, uname 철자(x86_64 · aarch64)를 tb 가 쓰는
#   GOARCH 철자(amd64 · arm64)로 바꿔 둔다 (같은 소스라도 x86_64 와 arm64 는 PCR 이 다르다).
step_start "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
$SCP "$SSH_USER@$AUDITOR_HOST:src/docker-nginx/build-config.json" "$WORK/auditor-build-config.json"
case "$(A 'uname -m' | tr -d '\r\n')" in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) ARCH="";; esac
[ -n "$ARCH" ] || { echo "cannot tell the Auditor instance's architecture from uname -m"; exit 1; }
step_stop "FETCHING BUILD VALUE"
echo "Auditor's own value (arch: $ARCH):"; cat "$WORK/auditor-measured.json"
echo "build config:"; cat "$WORK/auditor-build-config.json"

if [ -z "$CONTRACT" ]; then
  # ▶ 우리가 만든 tb 명령 — 등록부(레지스트리 컨트랙트)를 체인에 새로 배포한다.
  step_start "DEPLOYING REGISTRY"
  out=$("$TBA" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$AUDITOR_ADDR" 2>&1)
  step_stop "DEPLOYING REGISTRY"
  echo "$out"
  CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # 준비 — 출력 문장에서 주소만 뽑기
  [ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
fi
echo "$CONTRACT" > "$WORK/contract.txt"  # 준비 — 다음 단계가 읽을 파일에 남기기

# ▶ 우리가 만든 tb 명령 — 내 TEE 에 RA 를 보내 받은 값과 위에서 만든 빌드값이 같은지 대조하고, 같을 때만 체인에 등록한다.
#   service · version · arch 는 --config 의 build-config.json 에서 온다 — 플래그로 다른 값을 주면 그게 이기되 경고가 찍힌다.
extra=(); [ -n "$CHECK_VENDOR" ] && extra=(--vendor-tee "$VENDOR_HOST:5000")
step "AUDITING" "$TBA" audit --config "$WORK/auditor-build-config.json" --tee "$AUDITOR_HOST:5000" --measurements "$WORK/auditor-measured.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$AUDITOR_KEY" "${extra[@]}"
rc=$?
echo "audit exit=$rc   contract=$CONTRACT"
[ $rc -eq 0 ] || exit $rc

# 준비 — 여기부터는 handoff: 방금 만든 이미지·측정값과, 그게 어느 컨트랙트/키로 등록됐는지 적은 쪽지(record)를
# Vendor 에게 파일로 그대로 넘겨주는 것. tb 자체 기능이 아니라 우리가 손으로 짜는 전달 절차다.
step_start "TRANSFERRING TO VENDOR"
mkdir -p "$WORK/from-auditor"
$SCP "$SSH_USER@$AUDITOR_HOST:out/alpine-slim.eif" "$WORK/from-auditor/alpine-slim.eif"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/from-auditor/measurements.json"
if [ -z "$KEY" ]; then
  # ▶ 우리가 만든 tb 명령(안쪽) — 체인에서 이 측정값의 등록 key 를 조회한다.
  out=$("$TBA" lookup --rpc "$RPC" --contract "$CONTRACT" --measurements "$WORK/from-auditor/measurements.json" 2>&1)
  KEY=$(echo "$out" | grep -oE 'key +0x[0-9a-fA-F]{64}' | awk '{print $2}' | head -1)  # 준비 — 출력에서 key 뽑기
  [ -n "$KEY" ] || { step_stop "TRANSFERRING TO VENDOR"; echo "no registration of this value found on $CONTRACT"; exit 1; }
fi
pcr() { grep -oE "\"$1\": *\"[0-9a-fA-F]+\"" "$WORK/from-auditor/measurements.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/'; }  # 준비 — JSON 에서 PCR 값 꺼내는 헬퍼
cat > "$WORK/from-auditor/measurement-record.json" <<EOF
{
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws-nitro",
  "build_env": { "os": "linux", "arch": "$ARCH", "go": "$GOVER", "trimpath": true },
  "image": "alpine-slim.eif",
  "measurements": { "PCR0": "$(pcr PCR0)", "PCR1": "$(pcr PCR1)", "PCR2": "$(pcr PCR2)" },
  "registry": { "rpc": "$RPC", "contract": "$CONTRACT", "key": "$KEY", "approver": "$AUDITOR_ADDR" },
  "issued_by": "$AUDITOR_ADDR",
  "issued_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
(cd "$WORK/from-auditor" && sha256 alpine-slim.eif measurements.json | sed 's/^/  /')  # 준비 — 전달 파일 확인용 해시
V 'mkdir -p from-auditor'
$SCP "$WORK/from-auditor/alpine-slim.eif" "$SSH_USER@$VENDOR_HOST:from-auditor/alpine-slim.eif"
$SCP "$WORK/from-auditor/measurements.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurements.json"
$SCP "$WORK/from-auditor/measurement-record.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurement-record.json"
step_stop "TRANSFERRING TO VENDOR"
echo "measurement record:"; cat "$WORK/from-auditor/measurement-record.json"
echo "handed to the Vendor. next: 2-Vendor-launch-check-record"
