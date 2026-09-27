#!/usr/bin/env bash
# Vendor — Auditor 가 돌려준 기록과 내 엔클레이브를 맞댄다 (설계 흐름 1 의 11~13, 그리고 7↔6 을 내 쪽에서도).
#    입력: e2e-work/from-auditor/measurement-record.json (6-handback 이 만든 것).
#    하는 일: 내 엔클레이브에 attestation → 파싱 → (a) 받은 기록의 값과 대조, (b) 기록이 가리키는 체인 등록과 대조(값·만료·서명자).
#    실행: bash "e2e/variants/vendor-builds/vendor/(local)/mac/7-check-record.sh"
. "$(dirname "$0")/../../../../../setup/(local)/mac/env.sh"
REC="$WORK/from-auditor/measurement-record.json"
field() { grep -oE "\"$1\": *\"[^\"]*\"" "$REC" | head -1 | sed -E 's/.*": *"([^"]*)"$/\1/'; }
R_RPC=$(field rpc); R_CONTRACT=$(field contract); R_KEY=$(field key)
mkdir -p "$WORK/vendor-check"

echo "1. asking my enclave at $VENDOR_HOST:5000 for an attestation"
"$TBV" ra-request --tee "$VENDOR_HOST:5000" --out "$WORK/vendor-check/doc.cbor" --format aws-nitro || exit 1
echo "2. checking it and reading the measurements"
"$TBV" ra-parse --doc "$WORK/vendor-check/doc.cbor" --out "$WORK/vendor-check/parsed.json" || exit 1
echo "3. my enclave vs the value in the Auditor's record"
"$TBV" compare --a "$WORK/vendor-check/parsed.json" --b "$WORK/from-auditor/measurements.json"; a=$?
echo "4. my enclave vs the chain registration the record points to (contract $R_CONTRACT, key $R_KEY) — value, expiry, signer"
"$TBV" compare --a "$WORK/vendor-check/parsed.json" --rpc "$R_RPC" --contract "$R_CONTRACT" --key "$R_KEY"; b=$?
if [ $a -eq 0 ] && [ $b -eq 0 ]; then echo "OK — what I run is exactly what the Auditor audited and registered. Users will get 'trusted'."; else echo "MISMATCH — what I run is not what the Auditor registered. Users will be told not to trust it."; exit 1; fi
