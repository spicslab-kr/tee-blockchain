#!/usr/bin/env bash
# e2e/env.sh - settings and helpers shared by the e2e scripts. Every script runs on your own machine (macOS, Linux,
# Git Bash). Steps that build or launch something inside an instance call V() (Vendor), A() (Auditor) or R() (arm64
# host); each connects over SSH and streams the script to the instance, so no SSH session is opened by hand and no
# script file is uploaded. nitroBanner and localBanner print which environment is running and pause 2 seconds so the
# switch is visible.
#
# Instance addresses live in e2e/env.local.sh (gitignored; see env.local.example.sh). A value already set in the
# environment is kept unless env.local.sh sets it, so a one-off run can pass them on the command line:
#   VENDOR_HOST=... AUDITOR_HOST=... bash e2e/responder-check.sh
here_env="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENDOR_HOST=${VENDOR_HOST:-}
AUDITOR_HOST=${AUDITOR_HOST:-}
ARM64_HOST=${ARM64_HOST:-}
[ -f "$here_env/env.local.sh" ] && . "$here_env/env.local.sh"
need_host() { eval "[ -n \"\${$1:-}\" ]" || { echo "$1 is not set. Copy e2e/env.local.example.sh to e2e/env.local.sh and fill it in."; exit 1; }; }
SSH_USER=ec2-user
PEM=pem2.pem                     # SSH key, relative to the repository root (gitignored)

# Chain - Sepolia testnet by default. To run on mainnet:  NET=mainnet CONFIRM_MAINNET=1 bash e2e/...  (costs real ETH:
# deployment about 1.1M gas, one registration about 89k gas. The deployer key and the Auditor's signing key both need a mainnet balance)
# To run with no public network and no ETH:  NET=local bash e2e/...  against a local chain (anvil) at LOCAL_RPC, default
# http://127.0.0.1:8545. Every chain call (deploy, audit, self-register, lookup, compare, verify) runs on this machine,
# never on an instance, so the chain only has to be reachable from here. Keep the one anvil running for a whole flow:
# a later step reads the registry an earlier one deployed, and a restarted anvil starts empty.
NET=${NET:-sepolia}
case "$NET" in
  sepolia) RPC=https://ethereum-sepolia-rpc.publicnode.com;;
  mainnet) RPC=https://ethereum-rpc.publicnode.com
           [ "${CONFIRM_MAINNET:-}" = 1 ] || { echo "NET=mainnet spends real ETH. If you are sure, also pass CONFIRM_MAINNET=1."; exit 1; };;
  local) RPC=${LOCAL_RPC:-http://127.0.0.1:8545};;
  *) echo "NET must be sepolia, mainnet or local"; exit 1;;
esac
AUDITOR_SIGNING_KEY=auditor.key   # approver in flow 1
DEPLOYER_KEY=deployer.key; VENDOR_SIGNING_KEY=$DEPLOYER_KEY   # pays gas, approver in flow 2
# The addresses of the two key files above, set in env.local.sh. There is no default: a default would be somebody
# else's address, and a registry deployed with it names the wrong approver. The scripts that use the chain call
# need_addr first; the build-only checks run without them.
AUDITOR_ADDR=${AUDITOR_ADDR:-}
VENDOR_ADDR=${VENDOR_ADDR:-}
need_addr() {
  eval "[ -n \"\${$1:-}\" ]" || { echo "$1 is not set. Copy e2e/env.local.example.sh to e2e/env.local.sh and set AUDITOR_ADDR and VENDOR_ADDR to the addresses of auditor.key and deployer.key."; exit 1; }
  # With NET=local, stop now rather than after a build of several minutes when no chain answers at RPC.
  if [ "$NET" = local ] && ! curl -s -m 5 -X POST -H 'Content-Type: application/json' \
       --data '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}' "$RPC" | grep -q '"result"'; then
    echo "NET=local: no chain answers at $RPC. Start one in another terminal (anvil --port 8545) and keep it running for the whole flow."; exit 1
  fi
}

SERVICE=nginx
VERSION=1.31.6

# 0-Setup-local-upload.sh builds the binaries for your own machine here, apart from the Linux att-vendor and
# att-auditor binaries that go to the instances
BIN=e2e-work/bin
WORK=e2e-work
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) EXE=.exe;; *) EXE="";; esac   # Git Bash reports MINGW/MSYS
VENDOR_BIN="$BIN/att-vendor$EXE"; AUDITOR_BIN="$BIN/att-auditor$EXE"; USER_BIN="$BIN/att-user$EXE"
mkdir -p "$WORK"

# NET=local signs with anvil's development accounts 0 and 1 instead of auditor.key and deployer.key, as the cmd tests do
# (cmd/register_e2e_test.go). Every anvil start funds those accounts with test ETH; a key you made yourself has no
# balance there. They come from anvil's default mnemonic, are the same on every anvil, and are not secrets: never send
# anything to them on a public network. They override AUDITOR_ADDR and VENDOR_ADDR from env.local.sh for this run only.
if [ "$NET" = local ]; then
  mkdir -p "$WORK/anvil-keys"
  printf '%s\n' ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80 > "$WORK/anvil-keys/deployer.key"   # account 0
  printf '%s\n' 59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d > "$WORK/anvil-keys/auditor.key"    # account 1
  DEPLOYER_KEY=$WORK/anvil-keys/deployer.key; VENDOR_SIGNING_KEY=$DEPLOYER_KEY; VENDOR_ADDR=0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
  AUDITOR_SIGNING_KEY=$WORK/anvil-keys/auditor.key; AUDITOR_ADDR=0x70997970C51812dc3A010C7d01b50e0d17dc79C8
fi

SSH="ssh -i $PEM -o StrictHostKeyChecking=no -o ConnectionAttempts=2 -o ConnectTimeout=20"
SCP="net_retry scp -q -i $PEM -o StrictHostKeyChecking=no -o ConnectionAttempts=2 -o ConnectTimeout=20"
V() { need_host VENDOR_HOST; net_retry $SSH "$SSH_USER@$VENDOR_HOST" "$@"; }
A() { need_host AUDITOR_HOST; net_retry $SSH "$SSH_USER@$AUDITOR_HOST" "$@"; }
R() { need_host ARM64_HOST; net_retry $SSH "$SSH_USER@$ARM64_HOST" "$@"; }   # arm64 comparison instance

# net_retry <ssh|scp> <args...> - runs ssh or scp, and runs it again when the connection itself could not be set up.
# Some networks abort a burst of new SSH connections ("kex_exchange_identification: read: Software caused connection
# abort"; seen on 2026-11-12 at the sixth connection within seconds), before anything ran on the instance.
#   - Retried only when ssh/scp failed (ssh: exit 255, its own error code; scp: any failure) AND its error output
#     names a connection-setup failure. A remote command that fails, or a session that breaks after it started, is
#     never retried, so a failure is never hidden or a command run twice.
#   - At most SSH_TRIES attempts (default 3), waiting 3 s, then 6 s. After the last one the original exit code is
#     returned and the script fails as before.
#   - stdin (the heredoc of V 'bash -s' <<REMOTE) is kept in a file and given again to every attempt; a terminal is
#     left alone.
# ssh/scp error output is shown after the attempt ends rather than as it is written.
SSH_TRIES=${SSH_TRIES:-3}
net_retry() {
  local n=1 rc errf inf=""
  errf=$(mktemp)
  if [ ! -t 0 ]; then inf=$(mktemp); cat > "$inf"; fi
  while :; do
    if [ -n "$inf" ]; then "$@" < "$inf" 2> "$errf"; rc=$?; else "$@" 2> "$errf"; rc=$?; fi
    cat "$errf" >&2
    if [ $rc -eq 0 ] || { [ "$1" = ssh ] && [ $rc -ne 255 ]; } || [ $n -ge "$SSH_TRIES" ] \
       || ! grep -qE 'kex_exchange_identification|banner exchange|Software caused connection abort|Connection (refused|reset by peer|timed out)|Operation timed out|Network is unreachable|No route to host|Connection closed by [^ ]+ port [0-9]+$' "$errf"; then
      break
    fi
    echo "$1: the connection could not be set up (attempt $n of $SSH_TRIES); trying again in $((n * 3)) s" >&2
    sleep $((n * 3)); n=$((n + 1))
  done
  rm -f "$errf" ${inf:+"$inf"}
  return $rc
}
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }
# step <title> <command...> - print a one-line green banner, then run the command.
# step_start <title> prints the same banner for a block of several commands; step_stop closes the block and prints
# nothing, because att commands print their own progress.
step_start() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; }
step_stop() { :; }
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "$1"; shift; "$@"; }
nitroBanner() { echo; step "CONNECTING TO NITRO ($1)"; sleep 2; }
localBanner() { echo; step "RETURNING TO LOCAL"; sleep 2; }

# Results contract (docs/plan/test-matrix.md, "Making the results machine-readable"). Every verdict a script reaches is
# also appended as one JSON line to $WORK/results/<platform>.jsonl, so the test matrix can be filled from the files
# instead of by hand. e2e/cvm/in-guest-verify.sh writes the same fields for the confidential VMs.
#
#   result <test> <check> <result> <expected> <reason> [detail]
#
# test is the matrix row (T1..T10, or "-" for a check that is not a matrix row); check names the verdict within the
# script; result and expected are one of accepted, rejected, identical, different, refused, error; reason is the line
# the script printed; detail is optional free text (for example the measured values). RESULTS_PLATFORM is the format
# string, aws-nitro unless set. chain is NET (sepolia, mainnet or local), so a run on a local chain is never mistaken for
# one on a public network; it means something only on the rows that use the chain (T4, T5, T6).
RESULTS_PLATFORM=${RESULTS_PLATFORM:-aws-nitro}
RESULTS_RUN=${RESULTS_RUN:-$(date -u +%Y%m%dT%H%M%SZ)}
json_str() { printf '%s' "$1" | tr '\r\n\t' '   ' | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }
verdict_of() { case $1 in 0) echo accepted;; 1) echo rejected;; *) echo error;; esac; }   # att verify/compare exit code
result() {
  mkdir -p "$WORK/results"
  printf '{"platform":"%s","chain":"%s","test":"%s","check":"%s","script":"%s","result":"%s","expected":"%s","reason":"%s","detail":"%s","run":"%s","at":"%s"}\n' \
    "$(json_str "$RESULTS_PLATFORM")" "$(json_str "$NET")" "$(json_str "$1")" "$(json_str "$2")" "$(json_str "${0#./}")" "$(json_str "$3")" \
    "$(json_str "$4")" "$(json_str "$5")" "$(json_str "${6:-}")" "$(json_str "$RESULTS_RUN")" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    >> "$WORK/results/$RESULTS_PLATFORM.jsonl"
}

# Keep the window open after the script ends, with the exit status shown, until a key is pressed. A script started by
# double-click in a file explorer would otherwise close the window before the result can be read. Only when stdin is a
# terminal: without one, read fails, and under set -e that failure became the script's exit status (a run that passed
# exited 1), so the trap also exits with the status it saw.
trap 'ec=$?; echo; if [ $ec -eq 0 ]; then echo "== done (exit 0) =="; else echo "== exited (exit $ec) =="; fi; if [ -t 0 ]; then read -n 1 -s -r -p "Press any key to close this window..." || true; echo; fi; exit $ec' EXIT
