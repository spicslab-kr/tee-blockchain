#!/usr/bin/env bash
# Auditor - builds the source, launches it in my enclave, checks that the built value equals what actually runs, registers
#    it on the chain, and hands the image and the record to the Vendor. Run here, on my machine - only the build runs inside
#    the Auditor instance, connected to automatically.
#    Run: bash "e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh" [-c 0xCONTRACT] [-v] [0xKEY]
#      -c  register in an existing registry (the contract address is kept in e2e-work/contract.txt)
#      -v  during audit, also query the Vendor enclave (to catch a Vendor running a different image)
#      0xKEY  the registration key the handoff uses. If omitted, it is looked up on the chain by measurement
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST; need_host AUDITOR_HOST

# The att binaries on the instance were cross-built with this machine's Go (0-Setup refuses anything but the pinned Go),
#   and that Go compiled att-ra-responder. So the value for build-config.json and the record's build_env.go is this machine's Go version.
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

# Fetch the test source. git stores only the executable bit, so file modes depend on the umask of the shell that checked out.
# That would change file modes inside the docker image and make the measurement drift, so force the modes recorded in the
# source repository (the git index).
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"

# If the Vendor had handed over via send, build-config.json would sit in the clone root (receive reads it). This test source
#   has none, so write one in the same shape - so the audit below reads it with --config and service, version and arch need
#   not be typed again. It is outside the app directory ($SRC_DIR), so it does not enter the measurement. tee_info comes
#   from the instance metadata (unknown if unreadable).
step_start "WRITING BUILD CONFIG"
if [ -f src/docker-nginx/build-config.json ]; then
  echo "build-config.json already in the clone root — keeping it"
else
  case "$(uname -m)" in x86_64) GOARCH=amd64;; aarch64|arm64) GOARCH=arm64;; *) GOARCH=unknown;; esac
  TOK=$(curl -s -m 2 -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' 2>/dev/null || true)
  md() { curl -s -m 2 -H "X-aws-ec2-metadata-token: $TOK" "http://169.254.169.254/latest/meta-data/$1" 2>/dev/null || echo unknown; }
  cat > src/docker-nginx/build-config.json <<EOF
{
  "schema": "attestamp.build-config/1",
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

# att command - builds the source with docker into the enclave image (EIF) and the measurements (PCR0/1/2).
step "BUILDING SOURCE" ./att-auditor build --app-dir src/docker-nginx/$SRC_DIR --out out

# att command - actually launches that image in my TEE.
step "LAUNCHING ENCLAVE" ./att-auditor launch --eif out/alpine-slim.eif

echo
cat out/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# Parse options
CONTRACT=""; CHECK_VENDOR=""
while getopts "c:v" o; do case $o in c) CONTRACT=$OPTARG;; v) CHECK_VENDOR=1;; esac; done
shift $((OPTIND - 1))
KEY=${1:-}

# Fetch the measurements file just made on the Auditor instance and the build-config.json from the clone root to my machine.
#   audit reads service, version and arch from build-config.json (values the Vendor already handed over are not typed again).
#   The hand-built record in the handoff below needs the architecture separately, so convert the uname spelling
#   (x86_64, aarch64) to the GOARCH spelling att uses (amd64, arm64) (the same source gives different PCRs on x86_64 and arm64).
step_start "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
$SCP "$SSH_USER@$AUDITOR_HOST:src/docker-nginx/build-config.json" "$WORK/auditor-build-config.json"
case "$(A 'uname -m' | tr -d '\r\n')" in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) ARCH="";; esac
[ -n "$ARCH" ] || { echo "cannot tell the Auditor instance's architecture from uname -m"; exit 1; }
step_stop "FETCHING BUILD VALUE"
echo "Auditor's own value (arch: $ARCH):"; cat "$WORK/auditor-measured.json"
echo "build config:"; cat "$WORK/auditor-build-config.json"

if [ -z "$CONTRACT" ]; then
  # att command - deploys a new registry (registry contract) on the chain.
  step_start "DEPLOYING REGISTRY"
  out=$("$AUDITOR_BIN" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$AUDITOR_ADDR" 2>&1)
  step_stop "DEPLOYING REGISTRY"
  echo "$out"
  CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # extract just the address from the output line
  [ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
fi
echo "$CONTRACT" > "$WORK/contract.txt"  # save it in the file the next step reads

# att command - sends an RA request to my TEE, compares the value received with the build value made above, and registers
#   on the chain only if they match. service, version and arch come from the --config build-config.json - a different value
#   given as a flag wins, but a warning is printed.
extra=(); [ -n "$CHECK_VENDOR" ] && extra=(--vendor-tee "$VENDOR_HOST:5000")
step "AUDITING" "$AUDITOR_BIN" audit --config "$WORK/auditor-build-config.json" --tee "$AUDITOR_HOST:5000" --measurements "$WORK/auditor-measured.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$AUDITOR_KEY" "${extra[@]}"
rc=$?
echo "audit exit=$rc   contract=$CONTRACT"
[ $rc -eq 0 ] || exit $rc

# From here on it is the handoff: the image and measurements just made, plus a note (record) saying which contract/key they
# were registered under, are passed to the Vendor as plain files. This is not an att feature but a transfer procedure we
# assemble by hand.
step_start "TRANSFERRING TO VENDOR"
mkdir -p "$WORK/from-auditor"
$SCP "$SSH_USER@$AUDITOR_HOST:out/alpine-slim.eif" "$WORK/from-auditor/alpine-slim.eif"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/from-auditor/measurements.json"
if [ -z "$KEY" ]; then
  # att command - looks up this measurement's registration key on the chain.
  out=$("$AUDITOR_BIN" lookup --rpc "$RPC" --contract "$CONTRACT" --measurements "$WORK/from-auditor/measurements.json" 2>&1)
  KEY=$(echo "$out" | grep -oE 'key +0x[0-9a-fA-F]{64}' | awk '{print $2}' | head -1)  # extract the key from the output
  [ -n "$KEY" ] || { step_stop "TRANSFERRING TO VENDOR"; echo "no registration of this value found on $CONTRACT"; exit 1; }
fi
pcr() { grep -oE "\"$1\": *\"[0-9a-fA-F]+\"" "$WORK/from-auditor/measurements.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/'; }  # helper to pull a PCR value out of the JSON
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
(cd "$WORK/from-auditor" && sha256 alpine-slim.eif measurements.json | sed 's/^/  /')  # hashes to check the transferred files
V 'mkdir -p from-auditor'
$SCP "$WORK/from-auditor/alpine-slim.eif" "$SSH_USER@$VENDOR_HOST:from-auditor/alpine-slim.eif"
$SCP "$WORK/from-auditor/measurements.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurements.json"
$SCP "$WORK/from-auditor/measurement-record.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurement-record.json"
step_stop "TRANSFERRING TO VENDOR"
echo "measurement record:"; cat "$WORK/from-auditor/measurement-record.json"
echo "handed to the Vendor. next: 2-Vendor-launch-check-record"
