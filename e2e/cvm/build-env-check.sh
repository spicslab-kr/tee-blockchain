#!/bin/sh
# T7 on a confidential VM: the CVM version of e2e/build-env-check.sh.
#
# The Vendor records the environment it built in (OS, CPU architecture, Go toolchain, -trimpath) as build_env in
# build-config.json, and att-auditor receive compares it with the Auditor's own machine. The comparison has no platform
# branch (internal/core/transfer/auditor/build_env_check.go), so what this adds over the Nitro run is the other half:
# that a Vendor on the guest records the guest's environment, and that the Auditor on a guest accepts exactly that.
#
#   0. the Vendor sends a CVM build-config.json without build_env -> att-vendor send fills in the guest's own
#   1. a config declaring the other architecture                  -> receive refuses, naming both
#   2. the same, with --ignore-env                                 -> a warning, exit 0
#   3. the config att-vendor recorded in step 0, again             -> receive passes, "build env matches"
#
# Usage, on the guest, from a folder holding att-vendor and att-auditor (linux/amd64, built from the same commit):
#   sh build-env-check.sh amd-sev-snp-initrd
#   sh build-env-check.sh intel-tdx
# No root and no network: a bare repository in a temporary folder stands for the Vendor's repository. Prints PASS/FAIL
# per check, exits with the number of failed checks, and writes results.jsonl (the results contract of e2e/env.sh,
# docs/status/test-matrix.md) in the folder it prints. Copy it back and append it to e2e-work/results/<format>.jsonl.
set -u
FMT=$1
HERE=$(pwd)
W=$(mktemp -d)
echo "work folder: $W"
RUN=$(date -u +%Y%m%dT%H%M%SZ)

json_str() { printf '%s' "$1" | tr '\r\n\t' '   ' | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }
# record <check> <result> <expected> <reason>: one line of the results contract.
record() {
	printf '{"platform":"%s","chain":"local","test":"T7","check":"%s","script":"e2e/cvm/build-env-check.sh","result":"%s","expected":"%s","reason":"%s","detail":"on the guest, %s","run":"%s","at":"%s"}\n' \
		"$(json_str "$FMT")" "$1" "$2" "$3" "$(json_str "$4")" "$(json_str "$(uname -m)")" "$RUN" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$W/results.jsonl"
}
fail=0
# check <ok 0|1> <check> <result if ok> <expected> <text>
check() {
	if [ "$1" -eq 0 ]; then
		echo "PASS  $5"; record "$2" "$3" "$4" "$5"
	else
		echo "FAIL  $5"; fail=$((fail + 1)); record "$2" error "$4" "FAILED: $5"
	fi
}

case $(uname -m) in
x86_64) MINE=amd64; OTHER=arm64 ;;
aarch64 | arm64) MINE=arm64; OTHER=amd64 ;;
*) echo "unknown machine $(uname -m)"; exit 1 ;;
esac
# The Go att-vendor was built with, as the binary reports it ("this binary: goX.Y.Z").
GOVER=$("$HERE/att-vendor" version | sed -n 's/.*this binary: \(go[0-9.]*\).*/\1/p')
echo "this guest: $(uname -m) ($MINE), att-vendor and att-auditor built with $GOVER, format $FMT"

# Commits need an author; set it for this script only, not in the guest's git config.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
cd "$W" || exit 1
git init -q --bare --initial-branch=main remote.git
git clone -q remote.git seed 2> /dev/null
echo 'package main' > seed/main.go
git -C seed add main.go
git -C seed commit -qm seed
git -C seed push -q -u origin main
SEED=$(git -C seed rev-parse HEAD)

# The tee_info keys a CVM config must carry (TEEInfoKeys); placeholders, not this guest's.
cat > vendor-config.json <<CFG
{
  "schema": "attestamp.build-config/1",
  "service": "svc",
  "version": "1.2.0",
  "platform": "$FMT",
  "source": { "repo": "https://example.com/svc.git", "commit": "$SEED" },
  "tee_info": { "instance_name": "svc-guest", "project_id": "example-project", "zone": "us-central1-a", "image": "ubuntu-2404-noble-amd64-v20261105" }
}
CFG

echo
echo "== 0. the Vendor sends a $FMT config without build_env"
"$HERE/att-vendor" send --repo "$W/remote.git" --config vendor-config.json > send.txt 2>&1
rc=$?
cat send.txt
git clone -q remote.git sent 2> /dev/null
cp sent/build-config.json recorded.json
echo "recorded build_env:"
sed -n '/"build_env"/,/}/p' recorded.json
ok=1
if [ $rc -eq 0 ] && grep -q "build env: linux/$MINE $GOVER trimpath=true" send.txt &&
	tr -d ' \n' < recorded.json | grep -q "\"build_env\":{\"os\":\"linux\",\"arch\":\"$MINE\",\"go\":\"$GOVER\",\"trimpath\":true}"; then
	ok=0
fi
check $ok recorded-build-env identical identical "att-vendor send on the guest recorded build_env linux/$MINE $GOVER trimpath=true, this guest's own"

# push_arch <arch>: the recorded config with only build_env.arch changed, as a new commit.
push_arch() {
	sed "s/\"arch\": \"$MINE\"/\"arch\": \"$1\"/" recorded.json > sent/build-config.json
	git -C sent commit -qam "cfg $1"
	git -C sent push -q
}

echo
echo "== 1. a config declaring $OTHER must be refused"
push_arch $OTHER
"$HERE/att-auditor" receive --repo "$W/remote.git" --into src1 > out1.txt 2> err1.txt
rc=$?
cat out1.txt err1.txt
ok=1
if [ $rc -ne 0 ] && grep -q "$OTHER" err1.txt && grep -q "$MINE" err1.txt; then ok=0; fi
check $ok other-arch-refused refused refused "receive refused a $FMT config declaring $OTHER on a $MINE guest (exit $rc), naming both"

echo
echo "== 2. the same, with --ignore-env: a warning only"
"$HERE/att-auditor" receive --repo "$W/remote.git" --into src2 --ignore-env > out2.txt 2> err2.txt
rc=$?
cat out2.txt err2.txt
ok=1
if [ $rc -eq 0 ] && grep -q "warning:" err2.txt && grep -q "$OTHER" err2.txt; then ok=0; fi
check $ok ignore-env-warns accepted accepted "with --ignore-env receive passed with a warning naming $OTHER (exit $rc)"

echo
echo "== 3. the config att-vendor recorded in step 0 must pass"
cp recorded.json sent/build-config.json
git -C sent commit -qam "cfg as recorded"
git -C sent push -q
"$HERE/att-auditor" receive --repo "$W/remote.git" --into src3 > out3.txt 2> err3.txt
rc=$?
cat out3.txt err3.txt
ok=1
if [ $rc -eq 0 ] && grep -q "build env matches" out3.txt; then ok=0; fi
check $ok own-arch-accepted accepted accepted "receive accepted the build_env att-vendor recorded on this guest (build env matches, exit $rc)"

echo
echo "results: $W/results.jsonl"
if [ $fail -eq 0 ]; then echo "PASS: all four verdicts as expected"; else echo "FAIL: $fail verdict(s) wrong"; fi
exit $fail
