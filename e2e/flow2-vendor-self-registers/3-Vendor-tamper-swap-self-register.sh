#!/usr/bin/env bash
# 조작 · Vendor — 자기가 빌드해 등록한 이미지 대신 다른 이미지를 몰래 띄우고, 바뀐 값도 자기 등록부에 올린다.
#    여기, 내 컴퓨터에서 실행 — 교체만 Vendor 인스턴스 안에서 자동으로 접속해 실행된다. (1-Vendor-build-self-register 까지 끝난 뒤)
#    다음: 4-Auditor-tamper-audit-check-vendor → 5-User-tamper-verify-both-registries
#    실행: bash "e2e/flow2-vendor-self-registers/3-Vendor-tamper-swap-self-register.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"

nitroBanner "Vendor"
V 'bash -s' <<'REMOTE'
set -euo pipefail
step() { printf '\033[1;32m==================== %s' "$1"; for _ in 1 2 3; do sleep 0.15; printf '.'; done; printf ' ====================\033[0m\n'; }
umask 022
mkdir -p tampered
# 준비 — 조작 데모용으로 작은 이미지 하나를 직접 만든다. 아무도 검증한 적 없는 값이다.
cat > tampered/Dockerfile <<'DOCKER'
FROM alpine:3.20
RUN echo "not the audited image" > /etc/tb-tampered
CMD ["sleep", "1000000"]
DOCKER
step "BUILDING TAMPERED IMAGE"
sudo docker build -q -t tampered:latest tampered

# ▶ 우리가 만든 tb 명령 — 이 도커 이미지를 엔클레이브 이미지(EIF)로 포장한다.
step "PACKING IMAGE"
./tb-vendor pack --image tampered:latest --name tampered --out out-tampered

sudo nitro-cli terminate-enclave --all >/dev/null 2>&1 || true
pkill -x tb-vendor || true

# ▶ 우리가 만든 tb 명령 — 그 조작된 이미지를 내 TEE 에 띄운다 (내가 빌드해 등록한 것 대신).
step "LAUNCHING TAMPERED ENCLAVE"
./tb-vendor launch --eif out-tampered/tampered.eif

echo
echo "the enclave on :5000 now runs my own image, not what I built from the audited source."
echo "built value:     $(grep PCR0 out/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "running value:   $(grep PCR0 out-tampered/measurements.json | tr -d ' ,\"' | cut -d: -f2)"
echo "to put the built one back:  ./tb-vendor launch --eif out/alpine-slim.eif --measurements out/measurements.json"
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# 준비 — Vendor 인스턴스에서 바뀐 값(out-tampered/)을 내 컴퓨터로 가져온다.
$SCP "$SSH_USER@$VENDOR_HOST:out-tampered/measurements.json" "$WORK/vendor-build.json"
echo "Vendor's build value:"; cat "$WORK/vendor-build.json"

# ▶ 우리가 만든 tb 명령 — 새 등록부를 하나 더 배포한다. "A 여야 하는데 B 를 등록"하는 새 사건이라 기존 등록부에
#   더하지 않고 새로 만든다 (기존 등록부엔 이미 정직한 값 A 가 있다).
step "DEPLOYING REGISTRY"
out=$("$TBV" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$VENDOR_ADDR" 2>&1); echo "$out"
CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # 준비 — 출력에서 주소 뽑기
[ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
echo "$CONTRACT" > "$WORK/contract-flow2.txt"  # 준비 — 다음 단계가 읽을 파일에 남기기

# ▶ 우리가 만든 tb 명령 — 바뀐 값도 내 키로 서명해서 그대로 등록한다 (제3자 대조 없이 통과되는지 보여주는 것).
step "SELF-REGISTERING"
"$TBV" self-register --tee "$VENDOR_HOST:5000" --measurements "$WORK/vendor-build.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$VENDOR_KEY" --service "$SERVICE" --version "$VERSION"
echo "self-register exit=$?"
# ▶ 우리가 만든 tb 명령 — User 입장에서 검증해도 이 등록부만 보면 통과된다는 걸 보여준다.
step "VERIFYING"
"$TBU" verify --tee "$VENDOR_HOST:5000" --rpc "$RPC" --contract "$CONTRACT"
echo "verify exit=$?"
echo "next: 4-Auditor-tamper-audit-check-vendor"
