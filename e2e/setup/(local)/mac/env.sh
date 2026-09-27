#!/usr/bin/env bash
# e2e/setup/(local)/mac/env.sh — 자기 컴퓨터(macOS · Linux) 쪽 스크립트가 읽는 값. 여기만 고친다. env.ps1 과 같은 값.
# 인스턴스 쪽 스크립트(*/(nitro)/*.sh)는 IP 를 모르며 소스·이름만 안다.

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
mkdir -p "$WORK"
