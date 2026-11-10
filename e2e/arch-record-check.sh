#!/usr/bin/env bash
# Build-environment record check. The same source gives different PCRs on x86_64 and arm64 (measured), and a different Go
#    toolchain changes them too, so registrations and the handoff record (measurement-record.json) name the architecture
#    and toolchain they came from (build_env). The chain registration carries the architecture, and the record key depends
#    on the measurement hash alone. Three groups of checks run, and no transaction is sent:
#      1. the uname -m -> GOARCH mapping used by the flow scripts comes out right on the real instances (x86 Vendor -> amd64, arm64 host -> arm64)
#      2. att-auditor send refuses a record without build_env; for a record with it, send prints the build environment on
#         one line and pushes it to a local bare repository; with --config (build-config.json) and --measurements
#         (measurements.json) in place of --record, send builds the record itself
#      3. att-auditor register refuses the uname spelling (x86_64) before it reads the signing key or contacts the chain
#    A register --arch arm64 on the chain is a transaction; cmd/buildenv_record_e2e_test.go covers it with anvil.
#
#    Needs: ARM64_HOST in e2e/env.local.sh to check item 1 on the arm64 side too. When empty, only x86 is checked and a notice is printed.
#    Run from the repository root:  bash "e2e/arch-record-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST
fail=0

# The mapping the flow scripts use (the 1-*.sh in flow1 and flow2): uname -m name to the GOARCH name att uses.
goarch_of() { case "$1" in x86_64) echo amd64;; aarch64|arm64) echo arm64;; *) echo "";; esac; }

# 1. Read uname -m on the real instance and map it; the result is the arch the flow scripts write into build-config.json and measurement-record.json.
check_host() {  # <ssh-func> <label> <expected GOARCH>
  sshf=$1; label=$2; want=$3
  step "ARCH OF $label"
  raw=$($sshf 'uname -m' 2>/dev/null | tr -d '\r\n')
  got=$(goarch_of "$raw")
  if [ "$got" = "$want" ]; then
    echo "✅ $label: uname -m = $raw -> arch $got"
  else
    echo "❌ $label: uname -m = '$raw' -> arch '$got', expected $want"
    fail=1
  fi
}
check_host V "Vendor (x86_64)" amd64
if [ -n "$ARM64_HOST" ]; then
  check_host R "arm64 host" arm64
else
  echo "ARM64_HOST is empty - skipping the arm64 mapping (set it in e2e/env.local.sh to check it)."
fi

# 2. send - against a local bare repository, with no network or chain. The record layout is the one att-auditor send pushes.
#    The record's build_env.go is this machine's Go, the Go that built the att binaries (0-Setup-local-upload.sh accepts only the pinned one).
GOVER=$(go env GOVERSION)
step "SEND: RECORD WITHOUT BUILD_ENV IS REFUSED"
WD="$WORK/build-env-check"; rm -rf "$WD"; mkdir -p "$WD"
printf 'eif-bytes' > "$WD/svc.eif"
pcr_b64() { head -c 48 /dev/zero | tr '\0' "$1" | base64 | tr -d '\n'; }   # the values in the record file are base64 bytes
write_record() {  # <path> <build_env-line or empty>
  cat > "$1" <<EOF
{
  "schema": "attestamp.measurement-record/1",
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws-nitro",
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
export GIT_AUTHOR_NAME=att-e2e GIT_AUTHOR_EMAIL=att-e2e@example.invalid GIT_COMMITTER_NAME=att-e2e GIT_COMMITTER_EMAIL=att-e2e@example.invalid
write_record "$WD/no-build-env.json" ""
git init --quiet --bare --initial-branch=main "$WD/remote-a.git"
out=$("$AUDITOR_BIN" send --repo "$WD/remote-a.git" --record "$WD/no-build-env.json" --eif "$WD/svc.eif" 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -ne 0 ] && echo "$out" | grep -q 'build_env.arch: want one of amd64|arm64' && echo "$out" | grep -q 'build_env.go'; then
  echo "✅ send: a record without build_env is refused - the message names build_env (exit $rc)"
else
  echo "❌ send: a record without build_env was not refused, or the message does not name build_env (exit $rc)"
  fail=1
fi

step "SEND: RECORD WITH BUILD_ENV IS SENT, BUILD ENV PRINTED"
write_record "$WD/with-build-env.json" '  "build_env": { "os": "linux", "arch": "amd64", "go": "'"$GOVER"'", "trimpath": true },
'
git init --quiet --bare --initial-branch=main "$WD/remote-b.git"
out=$("$AUDITOR_BIN" send --repo "$WD/remote-b.git" --record "$WD/with-build-env.json" --eif "$WD/svc.eif" 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -eq 0 ] && echo "$out" | grep -q "build env: linux/amd64 $GOVER trimpath=true" && git -C "$WD/remote-b.git" show HEAD:measurement-record.json 2>/dev/null | grep -q '"build_env"'; then
  echo "✅ send: printed build env: linux/amd64 $GOVER trimpath=true, and the pushed measurement-record.json carries build_env"
else
  echo "❌ send: exit $rc - build env was not printed, or the pushed record has no build_env"
  fail=1
fi

# 2b. send --config --measurements - send builds the record from build-config.json and nitro-cli's measurements.json.
#     The pushed record's service and build_env equal those of the config.
step "SEND: RECORD BUILT FROM BUILD-CONFIG AND MEASUREMENTS"
pcr_hex() { head -c 48 /dev/zero | tr '\0' "$1" | od -An -v -tx1 | tr -d ' \n'; }
cat > "$WD/build-config.json" <<EOF
{
  "schema": "attestamp.build-config/1",
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws-nitro",
  "source": { "repo": "https://github.com/qixiangme/docker-nginx", "commit": "ef5a25a6314e652a2dcfbce6074bef729f4d9639", "path": "mainline/alpine-slim" },
  "tee_info": { "instance_id": "i-0check", "region": "eu-north-1", "ami_id": "ami-0check" },
  "build_env": { "os": "linux", "arch": "arm64", "go": "$GOVER", "trimpath": true }
}
EOF
cat > "$WD/measurements.json" <<EOF
{ "Measurements": { "HashAlgorithm": "Sha384 { ... }", "PCR0": "$(pcr_hex 1)", "PCR1": "$(pcr_hex 2)", "PCR2": "$(pcr_hex 3)" } }
EOF
git init --quiet --bare --initial-branch=main "$WD/remote-c.git"
out=$("$AUDITOR_BIN" send --repo "$WD/remote-c.git" --config "$WD/build-config.json" --measurements "$WD/measurements.json" --eif "$WD/svc.eif" --build-tool-version 1.3.0 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
pushed=$(git -C "$WD/remote-c.git" show HEAD:measurement-record.json 2>/dev/null)
if [ $rc -eq 0 ] && echo "$out" | grep -q "record $SERVICE $VERSION aws-nitro · build env: linux/arm64 $GOVER trimpath=true" \
   && echo "$pushed" | grep -q "\"service\": \"$SERVICE\"" && echo "$pushed" | grep -q '"arch": "arm64"' && echo "$pushed" | grep -q "\"go\": \"$GOVER\"" \
   && [ -f "$WD/measurement-record.json" ]; then
  echo "✅ send --config --measurements: built and sent the record; the pushed record's service and build_env are exactly those of build-config.json (measurement-record.json is also left next to measurements.json)"
else
  echo "❌ send --config --measurements: exit $rc - the record was not built, or the pushed record's service/build_env differ from the config"
  fail=1
fi

# 3. register - the uname spelling is refused. All required flags are given; the --arch check runs before the signing key file is read, so the RPC address and key file are placeholders.
step "REGISTER: --arch x86_64 IS REFUSED BEFORE TOUCHING THE CHAIN"
pcr_hex() { head -c 48 /dev/zero | tr '\0' "$1" | od -An -v -tx1 | tr -d ' \n'; }
out=$("$AUDITOR_BIN" register --rpc http://127.0.0.1:1 --registry 0x0000000000000000000000000000000000000001 --signing-key-file "$WD/no-such.key" \
  --measurements "PCR0=$(pcr_hex 1),PCR1=$(pcr_hex 2),PCR2=$(pcr_hex 3)" --service "$SERVICE" --version "$VERSION" --arch x86_64 2>&1); rc=$?
echo "$out" | sed 's/^/  /'
if [ $rc -ne 0 ] && echo "$out" | grep -q -- '--arch: want one of amd64|arm64'; then
  echo "✅ register: x86_64 is refused - it says to write amd64 (exit $rc)"
else
  echo "❌ register: --arch x86_64 was not refused, or the message differs (exit $rc)"
  fail=1
fi

echo
echo "===================== build env record check ====================="
[ $fail -eq 0 ] && echo "✅ all passed - the record and registration output state the build environment (architecture, Go toolchain)" || echo "❌ some checks failed (see the ❌ lines above)"
echo "next: none - this script is the end of this test"
exit $fail
