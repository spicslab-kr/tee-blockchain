#!/usr/bin/env bash
# 조작 · Auditor — 내가 빌드한 값을 등록하면서, Vendor 의 엔클레이브에도 직접 물어봐서 같은지 확인한다.
#    Vendor 가 다른 이미지를 띄웠으면 "✗ the Vendor's enclave runs something else" 가 찍히고,
#    그래도 내가 검증한 값은 정상적으로 등록된다. Auditor 인스턴스에 자기 빌드값(out/measurements.json)이
#    이미 있어야 한다 (flow1 의 1-Auditor-build-audit-handoff 를 먼저 돌려 둔 상태).
#    실행: bash "e2e/flow2-vendor-self-registers/4-Auditor-tamper-audit-check-vendor.sh" [-c 0x컨트랙트]
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"
CONTRACT=""
while getopts "c:" o; do case $o in c) CONTRACT=$OPTARG;; esac; done  # 준비 — 옵션 파싱

# 준비 — Auditor 인스턴스에서 자기 빌드값을 가져온다.
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/auditor-measured.json"
echo "Auditor's own value:"; cat "$WORK/auditor-measured.json"

if [ -z "$CONTRACT" ]; then
  # ▶ 우리가 만든 tb 명령 — 등록부(레지스트리 컨트랙트)를 체인에 새로 배포한다.
  step "DEPLOYING REGISTRY"
  out=$("$TBA" registry-deploy --rpc "$RPC" --key-file "$VENDOR_KEY" --approver "$AUDITOR_ADDR" 2>&1); echo "$out"
  CONTRACT=$(echo "$out" | grep -oE 'deployed at 0x[0-9a-fA-F]{40}' | awk '{print $3}')  # 준비 — 출력에서 주소 뽑기
  [ -n "$CONTRACT" ] || { echo "no contract address in registry-deploy output"; exit 1; }
fi
echo "$CONTRACT" > "$WORK/contract.txt"  # 준비 — 다음 단계가 읽을 파일에 남기기

# ▶ 우리가 만든 tb 명령 — 내 값을 등록하면서, --vendor-tee 로 Vendor 엔클레이브에도 RA 를 보내 직접 비교한다.
#   이게 바꿔치기를 실측으로 잡아내는 핵심 명령이다.
step "AUDITING (checking Vendor's enclave)"
"$TBA" audit --tee "$AUDITOR_HOST:5000" --measurements "$WORK/auditor-measured.json" --rpc "$RPC" --contract "$CONTRACT" --key-file "$AUDITOR_KEY" --service "$SERVICE" --version "$VERSION" --vendor-tee "$VENDOR_HOST:5000"
rc=$?
echo "audit exit=$rc   contract=$CONTRACT   next: 5-User-tamper-verify-both-registries"
exit $rc
