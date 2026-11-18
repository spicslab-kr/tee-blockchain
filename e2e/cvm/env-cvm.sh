#!/usr/bin/env bash
# e2e/cvm/env-cvm.sh - settings and helpers for the flow scripts on Intel TDX and AMD SEV-SNP guests on GCP. Sourced
# after e2e/env.sh, which gives the chain (NET, RPC, key files), the results contract and the step banners:
#
#   . "$here/../env.sh"; . "$here/../env-cvm.sh" "$@"
#
# Arguments, the same for every CVM script:  <format> [option]
#   intel-tdx [1|2]       Option 1 (application archive in the initrd, default) or Option 2 (dm-verity image in it)
#   amd-sev-snp-initrd    Option 1
#   amd-sev-snp-verity    Option 2
#
# The two guests are the ones e2e/cloud/gcp-cvm.sh makes: <PREFIX>-vendor and <PREFIX>-auditor, PREFIX default
# attestamp-tdx or attestamp-snp, in ZONE (default us-central1-a). Their addresses are read from gcloud, not written
# anywhere. Nothing here creates or deletes a VM.
#
# How commands reach a guest: a script file is copied with `gcloud compute scp` and then run with `gcloud compute ssh
# --command`. Nothing is piped into ssh's standard input: from Git Bash on Windows gcloud uses PuTTY (plink, pscp),
# whose handling of piped input was never checked, and pscp has already failed twice on this project.
here_cvm="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FORMAT=${1:-}
OPTION=${2:-1}
case "$FORMAT" in
  intel-tdx)          PLAT=tdx; EXTRACTOR=tdx
                      case "$OPTION" in 1|2) ;; *) echo "intel-tdx takes option 1 or 2, not '$OPTION'" >&2; exit 2;; esac ;;
  amd-sev-snp-initrd) PLAT=snp; EXTRACTOR=snp-initrd; OPTION=1 ;;
  amd-sev-snp-verity) PLAT=snp; EXTRACTOR=snp-verity; OPTION=2 ;;
  *) echo "usage: $(basename "$0") intel-tdx [1|2] | amd-sev-snp-initrd | amd-sev-snp-verity" >&2; exit 2 ;;
esac
# CVM_TAG names this format's work folder; TDX's two options share a format string, so the option is part of it.
CVM_TAG=$FORMAT; [ "$FORMAT" = intel-tdx ] && CVM_TAG=intel-tdx-opt$OPTION
CVM_WORK="$WORK/cvm/$CVM_TAG"
mkdir -p "$CVM_WORK"
RESULTS_PLATFORM=$FORMAT

PREFIX=${PREFIX:-attestamp-$PLAT}
ZONE=${ZONE:-us-central1-a}
VENDOR_VM=$PREFIX-vendor
AUDITOR_VM=$PREFIX-auditor
RA_PORT=5000
# SSH through Identity-Aware Proxy, as the firewall rule attestamp-ssh-iap allows (docs/build/tdx.md, step 3). IAP=0
# connects to the guest's public address instead.
IAP=${IAP:-1}
IAP_FLAG=(); [ "$IAP" = 1 ] && IAP_FLAG=(--tunnel-through-iap)
# A new boot answered 55 to 166 s after the reboot command on 2026-11-16 (docs/build/tdx.md, step 7).
REBOOT_TIMEOUT=${REBOOT_TIMEOUT:-600}
REBOOT_POLL=${REBOOT_POLL:-15}
CVM_SERVICE=${CVM_SERVICE:-hello}
CVM_VERSION=${CVM_VERSION:-1.0.0}
GUEST_STEP=attestamp-guest-step.sh   # the guest-side script's name on the guest

command -v gcloud >/dev/null || { echo "gcloud is not on PATH" >&2; exit 1; }

# gcloud_retry <gcloud args...> - runs gcloud, and runs it again only when the connection itself could not be set up
# (the patterns of net_retry in env.sh, and PuTTY's own wording). A command that ran and failed is never run twice.
gcloud_retry() {
  local n=1 rc errf
  errf=$(mktemp)
  while :; do
    gcloud "$@" 2> "$errf"; rc=$?
    cat "$errf" >&2
    if [ $rc -eq 0 ] || [ $n -ge "$SSH_TRIES" ] \
       || ! grep -qE 'kex_exchange_identification|Software caused connection abort|Connection (refused|reset by peer|timed out)|Network is unreachable|No route to host|Connection closed by [^ ]+ port [0-9]+$|^scp: Connection closed$|FATAL ERROR: Network error|Remote side unexpectedly closed network connection|failed to connect to backend' "$errf"; then
      break
    fi
    echo "gcloud: the connection could not be set up (attempt $n of $SSH_TRIES); trying again in $((10 << (n - 1))) s" >&2
    sleep $((10 << (n - 1))); n=$((n + 1))
  done
  rm -f "$errf"
  return $rc
}

# guest_run <vm> <command> - one command line on the guest, its output here.
guest_run() { gcloud_retry compute ssh "$1" --zone "$ZONE" "${IAP_FLAG[@]}" --command "$2"; }
# guest_put <vm> <local file> <remote name>, guest_get <vm> <remote file> <local file> - one file per call.
guest_put() { gcloud_retry compute scp "$2" "$1:$3" --zone "$ZONE" "${IAP_FLAG[@]}"; }
guest_get() { gcloud_retry compute scp "$1:$2" "$3" --zone "$ZONE" "${IAP_FLAG[@]}"; }
# guest_ip <vm> - the guest's public address, which the RA port (attestamp-ra) is reached on.
guest_ip() {
  gcloud compute instances describe "$1" --zone "$ZONE" --format='value(networkInterfaces[0].accessConfigs[0].natIP)'
}
# guest_step <vm> <role> <action> [args] - copies e2e/cvm/guest/guest-step.sh to the guest and runs one action of it.
guest_step() {
  local vm=$1 role=$2 action=$3; shift 3
  guest_put "$vm" "$here_cvm/guest/guest-step.sh" "$GUEST_STEP" || return 1
  guest_run "$vm" "bash $GUEST_STEP $action $role $FORMAT $OPTION $*"
}

# boot_id <vm> - the guest's current boot ID; one try, no retry, empty and non-zero when the guest does not answer.
boot_id() { gcloud compute ssh "$1" --zone "$ZONE" "${IAP_FLAG[@]}" --command 'cat /proc/sys/kernel/random/boot_id' 2>"$CVM_WORK/boot-id.err" | tr -d '\r\n '; }

# wait_new_boot <vm> <boot id before the reboot> - waits until the guest answers with another boot ID. The serial
# console is not used: it returns only the newest part of its log, and the old boot's "login:" line looks like the new
# one's (docs/results/trial-2026-11-16-clean.md, F6). Gives up after REBOOT_TIMEOUT seconds, saying what it last saw.
wait_new_boot() {
  local vm=$1 old=$2 start now id seen="" err=""
  start=$(date +%s)
  while :; do
    sleep "$REBOOT_POLL"
    id=$(boot_id "$vm")
    now=$(date +%s)
    if [ -n "$id" ] && [ "$id" != "$old" ]; then
      echo "new boot on $vm after $((now - start)) s (boot_id $old -> $id)"
      return 0
    fi
    if [ -n "$id" ]; then seen="the old boot ($id) still answered"; else err=$(tail -1 "$CVM_WORK/boot-id.err" 2>/dev/null); seen="no answer${err:+ ($err)}"; fi
    if [ $((now - start)) -ge "$REBOOT_TIMEOUT" ]; then
      echo "no new boot on $vm within $REBOOT_TIMEOUT s (REBOOT_TIMEOUT); the last try: $seen. Look at the guest with" >&2
      echo "  gcloud compute ssh $vm --zone $ZONE ${IAP_FLAG[*]} --command 'cat /proc/sys/kernel/random/boot_id; uptime'" >&2
      return 1
    fi
  done
}

# install_and_reboot <vm> <role> - installs the built initrd as the default boot entry (launch without --reboot, so the
# command returns before the connection drops), reboots in 3 s, and waits for the new boot.
install_and_reboot() {
  local vm=$1 role=$2 old
  old=$(boot_id "$vm")
  [ -n "$old" ] || { echo "cannot read the boot ID of $vm before the reboot" >&2; return 1; }
  echo "boot_id before the reboot: $old"
  guest_step "$vm" "$role" install || return 1
  guest_run "$vm" 'sudo systemd-run --on-active=3 systemctl reboot' || return 1
  wait_new_boot "$vm" "$old"
}

# need_bins - the binaries 0-Setup-guests.sh builds for this machine.
need_bins() {
  local b
  for b in "$AUDITOR_BIN" "$VENDOR_BIN" "$USER_BIN"; do
    [ -x "$b" ] || { echo "$b is missing: run bash e2e/cvm/0-Setup-guests.sh $FORMAT $OPTION first" >&2; exit 1; }
  done
}

# print_verify_line <vendor address> <registry> <expected> - the command a User types on another computer, complete.
print_verify_line() {
  local line="att-user verify --tee-addr $1:$RA_PORT --format $FORMAT --rpc $RPC --registry $2"
  echo "$line" > "$CVM_WORK/verify-command.txt"
  echo
  echo "On the other computer, type (expected: $3):"
  echo "  $line"
}

# deploy_registry - deploys a Registry with the Auditor's address as its approver and prints the address.
deploy_registry() {
  local out reg
  out=$("$AUDITOR_BIN" registry-deploy --rpc "$RPC" --deployer-key-file "$DEPLOYER_KEY" --approver "$AUDITOR_ADDR" 2>&1)
  echo "$out" >&2
  reg=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')
  [ -n "$reg" ] || { echo "no registry address in the registry-deploy output" >&2; return 1; }
  echo "$reg"
}
