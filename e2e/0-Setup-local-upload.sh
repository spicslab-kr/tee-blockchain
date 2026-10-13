#!/usr/bin/env bash
# Setup - build the Linux binaries and upload them to both instances (once, whoever runs it). Binaries for your own machine go to e2e-work/bin.
#    Scripts are not uploaded - every step runs directly over SSH, so the instances only need the binaries.
#    Run from TB-live:  bash "e2e/0-Setup-local-upload.sh"
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

# Build the tb executables themselves - both the Linux ones (to upload to the instances) and the ones for your machine.
#   -trimpath: keeps the absolute path of the build folder out of the binary. tb-ra-responder goes into the enclave
#   image as is, so without this each person's checkout path would change the measurement even for identical source.
need_host VENDOR_HOST; need_host AUDITOR_HOST
step "BUILDING BINARIES"
# Check the Go toolchain - even a different patch version changes the bytes of tb-ra-responder, and so the measurement (PCR).
#   internal/setup/build/GO_TOOLCHAIN is the only source, and tb itself checks the same value at build time.
WANT_GO="$(tr -d '[:space:]' < "$here/../internal/setup/build/GO_TOOLCHAIN")"
HAVE_GO="$(go version | awk '{print $3}')"
if [ "$HAVE_GO" != "$WANT_GO" ]; then
  echo "ERROR: Go toolchain is $HAVE_GO - this project is pinned to exactly $WANT_GO (internal/setup/build/GO_TOOLCHAIN)." >&2
  echo "  Building with another version changes the tb-ra-responder bytes, so the measurement will not match." >&2
  echo "  Fix it with bash e2e/0-Setup-local-install.sh, or rerun with GOTOOLCHAIN=$WANT_GO as a prefix." >&2
  exit 1
fi
echo "toolchain: $HAVE_GO (pinned)"
GOOS=linux GOARCH=amd64 go build -trimpath -o tb-vendor ./cmd/vendor
GOOS=linux GOARCH=amd64 go build -trimpath -o tb-auditor ./cmd/auditor
GOOS=linux GOARCH=amd64 go build -trimpath -o tb-ra-responder ./cmd/ra-responder
mkdir -p "$BIN"
go build -trimpath -o "$TBV" ./cmd/vendor
go build -trimpath -o "$TBA" ./cmd/auditor
go build -trimpath -o "$TBU" ./cmd/user
echo "responder sha256: $(sha256 tb-ra-responder | cut -c1-64)"  # to compare later that both instances got the same binary

# A running ra-proxy holding the old binary blocks overwriting it, so stop it first together with the enclave.
step "STOPPING OLD ENCLAVES"
V 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-vendor; true'
A 'sudo nitro-cli terminate-enclave --all >/dev/null 2>&1; pkill -x tb-auditor; true'
step "UPLOADING BINARIES"
$SCP tb-vendor tb-ra-responder "$SSH_USER@$VENDOR_HOST:~/"
$SCP tb-auditor tb-ra-responder "$SSH_USER@$AUDITOR_HOST:~/"
V 'chmod +x tb-vendor tb-ra-responder; sha256sum tb-ra-responder'
A 'chmod +x tb-auditor tb-ra-responder; sha256sum tb-ra-responder'
echo "uploaded. next: bash e2e/flow1-auditor-registers/1-Auditor-build-audit-handoff.sh   (from your machine; it connects to the Auditor instance automatically and builds there)"
