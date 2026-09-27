# User (아무 컴퓨터, 키 없음) — Vendor 엔클레이브에 RA → 그 측정값으로 체인을 찾아 → 등록과 대조.
#    실행: .\e2e\normal\user\(local)\windows\5-verify.ps1 [컨트랙트]     생략하면 e2e-work\contract.txt
#    등록부 여러 개를 나란히 보려면 쉼표로: .\e2e\normal\user\(local)\windows\5-verify.ps1 0xVendor등록부,0xAuditor등록부  — 전부가 보증해야 trusted
param([string]$Contract)
. "$PSScriptRoot\..\..\..\..\setup\(local)\windows\env.ps1"
if (-not $Contract) { $Contract = (Get-Content e2e-work\contract.txt).Trim() }

.\tb-user.exe verify --tee "$($E2E.VendorHost):5000" --rpc $E2E.Rpc --contract $Contract
"verify exit=$LASTEXITCODE  (0 = trusted, 1 = nothing registered or nothing matches)"
