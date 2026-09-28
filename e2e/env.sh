#!/usr/bin/env bash
# e2e/env.sh — 자기 컴퓨터(macOS · Linux) 쪽 스크립트가 읽는 값. 여기만 고친다. env.ps1 과 같은 값.
# 모든 단계는 여기, 자기 컴퓨터에서 실행한다. 인스턴스 안에서 실제 빌드·띄우기가 필요한 단계는 V()/A() 로 자동으로
# 접속해 그 스크립트를 SSH 로 직접 흘려 넣어 실행한다 (인스턴스에 파일로 올려 두지 않는다) — 직접 SSH 로 들어갈
# 필요는 없다. nitroBanner/localBanner 는 지금 어느 환경에서 실행 중인지 화면에 표시만 하는 용도
# (2초 멈춤, 보는 사람이 구분하기 쉽게).

VENDOR_HOST=13.61.175.104        # i-01f1ef6b04c10f37f
AUDITOR_HOST=13.60.50.242        # i-0200ce262d24549ea
SSH_USER=ec2-user
PEM=pem2.pem                     # TB-live 기준 상대경로

RPC=https://ethereum-sepolia-rpc.publicnode.com
AUDITOR_KEY=auditor.key          # 0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE — 흐름 1 의 approver
VENDOR_KEY=deployer.key          # 0xAf48305C64683eB72C28be61dD0E41478A2a0Df5 — 가스 지불 · 흐름 2 의 approver
AUDITOR_ADDR=0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE
VENDOR_ADDR=0xAf48305C64683eB72C28be61dD0E41478A2a0Df5

SERVICE=nginx
VERSION=1.31.6

# 자기 컴퓨터용 바이너리는 여기에 만든다 (0-upload.sh). 리눅스용 tb-vendor/tb-auditor 와 이름이 겹치지 않게 폴더를 나눈다
BIN=e2e-work/bin
WORK=e2e-work
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) EXE=.exe;; *) EXE="";; esac   # Git Bash 에서도 돌게
TBV="$BIN/tb-vendor$EXE"; TBA="$BIN/tb-auditor$EXE"; TBU="$BIN/tb-user$EXE"

SSH="ssh -i $PEM -o StrictHostKeyChecking=no"
SCP="scp -q -i $PEM -o StrictHostKeyChecking=no"
V() { $SSH "$SSH_USER@$VENDOR_HOST" "$@"; }
A() { $SSH "$SSH_USER@$AUDITOR_HOST" "$@"; }
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }
# step — 지금 무엇을 하고 있는지 초록색 띠로 표시. 아래 실제 명령의 출력과 구분되도록 항상 먼저 찍는다.
step() { printf '\033[1;32m==================== %s' "$1"; for _ in 1 2 3; do sleep 0.15; printf '.'; done; printf ' ====================\033[0m\n'; }
nitroBanner() { echo; step "CONNECTING TO NITRO ($1)"; sleep 2; }
localBanner() { echo; step "RETURNING TO LOCAL"; sleep 2; }
mkdir -p "$WORK"

# 파일 탐색기에서 더블클릭으로 실행하면 끝나자마자 창이 닫혀서 결과를 못 본다 — 끝나면(성공·실패 상관없이)
# 키 하나 누를 때까지 창을 붙잡아 둔다. 터미널에서 bash 로 직접 실행할 때도 그냥 Enter 한 번이면 된다.
trap 'ec=$?; echo; if [ $ec -eq 0 ]; then echo "== 완료 (exit 0) =="; else echo "== 종료 (exit $ec) =="; fi; read -n 1 -s -r -p "아무 키나 누르면 창이 닫힙니다..."; echo' EXIT
