#!/usr/bin/env bash
# Vendor - with no Auditor, builds and launches the source alone, and registers the value I made on the chain with my own key.
#    Run here, on my machine - only the build runs inside the Vendor instance, connected to automatically.
#    Run: bash "e2e/flow2-vendor-self-registers/1-Vendor-build-self-register.sh" [-c 0xCONTRACT]
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST

# The att binaries on the instance were cross-built with this machine's Go (0-Setup refuses anything but the pinned Go),
#   and that Go compiled att-ra-responder. So the value for build_env.go in build-config.json is this machine's Go version.
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

# Fetch the test source and set file modes as recorded in the source repository (git index), so they are the same whichever shell fetched it.
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"

# Write a build-config.json in the clone root in the same shape the Vendor hands to the Auditor via send (kept as is if it already exists).
#   So the self-register below reads it with --config and service, version and arch need not be typed again. It is outside the
#   app directory ($SRC_DIR), so it does not enter the measurement. tee_info comes from the instance metadata (unknown if unreadable).
step_start "WRITING BUILD CONFIG"
if [ -f src/docker-nginx/build-config.json ]; then
  echo "build-config.json already in the clone root - keeping it"
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

# att command - builds the source twice (--verify) to make the image and measurements, and checks for itself that the same value comes out.
#   Like the Auditor, the Vendor must confirm its own build is reproducible before registering.
step "BUILDING SOURCE (checking reproducibility)" ./att-vendor build --app-dir src/docker-nginx/$SRC_DIR --out-dir out --verify

# att command - launches that image in my TEE.
step "LAUNCHING ENCLAVE" ./att-vendor launch --eif out/alpine-slim.eif

echo
cat out/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# Parse options
REGISTRY=""
while getopts "c:" o; do case $o in c) REGISTRY=$OPTARG;; esac; done

# Fetch the measurements file just made on the Vendor instance and the build-config.json from the clone root to my machine.
#   self-register reads service, version and arch from build-config.json (the same source gives different PCRs on x86_64 and
#   arm64, so which architecture the value is for is written there too - on the instance, uname -m was converted to the GOARCH spelling).
step_start "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$VENDOR_HOST:out/measurements.json" "$WORK/vendor-build.json"
$SCP "$SSH_USER@$VENDOR_HOST:src/docker-nginx/build-config.json" "$WORK/vendor-build-config.json"
step_stop "FETCHING BUILD VALUE"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"
echo "build config:"; cat "$WORK/vendor-build-config.json"

if [ -z "$REGISTRY" ]; then
  # att command - deploys a new registry (the Registry contract) on the chain. The approver is the Vendor itself.
  step_start "DEPLOYING REGISTRY"
  out=$("$VENDOR_BIN" registry-deploy --rpc "$RPC" --deployer-key-file "$DEPLOYER_KEY" --approver "$VENDOR_ADDR" 2>&1)
  step_stop "DEPLOYING REGISTRY"
  echo "$out"
  REGISTRY=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # extract the address from the output
  [ -n "$REGISTRY" ] || { echo "no registry address in registry-deploy output"; exit 1; }
fi
echo "$REGISTRY" > "$WORK/registry-flow2.txt"  # save it in the file the next step reads

# att command - signs the value I built with my key and registers it on the chain as is (with no third-party comparison).
#   service, version and arch come from the --config build-config.json - a different value given as a flag wins, but a warning is printed.
step "SELF-REGISTERING" "$VENDOR_BIN" self-register --config "$WORK/vendor-build-config.json" --tee-addr "$VENDOR_HOST:5000" --build-measurements "$WORK/vendor-build.json" --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$VENDOR_SIGNING_KEY"
echo "self-register exit=$?"
# att command - verifies, from the User's side, whether what was just registered is actually trustworthy.
step "VERIFYING" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$REGISTRY"
echo "verify exit=$?"
echo "next: 2-User-verify"
