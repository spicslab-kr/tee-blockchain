#!/usr/bin/env bash
# Setup - builds the Linux binaries and uploads them to both instances. Run it once before the flows, and again after
#    any code change. Binaries for your own machine go to e2e-work/bin.
#    Every later step runs over SSH, so the instances hold only the binaries.
#    Run from the repository root:  bash "e2e/0-Setup-local-upload.sh"
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST; need_host AUDITOR_HOST
step "BUILDING BINARIES"
# Check the Go toolchain. A different patch version changes the bytes of att-ra-responder and so the measurement (PCR).
#   internal/setup/build/GO_TOOLCHAIN holds the pin, and the att binaries check the same value at build time.
WANT_GO="$(tr -d '[:space:]' < "$here/../internal/setup/build/GO_TOOLCHAIN")"
HAVE_GO="$(go version | awk '{print $3}')"
if [ "$HAVE_GO" != "$WANT_GO" ]; then
  echo "ERROR: Go toolchain is $HAVE_GO - this project is pinned to exactly $WANT_GO (internal/setup/build/GO_TOOLCHAIN)." >&2
  echo "  Building with another version changes the att-ra-responder bytes, so the measurement will not match." >&2
  echo "  Fix it with bash e2e/0-Setup-local-install.sh, or rerun with GOTOOLCHAIN=$WANT_GO as a prefix." >&2
  exit 1
fi
echo "toolchain: $HAVE_GO (pinned)"
# Linux binaries for the instances, then binaries for this machine.
#   -trimpath keeps the absolute build folder path out of the binary. att-ra-responder goes into the enclave image
#   unchanged, so the checkout path would otherwise change the measurement.
GOOS=linux GOARCH=amd64 go build -trimpath -o att-vendor ./cmd/vendor
GOOS=linux GOARCH=amd64 go build -trimpath -o att-auditor ./cmd/auditor
GOOS=linux GOARCH=amd64 go build -trimpath -o att-ra-responder ./cmd/ra-responder
mkdir -p "$BIN"
go build -trimpath -o "$VENDOR_BIN" ./cmd/vendor
go build -trimpath -o "$AUDITOR_BIN" ./cmd/auditor
go build -trimpath -o "$USER_BIN" ./cmd/user
echo "responder sha256: $(sha256 att-ra-responder | cut -c1-64)"  # compared with the checksum printed on each instance after the upload

# A running att-vendor or att-auditor process (the ra-proxy bridge) locks its binary, so stop it and the enclave before uploading.
step "STOPPING OLD ENCLAVES"
V 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x att-vendor; true'
A 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x att-auditor; true'
step "UPLOADING BINARIES"
$SCP att-vendor att-ra-responder "$SSH_USER@$VENDOR_HOST:~/"
$SCP att-auditor att-ra-responder "$SSH_USER@$AUDITOR_HOST:~/"
V 'chmod +x att-vendor att-ra-responder; sha256sum att-ra-responder'
A 'chmod +x att-auditor att-ra-responder; sha256sum att-ra-responder'
echo "uploaded. next: bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh   (run on your machine; it connects to the Auditor instance and builds there)"
