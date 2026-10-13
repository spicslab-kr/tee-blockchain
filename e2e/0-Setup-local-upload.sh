#!/usr/bin/env bash
# Setup - build the Linux binaries and upload them to both instances (once, whoever runs it). Binaries for your own machine go to e2e-work/bin.
#    Scripts are not uploaded - every step runs directly over SSH, so the instances only need the binaries.
#    Run from TB-live:  bash "e2e/0-Setup-local-upload.sh"
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

# Build the att executables themselves - both the Linux ones (to upload to the instances) and the ones for your machine.
#   -trimpath: keeps the absolute path of the build folder out of the binary. att-ra-responder goes into the enclave
#   image as is, so without this each person's checkout path would change the measurement even for identical source.
need_host VENDOR_HOST; need_host AUDITOR_HOST
step "BUILDING BINARIES"
# Check the Go toolchain - even a different patch version changes the bytes of att-ra-responder, and so the measurement (PCR).
#   internal/setup/build/GO_TOOLCHAIN is the only source, and att itself checks the same value at build time.
WANT_GO="$(tr -d '[:space:]' < "$here/../internal/setup/build/GO_TOOLCHAIN")"
HAVE_GO="$(go version | awk '{print $3}')"
if [ "$HAVE_GO" != "$WANT_GO" ]; then
  echo "ERROR: Go toolchain is $HAVE_GO - this project is pinned to exactly $WANT_GO (internal/setup/build/GO_TOOLCHAIN)." >&2
  echo "  Building with another version changes the att-ra-responder bytes, so the measurement will not match." >&2
  echo "  Fix it with bash e2e/0-Setup-local-install.sh, or rerun with GOTOOLCHAIN=$WANT_GO as a prefix." >&2
  exit 1
fi
echo "toolchain: $HAVE_GO (pinned)"
GOOS=linux GOARCH=amd64 go build -trimpath -o att-vendor ./cmd/vendor
GOOS=linux GOARCH=amd64 go build -trimpath -o att-auditor ./cmd/auditor
GOOS=linux GOARCH=amd64 go build -trimpath -o att-ra-responder ./cmd/ra-responder
mkdir -p "$BIN"
go build -trimpath -o "$VENDOR_BIN" ./cmd/vendor
go build -trimpath -o "$AUDITOR_BIN" ./cmd/auditor
go build -trimpath -o "$USER_BIN" ./cmd/user
echo "responder sha256: $(sha256 att-ra-responder | cut -c1-64)"  # to compare later that both instances got the same binary

# A running ra-proxy holding the old binary blocks overwriting it, so stop it first together with the enclave.
step "STOPPING OLD ENCLAVES"
V 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x att-vendor; true'
A 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x att-auditor; true'
step "UPLOADING BINARIES"
$SCP att-vendor att-ra-responder "$SSH_USER@$VENDOR_HOST:~/"
$SCP att-auditor att-ra-responder "$SSH_USER@$AUDITOR_HOST:~/"
V 'chmod +x att-vendor att-ra-responder; sha256sum att-ra-responder'
A 'chmod +x att-auditor att-ra-responder; sha256sum att-ra-responder'
echo "uploaded. next: bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh   (from your machine; it connects to the Auditor instance automatically and builds there)"
