# Auditor (자기 키가 있는 자기 컴퓨터에서) — 내가 빌드한 값(out/measurements.json)을 가져와,  (설계 흐름 1 의 3~8)
#    새 등록부를 깔고, audit: 자기 TEE 에 RA → 그 값과 대조 → 같을 때만 등록 → 자기확인.
#    실행: .\e2e\flow1-auditor-registers\2-Auditor-local-audit.ps1 [-Contract 0x…]     컨트랙트 주소는 e2e-work\contract.txt 에 남는다.
#    -Contract 를 주면 새로 깔지 않고 그 등록부에 등록한다 (운영: 한 번 깔고 계속 쓴다).
#    -CheckVendor 를 주면 Vendor 의 엔클레이브에도 물어봐 내 값과 같은지 찍는다 (설계 흐름 2 의 8·9·15). 등록되는 값은 어차피 내 값.
#    (테스트에선 같은 값을 같은 컨트랙트에 또 올리면 AlreadyRegistered 라 매번 새로 깐다.)
param([string]$Contract, [switch]$CheckVendor)
. "$PSScriptRoot\..\env.ps1"
$ErrorActionPreference = "Stop"
$a = "$($E2E.SshUser)@$($E2E.AuditorHost)"

New-Item -ItemType Directory -Force e2e-work | Out-Null
Invoke-Expression "$($E2E.Scp) ${a}:out/measurements.json e2e-work/auditor-measured.json"
"Auditor's own value:"; Get-Content e2e-work\auditor-measured.json

if (-not $Contract) {
  "== registry-deploy (approver = Auditor $($E2E.AuditorAddr))"
  $out = .\tb-auditor.exe registry-deploy --rpc $E2E.Rpc --key-file $E2E.VendorKey --approver $E2E.AuditorAddr 2>&1 | Out-String -Width 4096
  $out
  $Contract = ([regex]::Match($out, 'deployed at (0x[0-9a-fA-F]{40})')).Groups[1].Value
  if (-not $Contract) { throw "no contract address in registry-deploy output" }
}
Set-Content e2e-work\contract.txt $Contract

"== audit (contract $Contract)"
$ErrorActionPreference = "Continue"
$extra = @(); if ($CheckVendor) { $extra = @("--vendor-tee", "$($E2E.VendorHost):5000") }
.\tb-auditor.exe audit --tee "$($E2E.AuditorHost):5000" --measurements e2e-work\auditor-measured.json --rpc $E2E.Rpc --contract $Contract --key-file $E2E.AuditorKey --service $E2E.Service --version $E2E.Version @extra
"audit exit=$LASTEXITCODE   contract=$Contract   next: .\e2e\flow1-auditor-registers\3-Auditor-local-handoff.ps1  (then the Vendor deploys)"
