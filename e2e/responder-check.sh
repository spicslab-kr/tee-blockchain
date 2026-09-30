#!/usr/bin/env bash
# 응답기 빌드 정보 검사. 엔클레이브 이미지에 얹혀 측정되는 것은 tb 가 아니라 그 옆의 tb-ra-responder 파일이고,
#    그 파일의 바이트는 응답기를 빌드한 Go 패치 버전 · -trimpath 유무 · GOOS/GOARCH 로 정해진다. 툴체인 고정 검사
#    (toolchain-pin-check.sh)는 tb 자신이 어떤 Go 로 빌드됐는지만 보므로, 다른 go build 로 나온 응답기 파일은 그
#    검사를 그냥 지나간다. 그래서 tb 는 build/pack 시작 때 응답기 바이너리 안에 Go 가 남긴 빌드 정보를 직접 읽어
#    (go version -m 이 보여주는 것과 같은 것) 다음이면 거부한다: 고정 툴체인이 아님 · -trimpath 없음 · linux 가 아님 ·
#    이 인스턴스와 다른 아키텍처. 이 스크립트는 그 거부가 실제 Nitro 호스트에서 정말 일어나는지 본다:
#      1. 응답기 셋을 내 컴퓨터에서 크로스빌드한다 — 제대로 된 것(linux/amd64 -trimpath), -trimpath 없는 것, arm64 용
#      2. 셋을 Vendor 호스트에 서로 다른 이름으로 올려 각각 `build --responder <파일>` 을 친다
#      3. -trimpath 없는 것은 "without -trimpath" 로, arm64 용은 "GOARCH=arm64 … this instance is amd64" 로 거부되고
#         (exit ≠ 0), 제대로 된 것은 응답기 검사를 통과해 빌드 로그에 "responder: tb-ra-responder sha256 … -trimpath"
#         한 줄이 찍힌다 (그 뒤 다른 이유로 실패할 수는 있다 — 응답기 줄만 본다)
#    실행: TB-live 에서  bash "e2e/responder-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

PIN_FILE=internal/setup/build/GO_TOOLCHAIN
WANT="$(tr -d '[:space:]' < "$PIN_FILE")"
fail=0

step "1. LOCAL GO VERSION vs $PIN_FILE"
HAVE="$(go version | awk '{print $3}')"
if [ "$HAVE" = "$WANT" ]; then
  echo "✅ 내 컴퓨터 go version = $HAVE = 고정 파일 ($PIN_FILE)"
else
  echo "❌ 내 컴퓨터 go version 이 $HAVE — 고정 파일은 $WANT. bash e2e/0-Setup-local-install.sh 로 맞춰야 한다."
  fail=1
fi

mkdir -p "$WORK/responder"
step "2. CROSS-BUILDING THREE RESPONDERS (correct · without -trimpath · arm64) AND tb-vendor"
GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/responder/tb-ra-responder-ok"        ./cmd/ra-responder
GOOS=linux GOARCH=amd64 go build           -o "$WORK/responder/tb-ra-responder-notrimpath" ./cmd/ra-responder
GOOS=linux GOARCH=arm64 go build -trimpath -o "$WORK/responder/tb-ra-responder-arm64"     ./cmd/ra-responder
GOOS=linux GOARCH=amd64 go build -trimpath -o "$WORK/responder/tb-vendor" ./cmd/vendor
OK_SHA="$(sha256 "$WORK/responder/tb-ra-responder-ok" | cut -c1-16)"
echo "correct responder sha256 (first 16): $OK_SHA — 호스트의 빌드 로그에 같은 값이 찍혀야 한다"

step "3. UPLOADING TO THE VENDOR HOST"
nitroBanner "Vendor"
$SCP "$WORK/responder/tb-vendor" "$WORK/responder/tb-ra-responder-ok" "$WORK/responder/tb-ra-responder-notrimpath" \
     "$WORK/responder/tb-ra-responder-arm64" "$SSH_USER@$VENDOR_HOST:~/"

# remote_build <responder file> — Vendor 호스트에서 그 응답기로 `build` 를 치고 stderr+stdout 을 돌려준다. exit 코드는 파일로.
remote_build() {
  V "chmod +x tb-vendor $1; mkdir -p /tmp/rc-app; printf 'FROM scratch\n' > /tmp/rc-app/Dockerfile; ./tb-vendor build --app-dir /tmp/rc-app --out /tmp/rc-out --responder $1 2>&1; echo \"__EXIT=\$?\""
}
exit_of() { echo "$1" | sed -n 's/^__EXIT=//p' | tail -1; }

step "4. build WITH tb-ra-responder-notrimpath — MUST BE REFUSED, NAMING -trimpath"
out="$(remote_build tb-ra-responder-notrimpath)"
echo "$out" | sed 's/^/    /'
ec="$(exit_of "$out")"
if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "without -trimpath"; then
  echo "✅ 거부됨 (exit $ec) — 'tb-ra-responder was built without -trimpath'"
else
  echo "❌ -trimpath 없이 빌드한 응답기가 거부되지 않았다 (exit ${ec:-?}) — CheckResponder 가 build 시작에서 안 도는 것"
  fail=1
fi

step "5. build WITH tb-ra-responder-arm64 ON AN amd64 HOST — MUST BE REFUSED, NAMING BOTH ARCHITECTURES"
out="$(remote_build tb-ra-responder-arm64)"
echo "$out" | sed 's/^/    /'
ec="$(exit_of "$out")"
if [ "${ec:-0}" != 0 ] && echo "$out" | grep -q "GOARCH=arm64" && echo "$out" | grep -q "this instance is amd64"; then
  echo "✅ 거부됨 (exit $ec) — 'built for GOARCH=arm64 but this instance is amd64'"
else
  echo "❌ arm64 용 응답기가 amd64 호스트에서 거부되지 않았다 (exit ${ec:-?})"
  fail=1
fi

step "6. build WITH tb-ra-responder-ok — MUST PASS THE RESPONDER CHECK"
out="$(remote_build tb-ra-responder-ok)"
echo "$out" | sed 's/^/    /'
if echo "$out" | grep -q "responder: tb-ra-responder sha256 $OK_SHA .* -trimpath"; then
  echo "✅ 응답기 검사 통과 — 빌드 로그의 sha256 $OK_SHA 이 내 컴퓨터에서 잰 값과 같다 (이후 단계에서 다른 이유로 실패했다면 이 테스트 범위 밖)"
else
  echo "❌ 제대로 빌드한 응답기가 검사를 통과하지 못했거나, 로그에 'responder: … -trimpath' 줄이 없다"
  fail=1
fi
localBanner

echo
echo "===================== responder check ====================="
[ $fail = 0 ] && echo "✅ 전부 통과" || echo "❌ 실패 항목 있음 (위 ❌ 참고)"
echo "next: 없음 — 이 스크립트가 이 테스트의 끝이다"
exit $fail
