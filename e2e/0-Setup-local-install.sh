#!/usr/bin/env bash
# Installs what the e2e scripts need on your machine: git, an ssh client, and Go (the version pinned in
# internal/setup/build/GO_TOOLCHAIN). Anything already present is left alone, so it is safe to run repeatedly.
# The environment is detected automatically and each case uses its own installer:
#   - Ubuntu/Debian family (apt)
#   - Fedora/RHEL/Amazon Linux family (dnf)
#   - macOS (git ships with the system; Homebrew installs it when absent, see https://brew.sh)
#   - Windows (Git Bash): git and ssh ship with Git for Windows; Go is installed with winget
#    Run from the repository root:  bash "e2e/0-Setup-local-install.sh"
set -euo pipefail

# Go must match the pin exactly: any other version, lower or higher, changes the reproducible-build values.
# internal/setup/build/GO_TOOLCHAIN is the single source of the version (the att binaries embed that file and check
# it at build time). To change the version, edit that file.
here="$(cd "$(dirname "$0")" && pwd)"
GO_VERSION="$(sed 's/^go//' "$here/../internal/setup/build/GO_TOOLCHAIN" | tr -d '[:space:]')"

# -- detect environment --
OS_KIND=unknown
case "$(uname -s)" in
  Linux*)
    if command -v apt-get >/dev/null 2>&1; then OS_KIND=apt
    elif command -v dnf >/dev/null 2>&1; then OS_KIND=dnf
    elif command -v yum >/dev/null 2>&1; then OS_KIND=yum
    fi
    ;;
  Darwin*) OS_KIND=mac ;;
  MINGW*|MSYS*|CYGWIN*) OS_KIND=windows ;;
esac
echo "== detected environment: $(uname -s) -> $OS_KIND"

go_version_ok() {
  # Exact match: the reproducible-build comparison needs the identical compiler version.
  command -v go >/dev/null 2>&1 || return 1
  have="$(go version | awk '{print $3}' | sed 's/^go//')"
  [ "$have" = "$GO_VERSION" ]
}

install_go_tarball() {
  # Shared by apt/dnf/yum. The distribution golang packages are often outdated, so this fetches the official tar.gz.
  local os arch tarball url tmp
  os="$1"; arch="$2"
  tarball="go${GO_VERSION}.${os}-${arch}.tar.gz"
  url="https://go.dev/dl/${tarball}"
  echo "== downloading Go $GO_VERSION: $url"
  tmp=$(mktemp -d)
  curl -fsSL "$url" -o "$tmp/$tarball"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$tmp/$tarball"
  rm -rf "$tmp"
  if ! grep -q '/usr/local/go/bin' "$HOME/.bashrc" 2>/dev/null; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> "$HOME/.bashrc"
    echo "== added PATH to ~/.bashrc - open a new terminal or run 'source ~/.bashrc' before the go command is visible"
  fi
  export PATH="$PATH:/usr/local/go/bin"
}

case "$OS_KIND" in
  apt)
    echo "== installing git, ssh client, build tools (apt)"
    sudo apt-get update -qq
    sudo apt-get install -y -qq git openssh-client ca-certificates curl build-essential
    if go_version_ok; then
      echo "== Go is already at the right version - skipping"
    else
      install_go_tarball linux amd64
    fi
    ;;
  dnf|yum)
    echo "== installing git and ssh client ($OS_KIND)"
    sudo "$OS_KIND" install -y -q git openssh-clients ca-certificates curl gcc make
    if go_version_ok; then
      echo "== Go is already at the right version - skipping"
    else
      install_go_tarball linux amd64
    fi
    ;;
  mac)
    if command -v git >/dev/null 2>&1; then
      echo "== git already present (ships with macOS) - skipping"
    elif command -v brew >/dev/null 2>&1; then
      echo "== installing git (Homebrew)"
      brew install git
    else
      echo "git and Homebrew are both missing. Install Homebrew from https://brew.sh or install git yourself."
      exit 1
    fi
    if go_version_ok; then
      echo "== Go is already exactly $GO_VERSION - skipping"
    else
      # Homebrew installs the latest Go and cannot pin a version, so the official pkg from go.dev is used.
      echo "== downloading Go $GO_VERSION (official pkg from go.dev)"
      tmp=$(mktemp -d)
      curl -fsSL "https://go.dev/dl/go${GO_VERSION}.darwin-$(uname -m | sed 's/x86_64/amd64/').pkg" -o "$tmp/go.pkg"
      sudo installer -pkg "$tmp/go.pkg" -target /
      rm -rf "$tmp"
      export PATH="/usr/local/go/bin:$PATH"
    fi
    ;;
  windows)
    echo "== Git Bash already bundles git, ssh and bash - nothing to install"
    if go_version_ok; then
      echo "== Go is already exactly $GO_VERSION - skipping"
    elif command -v winget >/dev/null 2>&1; then
      # Go's official installer (.msi) installs machine-wide under Program Files and needs administrator rights
      # (winget has no per-user option for it). Without them winget fails with error 1603 or 1618, so the script
      # checks first and explains.
      if ! net session >/dev/null 2>&1; then
        echo "Installing Go needs administrator rights (the Go installer is machine-wide)."
        echo "Open a new Git Bash or PowerShell as administrator (search in the Start menu, right-click,"
        echo "'Run as administrator') and run this script again."
        exit 1
      fi
      echo "== installing Go $GO_VERSION (winget, version pinned with --version)"
      winget install --id GoLang.Go --version "$GO_VERSION" --silent --accept-package-agreements --accept-source-agreements
      echo "Open a new Git Bash window so go shows up on PATH."
    else
      echo "winget is missing. Download the Go $GO_VERSION installer from https://go.dev/dl and install it yourself."
      exit 1
    fi
    ;;
  *)
    echo "Could not detect the environment. Please install these yourself:"
    echo "  git, an openssh client, Go ${GO_VERSION} (https://go.dev/dl)"
    exit 1
    ;;
esac

echo
echo "== verifying install"
git --version
ssh -V
# A window that was open during the Go install keeps its old PATH, so 'go' may not be found yet.
# Look in the install locations of each method directly.
GOBIN=go
for cand in /usr/local/go/bin/go "/c/Program Files/Go/bin/go.exe"; do
  [ -x "$cand" ] && GOBIN="$cand" && break
done
if ! "$GOBIN" version >/dev/null 2>&1; then
  echo "Go is installed and this window does not see it yet. Open a new Git Bash window and"
  echo "check with 'go version'."
  exit 0
fi
"$GOBIN" version
HAVE_GO="$("$GOBIN" version | awk '{print $3}' | sed 's/^go//')"
if [ "$HAVE_GO" != "$GO_VERSION" ]; then
  echo
  echo "WARNING: Go is $HAVE_GO and the pin is $GO_VERSION. The reproducible-build comparison needs an exact match."
  echo "  Prefix the command with GOTOOLCHAIN=go${GO_VERSION} to force it for this run only."
  exit 1
fi
echo
echo "Ready - Go $GO_VERSION confirmed exactly. Next: bash e2e/0-Setup-local-upload.sh"
