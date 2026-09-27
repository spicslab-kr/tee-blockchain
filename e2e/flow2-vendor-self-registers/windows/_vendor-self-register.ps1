# Vendor — Auditor 없이 직접 등록 (흐름 2). Vendor 인스턴스에서 flow2 의 1-Vendor-build-self-register.sh 를 돌린 뒤 (Vendor 가 직접 빌드한 값을 자기 등록부에).
#    Vendor 의 등록부(approver = Vendor 주소)를 깔고, self-register: 자기 TEE 에 RA → 자기 빌드값과 대조 → 등록 → 자기확인.
#    실행: .\e2e\flow2-vendor-self-registers\windows\_vendor-self-register.ps1 [-Contract 0x…]      컨트랙트는 e2e-work\contract-flow2.txt
param([string]$Contract, [string]$RemoteOut = "out")
. "$PSScriptRoot\..\..\windows\env.ps1"
$ErrorActionPreference = "Stop"
$v = "$($E2E.SshUser)@$($E2E.VendorHost)"

New-Item -ItemType Directory -Force e2e-work | Out-Null
Invoke-Expression "$($E2E.Scp) ${v}:$RemoteOut/measurements.json e2e-work/vendor-build.json"
"Vendor's build value:"; Get-Content e2e-work\vendor-build.json

if (-not $Contract) {
  "== registry-deploy (approver = Vendor itself $($E2E.VendorAddr))"
  $out = .\tb-vendor.exe registry-deploy --rpc $E2E.Rpc --key-file $E2E.VendorKey --approver $E2E.VendorAddr 2>&1 | Out-String -Width 4096
  $out
  $Contract = ([regex]::Match($out, 'deployed at (0x[0-9a-fA-F]{40})')).Groups[1].Value
  if (-not $Contract) { throw "no contract address in registry-deploy output" }
}
Set-Content e2e-work\contract-flow2.txt $Contract

$ErrorActionPreference = "Continue"
"== self-register (contract $Contract)"
.\tb-vendor.exe self-register --tee "$($E2E.VendorHost):5000" --measurements e2e-work\vendor-build.json --rpc $E2E.Rpc --contract $Contract --key-file $E2E.VendorKey --service $E2E.Service --version $E2E.Version
"self-register exit=$LASTEXITCODE"

"== verify (User) — the signer will be the Vendor itself"
.\tb-user.exe verify --tee "$($E2E.VendorHost):5000" --rpc $E2E.Rpc --contract $Contract
"verify exit=$LASTEXITCODE"
