#!/bin/bash
# AWS 공급자 VM(Amazon Linux 2023, SEV-SNP, NitroTPM)에서 한 번: 닫힌 이미지의 재료를 ~/closed 에 모은다.
# 시험 때 손으로 실행한 명령을 순서대로 모은 것이다. 이 스크립트를 처음부터 끝까지 한 번에 다시 돌려 보지는 않았다.
# 먼저 홈 폴더에 mkcpio.py 를 올려 둘 것 (aws_closed_build.sh 가 쓴다).
set -e
KV=$(uname -r)
mkdir -p ~/closed/appfs/usr/lib64 ~/closed/appfs/root ~/closed/appfs/models
cd ~/closed

# 1. Ollama: CPU용 파일만 받는다 (GPU 라이브러리는 빼서 디스크와 initrd 크기를 줄인다). 받는 시점의 최신판이 온다. 시험은 0.40.16.
(cd appfs/usr && curl -sL https://ollama.com/download/ollama-linux-amd64.tar.zst | zstd -dc |
  tar x --exclude='lib/ollama/cuda*' --exclude='lib/ollama/rocm*' --exclude='lib/ollama/vulkan*' --exclude='lib/ollama/mlx*')
appfs/usr/bin/ollama --version 2>&1 | tail -1

# 2. Ollama가 쓰는 라이브러리. dracut 기본 initrd에는 libstdc++ 등이 없다.
for l in libresolv.so.2 libdl.so.2 libpthread.so.0 libstdc++.so.6 libc.so.6 libm.so.6 ld-linux-x86-64.so.2 libgcc_s.so.1; do
  cp -L /lib64/$l appfs/usr/lib64/
done

# 3. 모델을 이미지 안에 넣는다. 닫힌 VM은 부팅 뒤 모델을 받을 수 없다.
OLLAMA_MODELS=$PWD/appfs/models HOME=/tmp appfs/usr/bin/ollama serve >/tmp/ollama-pull.log 2>&1 &
pid=$!
sleep 4
appfs/usr/bin/ollama pull qwen2.5:0.5b
kill $pid; sleep 1
rm -rf appfs/models/metadata   # 실행 중 Ollama가 만든 캐시. 이미지에 넣지 않는다.

# 4. 기본 initrd: 네트워크(ena)와 SEV 보고서(sev-guest) 드라이버를 넣는다. 응답기가 PID 1로 직접 싣는다.
sudo dracut --force --add-drivers "ena sev-guest" base.img "$KV"
sudo chown "$USER" base.img
ls -la base.img; du -sh appfs
echo "다음: 응답기를 ~/closed-aproxy 로 올리고 aws_closed_build.sh build|debug|closed"
