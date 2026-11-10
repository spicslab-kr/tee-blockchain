#!/usr/bin/env bash
# Flow 1, Auditor - builds the source, launches it in my enclave, checks that the built value equals what runs there,
#    registers it on the registry, and hands the image and a measurement record to the Vendor. Run on my machine; only the
#    build and launch run inside the Auditor instance, which the script connects to.
#    Run from the repository root:  bash "e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh" [-c 0xREGISTRY] [-v] [0xRECORD_KEY]
#      -c  use an existing registry (the address is saved to e2e-work/registry.txt for the next steps)
#      -v  during the audit, also ask the Vendor's enclave which value it runs (--vendor-tee)
#      0xRECORD_KEY  the record key written into the handoff record. Without it, the key is looked up on the registry by measurement
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST; need_host AUDITOR_HOST

# 0-Setup-local-upload.sh cross-builds the att binaries and att-ra-responder with this machine's Go, and accepts only the
#   pinned version. build_env.go in build-config.json and in the record is therefore this machine's Go version.
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

# Fetch the test source. git stores only the executable bit, so file modes follow the umask of the shell that checked out,
# and modes inside the docker image change the measurement. fix_modes sets every file to the mode recorded in the
# git index (755 or 644) and every directory to 755.
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"

# A Vendor that ran att-vendor send leaves build-config.json in the clone root, where att-auditor receive reads it. The
#   test source has none, so this step writes one in the same shape and the audit below takes service, version and arch
#   from it with --config. The file sits outside the app directory ($SRC_DIR), so it is not part of the measurement.
#   tee_info comes from the instance metadata ("unknown" when unreadable).
step_start "WRITING BUILD CONFIG"
if [ -f src/docker-nginx/build-config.json ]; then
  echo "build-config.json already in the clone root: keeping it"
else
  case "$(uname -m)" in x86_64) GOARCH=amd64;; aarch64|arm64) GOARCH=arm64;; *) GOARCH=unknown;; esac
  TOK=$(curl -s -m 2 -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' 2>/dev/null || true)
  md() { curl -s -m 2 -H "X-aws-ec2-metadata-token: $TOK" "http://169.254.169.254/latest/meta-data/$1" 2>/dev/null || echo unknown; }
  cat > src/docker-nginx/build-config.json <<EOF
{
  "schema": "attestamp.build-config/1",
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws-nitro",
  "source": { "repo": "$SRC_REPO", "commit": "$SRC_COMMIT", "path": "$SRC_DIR" },
  "tee_info": { "instance_id": "$(md instance-id)", "region": "$(md placement/region)", "ami_id": "$(md ami-id)" },
  "build_env": { "os": "linux", "arch": "$GOARCH", "go": "$GOVER", "trimpath": true }
}
EOF
fi
cat src/docker-nginx/build-config.json
step_stop "WRITING BUILD CONFIG"

# att-auditor build - builds the source with docker into the enclave image (EIF) and writes the measurements (PCR0/1/2).
step "BUILDING SOURCE" ./att-auditor build --app-dir src/docker-nginx/$SRC_DIR --out-dir out

# att-auditor launch - runs that image as an enclave in my TEE.
step "LAUNCHING ENCLAVE" ./att-auditor launch --eif out/alpine-slim.eif

echo
cat out/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# Parse options
REGISTRY=""; CHECK_VENDOR=""
while getopts "c:v" o; do case $o in c) REGISTRY=$OPTARG;; v) CHECK_VENDOR=1;; esac; done
shift $((OPTIND - 1))
RECORD_KEY=${1:-}

# Copy the measurements file and the build-config.json from the Auditor instance to this machine.
#   audit reads service, version and arch from build-config.json. The handoff record below also needs the architecture,
#   so uname -m (x86_64, aarch64) is converted to the spelling att uses (amd64, arm64); the same source gives different
#   PCRs on x86_64 and arm64.
step_start "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
$SCP "$SSH_USER@$AUDITOR_HOST:src/docker-nginx/build-config.json" "$WORK/auditor-build-config.json"
case "$(A 'uname -m' | tr -d '\r\n')" in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) ARCH="";; esac
[ -n "$ARCH" ] || { echo "cannot tell the Auditor instance's architecture from uname -m"; exit 1; }
step_stop "FETCHING BUILD VALUE"
echo "Auditor's own value (arch: $ARCH):"; cat "$WORK/auditor-measured.json"
echo "build config:"; cat "$WORK/auditor-build-config.json"

if [ -z "$REGISTRY" ]; then
  # att-auditor registry-deploy - deploys a new Registry contract with the Auditor as its approver.
  step_start "DEPLOYING REGISTRY"
  out=$("$AUDITOR_BIN" registry-deploy --rpc "$RPC" --deployer-key-file "$DEPLOYER_KEY" --approver "$AUDITOR_ADDR" 2>&1)
  step_stop "DEPLOYING REGISTRY"
  echo "$out"
  REGISTRY=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # address from the 'deployed at' line
  [ -n "$REGISTRY" ] || { echo "no registry address in registry-deploy output"; exit 1; }
fi
echo "$REGISTRY" > "$WORK/registry.txt"  # the later steps read the registry address from this file

# att-auditor audit - requests an attestation document from my enclave, compares the attested value with the build value
#   from above, and registers it on the registry when they match. service, version and arch come from the build-config.json
#   given with --config; a flag with another value overrides it and prints a warning.
extra=(); [ -n "$CHECK_VENDOR" ] && extra=(--vendor-tee "$VENDOR_HOST:5000")
step "AUDITING" "$AUDITOR_BIN" audit --config "$WORK/auditor-build-config.json" --tee-addr "$AUDITOR_HOST:5000" --build-measurements "$WORK/auditor-measured.json" --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$AUDITOR_SIGNING_KEY" "${extra[@]}"
rc=$?
echo "audit exit=$rc   registry=$REGISTRY"
[ $rc -eq 0 ] || exit $rc

# Handoff: this script copies the image, the measurements and a measurement record (naming the registry and record key the
# value was registered under) to the Vendor instance as plain files.
step_start "TRANSFERRING TO VENDOR"
mkdir -p "$WORK/from-auditor"
$SCP "$SSH_USER@$AUDITOR_HOST:out/alpine-slim.eif" "$WORK/from-auditor/alpine-slim.eif"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/from-auditor/measurements.json"
if [ -z "$RECORD_KEY" ]; then
  # att-auditor lookup - finds this measurement's record key on the registry.
  out=$("$AUDITOR_BIN" lookup --rpc "$RPC" --registry "$REGISTRY" --measurements "$WORK/from-auditor/measurements.json" 2>&1)
  RECORD_KEY=$(echo "$out" | grep -oE 'record key +0x[0-9a-fA-F]{64}' | awk '{print $3}' | head -1)  # record key from the 'record key' line
  [ -n "$RECORD_KEY" ] || { step_stop "TRANSFERRING TO VENDOR"; echo "no registration of this value found on $REGISTRY"; exit 1; }
fi
pcr() { grep -oE "\"$1\": *\"[0-9a-fA-F]+\"" "$WORK/from-auditor/measurements.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/'; }  # prints one PCR value from measurements.json
cat > "$WORK/from-auditor/measurement-record.json" <<EOF
{
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws-nitro",
  "build_env": { "os": "linux", "arch": "$ARCH", "go": "$GOVER", "trimpath": true },
  "image": "alpine-slim.eif",
  "measurements": { "PCR0": "$(pcr PCR0)", "PCR1": "$(pcr PCR1)", "PCR2": "$(pcr PCR2)" },
  "registry": { "rpc": "$RPC", "address": "$REGISTRY", "record_key": "$RECORD_KEY", "approver": "$AUDITOR_ADDR" },
  "issued_by": "$AUDITOR_ADDR",
  "issued_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
(cd "$WORK/from-auditor" && sha256 alpine-slim.eif measurements.json | sed 's/^/  /')  # checksums of the transferred files
V 'mkdir -p from-auditor'
$SCP "$WORK/from-auditor/alpine-slim.eif" "$SSH_USER@$VENDOR_HOST:from-auditor/alpine-slim.eif"
$SCP "$WORK/from-auditor/measurements.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurements.json"
$SCP "$WORK/from-auditor/measurement-record.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurement-record.json"
step_stop "TRANSFERRING TO VENDOR"
echo "measurement record:"; cat "$WORK/from-auditor/measurement-record.json"
echo "handed to the Vendor. next: 2-Vendor-launch-check-record"
