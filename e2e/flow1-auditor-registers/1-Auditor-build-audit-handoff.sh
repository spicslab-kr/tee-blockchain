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

nitroBanner "Auditor"
A 'bash -s' <<'REMOTE'
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

# 준비 — Auditor 인스턴스에서 방금 만든 측정값 파일을 내 컴퓨터로 가져온다.
step_start "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
step_stop "FETCHING BUILD VALUE"
echo "Auditor's own value:"; cat "$WORK/auditor-measured.json"

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
extra=(); [ -n "$CHECK_VENDOR" ] && extra=(--vendor-tee "$VENDOR_HOST:5000")
step "AUDITING" "$TBA" audit --tee "$AUDITOR_HOST:5000" --measurements "$WORK/auditor-measured.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$AUDITOR_KEY" --service "$SERVICE" --version "$VERSION" "${extra[@]}"
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
