#!/usr/bin/env bash
# 내 컴퓨터에 e2e 를 돌리는 데 필요한 것들을 설치한다: git, ssh 클라이언트, Go 1.26+.
# 이미 있는 건 다시 안 깐다 — 몇 번 돌려도 안전하다. 아래 것들을 자동으로 구분해서 각자 맞는 방법으로 설치한다:
#   - Ubuntu/Debian 계열 (apt)
#   - Fedora/RHEL/Amazon Linux 계열 (dnf)
#   - mac (Homebrew — 없으면 설치 안내만 하고 https://brew.sh 로 보냄)
#   - Windows (Git Bash) — git·ssh 는 Git for Windows 에 이미 들어있어서 보통 자동으로 통과되고, Go 만 winget 으로 설치
#    실행: bash "e2e/0-Setup-local-install.sh"
set -euo pipefail

GO_VERSION=1.26.1  # 정확히 이 버전 — 다르면 재현빌드 값이 안 맞는다. 더 낮아도, 더 높아도 다시 이걸로 맞춘다

# -- 환경 구분 --
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
echo "== 감지된 환경: $(uname -s) → $OS_KIND"

go_version_ok() {
  # "이상"이 아니라 정확히 일치 — 재현빌드 대조에는 컴파일러 버전이 정확히 같아야 한다.
  command -v go >/dev/null 2>&1 || return 1
  have="$(go version | awk '{print $3}' | sed 's/^go//')"
  [ "$have" = "$GO_VERSION" ]
}

install_go_tarball() {
  # apt/dnf/yum 공통 — apt 리포의 golang 패키지는 버전이 오래된 경우가 많아서, 공식 tar.gz 로 직접 받는다.
  local os arch tarball url tmp
  os="$1"; arch="$2"
  tarball="go${GO_VERSION}.${os}-${arch}.tar.gz"
  url="https://go.dev/dl/${tarball}"
  echo "== Go $GO_VERSION 받는 중: $url"
  tmp=$(mktemp -d)
  curl -fsSL "$url" -o "$tmp/$tarball"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$tmp/$tarball"
  rm -rf "$tmp"
  if ! grep -q '/usr/local/go/bin' "$HOME/.bashrc" 2>/dev/null; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> "$HOME/.bashrc"
    echo "== ~/.bashrc 에 PATH 추가함 — 새 터미널을 열거나 'source ~/.bashrc' 해야 go 명령이 바로 보인다"
  fi
  export PATH="$PATH:/usr/local/go/bin"
}

case "$OS_KIND" in
  apt)
    echo "== git, ssh 클라이언트, 빌드 도구 설치 (apt)"
    sudo apt-get update -qq
    sudo apt-get install -y -qq git openssh-client ca-certificates curl build-essential
    if go_version_ok; then
      echo "== Go 는 이미 충분한 버전 — 건너뜀"
    else
      install_go_tarball linux amd64
    fi
    ;;
  dnf|yum)
    echo "== git, ssh 클라이언트 설치 ($OS_KIND)"
    sudo "$OS_KIND" install -y -q git openssh-clients ca-certificates curl gcc make
    if go_version_ok; then
      echo "== Go 는 이미 충분한 버전 — 건너뜀"
    else
      install_go_tarball linux amd64
    fi
    ;;
  mac)
    if command -v git >/dev/null 2>&1; then
      echo "== git 은 이미 있음 (mac 기본 제공) — 건너뜀"
    elif command -v brew >/dev/null 2>&1; then
      echo "== git 설치 (Homebrew)"
      brew install git
    else
      echo "git 이 없고 Homebrew 도 없습니다. https://brew.sh 에서 Homebrew 를 설치하거나 git 을 직접 설치해 주세요."
      exit 1
    fi
    if go_version_ok; then
      echo "== Go 는 이미 정확히 $GO_VERSION — 건너뜀"
    else
      # brew 는 항상 최신 버전만 받아서, --version 으로 정확히 고정할 수 없다 — go.dev 공식 pkg 로 직접 받는다.
      echo "== Go $GO_VERSION 받는 중 (공식 pkg, Homebrew 는 버전 고정이 안 돼서 안 씀)"
      tmp=$(mktemp -d)
      curl -fsSL "https://go.dev/dl/go${GO_VERSION}.darwin-$(uname -m | sed 's/x86_64/amd64/').pkg" -o "$tmp/go.pkg"
      sudo installer -pkg "$tmp/go.pkg" -target /
      rm -rf "$tmp"
      export PATH="/usr/local/go/bin:$PATH"
    fi
    ;;
  windows)
    echo "== Git Bash 안에는 git·ssh·bash 가 이미 들어있어서 따로 설치 안 함"
    if go_version_ok; then
      echo "== Go 는 이미 충분한 버전 — 건너뜀"
    elif command -v winget >/dev/null 2>&1; then
      echo "== Go $GO_VERSION 설치 (winget, 버전 고정 — --version 없이는 winget 이 그냥 최신을 받는다)"
      winget install --id GoLang.Go --version "$GO_VERSION" --silent --accept-package-agreements --accept-source-agreements
      echo "새 Git Bash 창을 열어야 PATH 에 go 가 잡힙니다."
    else
      echo "winget 이 없습니다. https://go.dev/dl 에서 Go $GO_VERSION 설치 파일을 받아 직접 설치해 주세요."
      exit 1
    fi
    ;;
  *)
    echo "환경을 자동으로 못 알아봤습니다. 아래를 직접 설치해 주세요:"
    echo "  git, openssh 클라이언트, Go ${GO_VERSION}+ (https://go.dev/dl)"
    exit 1
    ;;
esac

echo
echo "== 설치 확인"
git --version
ssh -V
GOBIN="$( [ -x /usr/local/go/bin/go ] && echo /usr/local/go/bin/go || echo go )"
"$GOBIN" version
HAVE_GO="$("$GOBIN" version | awk '{print $3}' | sed 's/^go//')"
if [ "$HAVE_GO" != "$GO_VERSION" ]; then
  echo
  echo "⚠ Go 버전이 정확히 $GO_VERSION 이 아니라 $HAVE_GO 입니다 — 재현빌드 대조를 하려면 정확히 맞아야 합니다."
  echo "  GOTOOLCHAIN=go${GO_VERSION} 를 앞에 붙여서 실행하면 이번만 강제로 맞출 수 있습니다."
  exit 1
fi
echo
echo "준비 끝 — Go $GO_VERSION 정확히 확인됨. 다음: bash e2e/0-Setup-local-upload.sh"
