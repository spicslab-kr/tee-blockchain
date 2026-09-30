#!/usr/bin/env bash
# 빌드 환경 선언 · 확인. Vendor 가 build-config.json 에 자기가 빌드한 환경(OS · CPU 아키텍처 · Go 툴체인 ·
#    -trimpath)을 build_env 로 적어 두고, Auditor 는 받을 때 그것을 자기 인스턴스와 견준다. 다르면 거절(exit ≠ 0) —
#    그 인스턴스에서는 Vendor 의 측정값이 나올 수 없고(PCR 은 아키텍처·Go 버전마다 갈린다), 그대로 빌드하면
#    Vendor 가 거짓말한 것과 똑같아 보이기 때문이다. --ignore-env 면 경고만 하고 넘어간다.
#
#    실제 인스턴스에서 실측한다. 각 호스트에 로컬 bare 저장소를 하나 만들어 Vendor 저장소로 쓰고(외부 git 원격 불필요),
#    build-config.json 을 세 가지로 push 해 tb-auditor receive 의 판정을 본다:
#      1. 다른 아키텍처를 선언  → 거절해야 한다 (x86_64 호스트에 arm64 라고, arm64 호스트에 amd64 라고)
#      2. 같은 것 + --ignore-env → 경고만 하고 0
#      3. 자기 아키텍처를 선언  → 통과, "build env matches" 가 찍힌다
#    x86_64 는 VENDOR_HOST(V), arm64 는 ARM64_HOST(R) 에서 각각 돈다 — 거울처럼. ARM64_HOST 가 비어 있으면
#    x86_64 쪽만 하고 그 사실을 알린다.
#
#    Go 버전: 여기서 크로스컴파일한 tb-auditor 의 runtime.Version() 은 이 컴퓨터의 go 와 같으니, 설정값에는
#    `go env GOVERSION` 을 그대로 적는다. 그래야 아키텍처 하나만 다르게 만들 수 있다.
#    실행: TB-live 에서  bash "e2e/build-env-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

GOVER=$(go env GOVERSION)

# ▶ tb-auditor 를 아키텍처별로 크로스컴파일. -trimpath: 체크아웃 경로가 바이너리에 안 박히게.
step "CROSS-BUILDING tb-auditor PER ARCHITECTURE ($GOVER)"
declare -A GOARCH_OF=( [x86_64]=amd64 [arm64]=arm64 )
mkdir -p "$WORK/arch"
for arch in x86_64 arm64; do
  GOOS=linux GOARCH=${GOARCH_OF[$arch]} go build -trimpath -o "$WORK/arch/tb-auditor-$arch" ./cmd/auditor || exit 1
done

host_of() { case $1 in V) echo "$VENDOR_HOST";; A) echo "$AUDITOR_HOST";; R) echo "$ARM64_HOST";; esac; }

fail=0
# check_on <ssh-func> <label> <arch> — 그 호스트에서 세 판정을 돌린다. 원격 스크립트는 실패한 판정 수를 exit 코드로 돌려준다.
check_on() {
  sshf=$1; label=$2; arch=$3
  mine=${GOARCH_OF[$arch]}
  if [ "$mine" = amd64 ]; then other=arm64; else other=amd64; fi
  nitroBanner "$label"
  $SCP "$WORK/arch/tb-auditor-$arch" "$SSH_USER@$(host_of "$sshf"):~/tb-auditor"
  $sshf 'bash -s' <<REMOTE
set -uo pipefail
step() { printf '\033[1;32m==================== %s (in progress) ====================\033[0m\n' "\$1"; shift; "\$@"; }
chmod +x tb-auditor
rm -rf g4 && mkdir g4 && cd g4
git init -q --bare --initial-branch=main remote.git
git clone -q remote.git work
git -C work config user.email t@t; git -C work config user.name t
echo 'package main' > work/main.go
git -C work add main.go; git -C work commit -qm seed; git -C work push -q -u origin main
seed=\$(git -C work rev-parse HEAD)

# push_cfg <arch> <trimpath> — build-config.json 하나를 새 커밋으로 push
push_cfg() {
  cat > work/build-config.json <<CFG
{
  "schema": "tb.build-config/1",
  "service": "svc",
  "version": "1.2.0",
  "platform": "aws",
  "source": { "repo": "https://example.com/svc.git", "commit": "\$seed" },
  "tee_info": { "instance_id": "i-0abc", "region": "eu-north-1", "ami_id": "ami-0123" },
  "build_env": { "os": "linux", "arch": "\$1", "go": "$GOVER", "trimpath": \$2 }
}
CFG
  git -C work add build-config.json; git -C work commit -qm "cfg \$1" ; git -C work push -q
}

n=0
echo "this host: \$(uname -m), tb-auditor built for $arch ($mine), $GOVER"

step "1. VENDOR DECLARES $other — MUST BE REFUSED ($label)"
push_cfg $other true
rm -rf src1; ~/tb-auditor receive --repo "\$PWD/remote.git" --into src1 > out1.txt 2> err1.txt; rc=\$?
cat out1.txt err1.txt
if [ \$rc -ne 0 ] && grep -q "$other" err1.txt && grep -q "$mine" err1.txt; then
  echo "✅ refused (exit \$rc), both architectures named"
else
  echo "❌ expected refusal naming $other and $mine, got exit \$rc"; n=\$((n+1))
fi

step "2. SAME, WITH --ignore-env — WARNING ONLY ($label)"
rm -rf src2; ~/tb-auditor receive --repo "\$PWD/remote.git" --into src2 --ignore-env > out2.txt 2> err2.txt; rc=\$?
cat out2.txt err2.txt
if [ \$rc -eq 0 ] && grep -q "warning:" err2.txt && grep -q "$other" err2.txt; then
  echo "✅ passed with a warning (exit 0)"
else
  echo "❌ expected exit 0 plus a warning, got exit \$rc"; n=\$((n+1))
fi

step "3. VENDOR DECLARES $mine — MUST PASS ($label)"
push_cfg $mine true
rm -rf src3; ~/tb-auditor receive --repo "\$PWD/remote.git" --into src3 > out3.txt 2> err3.txt; rc=\$?
cat out3.txt err3.txt
if [ \$rc -eq 0 ] && grep -q "build env matches" out3.txt; then
  echo "✅ matched (exit 0)"
else
  echo "❌ expected a match, got exit \$rc"; n=\$((n+1))
fi
exit \$n
REMOTE
  rc=$?
  localBanner
  if [ $rc -eq 0 ]; then echo "✅ $label: all three verdicts as expected"; else echo "❌ $label: $rc verdict(s) wrong"; fi
  fail=$((fail+rc))
}

check_on V "Vendor (x86_64)" x86_64
if [ -n "$ARM64_HOST" ]; then
  check_on R "arm64 comparison host" arm64
else
  echo "ARM64_HOST 가 비어 있다 — arm64 쪽 거울 검사는 건너뛴다 (e2e/env.sh 에 IP를 적으면 돈다)."
fi

echo
echo "===================== build env check ====================="
if [ $fail -eq 0 ]; then
  echo "✅ Auditor refuses to rebuild on an instance whose OS/architecture/Go differ from what the Vendor declared, and passes on a match"
else
  echo "❌ $fail verdict(s) wrong — see the ❌ lines above"
fi
echo "next: 없음 — 이 스크립트가 이 테스트의 끝이다"
exit $fail
