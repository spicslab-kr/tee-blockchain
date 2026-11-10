#!/usr/bin/env bash
# Flow 2, Vendor - builds and launches the source and registers the value on a registry with my own signing key. No
#    Auditor takes part in this step. Run on my machine; only the build and launch run inside the Vendor instance, which
#    the script connects to.
#    Run from the repository root:  bash "e2e/flow2-vendor-self-registers/1-Vendor-build-self-register.sh" [-c 0xREGISTRY]
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
need_host VENDOR_HOST

# 0-Setup-local-upload.sh cross-builds the att binaries and att-ra-responder with this machine's Go, and accepts only the
#   pinned version. build_env.go in build-config.json is therefore this machine's Go version.
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

# Fetch the test source and set file modes to those recorded in the git index, so the modes are the same whichever shell fetched it.
fix_modes() { (cd "$1" && git ls-files -s | while read -r mode _ _ path; do case $mode in 100755) chmod 755 "$path";; 100644) chmod 644 "$path";; esac; done && find . -type d -not -path './.git*' -exec chmod 755 {} +); }
step_start "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"
[ -d src/docker-nginx ] || git clone -q $SRC_REPO src/docker-nginx
git -C src/docker-nginx checkout -q $SRC_COMMIT
fix_modes src/docker-nginx
step_stop "FETCHING SOURCE ($SRC_REPO @ $SRC_COMMIT)"

# Write build-config.json in the clone root, in the shape att-vendor send pushes to the Auditor (an existing file is kept).
#   The self-register below takes service, version and arch from it with --config. The file sits outside the app directory
#   ($SRC_DIR), so it is not part of the measurement. tee_info comes from the instance metadata ("unknown" when unreadable).
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

# att-vendor build - builds the source into the image and measurements, and with --verify builds it a second time to
#   confirm the same value comes out. The Vendor checks reproducibility itself before registering.
step "BUILDING SOURCE (checking reproducibility)" ./att-vendor build --app-dir src/docker-nginx/$SRC_DIR --out-dir out --verify

# att-vendor launch - runs that image as an enclave in my TEE.
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

# Copy the measurements file and the build-config.json from the Vendor instance to this machine.
#   self-register reads service, version and arch from build-config.json. The same source gives different PCRs on x86_64 and
#   arm64, so build_env.arch (amd64 or arm64, converted from uname -m on the instance) says which one the value is for.
step_start "FETCHING BUILD VALUE"
$SCP "$SSH_USER@$VENDOR_HOST:out/measurements.json" "$WORK/vendor-build.json"
$SCP "$SSH_USER@$VENDOR_HOST:src/docker-nginx/build-config.json" "$WORK/vendor-build-config.json"
step_stop "FETCHING BUILD VALUE"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"
echo "build config:"; cat "$WORK/vendor-build-config.json"

if [ -z "$REGISTRY" ]; then
  # att-vendor registry-deploy - deploys a new Registry contract with the Vendor as its approver.
  step_start "DEPLOYING REGISTRY"
  out=$("$VENDOR_BIN" registry-deploy --rpc "$RPC" --deployer-key-file "$DEPLOYER_KEY" --approver "$VENDOR_ADDR" 2>&1)
  step_stop "DEPLOYING REGISTRY"
  echo "$out"
  REGISTRY=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # address from the 'deployed at' line
  [ -n "$REGISTRY" ] || { echo "no registry address in registry-deploy output"; exit 1; }
fi
echo "$REGISTRY" > "$WORK/registry-flow2.txt"  # the later steps read the registry address from this file

# att-vendor self-register - requests the attestation document from my enclave, signs the attested value with my signing
#   key, registers it, and reads the registration back. --build-measurements makes it register only a value that equals
#   my build. service, version and arch come from the build-config.json given with --config; a flag with another value
#   overrides it and prints a warning.
step "SELF-REGISTERING" "$VENDOR_BIN" self-register --config "$WORK/vendor-build-config.json" --tee-addr "$VENDOR_HOST:5000" --build-measurements "$WORK/vendor-build.json" --rpc "$RPC" --registry "$REGISTRY" --signing-key-file "$VENDOR_SIGNING_KEY"
echo "self-register exit=$?"
# att-user verify - runs the User's trust decision on what was just registered.
step "VERIFYING" "$USER_BIN" verify --tee-addr "$VENDOR_HOST:5000" --rpc "$RPC" --registry "$REGISTRY"
echo "verify exit=$?"
echo "next: 2-User-verify"
