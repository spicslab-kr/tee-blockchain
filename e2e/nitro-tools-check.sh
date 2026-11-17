#!/usr/bin/env bash
# Docker and nitro-cli between the Vendor and the Auditor (gap 27 of docs/results/build-info-gaps-2026-11.md), on the two real
#    instances. build installs both with dnf and no version, and both decide the measurements (PCR1 follows nitro-cli's
#    kernel and init; docker builds the image). att-vendor send records what the Vendor built with in build_env, and the
#    Auditor checks it at two points:
#      receive  the Amazon Linux release (build_env.ami_release), before anything is installed. A proxy, not a
#               guarantee: each release has its own package repository, but a dnf update with another release,
#               releasever=latest or a package installed by hand moves the versions without moving the release
#      build    with --config, the docker and aws-nitro-enclaves-cli versions themselves (build_env.docker, nitro_cli),
#               right after dnf installed them. This is the real check
#    Steps:
#      1. Vendor instance: att-vendor build of a one-file application (it installs docker and nitro-cli), then
#         att-vendor send of a build-config.json without build_env into a local bare repository. The pushed build_env
#         must carry this instance's release (/etc/os-release) and package versions (rpm -q)
#      2. Auditor instance, with that config: receive passes and prints "ami release matches"
#      3. the same config with another ami_release: receive is refused naming it; with --ignore-env, a warning, exit 0
#      4. build --config with the Vendor's config: "docker and nitro-cli match the Vendor's" is printed
#      5. build --config with another docker version: refused right after the install, naming docker, before any image
#         is built
#    Needs bash e2e/0-Setup-local-upload.sh first (att-vendor, att-auditor and att-ra-responder on the instances).
#    Both instances must be the same AMI release, as the flows use them.
#    Run from the repository root:  bash "e2e/nitro-tools-check.sh"
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/.."
. "$here/env.sh"

need_host VENDOR_HOST; need_host AUDITOR_HOST
mkdir -p "$WORK"
fail=0

# json_value <file> <key> - the string value of one key of a JSON file written with or without spaces after the colon
json_value() { grep -oE "\"$2\": *\"[^\"]*\"" "$1" | head -1 | sed -E 's/.*"([^"]*)"$/\1/'; }

step "1. VENDOR: BUILD (INSTALLS THE TOOLS), THEN send RECORDS THEM"
nitroBanner "Vendor"
V 'bash -s' <<'REMOTE' | tee "$WORK/nitro-tools-vendor.log"
set -uo pipefail
rm -rf ntc && mkdir ntc && cd ntc
mkdir app && printf 'hello\n' > app/hello && printf 'FROM scratch\nCOPY hello /hello\nCMD ["/hello"]\n' > app/Dockerfile
~/att-vendor build --app-dir app --out-dir out > build.txt 2>&1; echo "build exit $?"
grep -a 'tools:' build.txt
git init -q --bare --initial-branch=main remote.git
git clone -q remote.git work && git -C work config user.email t@t && git -C work config user.name t
echo seed > work/seed && git -C work add seed && git -C work commit -qm seed && git -C work push -q -u origin main
seed=$(git -C work rev-parse HEAD)
cat > cfg.json <<CFG
{ "schema": "attestamp.build-config/1", "service": "ntc", "version": "1.0.0", "platform": "aws-nitro",
  "source": { "repo": "https://example.com/ntc.git", "commit": "$seed" },
  "tee_info": { "instance_id": "i-0", "region": "r", "ami_id": "ami-0" } }
CFG
~/att-vendor send --repo "$PWD/remote.git" --config cfg.json --workdir work > send.txt 2>&1; echo "send exit $?"
cat send.txt
cp work/build-config.json sent.json && cat sent.json
echo "@@REL $(. /etc/os-release; echo "$PRETTY_NAME")"
echo "@@RPM $(rpm -q --queryformat '%{VERSION}-%{RELEASE} ' docker aws-nitro-enclaves-cli)"
REMOTE
$SCP "$SSH_USER@$VENDOR_HOST:ntc/sent.json" "$WORK/ntc-sent.json"
localBanner

REL_HERE=$(sed -n 's/^@@REL //p' "$WORK/nitro-tools-vendor.log" | grep -oE '20[0-9]{2}\.[0-9]+\.[0-9]{8}')
read -r DOCKER_HERE NITRO_HERE < <(sed -n 's/^@@RPM //p' "$WORK/nitro-tools-vendor.log")
REL_SENT=$(json_value "$WORK/ntc-sent.json" ami_release)
DOCKER_SENT=$(json_value "$WORK/ntc-sent.json" docker)
NITRO_SENT=$(json_value "$WORK/ntc-sent.json" nitro_cli)
echo "instance: release $REL_HERE, docker $DOCKER_HERE, aws-nitro-enclaves-cli $NITRO_HERE"
echo "sent:     release $REL_SENT, docker $DOCKER_SENT, aws-nitro-enclaves-cli $NITRO_SENT"
if [ -n "$REL_HERE" ] && [ -n "$DOCKER_HERE" ] && [ "$REL_SENT" = "$REL_HERE" ] && [ "$DOCKER_SENT" = "$DOCKER_HERE" ] && [ "$NITRO_SENT" = "$NITRO_HERE" ]; then
  echo "✅ send recorded the instance's own release and versions"
  result - send-records-nitro-env identical identical "send recorded ami_release $REL_SENT, docker $DOCKER_SENT, nitro_cli $NITRO_SENT, as /etc/os-release and rpm -q say"
else
  echo "❌ the recorded build_env differs from what the instance says, or something was not read"
  result - send-records-nitro-env different identical "sent ami_release '$REL_SENT' docker '$DOCKER_SENT' nitro_cli '$NITRO_SENT'; instance '$REL_HERE' '$DOCKER_HERE' '$NITRO_HERE'"
  fail=1
fi

# The two altered configs: another release (for receive) and another docker version (for build).
sed -E 's/("ami_release": *")[^"]*"/\12000.0.20000101"/' "$WORK/ntc-sent.json" > "$WORK/ntc-other-release.json"
sed -E 's/("docker": *")[^"]*"/\10.0.0-0.not-this-one"/' "$WORK/ntc-sent.json" > "$WORK/ntc-other-docker.json"

step "2-5. AUDITOR: receive COMPARES THE RELEASE, build --config THE VERSIONS"
nitroBanner "Auditor"
$SCP "$WORK/ntc-sent.json" "$WORK/ntc-other-release.json" "$WORK/ntc-other-docker.json" "$SSH_USER@$AUDITOR_HOST:~/"
A 'bash -s' <<'REMOTE' | tee "$WORK/nitro-tools-auditor.log"
set -uo pipefail
rm -rf ntc && mkdir ntc && cd ntc
mkdir app && printf 'hello\n' > app/hello && printf 'FROM scratch\nCOPY hello /hello\nCMD ["/hello"]\n' > app/Dockerfile
git init -q --bare --initial-branch=main remote.git
git clone -q remote.git work && git -C work config user.email t@t && git -C work config user.name t
echo seed > work/seed && git -C work add seed && git -C work commit -qm seed && git -C work push -q -u origin main
seed=$(git -C work rev-parse HEAD)
# receive requires source.commit to be in the repository it clones: point the Vendor's config at this repository's seed
push_cfg() { sed -E "s/(\"commit\": *\")[0-9a-f]+\"/\1$seed\"/" "$1" > work/build-config.json && git -C work add build-config.json && git -C work commit -qm "$1" && git -C work push -q; }
n=0
ok()  { echo "✅ $2"; echo "@@RESULT $1 $3 $3 $2"; }
bad() { echo "❌ $2"; echo "@@RESULT $1 $3 $4 $2"; n=$((n+1)); }

echo "== 2. receive, the Vendor's config"
push_cfg ~/ntc-sent.json
~/att-auditor receive --repo "$PWD/remote.git" --into src2 > out2.txt 2> err2.txt; rc=$?
cat out2.txt err2.txt
if [ $rc -eq 0 ] && grep -q "ami release matches" out2.txt; then ok receive-same-release "receive passed: ami release matches (exit 0)" accepted
else bad receive-same-release "receive did not report a matching release (exit $rc)" refused accepted; fi

echo "== 3. receive, another ami_release"
push_cfg ~/ntc-other-release.json
~/att-auditor receive --repo "$PWD/remote.git" --into src3 > out3.txt 2> err3.txt; rc=$?
cat out3.txt err3.txt
if [ $rc -ne 0 ] && grep -q "ami_release: Vendor built on Amazon Linux 2000.0.20000101" err3.txt; then ok receive-other-release "receive refused another release, naming it (exit $rc)" refused
else bad receive-other-release "receive did not refuse another release (exit $rc)" accepted refused; fi
~/att-auditor receive --repo "$PWD/remote.git" --into src3i --ignore-env > out3i.txt 2> err3i.txt; rc=$?
cat out3i.txt err3i.txt
if [ $rc -eq 0 ] && grep -q "warning:.*ami_release" err3i.txt; then ok receive-other-release-ignored "with --ignore-env: a warning naming ami_release, exit 0" accepted
else bad receive-other-release-ignored "with --ignore-env: no warning or not exit 0 (exit $rc)" refused accepted; fi

echo "== 4. build --config, the Vendor's config"
~/att-auditor build --config ~/ntc-sent.json --app-dir app --out-dir out4 > out4.txt 2>&1; rc=$?
cat out4.txt
if grep -q "docker and nitro-cli match the Vendor's" out4.txt; then ok build-same-tools "build: docker and nitro-cli match the Vendor's (build exit $rc)" accepted
else bad build-same-tools "build did not report matching tools (exit $rc)" refused accepted; fi

echo "== 5. build --config, another docker version"
~/att-auditor build --config ~/ntc-other-docker.json --app-dir app --out-dir out5 > out5.txt 2>&1; rc=$?
cat out5.txt
if [ $rc -ne 0 ] && grep -q "docker: Vendor built with 0.0.0-0.not-this-one" out5.txt && ! grep -q "build 2/5" out5.txt; then
  ok build-other-docker "build refused another docker right after installing, before building anything (exit $rc)" refused
else bad build-other-docker "build did not refuse another docker before building (exit $rc)" accepted refused; fi
exit $n
REMOTE
rc=${PIPESTATUS[0]}
localBanner
grep '^@@RESULT ' "$WORK/nitro-tools-auditor.log" | while read -r _ check res exp reason; do
  result - "$check" "$res" "$exp" "$reason"
done
[ "$rc" = 0 ] || fail=1

echo
echo "===================== docker and nitro-cli check ====================="
if [ $fail = 0 ]; then
  echo "✅ send records the release and the versions; receive compares the release; build --config compares the versions"
else
  echo "❌ some checks failed (see the ❌ lines above)"
fi
echo "next: none - this script is the end of this test"
exit $fail
