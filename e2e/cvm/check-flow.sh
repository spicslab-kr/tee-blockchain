#!/usr/bin/env bash
# Runs the CVM flow scripts (0-Setup-guests.sh, flow1-auditor-registers/ and flow2-vendor-self-registers/) against
# stand-in gcloud, go and curl and stand-in att binaries, and checks what each script asked of them. No guest, no
# chain and no network are used, and nothing is written outside a temporary folder.
#
#   bash e2e/cvm/check-flow.sh          # also run by go test ./cmd (TestCVMFlowScripts)
#
# What it checks: the arguments (format, option); which binary, file and hook script goes to which guest; that every
# guest command is a script copied with scp and run with ssh --command, never piped into ssh; the boot_id wait (a new
# boot after the old one and a failed connection, and the timeout with its message); the audit and lookup calls; the
# exit codes 2b and 4b accept (0 and 12); and the verify line the flow prints. For flow 2: which registry each step
# deploys and with which approver, self-register's calls, that step 4 needs flow 1's step 1 and passes on exit 12 only,
# and its verify line with both registries. It also runs the guest-side script here, with a stand-in sudo and att, for
# the launch and build flags each format gets.
# What it cannot check, because only a real guest has it: gcloud ssh and scp through IAP from this machine, the build,
# the GRUB entry, the reboot itself, launch --check, the responder on port 5000, and the chain.
#
# Safety: PATH is the stand-in folder and /usr/bin:/bin only, and nothing runs unless `command -v gcloud go curl` are the
# stand-ins. WORK and BIN (env.sh) point into the temporary folder; the stand-in go writes nothing outside it.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
flow="$here/flow1-auditor-registers"
flow2="$here/flow2-vendor-self-registers"
tmp=$(mktemp -d) || exit 1
trap 'rm -rf "$tmp"' EXIT
fake="$tmp/fakebin"; mkdir -p "$fake" "$tmp/state"
log="$tmp/calls.log"
export STANDIN_LOG="$log" STANDIN_STATE="$tmp/state" STANDIN_TMP="$tmp" STANDIN_ROOT="$root"

cat > "$fake/gcloud" <<'EOF'
#!/usr/bin/env bash
printf 'gcloud %s\n' "$*" >> "$STANDIN_LOG"
a=" $* "
case "$a" in
  *" compute disks describe "*)
    case "$4" in *auditor) echo "${FAKE_AUDITOR_IMAGE:-ubuntu-2404-noble-amd64-v20261105}" ;; *) echo ubuntu-2404-noble-amd64-v20261105 ;; esac ;;
  *" compute instances describe "*machineType*) echo c3-standard-4 ;;
  *" compute instances describe "*natIP*)
    case "$4" in *vendor) echo 192.0.2.10 ;; *auditor) echo 192.0.2.20 ;; esac ;;
  *" compute ssh "*boot_id*)
    # The boot ID: the first query (before the reboot) gives the old one; then FAKE_FAIL failed connections, then
    # FAKE_SAME answers from the old boot, then the new one. FAKE_BOOT=never keeps the old one.
    f="$STANDIN_STATE/boot-$3"; q=$(( $(cat "$f" 2>/dev/null || echo 0) + 1 )); echo "$q" > "$f"
    fail=${FAKE_FAIL:-0}; same=${FAKE_SAME:-0}
    if [ "$q" -eq 1 ] || [ "${FAKE_BOOT:-change}" = never ]; then echo 11111111-old; exit 0; fi
    if [ $((q - 1)) -le "$fail" ]; then echo "Connection closed by 192.0.2.10 port 22" >&2; exit 255; fi
    if [ $((q - 1)) -le $((fail + same)) ]; then echo 11111111-old; exit 0; fi
    echo 22222222-new ;;
  *" compute ssh "*"--command sha256sum "*)
    case "$3" in *auditor) [ -n "${FAKE_BASE_DIFF:-}" ] && { echo "2222  base"; exit 0; } ;; esac
    echo "1111  base" ;;
  # The exit code the guest-side action recorded (FAKE_MARKER=none: nothing recorded), read after gcloud failed.
  *" compute ssh "*"cat .attestamp-step.rc"*) [ "${FAKE_MARKER:-0}" = none ] || echo "${FAKE_MARKER:-0}" ;;
  # gcloud crashing as the check-serve session closes, after the action finished (seen on 2026-11-19).
  *" compute ssh "*"attestamp-guest-step.sh check-serve"*)
    [ -n "${FAKE_CRASH_CHECK:-}" ] && { echo "Segmentation fault" >&2; exit 139; } ;;
  *" compute ssh "*) exit "${FAKE_SSH_EXIT:-0}" ;;
  *" compute scp "*)
    case "$3" in *:*) printf '{"MRTD":"aa","RTMR0":"bb"}\n' > "$4" ;; esac ;;
esac
exit 0
EOF
cat > "$fake/go" <<'EOF'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >> "$STANDIN_LOG"
[ "$1 $2" = "env GOVERSION" ] && { tr -d '[:space:]' < "$STANDIN_ROOT/internal/setup/build/GO_TOOLCHAIN"; echo; }
exit 0   # builds nothing: the stand-in att binaries are already in place, and linux/ is never touched
EOF
cat > "$fake/curl" <<'EOF'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >> "$STANDIN_LOG"
echo '{"jsonrpc":"2.0","id":1,"result":"0x7a69"}'
EOF
chmod +x "$fake/gcloud" "$fake/go" "$fake/curl"

export PATH="$fake:/usr/bin:/bin"
for c in gcloud go curl; do
  if [ "$(command -v "$c")" != "$fake/$c" ]; then
    echo "check-flow: '$c' resolves to '$(command -v "$c")', not the stand-in; stopping before running any script" >&2
    exit 1
  fi
done
unset CLOUDSDK_CORE_PROJECT PREFIX ZONE IAP REBOOT_TIMEOUT REBOOT_POLL CVM_SERVICE CVM_VERSION RESULTS_PLATFORM
export CLOUDSDK_CONFIG="$tmp/gcloud-config"

# The stand-in att binaries env.sh's BIN points at (with .exe in Git Bash, as env.sh names them).
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) exe=.exe;; *) exe="";; esac
mkatt() {   # mkatt <work folder>
  mkdir -p "$1/bin"
  local b
  for b in att-auditor att-vendor att-user; do
    cat > "$1/bin/$b$exe" <<EOF
#!/usr/bin/env bash
printf '$b %s\n' "\$*" >> "\$STANDIN_LOG"
out=""; prev=""; for x in "\$@"; do [ "\$prev" = --out-file ] && out=\$x; prev=\$x; done
case "\$1" in
  registry-deploy) echo "Registry deployed at 0x5FbDB2315678afecb367f032d93F642f64180aa3" ;;
  audit) exit "\${FAKE_AUDIT_EXIT:-0}" ;;
  self-register) exit "\${FAKE_SELFREG_EXIT:-0}" ;;
  lookup) echo "  record key  0x$(printf 'ab%.0s' $(seq 32))" ;;
  ra-request|ra-parse) [ -n "\$out" ] && echo '{"MRTD":"aa"}' > "\$out" ;;
  compare) exit "\${FAKE_COMPARE_EXIT:-0}" ;;
esac
exit 0
EOF
    chmod +x "$1/bin/$b$exe"
  done
}

fails=0; n=0; cur=""; bad=""; out=""; got=0
AUDITOR=0x70997970C51812dc3A010C7d01b50e0d17dc79C8
VENDOR=0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
# run <env assignments...> <script> <args...> - runs a script with WORK and BIN in the current work folder.
run() {
  : > "$log"; rm -f "$STANDIN_STATE"/*
  out=$(env WORK="$work" BIN="$work/bin" AUDITOR_ADDR=$AUDITOR VENDOR_ADDR=$VENDOR REBOOT_POLL=0 "$@" 2>&1 </dev/null); got=$?
}
start()  { cur=$1; n=$((n + 1)); bad=""; }
exitis() { [ "$got" = "$1" ] || bad="$bad; exit $got, want $1"; }
call()   { grep -qE -- "$1" "$log" || bad="$bad; no call matching /$1/"; }
nocall() { ! grep -qE -- "$1" "$log" || bad="$bad; a call matches /$1/, want none"; }
said()   { printf '%s\n' "$out" | grep -qE -- "$1" || bad="$bad; output lacks /$1/"; }
nosaid() { ! printf '%s\n' "$out" | grep -qE -- "$1" || bad="$bad; output has /$1/, want none"; }
finish() {
  if [ -n "$bad" ]; then
    fails=$((fails + 1)); echo "FAIL  $cur:${bad#;}"
    echo "      output:"; printf '%s\n' "$out" | tail -25 | sed 's/^/        /'
    echo "      calls:";  sed 's/^/        /' "$log"
  else
    echo "ok    $cur"
  fi
}
newwork() { work="$tmp/work-$1"; rm -rf "$work"; mkatt "$work"; }

S="$here/0-Setup-guests.sh"
F1="$flow/1-Auditor-rebuild-launch-audit.sh"; F2="$flow/2-Vendor-build-launch-check-record.sh"
F2b="$flow/2b-Auditor-audit-vendor.sh"; F4="$flow/4-Vendor-tamper-rebuild-launch.sh"
F4b="$flow/4b-Auditor-audit-vendor-tampered.sh"
G1="$flow2/1-Vendor-build-launch-self-register.sh"; G3="$flow2/3-Vendor-tamper-rebuild-self-register.sh"
G4="$flow2/4-Auditor-audit-vendor.sh"
REG=0x5FbDB2315678afecb367f032d93F642f64180aa3
SEPOLIA=https://ethereum-sepolia-rpc.publicnode.com

# Arguments
newwork args
start "a format that does not exist is refused before any call"; run bash "$F1" bogus; exitis 2; nocall '^gcloud '; finish
start "intel-tdx takes option 1 or 2 only";                      run bash "$S" intel-tdx 3; exitis 2; nocall '^gcloud '; finish
start "step 2 without step 1's registration stops before any call"; run bash "$F2" intel-tdx; exitis 1; nocall '^gcloud '; said 'run 1-Auditor'; finish
# -c as the last argument: `shift 2` with one argument left shifts nothing, and the option loop ran for ever until
# 2026-11-20. timeout ends a script that loops again; 124 is its exit code then.
start "-c with no address stops with exit 2 before any call (flow 1 step 1)"
run timeout 20 bash "$F1" intel-tdx -c; exitis 2; nocall '^gcloud '; said '-c needs a registry address'; finish
start "-c with no address stops with exit 2 before any call (flow 2 step 4)"
run timeout 20 bash "$G4" intel-tdx -c; exitis 2; nocall '^gcloud '; said '-c needs a registry address'; finish

# 0-Setup-guests.sh
newwork setup
start "setup intel-tdx: binaries to their guests, both prepared, through IAP, no hooks"
run bash "$S" intel-tdx; exitis 0
call 'compute disks describe attestamp-tdx-vendor'; call 'compute instances describe attestamp-tdx-auditor .*machineType'
call 'compute scp linux/att-vendor attestamp-tdx-vendor:att-vendor\.attestamp-new .*--tunnel-through-iap'
call 'compute scp linux/att-ra-responder attestamp-tdx-auditor:att-ra-responder\.attestamp-new '
call 'compute scp linux/att-auditor attestamp-tdx-auditor:att-auditor\.attestamp-new '
call 'compute scp .*guest/guest-step.sh attestamp-tdx-vendor:attestamp-guest-step\.sh\.attestamp-new '
call 'compute ssh attestamp-tdx-vendor .*--command .*bash attestamp-guest-step.sh prepare vendor intel-tdx 1'
call 'compute ssh attestamp-tdx-auditor .*--command .*bash attestamp-guest-step.sh prepare auditor intel-tdx 1'
call 'go build .*-trimpath -buildvcs=false -ldflags=-buildid= -o linux/att-ra-responder'
nocall 'initramfs-hook'
# Every copy to a guest goes to a temporary name.
if grep -E 'compute scp [^ ]+ [^ ]+:' "$log" | grep -v '\.attestamp-new ' > "$tmp/direct.txt"; then bad="$bad; copied straight onto its name: $(cat "$tmp/direct.txt")"; fi
finish
start "setup renames each copied file into place, so a running responder cannot stop the copy (R8)"
call 'compute ssh attestamp-tdx-vendor .*--command mv -f att-vendor\.attestamp-new att-vendor && mv -f att-ra-responder\.attestamp-new att-ra-responder$'
call 'compute ssh attestamp-tdx-auditor .*--command mv -f att-auditor\.attestamp-new att-auditor && mv -f att-ra-responder\.attestamp-new att-ra-responder$'
call 'compute ssh attestamp-tdx-vendor .*--command rm -f \.attestamp-step\.rc; mv -f attestamp-guest-step\.sh\.attestamp-new attestamp-guest-step\.sh \|\| exit 1; bash attestamp-guest-step\.sh prepare '
finish
start "a copy to a path, not a plain name in the guest's home, is refused before any copy"
run bash -c "cd \"$root\" && . e2e/env.sh && . e2e/cvm/env-cvm.sh intel-tdx && guest_put attestamp-tdx-vendor linux/att-vendor sub/att-vendor"
exitis 2; nocall 'compute scp'; said 'not a plain file name'; finish
start "setup refuses guests of different images, before copying anything"
run FAKE_AUDITOR_IMAGE=ubuntu-2404-noble-amd64-v20990101 bash "$S" intel-tdx; exitis 1; nocall 'compute scp'; said 'differ in image'; finish
start "setup refuses a base initrd that differs between the guests"
run FAKE_BASE_DIFF=1 bash "$S" amd-sev-snp-initrd; exitis 1; said 'base initrd differs'; finish
start "setup intel-tdx option 2: TDX's hooks to the Vendor, its base-initrd-v.img to the Auditor"
run bash "$S" intel-tdx 2; exitis 0
call 'compute scp internal/tee/parse/tdx/testdata/record/initramfs-hook.sh attestamp-tdx-vendor:initramfs-hook\.sh\.attestamp-new '
call 'compute scp attestamp-tdx-vendor:base-initrd-v.img '
call 'compute scp .*vendor-base-initrd-v.img attestamp-tdx-auditor:base-initrd-v\.img\.attestamp-new '
call 'compute ssh attestamp-tdx-vendor .*--command mv -f .* && mv -f initramfs-local-bottom\.sh\.attestamp-new initramfs-local-bottom\.sh$'
call 'compute ssh attestamp-tdx-auditor .*--command mv -f .* && mv -f base-initrd-v\.img\.attestamp-new base-initrd-v\.img$'
call 'prepare auditor intel-tdx 2'; finish
start "setup amd-sev-snp-verity: SEV-SNP's hooks, guests named attestamp-snp-*"
run bash "$S" amd-sev-snp-verity; exitis 0
call 'compute scp internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh attestamp-snp-vendor:initramfs-local-bottom\.sh\.attestamp-new '
call 'prepare vendor amd-sev-snp-verity 2'; finish

# The flow, intel-tdx on a local chain
newwork tdx
start "1 intel-tdx: build, install, reboot after a failed connection and the old boot, check, audit, lookup"
run NET=local FAKE_FAIL=1 FAKE_SAME=2 bash "$F1" intel-tdx; exitis 0
call 'guest-step.sh app auditor intel-tdx 1 original'; call 'guest-step.sh build auditor intel-tdx 1'
call 'guest-step.sh install auditor intel-tdx 1'; call 'compute ssh attestamp-tdx-auditor .*--command sudo systemd-run --on-active=3 systemctl reboot'
call 'guest-step.sh check-serve auditor intel-tdx 1'
call 'att-auditor registry-deploy --rpc http://127.0.0.1:8545 '
call "att-auditor audit --tee-addr 192.0.2.20:5000 --format intel-tdx --build-measurements .*auditor-measurements.json --rpc http://127.0.0.1:8545 --registry $REG"
call 'att-auditor lookup .*--platform intel-tdx'
said 'new boot on attestamp-tdx-auditor after'; said 'registered under record key 0xabab'; finish
start "1: the result line names the TDX option (both options share the format intel-tdx)"
grep -q '"detail":"option 1, registry ' "$work/results/intel-tdx.jsonl" 2>/dev/null || bad="; no result line with detail \"option 1, registry …\" in $work/results/intel-tdx.jsonl"
finish
start "1: no new boot within REBOOT_TIMEOUT fails, says so, and audits nothing"
run NET=local FAKE_BOOT=never REBOOT_TIMEOUT=2 REBOOT_POLL=1 bash "$F1" intel-tdx; exitis 1
said 'no new boot on attestamp-tdx-auditor within 2 s'; said 'the old boot \(11111111-old\) still answered'
nocall 'att-auditor audit'; nocall 'check-serve'; finish
start "1: gcloud crashing after check-serve finished goes by the exit code the guest recorded"
run NET=local FAKE_CRASH_CHECK=1 FAKE_MARKER=0 bash "$F1" intel-tdx; exitis 0
call 'cat .attestamp-step.rc'; said 'finished with exit 0; going by the guest'; call 'att-auditor audit'; finish
start "1: gcloud crashing with no exit code recorded on the guest fails the step"
run NET=local FAKE_CRASH_CHECK=1 FAKE_MARKER=none bash "$F1" intel-tdx; exitis 1
said 'did not finish'; nocall 'att-auditor audit'; finish
start "1 -c: an existing registry, none deployed"
run NET=local bash "$F1" intel-tdx -c 0x1111111111111111111111111111111111111111; exitis 0
nocall 'registry-deploy'; call 'audit .*--registry 0x1111111111111111111111111111111111111111'; finish
start "1: audit refusing ends the step with its exit code"
run NET=local FAKE_AUDIT_EXIT=7 bash "$F1" intel-tdx; exitis 7; nocall 'att-auditor lookup'; finish
run NET=local bash "$F1" intel-tdx   # step 1 again with -c reset, for steps 2 to 4b below
start "2 intel-tdx: build and boot on the Vendor's guest, then compare, lookup and compare with the chain"
run NET=local bash "$F2" intel-tdx; exitis 0
call 'guest-step.sh install vendor intel-tdx 1'
call 'att-vendor ra-request --tee-addr 192.0.2.10:5000 --format intel-tdx'
call 'att-vendor compare --attested .*parsed.json --expected .*auditor-measurements.json'
call 'att-vendor lookup .*--platform intel-tdx'; call 'att-vendor compare .*--record-key 0xabab'
said "/bin/att-user(.exe)? verify --tee-addr 192.0.2.10:5000 --format intel-tdx --rpc http://127.0.0.1:8545 --registry $REG"
said "NET=local: only this machine reaches the chain"; nosaid "On another computer"
said 'expected: trusted'; finish
start "2: a Vendor guest that differs from the Auditor's values fails the step"
run NET=local FAKE_COMPARE_EXIT=1 bash "$F2" intel-tdx; exitis 1; said MISMATCH; finish
start "2b: audit --vendor-tee on a registry of its own, exit 0 passes"
run NET=local bash "$F2b" intel-tdx; exitis 0
call 'registry-deploy'; call 'att-auditor audit --tee-addr 192.0.2.20:5000 .*--vendor-tee 192.0.2.10:5000'; finish
start "2b: any other exit fails"
run NET=local FAKE_AUDIT_EXIT=12 bash "$F2b" intel-tdx; exitis 12; finish
start "4: one byte changed, rebuilt, rebooted; the verify line expects not trusted"
run NET=local bash "$F4" intel-tdx; exitis 0
call 'guest-step.sh app vendor intel-tdx 1 tampered'; call 'guest-step.sh build vendor intel-tdx 1'
call 'compute ssh attestamp-tdx-vendor .*systemd-run'
said "/bin/att-user(.exe)? verify --tee-addr 192.0.2.10:5000 --format intel-tdx --rpc http://127.0.0.1:8545 --registry $REG"
said 'expected: not trusted'; finish
start "4b: exit 12 is the pass"
run NET=local FAKE_AUDIT_EXIT=12 bash "$F4b" intel-tdx; exitis 0; call 'audit .*--vendor-tee 192.0.2.10:5000'; said 'exit 12'; finish
start "4b: exit 0 (the Vendor's guest not found different) fails"
run NET=local FAKE_AUDIT_EXIT=0 bash "$F4b" intel-tdx; exitis 1; finish

# SEV-SNP on the default chain, Sepolia
newwork snp
start "1 amd-sev-snp-initrd on Sepolia by default"
run bash "$F1" amd-sev-snp-initrd; exitis 0
call "att-auditor registry-deploy --rpc $SEPOLIA "; call 'guest-step.sh install auditor amd-sev-snp-initrd 1'
call 'compute ssh attestamp-snp-auditor'; nocall '^curl '; finish
start "2 amd-sev-snp-initrd: the verify line names Sepolia and the step-1 registry"
run bash "$F2" amd-sev-snp-initrd; exitis 0
said "/bin/att-user(.exe)? verify --tee-addr 192.0.2.10:5000 --format amd-sev-snp-initrd --rpc $SEPOLIA --registry $REG"
said "^  ./att-user verify --tee-addr 192.0.2.10:5000 --format amd-sev-snp-initrd --rpc $SEPOLIA --registry $REG"; finish
newwork verity
start "1 amd-sev-snp-verity on its own work folder"
run bash "$F1" amd-sev-snp-verity; exitis 0; call 'guest-step.sh build auditor amd-sev-snp-verity 2'; finish

# Flow 2, intel-tdx on a local chain
newwork f2tdx
OTHER=0x1111111111111111111111111111111111111111; AREG=0x2222222222222222222222222222222222222222
start "flow 2, 3 without step 1's self-registration stops before any call"
run NET=local bash "$G3" intel-tdx; exitis 1; nocall '^gcloud '; said 'run 1-Vendor-build-launch-self-register.sh intel-tdx first'; finish
start "flow 2, 1 intel-tdx: the Vendor's guest built and booted, a registry of the Vendor's, self-register; verify line: trusted"
run NET=local bash "$G1" intel-tdx; exitis 0
call 'guest-step.sh app vendor intel-tdx 1 original'; call 'guest-step.sh build vendor intel-tdx 1'
call 'guest-step.sh install vendor intel-tdx 1'; call 'compute ssh attestamp-tdx-vendor .*systemd-run'
call 'guest-step.sh check-serve vendor intel-tdx 1'; nocall 'attestamp-tdx-auditor'; nocall '^att-auditor '
call "att-vendor registry-deploy --rpc http://127.0.0.1:8545 .*--approver $VENDOR\$"
call "att-vendor self-register --tee-addr 192.0.2.10:5000 --format intel-tdx --build-measurements .*flow2-vendor-measurements.json --rpc http://127.0.0.1:8545 --registry $REG --signing-key-file .*deployer.key "
said "/bin/att-user(.exe)? verify --tee-addr 192.0.2.10:5000 --format intel-tdx --rpc http://127.0.0.1:8545 --registry $REG\$"
said 'expected: trusted'; said "NET=local: only this machine reaches the chain"; finish
start "flow 2, 1: self-register refusing ends the step with its exit code, and no verify line"
run NET=local FAKE_SELFREG_EXIT=1 bash "$G1" intel-tdx; exitis 1; nosaid 'verify --tee-addr'; finish
start "flow 2, 1 -c: the given registry, none deployed"
run NET=local bash "$G1" intel-tdx -c $OTHER; exitis 0
nocall 'registry-deploy'; call "att-vendor self-register .*--registry $OTHER "; finish
start "flow 2, 3: one byte changed, rebuilt, rebooted, self-registered on a new registry; step 1's registry kept"
run NET=local bash "$G3" intel-tdx; exitis 0
call 'guest-step.sh app vendor intel-tdx 1 tampered'; call 'guest-step.sh build vendor intel-tdx 1'
call 'compute ssh attestamp-tdx-vendor .*systemd-run'; call 'guest-step.sh check-serve vendor intel-tdx 1'
call "att-vendor registry-deploy --rpc http://127.0.0.1:8545 .*--approver $VENDOR\$"
call "att-vendor self-register --tee-addr 192.0.2.10:5000 --format intel-tdx --build-measurements .*flow2-vendor-tampered-measurements.json --rpc http://127.0.0.1:8545 --registry $REG "
nosaid 'verify --tee-addr'
[ "$(cat "$work/cvm/intel-tdx-opt1/flow2-vendor-registry.txt" 2>/dev/null)" = "$OTHER" ] || bad="$bad; step 1's registry file changed"
[ "$(cat "$work/cvm/intel-tdx-opt1/flow2-vendor-registry-tampered.txt" 2>/dev/null)" = "$REG" ] || bad="$bad; no registry file from step 3"
finish
start "flow 2, 4 without flow 1's step 1 stops before any call and names the script to run"
run NET=local FAKE_AUDIT_EXIT=12 bash "$G4" intel-tdx; exitis 1; nocall '^gcloud '; nocall '^att-'
said 'bash e2e/cvm/flow1-auditor-registers/1-Auditor-rebuild-launch-audit.sh intel-tdx$'; finish
run NET=local bash "$F1" intel-tdx   # flow 1's step 1, for the Auditor's own value
start "flow 2, 4 intel-tdx: audit --vendor-tee with the Auditor's own value, exit 12 passes; verify line: both registries"
run NET=local FAKE_AUDIT_EXIT=12 bash "$G4" intel-tdx -c $AREG; exitis 0; nocall 'registry-deploy'
call "att-auditor audit --tee-addr 192.0.2.20:5000 --format intel-tdx --build-measurements .*auditor-measurements.json --vendor-tee 192.0.2.10:5000 --rpc http://127.0.0.1:8545 --registry $AREG --signing-key-file .*auditor.key "
said "/bin/att-user(.exe)? verify --tee-addr 192.0.2.10:5000 --format intel-tdx --rpc http://127.0.0.1:8545 --registry $REG --registry $AREG\$"
said 'expected: not trusted, exit 1'; finish
start "flow 2, 4: by default a registry of its own, the Auditor its approver"
run NET=local FAKE_AUDIT_EXIT=12 bash "$G4" intel-tdx; exitis 0
call "att-auditor registry-deploy --rpc http://127.0.0.1:8545 .*--approver $AUDITOR\$"; call "att-auditor audit .*--registry $REG "; finish
start "flow 2, 4: exit 0 (the Vendor's guest not found different) fails, with no verify line"
run NET=local FAKE_AUDIT_EXIT=0 bash "$G4" intel-tdx; exitis 1; said 'run step 3 first'; nosaid 'verify --tee-addr'; finish
start "flow 2, 4: any other exit fails"
run NET=local FAKE_AUDIT_EXIT=5 bash "$G4" intel-tdx; exitis 1; nosaid 'verify --tee-addr'; finish

# Flow 2, SEV-SNP on the default chain, Sepolia
newwork f2snp
start "flow 2, 1 amd-sev-snp-verity on Sepolia: the Vendor's registry, and the line for another computer"
run bash "$G1" amd-sev-snp-verity; exitis 0
call "att-vendor registry-deploy --rpc $SEPOLIA .*--approver $VENDOR\$"; call 'guest-step.sh build vendor amd-sev-snp-verity 2'
said "^  ./att-user verify --tee-addr 192.0.2.10:5000 --format amd-sev-snp-verity --rpc $SEPOLIA --registry $REG\$"; finish
run bash "$G3" amd-sev-snp-verity; run bash "$F1" amd-sev-snp-verity
start "flow 2, 4 on Sepolia: both registries on the line for another computer"
run FAKE_AUDIT_EXIT=12 bash "$G4" amd-sev-snp-verity -c $AREG; exitis 0
said "^  ./att-user verify --tee-addr 192.0.2.10:5000 --format amd-sev-snp-verity --rpc $SEPOLIA --registry $REG --registry $AREG\$"; finish

# No guest command goes through ssh's standard input: no heredoc and no `bash -s` in the scripts that reach a guest.
start "no guest command is piped into ssh"
bad=""; out=""; : > "$log"
# A heredoc is << followed by its delimiter word (the arithmetic shift in env-cvm.sh, "10 << (n - 1)", is not one).
if grep -nE "<<-?[[:space:]]*['\"]?[A-Za-z_]|bash -s" "$here/env-cvm.sh" "$S" "$flow"/*.sh "$flow2"/*.sh > "$tmp/pipes.txt"; then bad="; $(cat "$tmp/pipes.txt")"; fi
finish

# The guest-side script, run here: which launch and build flags each format gets.
g="$tmp/guest"; mkdir -p "$g/bin" "$g/run/out"
printf '#!/usr/bin/env bash\nprintf "sudo %%s\\n" "$*" >> "$STANDIN_LOG"\n"$@"\n' > "$g/bin/sudo"
for b in att-vendor att-auditor; do printf '#!/usr/bin/env bash\nprintf "%s %%s\\n" "$*" >> "$STANDIN_LOG"\n' "$b" > "$g/run/$b"; chmod +x "$g/run/$b"; done
chmod +x "$g/bin/sudo"; echo '{}' > "$g/run/out/build-value.json"
gstep() { : > "$log"; out=$(cd "$g/run" && PATH="$g/bin:$PATH" bash "$here/guest/guest-step.sh" "$@" 2>&1); got=$?; }
start "guest: install on intel-tdx uses --initrd"
gstep install vendor intel-tdx 1; exitis 0; call '^att-vendor launch --platform intel-tdx --initrd out/initrd-app.img$'; finish
start "guest: install on amd-sev-snp-verity uses --build-value"
gstep install auditor amd-sev-snp-verity 2; exitis 0; call '^att-auditor launch --platform amd-sev-snp-verity --build-value out/build-value.json$'; finish
start "guest: build intel-tdx option 2 runs as root, with --option 2 and base-initrd-v.img"
gstep build vendor intel-tdx 2; exitis 0; call '^sudo \./att-vendor build'; call '^att-vendor build --platform intel-tdx --app-dir app --base-initrd base-initrd-v.img --option 2 --verify$'; finish
start "guest: after a build as root, out/ goes back to the user (R9)"
call "^sudo chown -R $(id -un) out$"; finish
start "guest: out/ goes back to the user also when the build as root fails"
printf '#!/usr/bin/env bash\nprintf "att-auditor %%s\\n" "$*" >> "$STANDIN_LOG"\nexit 3\n' > "$g/run/att-auditor"
gstep build auditor amd-sev-snp-verity 2; exitis 3; call '^sudo chown -R '; finish
printf '#!/usr/bin/env bash\nprintf "att-auditor %%s\\n" "$*" >> "$STANDIN_LOG"\n' > "$g/run/att-auditor"
start "guest: build amd-sev-snp-initrd, not as root, no --option"
gstep build vendor amd-sev-snp-initrd 1; exitis 0; nocall '^sudo'; call '^att-vendor build --platform amd-sev-snp-initrd --app-dir app --base-initrd base-initrd.img --verify$'; finish
start "guest: the tampered application differs from the original in exactly one byte"
gstep app vendor intel-tdx 1 original; cp "$g/run/app/hello.txt" "$tmp/original.txt"
gstep app vendor intel-tdx 1 tampered; exitis 0
[ "$(cmp -l "$tmp/original.txt" "$g/run/app/hello.txt" | wc -l)" -eq 1 ] || bad="$bad; not exactly one byte differs"; finish
start "guest: an unknown format is refused"
gstep build vendor intel-sgx 1; exitis 2; finish

echo
if [ "$fails" -gt 0 ]; then echo "check-flow: $fails of $n cases failed"; exit 1; fi
echo "check-flow: all $n cases passed"
