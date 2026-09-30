#!/usr/bin/env bash
# 빌드 환경 표기 확인. 같은 소스도 x86_64 와 arm64 에서 PCR 이 다르다는 걸 실측했고 Go 툴체인이 달라도 다르니, 이제 등록과
#    전달 기록(measurement-record.json)이 "어느 아키텍처·어느 툴체인 것인지"(build_env)를 말하는지 본다. 체인 Entry 는 그대로다 —
#    측정값 해시가 아키텍처마다 이미 다르니 키는 저절로 갈린다. 여기서 보는 것은 세 가지뿐이고, 트랜잭션은 하나도 안 보낸다.
#      1. 흐름 스크립트가 쓰는 uname -m → GOARCH 매핑이 실제 인스턴스에서 맞게 나오는지 (x86 Vendor → amd64, arm64 호스트 → arm64)
#      2. tb-auditor send 가 build_env 없는 기록을 거절하고, 있는 기록은 그 빌드 환경을 한 줄로 찍고 (로컬 bare 저장소로) 실제로 보내는지
#         — 그리고 --record 없이 --config(build-config.json) + --measurements(measurements.json) 로 기록을 직접 만들어 보내는지
#      3. tb-auditor register 가 uname 철자(x86_64)를 거절하는지 — 거절은 키를 읽기도, 체인에 붙기도 전에 난다
#    register --arch arm64 가 실제로 등록되는 것은 체인 트랜잭션이라 여기서는 안 본다 (cmd/buildenv_record_e2e_test.go 가 anvil 로 본다).
#
#    필요: ARM64_HOST 가 e2e/env.sh 에 있어야 1 번을 arm64 쪽까지 본다. 비어 있으면 x86 쪽만 보고 안내만 찍는다.
#    실행: TB-live 에서  bash "e2e/arch-record-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

fail=0

# 준비 — 흐름 스크립트(flow1/flow2 의 1-*.sh)와 똑같은 매핑. uname 철자를 tb 가 쓰는 GOARCH 철자로.
goarch_of() { case "$1" in x86_64) echo amd64;; aarch64|arm64) echo arm64;; *) echo "";; esac; }

# 1. 실제 인스턴스에서 uname -m 을 받아 매핑 — 흐름 스크립트가 measurement-record.json 에 적을 값이 이렇게 나온다.
check_host() {  # <ssh-func> <label> <expected GOARCH>
  sshf=$1; label=$2; want=$3
  step "ARCH OF $label"
  raw=$($sshf 'uname -m' 2>/dev/null | tr -d '\r\n')
  got=$(goarch_of "$raw")
  if [ "$got" = "$want" ]; then
    echo "✅ $label: uname -m = $raw → arch $got"
  else
    echo "❌ $label: uname -m = '$raw' → arch '$got', expected $want"
    fail=1
  fi
}
check_host V "Vendor (x86_64)" amd64
if [ -n "$ARM64_HOST" ]; then
  check_host R "arm64 host" arm64
else
  echo "ARM64_HOST 가 비어 있다 — arm64 쪽 매핑은 건너뛴다 (e2e/env.sh 에 적으면 같이 본다)."
fi

# 2. send — 로컬 bare 저장소를 상대로, 네트워크·체인 없이 끝까지. 흐름 1 의 handoff 가 Go 스키마로 보낼 때 그대로다.
#    기록의 build_env.go 는 이 컴퓨터의 Go 로 적는다 — tb 바이너리를 만든 그 Go 다 (0-Setup 이 고정된 것만 받는다).
GOVER=$(go env GOVERSION)
step "SEND: RECORD WITHOUT BUILD_ENV IS REFUSED"
WD="$WORK/build-env-check"; rm -rf "$WD"; mkdir -p "$WD"
printf 'eif-bytes' > "$WD/svc.eif"
pcr_b64() { head -c 48 /dev/zero | tr '\0' "$1" | base64 | tr -d '\n'; }   # 준비 — 기록 파일의 values 는 base64 바이트
write_record() {  # <path> <build_env-line or empty>
  cat > "$1" <<EOF
{
  "schema": "tb.measurement-record/1",
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws",
${2}  "image": { "path": "svc.eif" },
  "values": [
    { "name": "PCR0", "value": "$(pcr_b64 1)" },
    { "name": "PCR1", "value": "$(pcr_b64 2)" },
    { "name": "PCR2", "value": "$(pcr_b64 3)" }
  ],
  "build_tool": { "name": "nitro-cli", "version": "1.3.0" }
}
EOF
}
export GIT_AUTHOR_NAME=tb-e2e GIT_AUTHOR_EMAIL=tb-e2e@example.invalid GIT_COMMITTER_NAME=tb-e2e GIT_COMMITTER_EMAIL=tb-e2e@example.invalid
write_record "$WD/no-build-env.json" ""
git init --quiet --bare --initial-branch=main "$WD/remote-a.git"
out=$("$TBA" send --repo "$WD/remote-a.git" --record "$WD/no-build-env.json" --image "$WD/svc.eif" 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -ne 0 ] && echo "$out" | grep -q 'build_env.arch: want one of amd64|arm64' && echo "$out" | grep -q 'build_env.go'; then
  echo "✅ send: build_env 없는 기록은 거절 — 메시지가 build_env 를 지목한다 (exit $rc)"
else
  echo "❌ send: build_env 없는 기록이 거절되지 않았거나, 메시지가 build_env 를 지목하지 않는다 (exit $rc)"
  fail=1
fi

step "SEND: RECORD WITH BUILD_ENV IS SENT, BUILD ENV PRINTED"
write_record "$WD/with-build-env.json" '  "build_env": { "os": "linux", "arch": "amd64", "go": "'"$GOVER"'", "trimpath": true },
'
git init --quiet --bare --initial-branch=main "$WD/remote-b.git"
out=$("$TBA" send --repo "$WD/remote-b.git" --record "$WD/with-build-env.json" --image "$WD/svc.eif" 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -eq 0 ] && echo "$out" | grep -q "build env: linux/amd64 $GOVER trimpath=true" && git -C "$WD/remote-b.git" show HEAD:measurement-record.json 2>/dev/null | grep -q '"build_env"'; then
  echo "✅ send: build env: linux/amd64 $GOVER trimpath=true 를 찍고, push 된 measurement-record.json 에도 build_env 가 실려 있다"
else
  echo "❌ send: exit $rc — build env 가 안 찍혔거나, push 된 기록에 build_env 가 없다"
  fail=1
fi

# 2b. send --config --measurements — 기록을 손으로 쓰지 않고 build-config.json 과 nitro-cli 의 measurements.json 에서 만든다.
#     push 된 기록의 service · build_env 는 config 그대로여야 한다.
step "SEND: RECORD BUILT FROM BUILD-CONFIG AND MEASUREMENTS"
pcr_hex() { head -c 48 /dev/zero | tr '\0' "$1" | od -An -v -tx1 | tr -d ' \n'; }
cat > "$WD/build-config.json" <<EOF
{
  "schema": "tb.build-config/1",
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws",
  "source": { "repo": "https://github.com/qixiangme/docker-nginx", "commit": "ef5a25a6314e652a2dcfbce6074bef729f4d9639", "path": "mainline/alpine-slim" },
  "tee_info": { "instance_id": "i-0check", "region": "eu-north-1", "ami_id": "ami-0check" },
  "build_env": { "os": "linux", "arch": "arm64", "go": "$GOVER", "trimpath": true }
}
EOF
cat > "$WD/measurements.json" <<EOF
{ "Measurements": { "HashAlgorithm": "Sha384 { ... }", "PCR0": "$(pcr_hex 1)", "PCR1": "$(pcr_hex 2)", "PCR2": "$(pcr_hex 3)" } }
EOF
git init --quiet --bare --initial-branch=main "$WD/remote-c.git"
out=$("$TBA" send --repo "$WD/remote-c.git" --config "$WD/build-config.json" --measurements "$WD/measurements.json" --image "$WD/svc.eif" --build-tool-version 1.3.0 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
pushed=$(git -C "$WD/remote-c.git" show HEAD:measurement-record.json 2>/dev/null)
if [ $rc -eq 0 ] && echo "$out" | grep -q "record $SERVICE $VERSION aws · build env: linux/arm64 $GOVER trimpath=true" \
   && echo "$pushed" | grep -q "\"service\": \"$SERVICE\"" && echo "$pushed" | grep -q '"arch": "arm64"' && echo "$pushed" | grep -q "\"go\": \"$GOVER\"" \
   && [ -f "$WD/measurement-record.json" ]; then
  echo "✅ send --config --measurements: 기록을 만들어 보냈고, push 된 기록의 service · build_env 가 build-config.json 그대로다 (measurement-record.json 도 measurements.json 옆에 남는다)"
else
  echo "❌ send --config --measurements: exit $rc — 기록이 안 만들어졌거나, push 된 기록의 service · build_env 가 config 와 다르다"
  fail=1
fi

# 3. register — uname 철자 거절. 필수 플래그는 다 주되, --arch 검사는 키 파일을 읽기 전·체인에 붙기 전에 나니 아무 데도 안 간다.
step "REGISTER: --arch x86_64 IS REFUSED BEFORE TOUCHING THE CHAIN"
pcr_hex() { head -c 48 /dev/zero | tr '\0' "$1" | od -An -v -tx1 | tr -d ' \n'; }
out=$("$TBA" register --rpc http://127.0.0.1:1 --contract 0x0000000000000000000000000000000000000001 --key-file "$WD/no-such.key" \
  --measurements "PCR0=$(pcr_hex 1),PCR1=$(pcr_hex 2),PCR2=$(pcr_hex 3)" --service "$SERVICE" --version "$VERSION" --arch x86_64 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -ne 0 ] && echo "$out" | grep -q -- '--arch: want one of amd64|arm64'; then
  echo "✅ register: x86_64 는 거절 — amd64 라고 써야 한다고 알려준다 (exit $rc)"
else
  echo "❌ register: --arch x86_64 가 거절되지 않았거나 메시지가 다르다 (exit $rc)"
  fail=1
fi

echo
echo "===================== build env record check ====================="
[ $fail -eq 0 ] && echo "✅ 모두 통과 — 기록과 등록 출력이 빌드 환경(아키텍처·Go 툴체인)을 말한다" || echo "❌ 실패한 항목이 있다 (위 ❌ 참고)"
echo "next: 없음 — 이 스크립트가 이 테스트의 끝이다"
exit $fail
