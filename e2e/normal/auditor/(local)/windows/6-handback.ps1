# Auditor — 감사 결과를 Vendor 에게 돌려준다 (설계 흐름 1 의 9: 측정값 기록 + 등록 좌표).
#    4-audit.ps1 뒤에. e2e-work\auditor-measured.json(내가 잰·빌드한 값), e2e-work\contract.txt, 등록 key 를
#    한 파일(measurement-record.json)로 묶어 Vendor 쪽 폴더 e2e-work\from-auditor\ 에 둔다. 메일·카톡으로 보내도 같다.
#    실행: .\e2e\normal\auditor\(local)\windows\6-handback.ps1 [-Key 0x…]      key 를 생략하면 체인에서 measurement 로 찾는다.
param([string]$Key)
. "$PSScriptRoot\..\..\..\..\setup\(local)\windows\env.ps1"
$ErrorActionPreference = "Stop"

$contract = (Get-Content e2e-work\contract.txt).Trim()
if (-not $Key) {
  $out = .\tb-auditor.exe lookup --rpc $E2E.Rpc --contract $contract --measurements e2e-work\auditor-measured.json 2>&1 | Out-String -Width 4096
  $Key = ([regex]::Match($out, 'key\s+(0x[0-9a-fA-F]{64})')).Groups[1].Value
  if (-not $Key) { throw "no registration of e2e-work\auditor-measured.json found on $contract — run 4-audit.ps1 first" }
}
$measured = Get-Content e2e-work\auditor-measured.json -Raw | ConvertFrom-Json
$record = [ordered]@{
  service      = $E2E.Service
  version      = $E2E.Version
  platform     = "aws-nitro"
  measurements = $measured.Measurements
  registry     = [ordered]@{ rpc = $E2E.Rpc; contract = $contract; key = $Key; approver = $E2E.AuditorAddr }
  issued_by    = $E2E.AuditorAddr
  issued_at    = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
}
New-Item -ItemType Directory -Force e2e-work\from-auditor | Out-Null
$record | ConvertTo-Json -Depth 5 | Set-Content -Encoding UTF8 e2e-work\from-auditor\measurement-record.json
Copy-Item e2e-work\auditor-measured.json e2e-work\from-auditor\measurements.json -Force
"measurement record for the Vendor → e2e-work\from-auditor\measurement-record.json"
Get-Content e2e-work\from-auditor\measurement-record.json
"next (Vendor): .\e2e\normal\vendor\(local)\windows\7-check-record.ps1"
