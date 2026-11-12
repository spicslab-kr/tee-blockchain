#!/bin/bash
# GCP 공급자 VM에서: 응답기를 넣어 닫힌 initrd를 다시 만들고 기대값을 계산한다.
# 사용법: gcp_closed_build.sh info|build|boot   (boot = build 후 닫힌 항목으로 이번 한 번만 부팅)
set -e
cd ~/closed
KV=${KV:-$(uname -r)}
if [ "$1" = info ]; then
  ls -la . appfs appfs/attest; ls -la ~/mkcpio.py ~/vt/i.bin
  echo "old expected:"; cat expected-closed.json
  echo "kernel   $(sha256sum /boot/vmlinuz-$KV | cut -c1-64)"
  echo "cmdline  $(printf %s "$(cat cmdline-closed.txt)" | sha256sum | cut -c1-64)"
  tail -c 3 cmdline-closed.txt | od -c | head -1
  sed -n 1,12p /etc/grub.d/40_custom
  exit 0
fi
cp "${APROXY_BIN:-$HOME/closed/snp-aproxy}" appfs/attest/snp-aproxy && chmod 755 appfs/attest/snp-aproxy
sudo rm -rf appfs/models/metadata
python3 ~/mkcpio.py appfs app-closed.cpio
cat app-closed.cpio ~/vt/i.bin > initrd-closed.img
sudo cp initrd-closed.img /boot/initrd-closed.img
KH=$(sudo sha256sum /boot/vmlinuz-$KV | cut -c1-64)
python3 - "$KH" > expected-closed.json <<'EOF'
import hashlib, json, sys
H = lambda b: hashlib.sha256(b).hexdigest()
print(json.dumps({"kernel": sys.argv[1],
                  "initrd": H(open("initrd-closed.img", "rb").read()),
                  "cmdline": H(open("cmdline-closed.txt", "rb").read().rstrip(b"\n"))}, indent=1))
EOF
cat expected-closed.json
if [ "$1" = boot ]; then
  # 닫힌 VM은 TPM에 종료 신호를 못 보내고 끝나므로(강제 재시작) vTPM이 매번 실패 1회로 센다. GCP vTPM은 3회에 잠긴다.
  # 공급자는 일반 모드에서 잠금 횟수를 비우고 닫힌 부팅으로 들어간다 (잠금 비밀번호가 비어 있는 기본 상태).
  sudo tpm2_dictionarylockout -c
  sudo tpm2_getcap properties-variable | grep -E 'inLockout|LOCKOUT_COUNTER'
  sudo grub-reboot attclosed
  # 기본 부팅 항목이 grub이 아닌 VM(우리 시험 VM은 UKI가 기본)에서는 다음 한 번만 grub(shim) 항목으로 가게 한다.
  [ -n "$NEXT_BOOT" ] && sudo efibootmgr -n "$NEXT_BOOT" >/dev/null
  (sleep 2; sudo systemctl reboot) >/dev/null 2>&1 &
  echo rebooting
fi
