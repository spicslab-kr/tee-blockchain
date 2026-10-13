#!/usr/bin/env bash
# Build-environment declaration and check. The Vendor records the environment it built in (OS, CPU architecture, Go
#    toolchain, -trimpath) as build_env in build-config.json, and the Auditor compares it with its own instance on
#    receipt. If they differ it refuses (exit != 0): that instance cannot produce the Vendor's measurement (PCRs differ
#    per architecture and Go version), and building anyway would look exactly as if the Vendor had lied.
#    With --ignore-env it only warns and carries on.
#
#    Measured on real instances. On each host a local bare repository is created and used as the Vendor repository
#    (no external git remote needed), and build-config.json is pushed in three variants to see the verdict of
#    tb-auditor receive:
#      1. declares another architecture -> must be refused (arm64 on the x86_64 host, amd64 on the arm64 host)
#      2. same, with --ignore-env       -> only a warning, exit 0
#      3. declares its own architecture -> passes, "build env matches" is printed
#    x86_64 runs on VENDOR_HOST (V) and arm64 on ARM64_HOST (R), mirror-style. If ARM64_HOST is empty only the
#    x86_64 side runs and that is reported.
#
#    Go version: the runtime.Version() of the tb-auditor cross-compiled here equals this machine's go, so the config
#    gets `go env GOVERSION` as is. That way only the architecture can be made to differ.
#    Run from TB-live:  bash "e2e/build-env-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST
GOVER=$(go env GOVERSION)

# Cross-compile tb-auditor per architecture. -trimpath: keeps the checkout path out of the binary.
step "CROSS-BUILDING tb-auditor PER ARCHITECTURE ($GOVER)"
declare -A GOARCH_OF=( [x86_64]=amd64 [arm64]=arm64 )
mkdir -p "$WORK/arch"
for arch in x86_64 arm64; do
  GOOS=linux GOARCH=${GOARCH_OF[$arch]} go build -trimpath -o "$WORK/arch/tb-auditor-$arch" ./cmd/auditor || exit 1
done

host_of() { case $1 in V) echo "$VENDOR_HOST";; A) echo "$AUDITOR_HOST";; R) echo "$ARM64_HOST";; esac; }

fail=0
# check_on <ssh-func> <label> <arch> - run the three verdicts on that host. The remote script returns the number of wrong verdicts as its exit code.
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
rm -rf build-env-check && mkdir build-env-check && cd build-env-check
git init -q --bare --initial-branch=main remote.git
git clone -q remote.git work
git -C work config user.email t@t; git -C work config user.name t
echo 'package main' > work/main.go
git -C work add main.go; git -C work commit -qm seed; git -C work push -q -u origin main
seed=\$(git -C work rev-parse HEAD)

# push_cfg <arch> <trimpath> - push one build-config.json as a new commit
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
  echo "ARM64_HOST is empty - skipping the arm64 mirror check (set it in e2e/env.local.sh to run it)."
fi

echo
echo "===================== build env check ====================="
if [ $fail -eq 0 ]; then
  echo "✅ Auditor refuses to rebuild on an instance whose OS/architecture/Go differ from what the Vendor declared, and passes on a match"
else
  echo "❌ $fail verdict(s) wrong — see the ❌ lines above"
fi
echo "next: none - this script is the end of this test"
exit $fail
