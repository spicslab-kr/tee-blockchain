# Auditor — 소스를 재현빌드해 자기 TEE 에 띄우고, 대조해서 체인에 등록하고, 그 이미지와 기록을 Vendor 에게 넘긴다
#    (설계 흐름 1 의 2~9). 여기, 내 컴퓨터에서 실행 — 빌드만 Auditor 인스턴스 안에서 자동으로 접속해 실행된다.
#    실행: .\e2e\flow1-auditor-registers\windows\1-Auditor-build-audit-handoff.ps1 [-Contract 0x…] [-CheckVendor]
param([string]$Contract, [switch]$CheckVendor)
. "$PSScriptRoot\..\..\windows\env.ps1"

"== Auditor 인스턴스로 접속 중..."
Nitro-Banner "Auditor"
A "./_remote-auditor-build-source.sh"
Local-Banner

$auditArgs = @{}
if ($Contract) { $auditArgs.Contract = $Contract }
if ($CheckVendor) { $auditArgs.CheckVendor = $true }
& "$PSScriptRoot\_auditor-audit.ps1" @auditArgs
if ($LASTEXITCODE -ne 0) { exit 1 }
& "$PSScriptRoot\_auditor-handoff.ps1"
if ($LASTEXITCODE -ne 0) { exit 1 }
"next: 2-Vendor-launch-check-record"
