#!/usr/bin/env bash
# Auditor — 내가 빌드·등록한 것을 Vendor 에게 준다 (설계 흐름 1 의 9): 이미지 파일 + 측정값 + 등록 좌표(measurement-record.json).
#    2-audit 뒤에. Auditor 인스턴스의 out/alpine-slim.eif · out/measurements.json 을 받아, 등록 key 와 함께 Vendor 인스턴스의 from-auditor/ 로 올리고,
#    Vendor 쪽 컴퓨터 폴더 e2e-work/from-auditor/ 에도 둔다 (5-check-record 가 읽는다). 메일·카톡으로 보내도 같다.
#    실행: bash "e2e/normal/auditor/(local)/mac/3-handoff.sh" [0x키]     키를 생략하면 체인에서 measurement 로 찾는다.
set -euo pipefail
. "$(dirname "$0")/../../../../setup/(local)/mac/env.sh"
CONTRACT=$(cat "$WORK/contract.txt"); KEY=${1:-}
mkdir -p "$WORK/from-auditor"
$SCP "$SSH_USER@$AUDITOR_HOST:out/alpine-slim.eif" "$WORK/from-auditor/alpine-slim.eif"
$SCP "$SSH_USER@$AUDITOR_HOST:out/measurements.json" "$WORK/from-auditor/measurements.json"
if [ -z "$KEY" ]; then
  out=$("$TBA" lookup --rpc "$RPC" --contract "$CONTRACT" --measurements "$WORK/from-auditor/measurements.json" 2>&1)
  KEY=$(echo "$out" | grep -oE 'key +0x[0-9a-fA-F]{64}' | awk '{print $2}' | head -1)
  [ -n "$KEY" ] || { echo "no registration of this value found on $CONTRACT — run 2-audit.sh first"; exit 1; }
fi
pcr() { grep -oE "\"$1\": *\"[0-9a-fA-F]+\"" "$WORK/from-auditor/measurements.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/'; }
cat > "$WORK/from-auditor/measurement-record.json" <<EOF
{
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws-nitro",
  "image": "alpine-slim.eif",
  "measurements": { "PCR0": "$(pcr PCR0)", "PCR1": "$(pcr PCR1)", "PCR2": "$(pcr PCR2)" },
  "registry": { "rpc": "$RPC", "contract": "$CONTRACT", "key": "$KEY", "approver": "$AUDITOR_ADDR" },
  "issued_by": "$AUDITOR_ADDR",
  "issued_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
echo "measurement record:"; cat "$WORK/from-auditor/measurement-record.json"
(cd "$WORK/from-auditor" && sha256 alpine-slim.eif measurements.json | sed 's/^/  /')
V 'mkdir -p from-auditor'
$SCP "$WORK/from-auditor/alpine-slim.eif" "$SSH_USER@$VENDOR_HOST:from-auditor/alpine-slim.eif"
$SCP "$WORK/from-auditor/measurements.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurements.json"
$SCP "$WORK/from-auditor/measurement-record.json" "$SSH_USER@$VENDOR_HOST:from-auditor/measurement-record.json"
echo "handed to the Vendor. next: on the Vendor instance  ./4-launch-received.sh"
