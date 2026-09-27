# 조작 · User — Vendor 의 등록부와 Auditor 의 등록부를 나란히 본다 (설계 흐름 2 ②의 21~23).
#    정상 흐름의 6-User-local-verify 에 등록부 둘을 준 것: e2e-work\contract-flow2.txt (Vendor) + e2e-work\contract.txt (Auditor).
#    Vendor 가 바꿔 띄운 값을 자기 등록부에만 올렸다면: Vendor 등록부 ✓ / Auditor 등록부 ✗ nobody → not trusted — only 1 of 2 registries vouch.
#    실행: .\e2e\flow2-vendor-self-registers\7-User-local-tamper-verify-both-registries.ps1
$vendorReg = (Get-Content e2e-work\contract-flow2.txt).Trim()
$auditorReg = (Get-Content e2e-work\contract.txt).Trim()
"Vendor's registry  $vendorReg"
"Auditor's registry $auditorReg"
& "$PSScriptRoot\..\flow1-auditor-registers\6-User-local-verify.ps1" "$vendorReg,$auditorReg"
