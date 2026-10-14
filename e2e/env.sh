#!/usr/bin/env bash
# e2e/env.sh - settings and helpers shared by the e2e scripts. Every script runs on your own machine (macOS, Linux,
# Git Bash). Steps that build or launch something inside an instance call V() (Vendor), A() (Auditor) or R() (arm64
# host); each connects over SSH and streams the script to the instance, so no SSH session is opened by hand and no
# script file is uploaded. nitroBanner and localBanner print which environment is running and pause 2 seconds so the
# switch is visible.
#
# Instance addresses live in e2e/env.local.sh (gitignored; see env.local.example.sh).
here_env="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_HOST=""
AUDITOR_HOST=""
ARM64_HOST=""
[ -f "$here_env/env.local.sh" ] && . "$here_env/env.local.sh"
need_host() { eval "[ -n \"\${$1:-}\" ]" || { echo "$1 is not set. Copy e2e/env.local.example.sh to e2e/env.local.sh and fill it in."; exit 1; }; }
SSH_USER=ec2-user
PEM=pem2.pem                     # SSH key, relative to the repository root (gitignored)

# Chain - Sepolia testnet by default. To run on mainnet:  NET=mainnet CONFIRM_MAINNET=1 bash e2e/...  (costs real ETH:
# deployment about 1.1M gas, one registration about 89k gas. The deployer key and the Auditor's signing key both need a mainnet balance)
NET=${NET:-sepolia}
case "$NET" in
  sepolia) RPC=https://ethereum-sepolia-rpc.publicnode.com;;
  mainnet) RPC=https://ethereum-rpc.publicnode.com
           [ "${CONFIRM_MAINNET:-}" = 1 ] || { echo "NET=mainnet spends real ETH. If you are sure, also pass CONFIRM_MAINNET=1."; exit 1; };;
  *) echo "NET must be sepolia or mainnet"; exit 1;;
esac
AUDITOR_SIGNING_KEY=auditor.key   # 0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE - approver in flow 1
DEPLOYER_KEY=deployer.key; VENDOR_SIGNING_KEY=$DEPLOYER_KEY   # 0xAf48305C64683eB72C28be61dD0E41478A2a0Df5 - pays gas, approver in flow 2
AUDITOR_ADDR=0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE
VENDOR_ADDR=0xAf48305C64683eB72C28be61dD0E41478A2a0Df5

SERVICE=nginx
VERSION=1.31.6

# 0-Setup-local-upload.sh builds the binaries for your own machine here, apart from the Linux att-vendor and
# att-auditor binaries that go to the instances
BIN=e2e-work/bin
WORK=e2e-work
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) EXE=.exe;; *) EXE="";; esac   # Git Bash reports MINGW/MSYS
VENDOR_BIN="$BIN/att-vendor$EXE"; AUDITOR_BIN="$BIN/att-auditor$EXE"; USER_BIN="$BIN/att-user$EXE"

SSH="ssh -i $PEM -o StrictHostKeyChecking=no"
SCP="scp -q -i $PEM -o StrictHostKeyChecking=no"
V() { need_host VENDOR_HOST; $SSH "$SSH_USER@$VENDOR_HOST" "$@"; }
A() { need_host AUDITOR_HOST; $SSH "$SSH_USER@$AUDITOR_HOST" "$@"; }
R() { need_host ARM64_HOST; $SSH "$SSH_USER@$ARM64_HOST" "$@"; }   # arm64 comparison instance
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }
# step <title> <command...> - print a one-line green banner, then run the command.
# step_start <title> prints the same banner for a block of several commands; step_stop closes the block and prints
# nothing, because att commands print their own progress.
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; shift; "$@"; }
nitroBanner() { echo; step "CONNECTING TO NITRO ($1)"; sleep 2; }
localBanner() { echo; step "RETURNING TO LOCAL"; sleep 2; }
mkdir -p "$WORK"

# Keep the window open after the script ends, with the exit status shown, until a key is pressed. A script started by
# double-click in a file explorer would otherwise close the window before the result can be read.
trap 'ec=$?; echo; if [ $ec -eq 0 ]; then echo "== done (exit 0) =="; else echo "== exited (exit $ec) =="; fi; read -n 1 -s -r -p "Press any key to close this window..."; echo' EXIT
