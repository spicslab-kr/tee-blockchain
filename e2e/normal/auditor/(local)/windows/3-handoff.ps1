# Auditor — 내가 빌드·등록한 것을 Vendor 에게 준다 (설계 흐름 1 의 9): 이미지 파일 + 측정값 + 등록 좌표(measurement-record.json).
#    2-audit 뒤에. Auditor 인스턴스의 out/alpine-slim.eif · out/measurements.json 을 받아, 등록 key 와 함께 Vendor 인스턴스의 from-auditor/ 로 올리고,
#    Vendor 쪽 컴퓨터 폴더 e2e-work\from-auditor\ 에도 둔다 (5-check-record 가 읽는다). 메일·카톡으로 보내도 같다.
#    실행: .\e2e\normal\auditor\(local)\windows\3-handoff.ps1 [-Key 0x…]     key 를 생략하면 체인에서 measurement 로 찾는다.
param([string]$Key)
. "$PSScriptRoot\..\..\..\..\setup\(local)\windows\env.ps1"
$ErrorActionPreference = "Stop"
$a = "$($E2E.SshUser)@$($E2E.AuditorHost)"; $v = "$($E2E.SshUser)@$($E2E.VendorHost)"
$contract = (Get-Content e2e-work\contract.txt).Trim()

New-Item -ItemType Directory -Force e2e-work\from-auditor | Out-Null
Invoke-Expression "$($E2E.Scp) ${a}:out/alpine-slim.eif e2e-work/from-auditor/alpine-slim.eif"
Invoke-Expression "$($E2E.Scp) ${a}:out/measurements.json e2e-work/from-auditor/measurements.json"

if (-not $Key) {
  $out = .\tb-auditor.exe lookup --rpc $E2E.Rpc --contract $contract --measurements e2e-work\from-auditor\measurements.json 2>&1 | Out-String -Width 4096
  $Key = ([regex]::Match($out, 'key\s+(0x[0-9a-fA-F]{64})')).Groups[1].Value
  if (-not $Key) { throw "no registration of this value found on $contract — run 2-audit.ps1 first" }
}
$measured = Get-Content e2e-work\from-auditor\measurements.json -Raw | ConvertFrom-Json
$record = [ordered]@{
  service      = $E2E.Service
  version      = $E2E.Version
  platform     = "aws-nitro"
  image        = "alpine-slim.eif"
  measurements = $measured.Measurements
  registry     = [ordered]@{ rpc = $E2E.Rpc; contract = $contract; key = $Key; approver = $E2E.AuditorAddr }
  issued_by    = $E2E.AuditorAddr
  issued_at    = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
}
$record | ConvertTo-Json -Depth 5 | Set-Content -Encoding UTF8 e2e-work\from-auditor\measurement-record.json
"measurement record:"; Get-Content e2e-work\from-auditor\measurement-record.json
Get-FileHash e2e-work\from-auditor\alpine-slim.eif, e2e-work\from-auditor\measurements.json -Algorithm SHA256 | ForEach-Object { "  " + $_.Hash.ToLower() + "  " + (Split-Path $_.Path -Leaf) }

V 'mkdir -p from-auditor'
Invoke-Expression "$($E2E.Scp) e2e-work/from-auditor/alpine-slim.eif ${v}:from-auditor/alpine-slim.eif"
Invoke-Expression "$($E2E.Scp) e2e-work/from-auditor/measurements.json ${v}:from-auditor/measurements.json"
Invoke-Expression "$($E2E.Scp) e2e-work/from-auditor/measurement-record.json ${v}:from-auditor/measurement-record.json"
"handed to the Vendor. next: on the Vendor instance  ./4-launch-received.sh"
