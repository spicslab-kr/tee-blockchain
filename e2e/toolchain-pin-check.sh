#!/usr/bin/env bash
# Go 툴체인 고정 검사. tb-ra-responder 의 바이트는 Go 패치 버전에 따라 달라지고(실측: go1.26.2 와 go1.26.8 이
#    -trimpath 로도 다른 바이너리를 낸다), 그래서 측정값(PCR0·2)도 달라진다. 고정 버전은 소스에 들어 있는
#    internal/setup/build/GO_TOOLCHAIN 한 파일이 유일한 출처이고, tb 는 그 파일을 안에 품고 build 시작 때 자기가
#    어떤 툴체인으로 빌드됐는지 대조해 다르면 거부한다. 이 스크립트는 그 거부가 실제 Nitro 호스트에서 정말 일어나는지 본다:
#      1. 내 컴퓨터의 go version 이 고정 파일과 정확히 같다
#      2. 일부러 다른 패치 버전(GOTOOLCHAIN=$WRONG)으로 크로스빌드한 tb-vendor-wrongtc 를 Vendor 호스트에 올려
#         `build` 를 치면 exit ≠ 0 이고 stderr 에 "pins go1.26.1" 이 나온다
#      3. 제대로 빌드한 tb-vendor 는 그 검사를 통과한다 (그 뒤 다른 이유로 실패할 수는 있다 — 툴체인 메시지만 본다)
#    2 는 다른 툴체인을 받아올 수 없는(오프라인) 환경이면 안내만 하고 건너뛴다.
#    실행: TB-live 에서  bash "e2e/toolchain-pin-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

PIN_FILE=internal/setup/build/GO_TOOLCHAIN
WANT="$(tr -d '[:space:]' < "$PIN_FILE")"
WRONG=go1.26.0   # 고정과 다른 패치 버전이면 아무거나 — 1.26.1 이 고정이므로 1.26.0 은 항상 다르다
fail=0

step "1. LOCAL GO VERSION vs $PIN_FILE"
HAVE="$(go version | awk '{print $3}')"
if [ "$HAVE" = "$WANT" ]; then
  echo "✅ 내 컴퓨터 go version = $HAVE = 고정 파일 ($PIN_FILE)"
else
  echo "❌ 내 컴퓨터 go version 이 $HAVE — 고정 파일은 $WANT. bash e2e/0-Setup-local-install.sh 로 맞춰야 한다."
  fail=1
fi

mkdir -p "$WORK/toolchain"
step "2. CROSS-BUILDING tb-vendor WITH THE WRONG TOOLCHAIN ($WRONG)"
if GOTOOLCHAIN=$WRONG go version 2>/dev/null | grep -q "$WRONG"; then
  GOTOOLCHAIN=$WRONG GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/toolchain/tb-vendor-wrongtc" ./cmd/vendor
  HAVE_WRONG=1
else
  echo "⚠ GOTOOLCHAIN=$WRONG 을 받아올 수 없다 (오프라인?) — 거부 경로 실측은 건너뛴다. 단위 테스트(go test ./internal/setup/build ./cmd -run Toolchain)가 같은 경로를 덮는다."
  HAVE_WRONG=0
fi
GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/toolchain/tb-vendor" ./cmd/vendor

step "3. UPLOADING TO THE VENDOR HOST"
nitroBanner "Vendor"
[ $HAVE_WRONG = 1 ] && $SCP "$WORK/toolchain/tb-vendor-wrongtc" "$SSH_USER@$VENDOR_HOST:~/tb-vendor-wrongtc"
$SCP "$WORK/toolchain/tb-vendor" "$SSH_USER@$VENDOR_HOST:~/tb-vendor"

# remote_build <binary> — Vendor 호스트에서 `build` 를 치고 stderr+stdout 을 돌려준다. exit 코드는 파일로.
remote_build() {
  V "chmod +x $1; mkdir -p /tmp/g5-app; printf 'FROM scratch\n' > /tmp/g5-app/Dockerfile; ./$1 build --app-dir /tmp/g5-app --out /tmp/g5-out 2>&1; echo \"__EXIT=\$?\""
}

if [ $HAVE_WRONG = 1 ]; then
  step "4. build WITH tb-vendor-wrongtc ($WRONG) — MUST BE REFUSED"
  out="$(remote_build tb-vendor-wrongtc)"
  echo "$out" | sed 's/^/    /'
  ec="$(echo "$out" | sed -n 's/^__EXIT=//p' | tail -1)"
  if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "pins $WANT"; then
    echo "✅ 거부됨 (exit $ec) — 'built with $WRONG but the project pins $WANT'"
  else
    echo "❌ 다른 툴체인으로 빌드한 tb-vendor 가 거부되지 않았다 (exit ${ec:-?}) — CheckToolchain 이 build 시작에서 안 도는 것"
    fail=1
  fi
fi

step "5. build WITH THE CORRECTLY BUILT tb-vendor ($WANT) — MUST PASS THE TOOLCHAIN CHECK"
out="$(remote_build tb-vendor)"
echo "$out" | sed 's/^/    /'
if echo "$out" | grep -q "pins $WANT"; then
  echo "❌ 고정 툴체인으로 빌드한 tb-vendor 가 툴체인 검사에서 거부됐다 — GO_TOOLCHAIN 과 실제 빌드 툴체인이 어긋난다"
  fail=1
else
  echo "✅ 툴체인 검사 통과 (이후 단계에서 다른 이유로 실패했다면 이 테스트 범위 밖)"
fi
localBanner

echo
echo "===================== toolchain pin ====================="
[ $fail = 0 ] && echo "✅ 전부 통과" || echo "❌ 실패 항목 있음 (위 ❌ 참고)"
echo "next: 없음 — 이 스크립트가 이 테스트의 끝이다"
exit $fail
