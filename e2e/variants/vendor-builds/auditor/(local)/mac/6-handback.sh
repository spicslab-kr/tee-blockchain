#!/usr/bin/env bash
# Auditor — 감사 결과를 Vendor 에게 돌려준다 (설계 흐름 1 의 9: 측정값 기록 + 등록 좌표).
#    4-audit 뒤에. e2e-work/auditor-measured.json, e2e-work/contract.txt, 등록 key 를 한 파일(measurement-record.json)로 묶어
#    Vendor 쪽 폴더 e2e-work/from-auditor/ 에 둔다. 메일·카톡으로 보내도 같다.
#    실행: bash "e2e/variants/vendor-builds/auditor/(local)/mac/6-handback.sh" [0x키]     키를 생략하면 체인에서 measurement 로 찾는다.
set -euo pipefail
. "$(dirname "$0")/../../../../../setup/(local)/mac/env.sh"
CONTRACT=$(cat "$WORK/contract.txt")
KEY=${1:-}
if [ -z "$KEY" ]; then
  out=$("$TBA" lookup --rpc "$RPC" --contract "$CONTRACT" --measurements "$WORK/auditor-measured.json" 2>&1)
  KEY=$(echo "$out" | grep -oE 'key +0x[0-9a-fA-F]{64}' | awk '{print $2}' | head -1)
  [ -n "$KEY" ] || { echo "no registration of $WORK/auditor-measured.json found on $CONTRACT — run 4-audit.sh first"; exit 1; }
fi
pcr() { grep -oE "\"$1\": *\"[0-9a-fA-F]+\"" "$WORK/auditor-measured.json" | sed -E 's/.*"([0-9a-fA-F]+)"$/\1/'; }
mkdir -p "$WORK/from-auditor"
cat > "$WORK/from-auditor/measurement-record.json" <<EOF
{
  "service": "$SERVICE",
  "version": "$VERSION",
  "platform": "aws-nitro",
  "measurements": { "PCR0": "$(pcr PCR0)", "PCR1": "$(pcr PCR1)", "PCR2": "$(pcr PCR2)" },
  "registry": { "rpc": "$RPC", "contract": "$CONTRACT", "key": "$KEY", "approver": "$AUDITOR_ADDR" },
  "issued_by": "$AUDITOR_ADDR",
  "issued_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
cp "$WORK/auditor-measured.json" "$WORK/from-auditor/measurements.json"
echo "measurement record for the Vendor → $WORK/from-auditor/measurement-record.json"
cat "$WORK/from-auditor/measurement-record.json"
echo "next (Vendor): bash \"e2e/variants/vendor-builds/vendor/(local)/mac/7-check-record.sh\""
