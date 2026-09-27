# Vendor — Auditor 없이 직접 빌드해 띄우고, 그 값을 자기 등록부에 올린다 (흐름 2). 여기, 내 컴퓨터에서 실행 —
#    빌드만 Vendor 인스턴스 안에서 자동으로 접속해 실행된다.
#    실행: .\e2e\flow2-vendor-self-registers\windows\1-Vendor-build-self-register.ps1 [-Contract 0x…]
param([string]$Contract)
. "$PSScriptRoot\..\..\windows\env.ps1"

"== Vendor 인스턴스로 접속 중..."
Nitro-Banner "Vendor"
V "./_remote-vendor-build-source.sh"
Local-Banner

$regArgs = @{}
if ($Contract) { $regArgs.Contract = $Contract }
& "$PSScriptRoot\_vendor-self-register.ps1" @regArgs
"next: 2-User-verify"
