# Vendor — Auditor 가 준 이미지를 받아 그대로 띄우고, 그 다음 자기 엔클레이브를 Auditor 의 기록·체인과 맞춰 본다
#    (설계 흐름 1 의 10~13). 여기, 내 컴퓨터에서 실행 — 띄우기만 Vendor 인스턴스 안에서 자동으로 접속해 실행된다.
#    입력: from-auditor/alpine-slim.eif, from-auditor/measurements.json (1-Auditor-build-audit-handoff 가 그 인스턴스에 올려 둔다)
#    실행: .\e2e\flow1-auditor-registers\windows\2-Vendor-launch-check-record.ps1
. "$PSScriptRoot\..\..\windows\env.ps1"

"== Vendor 인스턴스로 접속 중..."
Nitro-Banner "Vendor"
V "./_remote-vendor-launch-received.sh"
Local-Banner

& "$PSScriptRoot\_vendor-check-record.ps1"
"next: 3-User-verify"
