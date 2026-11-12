#!/bin/bash
# AWS 공급자 VM에서: 응답기를 넣어 닫힌 initrd를 다시 만들고, 기대값을 계산하고, 부팅 항목 두 개(본 시험용, 디버그용)를 맞춘다.
# 사용법: aws_closed_build.sh closed|debug   (고른 항목으로 이번 한 번만 부팅. 기본 항목은 Amazon Linux 그대로)
set -e
MODE=$1
KV=$(uname -r)
cd ~/closed
cp "${APROXY_BIN:-$HOME/closed-aproxy}" appfs/attest-init && chmod 755 appfs/attest-init
python3 ~/mkcpio.py appfs app.cpio
cat app.cpio base.img > initrd-closed.img && rm app.cpio
sudo cp initrd-closed.img /boot/initrd-closed.img
ARGS="console=ttyS0,115200n8 rdinit=/attest-init panic=-1 attestamp_closed=1"
sudo grubby --info=ALL | grep -q 'title="attdebug"' || sudo grubby --add-kernel=/boot/vmlinuz-$KV --initrd=/boot/initrd-closed.img --title=attdebug --args="$ARGS attestamp_debuglog=1"
python3 - "$KV" "$ARGS" > expected-aws.json <<'EOF'
import hashlib, json, sys
kv, args = sys.argv[1:3]
H = lambda b: hashlib.sha384(b).hexdigest()
print(json.dumps({"kernel": H(open("/boot/vmlinuz-%s" % kv, "rb").read()),
                  "initrd": H(open("initrd-closed.img", "rb").read()),
                  "cmdline": H(("(hd0,gpt1)/boot/vmlinuz-%s %s" % (kv, args)).encode())}, indent=1))
EOF
cat expected-aws.json
id_of() { sudo grubby --info=ALL | awk -v t="title=\"$1" 'index($0,t)==1{f=1;next} f&&/^id=/{gsub(/id=|"/,""); print; exit}'; }
OLD=$(id_of "Amazon Linux")
case "$MODE" in closed) ONCE=$(id_of attclosed);; debug) ONCE=$(id_of attdebug);; *) echo "built only"; exit 0;; esac
sudo rm -f /var/tmp/closed-debug.log /var/tmp/closed-selftest.json
sudo grub2-set-default "$OLD"
sudo grub2-reboot "$ONCE"
sudo grub2-editenv list | grep -E 'saved_entry|next_entry'
(sleep 2; sudo reboot) >/dev/null 2>&1 &
