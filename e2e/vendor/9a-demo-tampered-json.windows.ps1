# 데모 — Vendor 가 measurements.json 을 조작해서 보내면 Auditor 가 잡는다.
#    handoff 와 같지만 PCR0 의 첫 글자를 바꿔 보낸다. 그리고 Auditor 인스턴스에서 check-received-image.nitro.sh 를 돌려
#    빨간 ✗ (it does NOT measure to what … says — not running it) 로 거부되는 것을 보인다. 엔클레이브는 뜨지 않는다.
#    실행: .\e2e\vendor\9a-demo-tampered-json.windows.ps1     (1-pack-image.nitro.sh 를 Vendor 인스턴스에서 먼저)
. "$PSScriptRoot\..\setup\env.windows.ps1"
$ErrorActionPreference = "Stop"
$v = "$($E2E.SshUser)@$($E2E.VendorHost)"; $a = "$($E2E.SshUser)@$($E2E.AuditorHost)"

New-Item -ItemType Directory -Force e2e-work\tampered | Out-Null
Invoke-Expression "$($E2E.Scp) ${v}:out/nginx.eif e2e-work/tampered/nginx.eif"
Invoke-Expression "$($E2E.Scp) ${v}:out/measurements.json e2e-work/tampered/measurements.json"
$j = Get-Content e2e-work\tampered\measurements.json -Raw | ConvertFrom-Json
$real = $j.Measurements.PCR0
$fake = $(if ($real[0] -eq '0') { '1' } else { '0' }) + $real.Substring(1)
$j.Measurements.PCR0 = $fake
$j | ConvertTo-Json -Depth 5 | Set-Content -Encoding UTF8 e2e-work\tampered\measurements.json
"the Vendor claims PCR0 = $fake"
"the EIF really is   PCR0 = $real"

A 'mkdir -p from-vendor'
Invoke-Expression "$($E2E.Scp) e2e-work/tampered/nginx.eif ${a}:from-vendor/nginx.eif"
Invoke-Expression "$($E2E.Scp) e2e-work/tampered/measurements.json ${a}:from-vendor/measurements.json"
"handed the tampered pair to the Auditor. now the Auditor checks it:"
""
$ErrorActionPreference = "Continue"
A './3-check-received-image.nitro.sh'
"exit=$LASTEXITCODE  (expected: not 0 — the Auditor measured the file itself and refused to run it)"
