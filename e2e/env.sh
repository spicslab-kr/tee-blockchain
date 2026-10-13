#!/usr/bin/env bash
# e2e/env.sh - values read by the scripts that run on your own machine (macOS, Linux, Git Bash). Edit here only.
# Every step runs here, on your machine. Steps that must build or launch something inside an instance use
# V() / A() to connect automatically and stream the script over SSH (nothing is uploaded to the instance as a
# file), so you never need to SSH in by hand. nitroBanner/localBanner only show on screen which environment is
# running (2 second pause so a human can tell them apart).
#
# Instance addresses live in e2e/env.local.sh (gitignored; see env.local.example.sh).
here_env="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_HOST=""
AUDITOR_HOST=""
ARM64_HOST=""
[ -f "$here_env/env.local.sh" ] && . "$here_env/env.local.sh"
need_host() { eval "[ -n \"\${$1:-}\" ]" || { echo "$1 is not set. Copy e2e/env.local.example.sh to e2e/env.local.sh and fill it in."; exit 1; }; }
SSH_USER=ec2-user
PEM=pem2.pem                     # relative to TB-live; the key must sit in the repo root (it is gitignored)

# Chain - Sepolia testnet by default. To run on mainnet:  NET=mainnet CONFIRM_MAINNET=1 bash e2e/...  (costs real ETH:
# deployment about 1.1M gas, one registration about 89k gas. Both the deployer key and the auditor key need a mainnet balance)
NET=${NET:-sepolia}
case "$NET" in
  sepolia) RPC=https://ethereum-sepolia-rpc.publicnode.com;;
  mainnet) RPC=https://ethereum-rpc.publicnode.com
           [ "${CONFIRM_MAINNET:-}" = 1 ] || { echo "NET=mainnet spends real ETH. If you are sure, also pass CONFIRM_MAINNET=1."; exit 1; };;
  *) echo "NET must be sepolia or mainnet"; exit 1;;
esac
AUDITOR_KEY=auditor.key          # 0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE - approver in flow 1
VENDOR_KEY=deployer.key          # 0xAf48305C64683eB72C28be61dD0E41478A2a0Df5 - pays gas, approver in flow 2
AUDITOR_ADDR=0xD2082a2513781b29beeF5DB847b2c85Cbc50ECeE
VENDOR_ADDR=0xAf48305C64683eB72C28be61dD0E41478A2a0Df5

SERVICE=nginx
VERSION=1.31.6

# Binaries for your own machine are built here (0-Setup-local-upload.sh). Kept in a separate folder so they do not
# collide with the Linux tb-vendor/tb-auditor binaries
BIN=e2e-work/bin
WORK=e2e-work
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) EXE=.exe;; *) EXE="";; esac   # also works in Git Bash
TBV="$BIN/tb-vendor$EXE"; TBA="$BIN/tb-auditor$EXE"; TBU="$BIN/tb-user$EXE"

SSH="ssh -i $PEM -o StrictHostKeyChecking=no"
SCP="scp -q -i $PEM -o StrictHostKeyChecking=no"
V() { need_host VENDOR_HOST; $SSH "$SSH_USER@$VENDOR_HOST" "$@"; }
A() { need_host AUDITOR_HOST; $SSH "$SSH_USER@$AUDITOR_HOST" "$@"; }
R() { need_host ARM64_HOST; $SSH "$SSH_USER@$ARM64_HOST" "$@"; }   # arm64 comparison instance
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }
# step - print a one-line green banner saying what is being done, then do it. step_start/step_stop are for when the
# same banner must be printed separately at the start and end (several other commands in between); for now both
# just print once, with no animation. tb commands usually print their own progress, and animating dots here would
# collide with that output.
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; shift; "$@"; }
nitroBanner() { echo; step "CONNECTING TO NITRO ($1)"; sleep 2; }
localBanner() { echo; step "RETURNING TO LOCAL"; sleep 2; }
mkdir -p "$WORK"

# Double-clicking in a file explorer closes the window as soon as the script ends, so you cannot see the result - hold
# the window open until a key is pressed (success or failure). From a terminal, just press Enter once.
trap 'ec=$?; echo; if [ $ec -eq 0 ]; then echo "== done (exit 0) =="; else echo "== exited (exit $ec) =="; fi; read -n 1 -s -r -p "Press any key to close this window..."; echo' EXIT
