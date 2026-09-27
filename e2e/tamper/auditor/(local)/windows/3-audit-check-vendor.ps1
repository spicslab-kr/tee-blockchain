# 조작 · Auditor — 내 값을 등록하면서 Vendor 의 엔클레이브에도 물어본다 (설계 흐름 2 ②의 8·9·15).
#    정상 흐름의 2-audit 에 -CheckVendor 를 붙인 것. Vendor 가 다른 이미지를 띄웠으면
#    "✗ the Vendor's enclave runs something else" 가 찍히고, 그래도 내가 검증한 값은 등록된다.
#    실행: .\e2e\tamper\auditor\(local)\windows\3-audit-check-vendor.ps1 [-Contract 0x…]
param([string]$Contract)
$args = @(); if ($Contract) { $args = @("-Contract", $Contract) }
& "$PSScriptRoot\..\..\..\..\normal\auditor\(local)\windows\2-audit.ps1" -CheckVendor @args
