# 흐름 2 · User — Vendor 가 자기 등록부에 올린 것을 검증한다 (설계 흐름 2 의 19~23, 등록부 하나). 등록부는 e2e-work\contract-flow2.txt.
#    통과해도 서명자가 Vendor 자신이다 — "Vendor 가 자기 것을 보증했다" 까지. Auditor 가 들어오는 11~14 가 그 뒤 이야기.
#    실행: .\e2e\flow2-vendor-self-registers\3-User-local-verify.ps1
& "$PSScriptRoot\..\flow1-auditor-registers\6-User-local-verify.ps1" (Get-Content e2e-work\contract-flow2.txt).Trim()
