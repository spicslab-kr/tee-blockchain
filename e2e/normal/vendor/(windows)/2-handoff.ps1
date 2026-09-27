# Vendor — 만든 EIF 를 Auditor 에게 파일로 넘긴다: Vendor 의 out/nginx.eif · out/measurements.json → 노트북 → Auditor 의 from-vendor/
#     (인스턴스끼리 ssh 키가 없어서 노트북을 거친다. 메일·카톡으로 보내도 같다.)
. "$PSScriptRoot\..\..\..\setup\(windows)\env.ps1"
$ErrorActionPreference = "Stop"
$v = "$($E2E.SshUser)@$($E2E.VendorHost)"; $a = "$($E2E.SshUser)@$($E2E.AuditorHost)"

New-Item -ItemType Directory -Force e2e-work\from-vendor | Out-Null
Invoke-Expression "$($E2E.Scp) ${v}:out/nginx.eif e2e-work/from-vendor/nginx.eif"
Invoke-Expression "$($E2E.Scp) ${v}:out/measurements.json e2e-work/from-vendor/measurements.json"
"from Vendor:"
Get-FileHash e2e-work\from-vendor\* -Algorithm SHA256 | ForEach-Object { "  " + $_.Hash.ToLower() + "  " + (Split-Path $_.Path -Leaf) }

A 'mkdir -p from-vendor'
Invoke-Expression "$($E2E.Scp) e2e-work/from-vendor/nginx.eif ${a}:from-vendor/nginx.eif"
Invoke-Expression "$($E2E.Scp) e2e-work/from-vendor/measurements.json ${a}:from-vendor/measurements.json"
"handed to the Auditor. next: on the Auditor instance  ./3-check-received-image.sh"
