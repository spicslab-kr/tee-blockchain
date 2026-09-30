#!/usr/bin/env bash
# 아키텍처 표기 확인. 같은 소스도 x86_64 와 arm64 에서 PCR 이 다르다는 걸 실측했으니, 이제 등록과
#    전달 기록(measurement-record.json)이 "어느 아키텍처 것인지"를 말하는지 본다. 체인 Entry 는 그대로다 — 측정값
#    해시가 아키텍처마다 이미 다르니 키는 저절로 갈린다. 여기서 보는 것은 세 가지뿐이고, 트랜잭션은 하나도 안 보낸다.
#      1. 흐름 스크립트가 쓰는 uname -m → GOARCH 매핑이 실제 인스턴스에서 맞게 나오는지 (x86 Vendor → amd64, arm64 호스트 → arm64)
#      2. tb-auditor send 가 arch 없는 기록을 거절하고, arch 있는 기록은 그 arch 를 찍고 (로컬 bare 저장소로) 실제로 보내는지
#      3. tb-auditor register 가 uname 철자(x86_64)를 거절하는지 — 거절은 키를 읽기도, 체인에 붙기도 전에 난다
#    register --arch arm64 가 실제로 등록되는 것은 체인 트랜잭션이라 여기서는 안 본다 (cmd/arch_e2e_test.go 가 anvil 로 본다).
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
step "SEND: RECORD WITHOUT ARCH IS REFUSED"
WD="$WORK/g6"; rm -rf "$WD"; mkdir -p "$WD"
printf 'eif-bytes' > "$WD/svc.eif"
pcr_b64() { head -c 48 /dev/zero | tr '\0' "$1" | base64 | tr -d '\n'; }   # 준비 — 기록 파일의 values 는 base64 바이트
write_record() {  # <path> <arch-line or empty>
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
write_record "$WD/no-arch.json" ""
git init --quiet --bare --initial-branch=main "$WD/remote-a.git"
out=$("$TBA" send --repo "$WD/remote-a.git" --record "$WD/no-arch.json" --image "$WD/svc.eif" 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -ne 0 ] && echo "$out" | grep -q 'arch: want one of amd64|arm64'; then
  echo "✅ send: arch 없는 기록은 거절 — 메시지가 arch 를 지목한다 (exit $rc)"
else
  echo "❌ send: arch 없는 기록이 거절되지 않았거나, 메시지가 arch 를 지목하지 않는다 (exit $rc)"
  fail=1
fi

step "SEND: RECORD WITH ARCH IS SENT, ARCH PRINTED"
write_record "$WD/with-arch.json" '  "arch": "amd64",
'
git init --quiet --bare --initial-branch=main "$WD/remote-b.git"
out=$("$TBA" send --repo "$WD/remote-b.git" --record "$WD/with-arch.json" --image "$WD/svc.eif" 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -eq 0 ] && echo "$out" | grep -q 'arch: amd64' && git -C "$WD/remote-b.git" show HEAD:measurement-record.json 2>/dev/null | grep -q '"arch": "amd64"'; then
  echo "✅ send: arch: amd64 를 찍고, push 된 measurement-record.json 에도 arch 가 실려 있다"
else
  echo "❌ send: exit $rc — arch 가 안 찍혔거나, push 된 기록에 arch 가 없다"
  fail=1
fi

# 3. register — uname 철자 거절. 필수 플래그는 다 주되, --arch 검사는 키 파일을 읽기 전·체인에 붙기 전에 나니 아무 데도 안 간다.
step "REGISTER: --arch x86_64 IS REFUSED BEFORE TOUCHING THE CHAIN"
pcr_hex() { head -c 48 /dev/zero | tr '\0' "$1" | od -An -tx1 | tr -d ' \n'; }
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
echo "===================== arch record check ====================="
[ $fail -eq 0 ] && echo "✅ 모두 통과 — 기록과 등록 출력이 아키텍처를 말한다" || echo "❌ 실패한 항목이 있다 (위 ❌ 참고)"
echo "next: 없음 — 이 스크립트가 이 테스트의 끝이다"
exit $fail
