#!/usr/bin/env bash
# Vendor — Auditor 가 준 이미지를 그대로 띄우고, 내 엔클레이브가 실제로 그 값과 등록된 값 둘 다와 같은지 스스로 확인한다.
#    여기, 내 컴퓨터에서 실행 — 띄우기만 Vendor 인스턴스 안에서 자동으로 접속해 실행된다.
#    입력: from-auditor/alpine-slim.eif, from-auditor/measurements.json (1-Auditor-build-audit-handoff 가 그 인스턴스에 올려 둔다)
#    실행: bash "e2e/flow1-auditor-registers/2-Vendor-launch-check-record.sh"
here="$(cd "$(dirname "$0")" && pwd)"
cd "$here/../.."
. "$here/../env.sh"

echo "== Vendor 인스턴스로 접속 중..."
nitroBanner "Vendor"
V 'bash -s' <<'REMOTE'
set -euo pipefail
test -f from-auditor/alpine-slim.eif -a -f from-auditor/measurements.json || { echo "from-auditor/alpine-slim.eif and measurements.json are missing — the Auditor runs 1-Auditor-build-audit-handoff first"; exit 1; }

echo "== received from the Auditor"
sha256sum from-auditor/alpine-slim.eif from-auditor/measurements.json  # 준비 — 받은 파일 확인용 해시

# ▶ 우리가 만든 tb 명령 — 받은 이미지를 직접 재서, Auditor 가 넘겨준 측정값과 같을 때만 내 TEE 에 띄운다.
echo "== tb-vendor launch (측정값과 대조하며 띄움)"
./tb-vendor launch --eif from-auditor/alpine-slim.eif --measurements from-auditor/measurements.json
REMOTE
rc=$?
[ $rc -eq 0 ] || exit $rc
localBanner

# 준비 — Auditor 가 남겨 둔 등록 좌표(어느 컨트랙트·어느 key 인지)를 그 기록 파일에서 읽는다.
REC="$WORK/from-auditor/measurement-record.json"
field() { grep -oE "\"$1\": *\"[^\"]*\"" "$REC" | head -1 | sed -E 's/.*": *"([^"]*)"$/\1/'; }
R_RPC=$(field rpc); R_CONTRACT=$(field contract); R_KEY=$(field key)
mkdir -p "$WORK/vendor-check"

# ▶ 우리가 만든 tb 명령 — 내 엔클레이브에 RA 를 보내 증명서를 받는다.
echo "1. asking my enclave at $VENDOR_HOST:5000 for an attestation"
"$TBV" ra-request --tee "$VENDOR_HOST:5000" --out "$WORK/vendor-check/doc.cbor" --format aws-nitro || exit 1
# ▶ 우리가 만든 tb 명령 — 그 증명서를 검증하고 안의 측정값을 꺼낸다.
echo "2. checking it and reading the measurements"
"$TBV" ra-parse --doc "$WORK/vendor-check/doc.cbor" --out "$WORK/vendor-check/parsed.json" || exit 1
# ▶ 우리가 만든 tb 명령 — 내 엔클레이브 값과 Auditor 가 준 기록의 값을 대조한다.
echo "3. my enclave vs the value in the Auditor's record"
"$TBV" compare --a "$WORK/vendor-check/parsed.json" --b "$WORK/from-auditor/measurements.json"; a=$?
# ▶ 우리가 만든 tb 명령 — 내 엔클레이브 값과 실제 체인 등록(값·만료·서명자)을 대조한다.
echo "4. my enclave vs the chain registration the record points to (contract $R_CONTRACT, key $R_KEY)"
"$TBV" compare --a "$WORK/vendor-check/parsed.json" --rpc "$R_RPC" --contract "$R_CONTRACT" --key "$R_KEY"; b=$?
if [ $a -eq 0 ] && [ $b -eq 0 ]; then
  echo "OK — what I run is exactly what the Auditor audited and registered. Users will get 'trusted'."
else
  echo "MISMATCH — what I run is not what the Auditor registered. Users will be told not to trust it."
  exit 1
fi
echo "next: 3-User-verify"
