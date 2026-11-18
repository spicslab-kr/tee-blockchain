#!/usr/bin/env bash
# Runs on an Intel TDX or AMD SEV-SNP guest, one action per call. The e2e/cvm scripts copy it to the guest with
# `gcloud compute scp` and run it with `gcloud compute ssh --command`; nothing is piped into ssh. Each action is a step
# of docs/build/tdx.md and docs/build/sev-snp.md ("Getting started" and "The Auditor's half"), run as written there.
#
#   bash attestamp-guest-step.sh <action> <role> <format> <option> [arg]
#     role     vendor | auditor (which att binary: ./att-vendor or ./att-auditor)
#     format   intel-tdx | amd-sev-snp-initrd | amd-sev-snp-verity
#     option   1 | 2 (TDX's two options share one format; for SEV-SNP the format decides it)
#   actions:
#     prepare           step 4 (A3): boot with an initrd, the base initrd, the Option 2 packages, a safety stop
#     app <original|tampered>   step 5: the application, or the same with one byte changed
#     build             step 6 (A4): build --verify
#     install           step 7 (A5) without its reboot: launch --initrd (TDX) or --build-value (SEV-SNP)
#     check-serve       steps 8 and 9 (A6, A7): launch --check, then the responder on port 5000
set -euo pipefail
action=${1:?action} role=${2:?role} fmt=${3:?format} opt=${4:?option}
att=./att-$role
case "$role" in vendor|auditor) ;; *) echo "role must be vendor or auditor, not '$role'" >&2; exit 2 ;; esac
case "$fmt" in
  intel-tdx)          extractor=tdx ;;
  amd-sev-snp-initrd) extractor=snp-initrd ;;
  amd-sev-snp-verity) extractor=snp-verity ;;
  *) echo "unknown format '$fmt'" >&2; exit 2 ;;
esac
verity=0
if [ "$fmt" = amd-sev-snp-verity ] || { [ "$fmt" = intel-tdx ] && [ "$opt" = 2 ]; }; then verity=1; fi
base=base-initrd.img; [ "$verity" = 1 ] && base=base-initrd-v.img
# Option 2 runs build as root (it makes the image with mksquashfs and veritysetup), as in the guides.
as_root=(); [ "$verity" = 1 ] && as_root=(sudo)

safety_stop() { sudo shutdown -h +240 >/dev/null 2>&1 || true; }   # every reboot cancels it, so set it again

case "$action" in
prepare)
  chmod +x "$att" att-ra-responder
  safety_stop
  # Make the stock boot load an initrd (Google's Ubuntu images boot without one: GRUB_FORCE_PARTUUID).
  f=/etc/default/grub.d/40-force-partuuid.cfg
  if grep -q '^GRUB_FORCE_PARTUUID=' "$f" 2>/dev/null; then
    [ -f /root/40-force-partuuid.cfg.orig ] || sudo cp -p "$f" /root/40-force-partuuid.cfg.orig
    sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' "$f"
    sudo update-grub
  fi
  if [ "$verity" = 0 ]; then
    # Option 1: the distribution's own initrd, the same file on every guest of the image.
    if [ ! -f base-initrd.img ]; then
      sudo cp "/boot/initrd.img-$(uname -r)" base-initrd.img
      sudo chown "$USER" base-initrd.img
    fi
  else
    pkgs=(squashfs-tools cryptsetup-bin); [ "$fmt" = amd-sev-snp-verity ] && pkgs+=(tpm2-tools)
    sudo apt-get update -qq
    sudo apt-get install -y -qq "${pkgs[@]}"
    sudo mkdir -p /opt/app-verity
    if [ ! -f base-initrd-v.img ]; then
      # mkinitramfs writes a random seed, so the Vendor makes this file once and the Auditor uses the Vendor's copy.
      [ "$role" = vendor ] || { echo "base-initrd-v.img is missing: the Auditor uses the Vendor's (0-Setup-guests.sh copies it)" >&2; exit 1; }
      sudo install -m 755 initramfs-hook.sh         /etc/initramfs-tools/hooks/attestamp-verity
      sudo install -m 755 initramfs-local-bottom.sh /etc/initramfs-tools/scripts/local-bottom/attestamp-verity
      sudo mkinitramfs -o base-initrd-v.img "$(uname -r)"
      sudo rm /etc/initramfs-tools/hooks/attestamp-verity /etc/initramfs-tools/scripts/local-bottom/attestamp-verity
      sudo chown "$USER" base-initrd-v.img
    fi
    dpkg-query -W "${pkgs[@]}"
  fi
  echo "kernel $(uname -r)"
  sha256sum "$base"
  ;;
app)
  mkdir -p app
  case "${5:-original}" in
    original) printf 'hello from attestamp\n' > app/hello.txt ;;
    tampered) printf 'Hello from attestamp\n' > app/hello.txt ;;   # one byte differs: h -> H
    *) echo "app takes original or tampered" >&2; exit 2 ;;
  esac
  sha256sum app/hello.txt
  ;;
build)
  extra=(); [ "$fmt" = intel-tdx ] && extra=(--option "$opt")
  "${as_root[@]}" "$att" build --platform "$fmt" --app-dir app --base-initrd "$base" "${extra[@]}" --verify
  cat out/build-value.json
  ;;
install)
  if [ "$fmt" = intel-tdx ]; then
    sudo "$att" launch --platform "$fmt" --initrd out/initrd-app.img
  else
    sudo "$att" launch --platform "$fmt" --build-value out/build-value.json
  fi
  ;;
check-serve)
  safety_stop
  sudo "$att" launch --platform "$fmt" --check
  sudo pkill -f "[a]tt-ra-responder ra-serve" || true
  sudo nohup ./att-ra-responder ra-serve --extractor "$extractor" --listen "tcp:0.0.0.0:5000" > ra.log 2>&1 < /dev/null &
  sleep 2
  pgrep -f "[a]tt-ra-responder ra-serve" > /dev/null || { echo "the responder did not start:" >&2; cat ra.log >&2; exit 1; }
  echo "responder: att-ra-responder ra-serve --extractor $extractor on port 5000"
  cat out/measurements.json
  ;;
*) echo "unknown action '$action'" >&2; exit 2 ;;
esac
