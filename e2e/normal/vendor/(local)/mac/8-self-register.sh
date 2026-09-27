#!/usr/bin/env bash
# Vendor — Auditor 없이 직접 등록 (흐름 2). Vendor 인스턴스에서 1-pack-image.sh 또는 1-build-source.sh 를 돌린 뒤.
#    Vendor 의 등록부(approver = Vendor 주소)를 깔고, self-register: 자기 TEE 에 RA → 자기 빌드값과 대조 → 등록 → 자기확인 → User verify.
#    실행: bash "e2e/normal/vendor/(local)/mac/8-self-register.sh" [-c 0x컨트랙트] [-o out]     컨트랙트는 e2e-work/contract-flow2.txt
#      -o  Vendor 인스턴스에서 measurements.json 을 읽을 폴더 (조작 데모: 2-swap-enclave 뒤엔 out-tampered)
set -uo pipefail
. "$(dirname "$0")/../../../../setup/(local)/mac/env.sh"
CONTRACT=""; REMOTE_OUT=out
while getopts "c:o:" o; do case $o in c) CONTRACT=$OPTARG;; o) REMOTE_OUT=$OPTARG;; esac; done

$SCP "$SSH_USER@$VENDOR_HOST:$REMOTE_OUT/measurements.json" "$WORK/vendor-build.json"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"

if [ -z "$CONTRACT" ]; then
  echo "== registry-deploy (approver = Vendor itself $VENDOR_ADDR)"
  out=$("$TBV" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$VENDOR_ADDR" 2>&1); echo "$out"
  CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')
  [ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
fi
echo "$CONTRACT" > "$WORK/contract-flow2.txt"

echo "== self-register (contract $CONTRACT)"
"$TBV" self-register --tee "$VENDOR_HOST:5000" --measurements "$WORK/vendor-build.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$VENDOR_KEY" --service "$SERVICE" --version "$VERSION"
echo "self-register exit=$?"
echo "== verify (User) — the signer will be the Vendor itself"
"$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
echo "verify exit=$?"
