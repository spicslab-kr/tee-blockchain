#!/bin/bash
# GCP 공급자 VM(Ubuntu 24.04, SEV-SNP, Shielded vTPM)에서 한 번: 닫힌 이미지의 재료를 ~/closed 에 모으고 grub 항목을 만든다.
# 시험 때 손으로 실행한 명령을 순서대로 모은 것이다. 이 스크립트를 처음부터 끝까지 한 번에 다시 돌려 보지는 않았다.
# 시험 VM에서는 Ollama를 설치본(/usr/bin/ollama, /usr/lib/ollama)에서 복사했다. 여기서는 AWS와 같은 방식으로 받는다.
# 먼저 홈 폴더에 mkcpio.py 를, ~/closed 에 attest-init 를 올려 둘 것.
set -e
KV=${KV:-$(uname -r)}
mkdir -p ~/vt ~/closed/appfs/attest ~/closed/appfs/root ~/closed/appfs/models ~/closed/appfs/usr/lib/x86_64-linux-gnu
cd ~/closed

# 1. 배포판 기본 initrd. 매번 다르게 만들어지므로(난수 포함) 공급자가 쓴 파일을 그대로 공개해야 한다.
sudo cp "/boot/initrd.img-$KV" ~/vt/i.bin && sudo chown "$USER" ~/vt/i.bin

# 2. 첫 프로세스(rdinit): 네트워크를 잡고 응답기를 실행하는 셸 스크립트
cp ~/closed/attest-init appfs/attest-init && chmod 755 appfs/attest-init

# 3. Ollama (CPU용 파일만)와, Ubuntu 기본 initrd에 없는 libdl
(cd appfs/usr && curl -sL https://ollama.com/download/ollama-linux-amd64.tar.zst | zstd -dc |
  tar x --exclude='lib/ollama/cuda*' --exclude='lib/ollama/rocm*' --exclude='lib/ollama/vulkan*' --exclude='lib/ollama/mlx*')
cp -L /usr/lib/x86_64-linux-gnu/libdl.so.2 appfs/usr/lib/x86_64-linux-gnu/

# 4. 모델을 이미지 안에 넣는다
OLLAMA_MODELS=$PWD/appfs/models HOME=/tmp appfs/usr/bin/ollama serve >/tmp/ollama-pull.log 2>&1 &
pid=$!
sleep 4
appfs/usr/bin/ollama pull qwen2.5:0.5b
kill $pid; sleep 1
rm -rf appfs/models/metadata

# 5. 부팅 설정과 grub 항목. grub이 기록하는 부팅 설정은 "커널 경로 + 인자" 이고, 기대값은 이 파일로 계산한다.
if [ "$(findmnt -no TARGET --target /boot)" = /boot ]; then KP=/vmlinuz-$KV; IP=/initrd-closed.img; else KP=/boot/vmlinuz-$KV; IP=/boot/initrd-closed.img; fi
ARGS="console=ttyS0,115200 rdinit=/attest-init panic=-1 attestamp_closed=1"
printf '%s %s' "$KP" "$ARGS" > cmdline-closed.txt
if ! grep -q 'menuentry "attclosed"' /etc/grub.d/40_custom; then
  UUID=$(findmnt -no UUID --target /boot)
  sudo tee -a /etc/grub.d/40_custom >/dev/null <<EOF
menuentry "attclosed" {
	insmod gzio
	insmod part_gpt
	insmod ext2
	search --no-floppy --fs-uuid --set=root $UUID
	linux $KP $ARGS
	initrd $IP
}
EOF
  sudo update-grub
fi
grep -q '^GRUB_DEFAULT=saved' /etc/default/grub || echo "주의: grub-reboot 은 /etc/default/grub 의 GRUB_DEFAULT=saved 가 있어야 동작한다"
cat cmdline-closed.txt; echo
echo "다음: 응답기를 ~/closed/snp-aproxy 로 올리고 gcp_closed_build.sh build|boot"
