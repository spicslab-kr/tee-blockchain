# Vendor — Auditor 가 돌려준 기록과 내 엔클레이브를 맞댄다 (설계 흐름 1 의 11~13).
#    입력: e2e-work\from-auditor\measurement-record.json (1-Auditor-build-audit-handoff 이 만든 것).
#    하는 일: 내 엔클레이브에 attestation → 파싱 → (a) 받은 기록의 값과 대조, (b) 기록이 가리키는 체인 등록과 대조(값·만료·서명자).
#    둘 다 PASS 면 "Auditor 가 보증한 것 = 내가 지금 띄운 것" 이 내 눈으로도 확인된 것.
#    실행: .\e2e\flow1-auditor-registers\windows\_vendor-check-record.ps1
. "$PSScriptRoot\..\..\windows\env.ps1"
$rec = Get-Content e2e-work\from-auditor\measurement-record.json -Raw | ConvertFrom-Json
New-Item -ItemType Directory -Force e2e-work\vendor-check | Out-Null

"1. asking my enclave at $($E2E.VendorHost):5000 for an attestation"
.\tb-vendor.exe ra-request --tee "$($E2E.VendorHost):5000" --out e2e-work\vendor-check\doc.cbor --format aws-nitro
if ($LASTEXITCODE -ne 0) { exit 1 }
"2. checking it and reading the measurements"
.\tb-vendor.exe ra-parse --doc e2e-work\vendor-check\doc.cbor --out e2e-work\vendor-check\parsed.json
if ($LASTEXITCODE -ne 0) { exit 1 }
"3. my enclave vs the value in the Auditor's record"
.\tb-vendor.exe compare --a e2e-work\vendor-check\parsed.json --b e2e-work\from-auditor\measurements.json
$a = $LASTEXITCODE
"4. my enclave vs the chain registration the record points to (contract $($rec.registry.contract), key $($rec.registry.key)) — value, expiry, signer"
.\tb-vendor.exe compare --a e2e-work\vendor-check\parsed.json --rpc $rec.registry.rpc --contract $rec.registry.contract --key $rec.registry.key
$b = $LASTEXITCODE
if ($a -eq 0 -and $b -eq 0) { "OK — what I run is exactly what the Auditor audited and registered. Users will get 'trusted'." } else { "MISMATCH — what I run is not what the Auditor registered. Users will be told not to trust it."; exit 1 }
